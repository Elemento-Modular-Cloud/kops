/*
Copyright 2019 The Kubernetes Authors.

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

package bootstrap

import (
	"io"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/kops/pkg/testutils"
	"k8s.io/kops/upup/pkg/fi"
)

func TestBootstarapBuilder_Simple(t *testing.T) {
	t.Setenv("AWS_REGION", "us-test1")

	runInstallBuilderTest(t, "tests/simple")
}

func TestInstallationPersistsElementoVerifierCredentials(t *testing.T) {
	t.Setenv("ELEMENTO_AUTH_URL", "https://auth.example.com")
	t.Setenv("ELEMENTO_AUTH_VERIFIER_API_KEY", "test-verifier-key")

	task := (&Installation{}).buildEnvFile()
	if task.Mode == nil || *task.Mode != "0600" {
		t.Fatalf("environment file mode = %v, want 0600", task.Mode)
	}
	reader, err := task.Contents.Open()
	if err != nil {
		t.Fatalf("opening environment file contents: %v", err)
	}
	contents, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading environment file contents: %v", err)
	}
	for _, expected := range []string{
		"ELEMENTO_AUTH_URL=https://auth.example.com",
		"ELEMENTO_AUTH_VERIFIER_API_KEY=test-verifier-key",
	} {
		if !strings.Contains(string(contents), expected) {
			t.Errorf("environment file does not contain %q", expected)
		}
	}
}

func runInstallBuilderTest(t *testing.T, basedir string) {
	installation := Installation{
		Command: []string{"/opt/kops/bin/nodeup", "--conf=/opt/kops/conf/kube_env.yaml", "--v=8"},
	}
	tasks := make(map[string]fi.InstallTask)
	buildContext := &fi.InstallModelBuilderContext{
		Tasks: tasks,
	}
	installation.Build(buildContext)

	testutils.ValidateTasks(t, filepath.Join(basedir, "tasks.yaml"), buildContext)
}
