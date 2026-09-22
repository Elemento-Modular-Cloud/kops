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

package azure

import (
	"fmt"

	"k8s.io/kops/pkg/apis/kops"
)

// CloudConfig is the cloud provider configuration consumed by the Azure
// cloud-controller-manager and the Azure CSI drivers. The schema is documented
// at https://cloud-provider-azure.sigs.k8s.io/install/configs/.
type CloudConfig struct {
	// Auth config
	TenantID                    string `json:"tenantId,omitempty"`
	SubscriptionID              string `json:"subscriptionId,omitempty"`
	UseManagedIdentityExtension bool   `json:"useManagedIdentityExtension,omitempty"`

	// Cluster config
	ResourceGroup               string `json:"resourceGroup,omitempty"`
	Location                    string `json:"location,omitempty"`
	VnetName                    string `json:"vnetName,omitempty"`
	SubnetName                  string `json:"subnetName,omitempty"`
	RouteTableName              string `json:"routeTableName,omitempty"`
	SecurityGroupName           string `json:"securityGroupName,omitempty"`
	UseInstanceMetadata         bool   `json:"useInstanceMetadata,omitempty"`
	DisableAvailabilitySetNodes bool   `json:"disableAvailabilitySetNodes,omitempty"`
}

// BuildCloudConfig assembles the Azure cloud provider configuration for a
// cluster. kOps publishes the result in the azure-cloud-provider Secret, which
// the cloud-controller-manager and CSI drivers load via the
// --cloud-config-secret-name flag.
func BuildCloudConfig(cluster *kops.Cluster) (*CloudConfig, error) {
	azure := cluster.Spec.CloudProvider.Azure
	if azure == nil {
		return nil, fmt.Errorf("cluster is not an Azure cluster")
	}

	subnets := cluster.Spec.Networking.Subnets
	if len(subnets) == 0 {
		return nil, fmt.Errorf("cluster has no subnets")
	}

	// In kOps the virtual network and the network security group share a name.
	networkName := cluster.AzureNetworkSecurityGroupName()

	return &CloudConfig{
		TenantID:                    azure.TenantID,
		SubscriptionID:              azure.SubscriptionID,
		UseManagedIdentityExtension: true,
		ResourceGroup:               cluster.AzureResourceGroupName(),
		Location:                    subnets[0].Region,
		VnetName:                    networkName,
		SubnetName:                  subnets[0].Name,
		RouteTableName:              cluster.AzureRouteTableName(),
		SecurityGroupName:           networkName,
		UseInstanceMetadata:         true,
		DisableAvailabilitySetNodes: true,
	}, nil
}
