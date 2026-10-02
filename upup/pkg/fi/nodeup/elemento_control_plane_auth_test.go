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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	api "k8s.io/kops/pkg/apis/kops"
	apiNodeup "k8s.io/kops/pkg/apis/nodeup"
)

func TestVerifyElementoControlPlaneBootstrap(t *testing.T) {
	const (
		token  = "elemento-bootstrap.enrollment.secret"
		apiKey = "test-verifier-key"
	)
	bootConfigBytes := []byte("control-plane boot configuration")
	expectedHash := sha256.Sum256(bootConfigBytes)
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if got := r.Header.Get("X-Verifier-API-Key"); got != apiKey {
			t.Errorf("verifier API key = %q, want %q", got, apiKey)
		}
		var request struct {
			Token       string `json:"token"`
			ClusterName string `json:"cluster_name"`
			RequestHash string `json:"request_hash"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Token != token || request.ClusterName != "test.k8s" || request.RequestHash != hex.EncodeToString(expectedHash[:]) {
			t.Errorf("unexpected verification request: %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
            "valid": true,
            "cluster_name": "test.k8s",
            "node_name": "control-plane-europe-1",
            "instance_group": "control-plane-europe",
            "role": "ControlPlane",
            "internal_ip": "10.0.18.5",
            "provider": "elemento",
            "provider_instance_id": "vm-uuid"
        }`))
	}))

	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "bootstrap-token")
	nodeIPFile := filepath.Join(dir, "node-ip")
	receiptFile := filepath.Join(dir, "state", "receipt.json")
	writeTestFile(t, tokenFile, token)
	writeTestFile(t, nodeIPFile, "10.0.18.5")
	bootConfig := &apiNodeup.BootConfig{
		CloudProvider:     api.CloudProviderElemento,
		ClusterName:       "test.k8s",
		InstanceGroupName: "control-plane-europe",
		InstanceGroupRole: api.InstanceGroupRoleControlPlane,
	}
	opt := elementoControlPlaneBootstrapOptions{
		AuthServiceURL: server.URL,
		VerifierAPIKey: apiKey,
		TokenFile:      tokenFile,
		NodeIPFile:     nodeIPFile,
		ReceiptFile:    receiptFile,
		NodeName:       "control-plane-europe-1",
		HTTPClient:     server.Client(),
	}

	if err := verifyElementoControlPlaneBootstrapWithOptions(context.Background(), bootConfig, bootConfigBytes, opt); err != nil {
		t.Fatalf("initial verification failed: %v", err)
	}
	server.Close()
	if err := verifyElementoControlPlaneBootstrapWithOptions(context.Background(), bootConfig, bootConfigBytes, opt); err != nil {
		t.Fatalf("receipt verification failed: %v", err)
	}
	if got := requestCount.Load(); got != 1 {
		t.Fatalf("authentication service request count = %d, want 1", got)
	}
	if info, err := os.Stat(receiptFile); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm() != 0o600 {
		t.Errorf("receipt mode = %o, want 600", info.Mode().Perm())
	}
}

func TestVerifyElementoControlPlaneBootstrapRejectsWorkerIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
            "valid": true,
            "cluster_name": "test.k8s",
            "node_name": "control-plane-europe-1",
            "instance_group": "control-plane-europe",
            "role": "Node",
            "internal_ip": "10.0.18.5",
            "provider": "elemento",
            "provider_instance_id": "vm-uuid"
        }`))
	}))
	defer server.Close()

	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "bootstrap-token")
	nodeIPFile := filepath.Join(dir, "node-ip")
	writeTestFile(t, tokenFile, "elemento-bootstrap.enrollment.secret")
	writeTestFile(t, nodeIPFile, "10.0.18.5")
	bootConfig := &apiNodeup.BootConfig{
		CloudProvider:     api.CloudProviderElemento,
		ClusterName:       "test.k8s",
		InstanceGroupName: "control-plane-europe",
		InstanceGroupRole: api.InstanceGroupRoleControlPlane,
	}
	err := verifyElementoControlPlaneBootstrapWithOptions(context.Background(), bootConfig, []byte("config"), elementoControlPlaneBootstrapOptions{
		AuthServiceURL: server.URL,
		VerifierAPIKey: "test-verifier-key",
		TokenFile:      tokenFile,
		NodeIPFile:     nodeIPFile,
		ReceiptFile:    filepath.Join(dir, "receipt.json"),
		NodeName:       "control-plane-europe-1",
		HTTPClient:     server.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "role mismatch") {
		t.Fatalf("verification error = %v, want role mismatch", err)
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
