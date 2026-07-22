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

package model

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/apis/nodeup"
)

func TestReadElementoNodeIP(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "node-ip")
	if err := os.WriteFile(filePath, []byte(" 10.0.1.5\n"), 0o600); err != nil {
		t.Fatalf("writing node IP: %v", err)
	}

	got, err := readElementoNodeIP(filePath)
	if err != nil {
		t.Fatalf("reading node IP: %v", err)
	}
	if got != "10.0.1.5" {
		t.Fatalf("expected node IP %q, got %q", "10.0.1.5", got)
	}
}

func TestReadElementoNodeIPRejectsInvalidAddress(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "node-ip")
	if err := os.WriteFile(filePath, []byte("not-an-ip\n"), 0o600); err != nil {
		t.Fatalf("writing node IP: %v", err)
	}

	if _, err := readElementoNodeIP(filePath); err == nil {
		t.Fatal("expected invalid node IP to be rejected")
	}
}

func TestElementoUsesMetadataNodeIP(t *testing.T) {
	c := &NodeupModelContext{
		BootConfig:   &nodeup.BootConfig{CloudProvider: kops.CloudProviderElemento},
		NodeupConfig: &nodeup.Config{},
	}
	if !c.UsesMetadataNodeIP() {
		t.Fatal("expected Elemento to configure kubelet with the injected node IP")
	}
}
