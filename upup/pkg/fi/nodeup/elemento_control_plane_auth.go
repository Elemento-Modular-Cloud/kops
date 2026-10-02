/*
Copyright 2026 The Kubernetes Authors.

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

package nodeup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/klog/v2"
	api "k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/apis/nodeup"
	"k8s.io/kops/upup/pkg/fi/cloudup/elemento"
)

const (
	elementoAuthURLEnvironmentVariable       = "ELEMENTO_AUTH_URL"
	elementoVerifierKeyEnvironmentVariable   = "ELEMENTO_AUTH_VERIFIER_API_KEY"
	elementoNodeIPFile                       = "/etc/elemento/node-ip"
	elementoControlPlaneBootstrapReceiptFile = "/var/lib/kops/elemento-control-plane-bootstrap.json"
)

type elementoControlPlaneBootstrapReceipt struct {
	Version            int    `json:"version"`
	ClusterName        string `json:"cluster_name"`
	NodeName           string `json:"node_name"`
	InstanceGroup      string `json:"instance_group"`
	Role               string `json:"role"`
	InternalIP         string `json:"internal_ip"`
	Provider           string `json:"provider"`
	ProviderInstanceID string `json:"provider_instance_id"`
	BootConfigHash     string `json:"boot_config_hash"`
}

type elementoControlPlaneBootstrapOptions struct {
	AuthServiceURL string
	VerifierAPIKey string
	TokenFile      string
	NodeIPFile     string
	ReceiptFile    string
	NodeName       string
	HTTPClient     *http.Client
}

func verifyElementoControlPlaneBootstrap(ctx context.Context, bootConfig *nodeup.BootConfig, bootConfigBytes []byte) error {
	if bootConfig.CloudProvider != api.CloudProviderElemento || bootConfig.InstanceGroupRole != api.InstanceGroupRoleControlPlane {
		return nil
	}

	nodeName, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("determining Elemento control-plane hostname: %w", err)
	}
	return verifyElementoControlPlaneBootstrapWithOptions(ctx, bootConfig, bootConfigBytes, elementoControlPlaneBootstrapOptions{
		AuthServiceURL: os.Getenv(elementoAuthURLEnvironmentVariable),
		VerifierAPIKey: os.Getenv(elementoVerifierKeyEnvironmentVariable),
		TokenFile:      elemento.ElementoBootstrapTokenFile,
		NodeIPFile:     elementoNodeIPFile,
		ReceiptFile:    elementoControlPlaneBootstrapReceiptFile,
		NodeName:       nodeName,
	})
}

func verifyElementoControlPlaneBootstrapWithOptions(ctx context.Context, bootConfig *nodeup.BootConfig, bootConfigBytes []byte, opt elementoControlPlaneBootstrapOptions) error {
	internalIP, err := readTrimmedFile(opt.NodeIPFile, "Elemento node IP")
	if err != nil {
		return err
	}
	if net.ParseIP(internalIP) == nil {
		return fmt.Errorf("Elemento node IP file %q contains invalid IP %q", opt.NodeIPFile, internalIP)
	}

	digest := sha256.Sum256(bootConfigBytes)
	expected := elementoControlPlaneBootstrapReceipt{
		Version:        1,
		ClusterName:    strings.TrimSpace(bootConfig.ClusterName),
		NodeName:       strings.TrimSpace(opt.NodeName),
		InstanceGroup:  strings.TrimSpace(bootConfig.InstanceGroupName),
		Role:           string(api.InstanceGroupRoleControlPlane),
		InternalIP:     internalIP,
		Provider:       string(api.CloudProviderElemento),
		BootConfigHash: hex.EncodeToString(digest[:]),
	}
	if expected.ClusterName == "" || expected.NodeName == "" || expected.InstanceGroup == "" {
		return fmt.Errorf("Elemento control-plane bootstrap identity is incomplete")
	}

	receipt, found, err := readElementoControlPlaneBootstrapReceipt(opt.ReceiptFile)
	if err != nil {
		return err
	}
	if found && receipt.matches(expected) {
		klog.Infof("Using verified Elemento control-plane bootstrap receipt for node %q", expected.NodeName)
		return nil
	}

	token, err := readTrimmedFile(opt.TokenFile, "Elemento bootstrap token")
	if err != nil {
		return err
	}
	identity, err := elemento.VerifyBootstrapIdentity(ctx, elemento.BootstrapVerificationOptions{
		AuthServiceURL: opt.AuthServiceURL,
		VerifierAPIKey: opt.VerifierAPIKey,
		Token:          token,
		ClusterName:    expected.ClusterName,
		RequestBody:    bootConfigBytes,
		HTTPClient:     opt.HTTPClient,
	})
	if err != nil {
		return fmt.Errorf("verifying Elemento control-plane bootstrap enrollment: %w", err)
	}

	if err := validateElementoControlPlaneIdentity(identity, expected); err != nil {
		return err
	}
	expected.ProviderInstanceID = strings.TrimSpace(identity.ProviderInstanceID)
	if err := writeElementoControlPlaneBootstrapReceipt(opt.ReceiptFile, expected); err != nil {
		return err
	}
	klog.Infof("Verified Elemento control-plane bootstrap enrollment for node %q", expected.NodeName)
	return nil
}

func validateElementoControlPlaneIdentity(identity *elemento.BootstrapIdentity, expected elementoControlPlaneBootstrapReceipt) error {
	if identity == nil || !identity.Valid {
		return fmt.Errorf("Elemento authentication service returned an invalid control-plane identity")
	}
	checks := []struct {
		name string
		got  string
		want string
	}{
		{name: "cluster", got: identity.ClusterName, want: expected.ClusterName},
		{name: "node", got: identity.NodeName, want: expected.NodeName},
		{name: "instance group", got: identity.InstanceGroup, want: expected.InstanceGroup},
		{name: "role", got: identity.Role, want: expected.Role},
		{name: "internal IP", got: identity.InternalIP, want: expected.InternalIP},
		{name: "provider", got: identity.Provider, want: expected.Provider},
	}
	for _, check := range checks {
		if strings.TrimSpace(check.got) != check.want {
			return fmt.Errorf("Elemento control-plane %s mismatch: authentication service returned %q, expected %q", check.name, check.got, check.want)
		}
	}
	if strings.TrimSpace(identity.ProviderInstanceID) == "" {
		return fmt.Errorf("Elemento control-plane identity has no bound provider instance ID")
	}
	return nil
}

func (r elementoControlPlaneBootstrapReceipt) matches(expected elementoControlPlaneBootstrapReceipt) bool {
	return r.Version == expected.Version &&
		r.ClusterName == expected.ClusterName &&
		r.NodeName == expected.NodeName &&
		r.InstanceGroup == expected.InstanceGroup &&
		r.Role == expected.Role &&
		r.InternalIP == expected.InternalIP &&
		r.Provider == expected.Provider &&
		r.ProviderInstanceID != "" &&
		r.BootConfigHash == expected.BootConfigHash
}

func readElementoControlPlaneBootstrapReceipt(path string) (elementoControlPlaneBootstrapReceipt, bool, error) {
	var receipt elementoControlPlaneBootstrapReceipt
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return receipt, false, nil
	}
	if err != nil {
		return receipt, false, fmt.Errorf("reading Elemento control-plane bootstrap receipt %q: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return receipt, false, fmt.Errorf("checking Elemento control-plane bootstrap receipt %q: %w", path, err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return receipt, false, fmt.Errorf("Elemento control-plane bootstrap receipt %q must not be accessible by group or others", path)
	}
	if err := json.Unmarshal(b, &receipt); err != nil {
		return receipt, false, fmt.Errorf("decoding Elemento control-plane bootstrap receipt %q: %w", path, err)
	}
	return receipt, true, nil
}

func writeElementoControlPlaneBootstrapReceipt(path string, receipt elementoControlPlaneBootstrapReceipt) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating Elemento control-plane bootstrap receipt directory %q: %w", dir, err)
	}
	b, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding Elemento control-plane bootstrap receipt: %w", err)
	}
	b = append(b, '\n')
	temporaryFile, err := os.CreateTemp(dir, ".elemento-control-plane-bootstrap-*")
	if err != nil {
		return fmt.Errorf("creating temporary Elemento control-plane bootstrap receipt: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)
	if err := temporaryFile.Chmod(0o600); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("securing temporary Elemento control-plane bootstrap receipt: %w", err)
	}
	if _, err := temporaryFile.Write(b); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("writing temporary Elemento control-plane bootstrap receipt: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("closing temporary Elemento control-plane bootstrap receipt: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("installing Elemento control-plane bootstrap receipt %q: %w", path, err)
	}
	return nil
}

func readTrimmedFile(path, description string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s file %q: %w", description, path, err)
	}
	value := strings.TrimSpace(string(b))
	if value == "" {
		return "", fmt.Errorf("%s file %q is empty", description, path)
	}
	return value, nil
}
