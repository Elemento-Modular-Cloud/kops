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
	"bytes"
	"context"
	"testing"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"

	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/apis/nodeup"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/nodeup/nodetasks"
	"k8s.io/kops/util/pkg/vfs"
)

// stubChannelsManifest is the pod YAML the test seeds into VFS; real content is exercised by
// the cloudup ChannelsBuilder tests.
const stubChannelsManifest = "apiVersion: v1\nkind: Pod\nmetadata:\n  name: kops-channels\n  namespace: kube-system\n"

func TestChannelsBuilderElemento(t *testing.T) {
	vfs.Context.ResetMemfsContext(true)
	for _, role := range []kops.InstanceGroupRole{kops.InstanceGroupRoleControlPlane, kops.InstanceGroupRoleNode} {
		t.Run(string(role), func(t *testing.T) {
			manifestPath := "memfs://elemento.example/cluster/manifests/channels/kops-channels.yaml"
			p, err := vfs.Context.BuildVfsPath(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := p.WriteFile(context.Background(), bytes.NewReader([]byte(stubChannelsManifest)), nil); err != nil {
				t.Fatal(err)
			}
			modelContext := &NodeupModelContext{
				BootConfig: &nodeup.BootConfig{
					CloudProvider:     kops.CloudProviderElemento,
					InstanceGroupRole: role,
				},
				NodeupConfig: &nodeup.Config{
					KubernetesVersion: "1.36.0",
					ChannelsManifest:  manifestPath,
				},
			}
			if err := modelContext.Init(); err != nil {
				t.Fatal(err)
			}
			target := &fi.NodeupModelBuilderContext{Tasks: make(map[string]fi.NodeupTask)}
			builder := ChannelsBuilder{NodeupModelContext: modelContext}
			if err := builder.Build(target); err != nil {
				t.Fatal(err)
			}
			if role == kops.InstanceGroupRoleNode {
				if len(target.Tasks) != 0 {
					t.Fatalf("worker should not install channels: %v", target.Tasks)
				}
				return
			}
			var manifestFound, kubeconfigFound bool
			for _, task := range target.Tasks {
				switch task := task.(type) {
				case *nodetasks.File:
					if task.Path == channelsManifestPath {
						data, err := fi.ResourceAsBytes(task.Contents)
						if err != nil {
							t.Fatal(err)
						}
						if string(data) != stubChannelsManifest {
							t.Fatalf("unexpected channels manifest: %s", data)
						}
						manifestFound = true
					}
				case *nodetasks.KubeConfig:
					if task.Name == "kops" && task.ServerURL == "https://127.0.0.1" {
						kubeconfigFound = true
					}
				case *nodetasks.Service:
					t.Fatalf("channels should be a static pod, not a systemd service: %v", task)
				}
			}
			if !manifestFound || !kubeconfigFound {
				t.Fatalf("missing channels artifacts: manifest=%v kubeconfig=%v", manifestFound, kubeconfigFound)
			}
		})
	}
}

func TestChannelsBuilder(t *testing.T) {
	manifestPath := "memfs://clusters.example.com/minimal.example.com/manifests/channels/kops-channels.yaml"
	RunGoldenTest(t, "tests/channels/", "channels", func(nodeupModelContext *NodeupModelContext, target *fi.NodeupModelBuilderContext) error {
		p, err := vfs.Context.BuildVfsPath(manifestPath)
		if err != nil {
			t.Fatalf("building vfs path: %v", err)
		}
		if err := p.WriteFile(target.Context(), bytes.NewReader([]byte(stubChannelsManifest)), nil); err != nil {
			t.Fatalf("seeding kops-channels manifest: %v", err)
		}
		nodeupModelContext.NodeupConfig.ChannelsManifest = manifestPath
		builder := ChannelsBuilder{NodeupModelContext: nodeupModelContext}
		return builder.Build(target)
	})
}

// TestReadChannelsManifest_SELinux pins the parse/decorate/reserialize toggle:
// SeLinuxEnabled=true adds the permissive context, false/nil leaves it untouched.
func TestReadChannelsManifest_SELinux(t *testing.T) {
	vfs.Context.ResetMemfsContext(true)
	manifestPath := "memfs://test/manifests/channels/kops-channels.yaml"
	p, err := vfs.Context.BuildVfsPath(manifestPath)
	if err != nil {
		t.Fatalf("building vfs path: %v", err)
	}
	if err := p.WriteFile(context.Background(), bytes.NewReader([]byte(stubChannelsManifest)), nil); err != nil {
		t.Fatalf("seeding manifest: %v", err)
	}

	cases := []struct {
		name    string
		cfg     *kops.ContainerdConfig
		wantSEL bool
	}{
		{name: "enabled", cfg: &kops.ContainerdConfig{SeLinuxEnabled: true}, wantSEL: true},
		{name: "disabled", cfg: &kops.ContainerdConfig{SeLinuxEnabled: false}, wantSEL: false},
		{name: "nil", cfg: nil, wantSEL: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := &ChannelsBuilder{NodeupModelContext: &NodeupModelContext{
				NodeupConfig: &nodeup.Config{
					ChannelsManifest: manifestPath,
					ContainerdConfig: tc.cfg,
				},
			}}
			out, err := b.readChannelsManifest(&fi.NodeupModelBuilderContext{})
			if err != nil {
				t.Fatalf("readChannelsManifest: %v", err)
			}
			pod := &v1.Pod{}
			if err := yaml.Unmarshal(out, pod); err != nil {
				t.Fatalf("parsing output: %v", err)
			}
			got := pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SELinuxOptions != nil &&
				pod.Spec.SecurityContext.SELinuxOptions.Type == "spc_t"
			if got != tc.wantSEL {
				t.Fatalf("SELinux context present=%v, want=%v\noutput:\n%s", got, tc.wantSEL, out)
			}
		})
	}
}
