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

package elementomodel

import (
	"os"
	"strings"

	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/elementotasks"
)

// KubernetesAuthModelBuilder registers the authentication resources before
// any Elemento server is created.
type KubernetesAuthModelBuilder struct {
	*ElementoModelContext
	Lifecycle fi.Lifecycle
}

var _ fi.CloudupModelBuilder = &KubernetesAuthModelBuilder{}

func (b *KubernetesAuthModelBuilder) Build(c *fi.CloudupModelBuilderContext) error {
	target := strings.TrimSpace(os.Getenv("ATOMOS_SERVER1"))
	tailnetCIDR := strings.TrimSpace(b.Cluster.Spec.Networking.NetworkCIDR)
	if tailnetCIDR == "" {
		tailnetCIDR = "10.0.0.0/16"
	}
	service := &elementotasks.KubernetesAuthService{
		Name:         fi.PtrTo(b.ClusterName()),
		Lifecycle:    b.Lifecycle,
		Network:      b.LinkToNetwork(),
		AtomOSTarget: target,
		TailnetCIDR:  tailnetCIDR,
	}
	c.AddTask(service)

	cluster := &elementotasks.KubernetesAuthCluster{
		Name:         fi.PtrTo(b.ClusterName()),
		Lifecycle:    b.Lifecycle,
		AuthService:  service,
		AtomOSTarget: target,
		Description:  "Kubernetes cluster managed by kOps",
	}
	c.AddTask(cluster)

	for _, ig := range b.InstanceGroups {
		c.AddTask(&elementotasks.KubernetesAuthInstanceGroup{
			Name:         fi.PtrTo(ig.Name),
			Lifecycle:    b.Lifecycle,
			AuthCluster:  cluster,
			AtomOSTarget: target,
			ClusterName:  b.ClusterName(),
			Role:         string(ig.Spec.Role),
		})
	}
	return nil
}
