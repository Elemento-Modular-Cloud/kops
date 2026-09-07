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

package elemento

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/kops/pkg/bootstrap"
)

func TestElementoVerifier(t *testing.T) {
	const (
		apiKey = "test-verifier-key"
		token  = "elemento-bootstrap.enrollment.secret"
	)
	body := []byte(`{"certificateRequest":"test"}`)
	expectedDigest := sha256.Sum256(body)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != elementoVerifyPath {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get(verifierAPIKeyHeader); got != apiKey {
			t.Fatalf("%s = %q, want %q", verifierAPIKeyHeader, got, apiKey)
		}

		var request verifyRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		if request.Token != token {
			t.Errorf("token = %q, want %q", request.Token, token)
		}
		if request.ClusterName != "test.k8s" {
			t.Errorf("cluster_name = %q, want test.k8s", request.ClusterName)
		}
		if request.RequestHash != hex.EncodeToString(expectedDigest[:]) {
			t.Errorf("request_hash = %q, want %q", request.RequestHash, hex.EncodeToString(expectedDigest[:]))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
            "valid": true,
            "node_name": "nodes-europe-7",
            "instance_group": "nodes-europe",
            "internal_ip": "10.0.42.7"
        }`))
	}))
	defer server.Close()

	verifier := newTestVerifier(t, server.URL, apiKey)
	result, err := verifier.VerifyToken(context.Background(), nil, ElementoAuthenticationTokenPrefix+token, body)
	if err != nil {
		t.Fatalf("VerifyToken returned error: %v", err)
	}
	if result.NodeName != "nodes-europe-7" {
		t.Errorf("NodeName = %q, want nodes-europe-7", result.NodeName)
	}
	if result.InstanceGroupName != "nodes-europe" {
		t.Errorf("InstanceGroupName = %q, want nodes-europe", result.InstanceGroupName)
	}
	if got := strings.Join(result.CertificateNames, ","); got != "nodes-europe-7,10.0.42.7" {
		t.Errorf("CertificateNames = %q, want node name and internal IP", got)
	}
}

func TestElementoVerifierRejectsOtherTokenPrefix(t *testing.T) {
	verifier := newTestVerifier(t, "http://127.0.0.1", "test-verifier-key")
	_, err := verifier.VerifyToken(context.Background(), nil, "other token", nil)
	if !errors.Is(err, bootstrap.ErrNotThisVerifier) {
		t.Fatalf("VerifyToken error = %v, want ErrNotThisVerifier", err)
	}
}

func TestElementoVerifierPropagatesServiceRejection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"detail":"Enrollment is waiting for provider instance binding"}`, http.StatusConflict)
	}))
	defer server.Close()

	verifier := newTestVerifier(t, server.URL, "test-verifier-key")
	_, err := verifier.VerifyToken(context.Background(), nil, ElementoAuthenticationTokenPrefix+"token", []byte("request"))
	if err == nil || !strings.Contains(err.Error(), "HTTP 409") {
		t.Fatalf("VerifyToken error = %v, want HTTP 409 rejection", err)
	}
}

func newTestVerifier(t *testing.T, serviceURL, apiKey string) bootstrap.Verifier {
	t.Helper()
	dir := t.TempDir()
	urlPath := filepath.Join(dir, "auth-service-url")
	keyPath := filepath.Join(dir, "verifier-api-key")
	if err := os.WriteFile(urlPath, []byte(serviceURL+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte(apiKey+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	verifier, err := NewElementoVerifier(&ElementoVerifierOptions{
		ClusterName:        "test.k8s",
		AuthServiceURLFile: urlPath,
		VerifierAPIKeyFile: keyPath,
	})
	if err != nil {
		t.Fatalf("NewElementoVerifier returned error: %v", err)
	}
	return verifier
}
