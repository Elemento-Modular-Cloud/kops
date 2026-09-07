/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package elemento

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"k8s.io/kops/pkg/bootstrap"
)

const (
	ElementoAuthHostDirectory      = "/etc/elemento/kops-controller-auth"
	ElementoAuthContainerDirectory = "/var/run/secrets/elemento-auth"
	ElementoAuthServiceURLFile     = ElementoAuthContainerDirectory + "/auth-service-url"
	ElementoVerifierAPIKeyFile     = ElementoAuthContainerDirectory + "/verifier-api-key"

	elementoVerifyPath       = "/v1/bootstrap/verify"
	verifierAPIKeyHeader     = "X-Verifier-API-Key"
	maxVerifierResponseBytes = 1 << 20
)

type ElementoVerifierOptions struct {
	ClusterName        string `json:"clusterName"`
	AuthServiceURLFile string `json:"authServiceURLFile"`
	VerifierAPIKeyFile string `json:"verifierAPIKeyFile"`
}

type elementoVerifier struct {
	clusterName    string
	authServiceURL string
	verifierAPIKey string
	httpClient     *http.Client
}

var _ bootstrap.Verifier = &elementoVerifier{}

type verifyRequest struct {
	Token       string `json:"token"`
	ClusterName string `json:"cluster_name"`
	RequestHash string `json:"request_hash"`
}

// BootstrapIdentity is the node identity returned by the Elemento
// authentication service after it validates an enrollment token.
type BootstrapIdentity struct {
	Valid              bool   `json:"valid"`
	ClusterName        string `json:"cluster_name"`
	NodeName           string `json:"node_name"`
	InstanceGroup      string `json:"instance_group"`
	Role               string `json:"role"`
	InternalIP         string `json:"internal_ip"`
	Provider           string `json:"provider"`
	ProviderInstanceID string `json:"provider_instance_id"`
}

// BootstrapVerificationOptions contains the credentials and request material
// needed to verify an Elemento bootstrap enrollment.
type BootstrapVerificationOptions struct {
	AuthServiceURL string
	VerifierAPIKey string
	Token          string
	ClusterName    string
	RequestBody    []byte
	HTTPClient     *http.Client
}

func NewElementoVerifier(opt *ElementoVerifierOptions) (bootstrap.Verifier, error) {
	if opt == nil {
		return nil, fmt.Errorf("Elemento verifier options are required")
	}
	if strings.TrimSpace(opt.ClusterName) == "" {
		return nil, fmt.Errorf("Elemento verifier cluster name is required")
	}

	authServiceURL, err := readRequiredFile(opt.AuthServiceURLFile, "auth service URL")
	if err != nil {
		return nil, err
	}
	parsedURL, err := url.Parse(authServiceURL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, fmt.Errorf("invalid Elemento auth service URL in %q", opt.AuthServiceURLFile)
	}

	verifierAPIKey, err := readRequiredFile(opt.VerifierAPIKeyFile, "verifier API key")
	if err != nil {
		return nil, err
	}

	return &elementoVerifier{
		clusterName:    opt.ClusterName,
		authServiceURL: strings.TrimRight(authServiceURL, "/"),
		verifierAPIKey: verifierAPIKey,
		httpClient:     &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func readRequiredFile(path, description string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("Elemento %s file path is required", description)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading Elemento %s file %q: %w", description, path, err)
	}
	value := strings.TrimSpace(string(b))
	if value == "" {
		return "", fmt.Errorf("Elemento %s file %q is empty", description, path)
	}
	return value, nil
}

func (e elementoVerifier) VerifyToken(ctx context.Context, _ *http.Request, token string, body []byte) (*bootstrap.VerifyResult, error) {
	if !strings.HasPrefix(token, ElementoAuthenticationTokenPrefix) {
		return nil, bootstrap.ErrNotThisVerifier
	}
	token = strings.TrimSpace(strings.TrimPrefix(token, ElementoAuthenticationTokenPrefix))
	if token == "" {
		return nil, fmt.Errorf("Elemento bootstrap token is empty")
	}

	identity, err := VerifyBootstrapIdentity(ctx, BootstrapVerificationOptions{
		AuthServiceURL: e.authServiceURL,
		VerifierAPIKey: e.verifierAPIKey,
		Token:          token,
		ClusterName:    e.clusterName,
		RequestBody:    body,
		HTTPClient:     e.httpClient,
	})
	if err != nil {
		return nil, err
	}

	certificateNames := []string{identity.NodeName}
	if identity.InternalIP != "" {
		if net.ParseIP(identity.InternalIP) == nil {
			return nil, fmt.Errorf("Elemento authentication service returned invalid internal IP %q", identity.InternalIP)
		}
		certificateNames = append(certificateNames, identity.InternalIP)
	}

	return &bootstrap.VerifyResult{
		NodeName:          identity.NodeName,
		CertificateNames:  certificateNames,
		InstanceGroupName: identity.InstanceGroup,
	}, nil
}

// VerifyBootstrapIdentity verifies an enrollment directly with the Elemento
// authentication service. It is shared by kops-controller and the initial
// control-plane bootstrap, which must run before kops-controller exists.
func VerifyBootstrapIdentity(ctx context.Context, opt BootstrapVerificationOptions) (*BootstrapIdentity, error) {
	authServiceURL := strings.TrimSpace(opt.AuthServiceURL)
	parsedURL, err := url.Parse(authServiceURL)
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, fmt.Errorf("invalid Elemento auth service URL %q", opt.AuthServiceURL)
	}
	verifierAPIKey := strings.TrimSpace(opt.VerifierAPIKey)
	if verifierAPIKey == "" {
		return nil, fmt.Errorf("Elemento verifier API key is required")
	}
	token := strings.TrimSpace(opt.Token)
	if token == "" {
		return nil, fmt.Errorf("Elemento bootstrap token is empty")
	}
	clusterName := strings.TrimSpace(opt.ClusterName)
	if clusterName == "" {
		return nil, fmt.Errorf("Elemento bootstrap cluster name is required")
	}

	requestDigest := sha256.Sum256(opt.RequestBody)
	payload, err := json.Marshal(verifyRequest{
		Token:       token,
		ClusterName: clusterName,
		RequestHash: hex.EncodeToString(requestDigest[:]),
	})
	if err != nil {
		return nil, fmt.Errorf("encoding Elemento bootstrap verification request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(authServiceURL, "/")+elementoVerifyPath, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating Elemento bootstrap verification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(verifierAPIKeyHeader, verifierAPIKey)

	httpClient := opt.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling Elemento authentication service: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, maxVerifierResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("reading Elemento authentication service response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Elemento authentication service returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var identity BootstrapIdentity
	if err := json.Unmarshal(responseBody, &identity); err != nil {
		return nil, fmt.Errorf("decoding Elemento authentication service response: %w", err)
	}
	if !identity.Valid || identity.NodeName == "" || identity.InstanceGroup == "" {
		return nil, fmt.Errorf("Elemento authentication service returned an invalid node identity")
	}
	return &identity, nil
}
