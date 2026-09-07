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
	"os"
	"path/filepath"
	"testing"
)

func TestReadElementoBootstrapToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bootstrap-token")
	if err := os.WriteFile(path, []byte("  elemento-bootstrap.enrollment.secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	token, err := readElementoBootstrapToken(path)
	if err != nil {
		t.Fatalf("readElementoBootstrapToken returned error: %v", err)
	}
	if token != "elemento-bootstrap.enrollment.secret" {
		t.Fatalf("token = %q, want trimmed bootstrap token", token)
	}
}

func TestReadElementoBootstrapTokenRejectsEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bootstrap-token")
	if err := os.WriteFile(path, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := readElementoBootstrapToken(path); err == nil {
		t.Fatal("expected an empty token file to be rejected")
	}
}
