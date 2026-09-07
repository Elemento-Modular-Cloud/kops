/*
Copyright 2021 The Kubernetes Authors.

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

package resources

import (
	"strings"
	"testing"

	"k8s.io/kops/pkg/apis/kops"
)

func Test_NodeUpTabs(t *testing.T) {
	for i, line := range strings.Split(nodeUpTemplate, "\n") {
		if strings.Contains(line, "\t") {
			t.Errorf("NodeUpTemplate contains unexpected character %q on line %d: %q", "\t", i, line)
		}
	}
}

func TestBuildEnvironmentVariablesElementoAuth(t *testing.T) {
	t.Setenv("ELEMENTO_AUTH_URL", "https://auth.example.com")
	t.Setenv("ELEMENTO_AUTH_VERIFIER_API_KEY", "test-verifier-key")

	cluster := &kops.Cluster{
		Spec: kops.ClusterSpec{
			CloudProvider: kops.CloudProviderSpec{Elemento: &kops.ElementoSpec{}},
		},
	}
	controlPlane := &kops.InstanceGroup{
		Spec: kops.InstanceGroupSpec{Role: kops.InstanceGroupRoleControlPlane},
	}
	worker := &kops.InstanceGroup{
		Spec: kops.InstanceGroupSpec{Role: kops.InstanceGroupRoleNode},
	}

	env, err := buildEnvironmentVariables(cluster, controlPlane)
	if err != nil {
		t.Fatalf("buildEnvironmentVariables returned error: %v", err)
	}
	if _, found := env["ELEMENTO_AUTH_URL"]; found {
		t.Errorf("ELEMENTO_AUTH_URL must be supplied by the runtime authentication task")
	}
	if _, found := env["ELEMENTO_AUTH_VERIFIER_API_KEY"]; found {
		t.Errorf("ELEMENTO_AUTH_VERIFIER_API_KEY must be supplied by the runtime authentication task")
	}

	env, err = buildEnvironmentVariables(cluster, worker)
	if err != nil {
		t.Fatalf("buildEnvironmentVariables returned error: %v", err)
	}
	if _, found := env["ELEMENTO_AUTH_VERIFIER_API_KEY"]; found {
		t.Error("worker user-data must not contain the verifier API key")
	}
}
