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

package channels

import (
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/assets"
	"k8s.io/kops/pkg/featureflag"
	"k8s.io/kops/pkg/model"
	"k8s.io/kops/pkg/model/iam"
	"k8s.io/kops/pkg/nodelabels"
	"k8s.io/kops/pkg/testutils"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/util/pkg/vfs"
)

func TestElementoChannelsBootstrap(t *testing.T) {
	t.Setenv("KOPS_BASE_URL", "https://assets.example/elemento")
	vfs.Context.ResetMemfsContext(true)
	modelContext, err := loadKopsModelContext("tests/minimal")
	if err != nil {
		t.Fatal(err)
	}
	modelContext.Cluster.Spec.CloudProvider = kops.CloudProviderSpec{Elemento: &kops.ElementoSpec{}}
	modelContext.AllInstanceGroups = []*kops.InstanceGroup{{Spec: kops.InstanceGroupSpec{Role: kops.InstanceGroupRoleControlPlane}}}
	builder := ChannelsBuilder{
		KopsModelContext: modelContext,
		AssetBuilder:     assets.NewAssetBuilder(vfs.Context, modelContext.Cluster.Spec.Assets, false),
	}
	channels, err := builder.channelList()
	if err != nil {
		t.Fatal(err)
	}
	pod, err := builder.buildPod(channels)
	if err != nil {
		t.Fatal(err)
	}
	if !pod.Spec.HostNetwork || pod.Spec.DNSPolicy != v1.DNSDefault {
		t.Fatal("channels bootstrap must not depend on Cilium or CoreDNS")
	}
	args := pod.Spec.Containers[0].Args
	for _, arg := range []string{"--node-labels=" + nodelabels.RoleLabelControlPlane20, "--node-name=$(NODE_NAME)", channels[0]} {
		if !slices.Contains(args, arg) {
			t.Errorf("missing bootstrap argument %q in %v", arg, args)
		}
	}
}

func Test_RunChannelsBuilder(t *testing.T) {
	h := testutils.NewIntegrationTestHarness(t)
	defer h.Close()
	h.MockKopsVersion("1.37.0-beta.1")
	// Stable golden image strings.
	featureflag.ParseFlags("-ImageDigest")
	// channelList builds VFS paths from configStore.base — fixtures use memfs://.
	vfs.Context.ResetMemfsContext(true)
	tests := []string{
		"tests/minimal",
		"tests/container_registry",
	}
	for _, basedir := range tests {
		t.Run(fmt.Sprintf("basedir=%s", basedir), func(t *testing.T) {
			context := &fi.CloudupModelBuilderContext{
				Tasks: make(map[string]fi.CloudupTask),
			}
			kopsModelContext, err := loadKopsModelContext(basedir)
			if err != nil {
				t.Fatalf("error loading model %q: %v", basedir, err)
			}

			builder := ChannelsBuilder{
				KopsModelContext: kopsModelContext,
				AssetBuilder:     assets.NewAssetBuilder(vfs.Context, kopsModelContext.Cluster.Spec.Assets, false),
			}

			if err := builder.Build(context); err != nil {
				t.Fatalf("error from Build: %v", err)
			}

			testutils.ValidateTasks(t, filepath.Join(basedir, "tasks.yaml"), context)
		})
	}
}

func loadKopsModelContext(basedir string) (*model.KopsModelContext, error) {
	spec, err := testutils.LoadModel(basedir)
	if err != nil {
		return nil, err
	}
	if spec.Cluster == nil {
		return nil, fmt.Errorf("no cluster found in %s", basedir)
	}
	return &model.KopsModelContext{
		IAMModelContext:   iam.IAMModelContext{Cluster: spec.Cluster},
		AllInstanceGroups: spec.InstanceGroups,
		InstanceGroups:    spec.InstanceGroups,
	}, nil
}
