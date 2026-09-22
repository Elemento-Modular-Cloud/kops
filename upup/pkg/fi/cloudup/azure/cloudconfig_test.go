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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kops/pkg/apis/kops"
)

func TestBuildCloudConfig(t *testing.T) {
	grid := []struct {
		name     string
		cluster  *kops.Cluster
		expected CloudConfig
	}{
		{
			name: "owned resources default to the cluster name",
			cluster: &kops.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test.k8s.local"},
				Spec: kops.ClusterSpec{
					CloudProvider: kops.CloudProviderSpec{
						Azure: &kops.AzureSpec{
							SubscriptionID: "sub-id",
							TenantID:       "tenant-id",
						},
					},
					Networking: kops.NetworkingSpec{
						Subnets: []kops.ClusterSubnetSpec{
							{Name: "subnet-a", Region: "eastus"},
						},
					},
				},
			},
			expected: CloudConfig{
				TenantID:                    "tenant-id",
				SubscriptionID:              "sub-id",
				UseManagedIdentityExtension: true,
				ResourceGroup:               "test.k8s.local",
				Location:                    "eastus",
				VnetName:                    "test.k8s.local",
				SubnetName:                  "subnet-a",
				RouteTableName:              "test.k8s.local",
				SecurityGroupName:           "test.k8s.local",
				UseInstanceMetadata:         true,
				DisableAvailabilitySetNodes: true,
			},
		},
		{
			name: "shared resources use the configured names",
			cluster: &kops.Cluster{
				ObjectMeta: metav1.ObjectMeta{Name: "test.k8s.local"},
				Spec: kops.ClusterSpec{
					CloudProvider: kops.CloudProviderSpec{
						Azure: &kops.AzureSpec{
							SubscriptionID:    "sub-id",
							TenantID:          "tenant-id",
							ResourceGroupName: "shared-rg",
							RouteTableName:    "shared-rt",
						},
					},
					Networking: kops.NetworkingSpec{
						NetworkID: "shared-vnet",
						Subnets: []kops.ClusterSubnetSpec{
							{Name: "subnet-a", Region: "westus2"},
						},
					},
				},
			},
			expected: CloudConfig{
				TenantID:                    "tenant-id",
				SubscriptionID:              "sub-id",
				UseManagedIdentityExtension: true,
				ResourceGroup:               "shared-rg",
				Location:                    "westus2",
				VnetName:                    "shared-vnet",
				SubnetName:                  "subnet-a",
				RouteTableName:              "shared-rt",
				SecurityGroupName:           "shared-vnet",
				UseInstanceMetadata:         true,
				DisableAvailabilitySetNodes: true,
			},
		},
	}
	for _, g := range grid {
		t.Run(g.name, func(t *testing.T) {
			config, err := BuildCloudConfig(g.cluster)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if *config != g.expected {
				t.Errorf("unexpected cloud config:\n got: %+v\nwant: %+v", *config, g.expected)
			}
		})
	}
}

func TestBuildCloudConfigErrors(t *testing.T) {
	grid := []struct {
		name    string
		cluster *kops.Cluster
	}{
		{
			name: "not an Azure cluster",
			cluster: &kops.Cluster{
				Spec: kops.ClusterSpec{
					Networking: kops.NetworkingSpec{
						Subnets: []kops.ClusterSubnetSpec{{Name: "subnet-a", Region: "eastus"}},
					},
				},
			},
		},
		{
			name: "cluster has no subnets",
			cluster: &kops.Cluster{
				Spec: kops.ClusterSpec{
					CloudProvider: kops.CloudProviderSpec{Azure: &kops.AzureSpec{}},
				},
			},
		},
	}
	for _, g := range grid {
		t.Run(g.name, func(t *testing.T) {
			if _, err := BuildCloudConfig(g.cluster); err == nil {
				t.Errorf("expected an error, got none")
			}
		})
	}
}
