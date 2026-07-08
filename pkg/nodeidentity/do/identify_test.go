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

package do

import (
	"testing"

	"k8s.io/kops/pkg/nodelabels"
)

func TestLabelsFromTags(t *testing.T) {
	tests := []struct {
		name      string
		tags      []string
		wantLabel string
		wantEmpty bool
	}{
		{
			name:      "control plane via role tag",
			tags:      []string{"kops-instance-role:ControlPlane", "kops-instancegroup:master-nyc3"},
			wantLabel: nodelabels.RoleLabelControlPlane20,
		},
		{
			name:      "node via role tag",
			tags:      []string{"kops-instance-role:Node", "kops-instancegroup:nodes"},
			wantLabel: nodelabels.RoleLabelNode16,
		},
		{
			name:      "api server via role tag",
			tags:      []string{"kops-instance-role:APIServer", "kops-instancegroup:apiservers"},
			wantLabel: nodelabels.RoleLabelAPIServer16,
		},
		{
			name: "bastion role produces no labels",
			tags: []string{"kops-instance-role:Bastion"},
			// Bastion nodes don't join the cluster.
			wantEmpty: true,
		},
		{
			name:      "fallback: control plane detected via KubernetesCluster-Master",
			tags:      []string{"KubernetesCluster:cluster", "KubernetesCluster-Master:cluster", "kops-instancegroup:master-nyc3"},
			wantLabel: nodelabels.RoleLabelControlPlane20,
		},
		{
			name:      "fallback: worker defaults to Node role",
			tags:      []string{"KubernetesCluster:cluster", "kops-instancegroup:nodes"},
			wantLabel: nodelabels.RoleLabelNode16,
		},
		{
			name:      "role tag takes precedence over legacy master tag",
			tags:      []string{"KubernetesCluster-Master:cluster", "kops-instance-role:Node"},
			wantLabel: nodelabels.RoleLabelNode16,
		},
		{
			name:      "unknown role produces no labels",
			tags:      []string{"kops-instance-role:Unknown"},
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := labelsFromTags(tt.tags)
			if tt.wantEmpty {
				if len(labels) != 0 {
					t.Fatalf("expected no labels, got %v", labels)
				}
				return
			}
			if _, ok := labels[tt.wantLabel]; !ok {
				t.Fatalf("expected label %q, got %v", tt.wantLabel, labels)
			}
		})
	}
}

func TestRoleFromTags(t *testing.T) {
	role, ok := roleFromTags([]string{"KubernetesCluster:c", "kops-instancegroup:nodes"})
	if ok {
		t.Fatalf("expected no role tag, got %q", role)
	}

	role, ok = roleFromTags([]string{"kops-instance-role:ControlPlane"})
	if !ok || string(role) != "ControlPlane" {
		t.Fatalf("expected ControlPlane, got %q ok=%v", role, ok)
	}
}
