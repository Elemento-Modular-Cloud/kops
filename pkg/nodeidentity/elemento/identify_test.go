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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kops/pkg/nodelabels"
	cloudelemento "k8s.io/kops/upup/pkg/fi/cloudup/elemento"
)

func TestStaticNodeIdentityIsDisabledForProvisionedNodes(t *testing.T) {
	for _, nodeName := range []string{"control-plane-europe-1", "nodes-europe-1"} {
		if _, found := staticNodeIdentity(nodeName); found {
			t.Fatalf("static node identity for %q must be disabled", nodeName)
		}
	}
}

func TestIdentifyNodeFromAuthService(t *testing.T) {
	const verifierKey = "test-verifier-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/v1/clusters/test.k8s/nodes/by-name/nodes-europe-1"; got != want {
			t.Errorf("request path = %q, want %q", got, want)
		}
		if got := r.Header.Get("X-Verifier-API-Key"); got != verifierKey {
			t.Errorf("verifier key = %q, want %q", got, verifierKey)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"node-id",
			"cluster_name":"test.k8s",
			"node_name":"nodes-europe-1",
			"instance_group":"nodes-europe",
			"role":"Node",
			"internal_ip":"10.0.18.6",
			"provider":"elemento",
			"provider_instance_id":"provider-uuid",
			"labels":{
				"topology.kubernetes.io/zone":"europe",
				"k8s.io/cluster-autoscaler/node-template/label/example.com/workload":"general",
				"k8s.io/role/node":"1",
				"invalid/label/key":"ignored"
			},
			"state":"certificate-issued"
		}`))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	urlFile := filepath.Join(tempDir, "auth-service-url")
	keyFile := filepath.Join(tempDir, "verifier-api-key")
	if err := os.WriteFile(urlFile, []byte(server.URL), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, []byte(verifierKey), 0o600); err != nil {
		t.Fatal(err)
	}

	identifier, err := New(false, "test.k8s", &cloudelemento.ElementoVerifierOptions{
		ClusterName:        "test.k8s",
		AuthServiceURLFile: urlFile,
		VerifierAPIKeyFile: keyFile,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	info, err := identifier.IdentifyNode(context.Background(), &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "nodes-europe-1"},
	})
	if err != nil {
		t.Fatalf("IdentifyNode returned error: %v", err)
	}
	if got, want := info.ProviderID, "elemento://provider-uuid"; got != want {
		t.Fatalf("ProviderID = %q, want %q", got, want)
	}
	if _, found := info.Labels[nodelabels.RoleLabelNode16]; !found {
		t.Fatalf("expected role label %q in %#v", nodelabels.RoleLabelNode16, info.Labels)
	}
	if got, want := info.Labels["topology.kubernetes.io/zone"], "europe"; got != want {
		t.Fatalf("zone label = %q, want %q", got, want)
	}
	if got, want := info.Labels["example.com/workload"], "general"; got != want {
		t.Fatalf("decoded node label = %q, want %q", got, want)
	}
	for _, key := range []string{
		"k8s.io/cluster-autoscaler/node-template/label/example.com/workload",
		"k8s.io/role/node",
		"invalid/label/key",
	} {
		if _, found := info.Labels[key]; found {
			t.Fatalf("cloud tag %q must not be copied as a Kubernetes node label", key)
		}
	}
}
