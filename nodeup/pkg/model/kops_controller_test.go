/*
Copyright 2020 The Kubernetes Authors.

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
	"io"
	"strings"
	"testing"

	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/elemento"
	"k8s.io/kops/upup/pkg/fi/nodeup/nodetasks"
)

func TestKopsControllerBuilder(t *testing.T) {
	RunGoldenTest(t, "tests/golden/minimal", "kops-controller", func(nodeupModelContext *NodeupModelContext, target *fi.NodeupModelBuilderContext) error {
		builder := KopsControllerBuilder{NodeupModelContext: nodeupModelContext}
		return builder.Build(target)
	})
}

func TestKopsControllerBuilderElementoAuthFiles(t *testing.T) {
	t.Setenv("ELEMENTO_AUTH_URL", "https://auth.example.com/")
	t.Setenv("ELEMENTO_AUTH_VERIFIER_API_KEY", "test-verifier-key")

	context := &fi.NodeupModelBuilderContext{Tasks: make(map[string]fi.NodeupTask)}
	builder := KopsControllerBuilder{}
	if err := builder.buildElementoAuthFiles(context); err != nil {
		t.Fatalf("buildElementoAuthFiles returned error: %v", err)
	}

	expected := map[string]string{
		elemento.ElementoAuthHostDirectory + "/auth-service-url": "https://auth.example.com/\n",
		elemento.ElementoAuthHostDirectory + "/verifier-api-key": "test-verifier-key\n",
	}
	for path, expectedContents := range expected {
		var task *nodetasks.File
		for _, candidate := range context.Tasks {
			if file, ok := candidate.(*nodetasks.File); ok && file.Path == path {
				task = file
				break
			}
		}
		if task == nil {
			t.Fatalf("file task %q was not created", path)
		}
		if task.Mode == nil || *task.Mode != "0600" {
			t.Errorf("file task %q mode = %v, want 0600", path, task.Mode)
		}
		reader, err := task.Contents.Open()
		if err != nil {
			t.Fatalf("opening contents for %q: %v", path, err)
		}
		contents, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("reading contents for %q: %v", path, err)
		}
		if string(contents) != expectedContents {
			t.Errorf("file task %q contents = %q, want %q", path, contents, expectedContents)
		}
	}

	for _, task := range context.Tasks {
		file, ok := task.(*nodetasks.File)
		if !ok || file.Path != elemento.ElementoAuthHostDirectory {
			continue
		}
		if file.Mode == nil || *file.Mode != "0700" {
			t.Errorf("auth directory mode = %v, want 0700", file.Mode)
		}
		if file.Owner == nil || *file.Owner != "kops-controller" {
			t.Errorf("auth directory owner = %v, want kops-controller", file.Owner)
		}
		return
	}
	t.Fatal("Elemento auth directory task was not created")
}

func TestKopsControllerBuilderElementoAuthFilesRequiresBothValues(t *testing.T) {
	t.Setenv("ELEMENTO_AUTH_URL", "https://auth.example.com")
	t.Setenv("ELEMENTO_AUTH_VERIFIER_API_KEY", "")

	context := &fi.NodeupModelBuilderContext{Tasks: make(map[string]fi.NodeupTask)}
	err := (&KopsControllerBuilder{}).buildElementoAuthFiles(context)
	if err == nil || !strings.Contains(err.Error(), "ELEMENTO_AUTH_VERIFIER_API_KEY") {
		t.Fatalf("buildElementoAuthFiles error = %v, want missing key error", err)
	}
}
