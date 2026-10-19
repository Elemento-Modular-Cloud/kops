/*
Copyright 2025 The Kubernetes Authors.

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

// Package elementomodel translates kOps cluster and instance-group specs into
// Elemento provisioning tasks. Builders share a validated multicloud plan;
// elementotasks reconciles those resources through the ecloud-go SDK.
package elementomodel

import (
	"github.com/Elemento-Modular-Cloud/ecloud-go/ecloud"
	"k8s.io/kops/pkg/model"
	"k8s.io/kops/upup/pkg/fi/cloudup/elemento"
	"k8s.io/kops/upup/pkg/fi/cloudup/elementotasks"
)

// ElementoModelContext carries planning results between builders. External
// provider placement does not change the cluster's Kubernetes provider identity.
type ElementoModelContext struct {
	*model.KopsModelContext
	// Populated from the validated plan, never from provider-specific IP env vars.
	externalNodeIPs         map[string]string
	delayAtomosControlPlane bool
	multicloudPlan          *ecloud.MulticloudPlan
	loadBalancerDNSRecords  []*elementotasks.DNSRecord
}

// LinkToNetwork describes the same named task used by NetworkModelBuilder;
// dependent tasks use it to obtain the provisioned Elemento network UUID.
func (b *ElementoModelContext) LinkToNetwork() *elementotasks.Network {
	name := b.ClusterName()
	network := &elementotasks.Network{Name: &name}

	// If NetworkID is not specified, provide default values
	if b.Cluster.Spec.Networking.NetworkID == "" {
		networkCIDR := b.Cluster.Spec.Networking.NetworkCIDR
		if networkCIDR == "" {
			networkCIDR = "10.0.0.0/16" // Default CIDR for Elemento networks
		}

		network.IPRange = networkCIDR
		network.Region = b.Region
		network.Subnets = []string{
			networkCIDR,
		}
		network.Labels = map[string]string{
			elemento.TagKubernetesClusterName: b.ClusterName(),
		}
	} else {
		network.ID = new(b.Cluster.Spec.Networking.NetworkID)
	}

	return network
}
