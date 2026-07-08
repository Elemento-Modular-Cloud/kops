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

package azuretasks

import (
	"fmt"

	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/azure"
	"k8s.io/kops/upup/pkg/fi/cloudup/terraform"
	"k8s.io/kops/upup/pkg/fi/cloudup/terraformWriter"
)

type subscriptionIDProvider interface {
	SubscriptionID() string
}

type terraformAzureSubnet struct {
	Name               *string                  `cty:"name"`
	ResourceGroupName  *terraformWriter.Literal `cty:"resource_group_name"`
	VirtualNetworkName *terraformWriter.Literal `cty:"virtual_network_name"`
	AddressPrefixes    []string                 `cty:"address_prefixes"`
}

type terraformAzureSubnetNetworkSecurityGroupAssociation struct {
	SubnetID               *terraformWriter.Literal `cty:"subnet_id"`
	NetworkSecurityGroupID *terraformWriter.Literal `cty:"network_security_group_id"`
}

type terraformAzureSubnetNatGatewayAssociation struct {
	SubnetID     *terraformWriter.Literal `cty:"subnet_id"`
	NatGatewayID *terraformWriter.Literal `cty:"nat_gateway_id"`
}

type terraformAzureSubnetRouteTableAssociation struct {
	SubnetID     *terraformWriter.Literal `cty:"subnet_id"`
	RouteTableID *terraformWriter.Literal `cty:"route_table_id"`
}

func (*Subnet) RenderTerraform(t *terraform.TerraformTarget, a, e, changes *Subnet) error {
	if !fi.ValueOf(e.Shared) {
		tf := &terraformAzureSubnet{
			Name:               e.Name,
			ResourceGroupName:  e.ResourceGroup.terraformName(),
			VirtualNetworkName: e.VirtualNetwork.terraformName(),
			AddressPrefixes:    []string{fi.ValueOf(e.CIDR)},
		}
		if err := t.RenderResource("azurerm_subnet", fi.ValueOf(e.Name), tf); err != nil {
			return err
		}
	}

	if e.NetworkSecurityGroup != nil {
		subnetID, err := e.terraformID(t)
		if err != nil {
			return err
		}
		tf := &terraformAzureSubnetNetworkSecurityGroupAssociation{
			SubnetID:               subnetID,
			NetworkSecurityGroupID: e.NetworkSecurityGroup.terraformID(),
		}
		if err := t.RenderResource("azurerm_subnet_network_security_group_association", e.terraformAssociationName("nsg"), tf); err != nil {
			return err
		}
	}

	if e.NatGateway != nil {
		subnetID, err := e.terraformID(t)
		if err != nil {
			return err
		}
		tf := &terraformAzureSubnetNatGatewayAssociation{
			SubnetID:     subnetID,
			NatGatewayID: e.NatGateway.terraformID(),
		}
		if err := t.RenderResource("azurerm_subnet_nat_gateway_association", e.terraformAssociationName("natgw"), tf); err != nil {
			return err
		}
	}

	if e.RouteTable != nil {
		subnetID, err := e.terraformID(t)
		if err != nil {
			return err
		}
		tf := &terraformAzureSubnetRouteTableAssociation{
			SubnetID:     subnetID,
			RouteTableID: e.RouteTable.terraformID(),
		}
		if err := t.RenderResource("azurerm_subnet_route_table_association", e.terraformAssociationName("rt"), tf); err != nil {
			return err
		}
	}

	return nil
}

func (s *Subnet) terraformAssociationName(suffix string) string {
	return fmt.Sprintf("%s-%s-%s", fi.ValueOf(s.VirtualNetwork.Name), fi.ValueOf(s.Name), suffix)
}

func (s *Subnet) terraformID(t *terraform.TerraformTarget) (*terraformWriter.Literal, error) {
	if !fi.ValueOf(s.Shared) {
		return terraformWriter.LiteralProperty("azurerm_subnet", fi.ValueOf(s.Name), "id"), nil
	}

	subscriptionID, err := azureSubscriptionID(t)
	if err != nil {
		return nil, err
	}
	subnetID := azure.SubnetID{
		SubscriptionID:     subscriptionID,
		ResourceGroupName:  fi.ValueOf(s.ResourceGroup.Name),
		VirtualNetworkName: fi.ValueOf(s.VirtualNetwork.Name),
		SubnetName:         fi.ValueOf(s.Name),
	}
	return terraformWriter.LiteralFromStringValue(subnetID.String()), nil
}

func azureSubscriptionID(t *terraform.TerraformTarget) (string, error) {
	cloud, ok := t.Cloud.(subscriptionIDProvider)
	if !ok {
		return "", fmt.Errorf("cloud %T does not expose Azure subscription ID", t.Cloud)
	}
	subscriptionID := cloud.SubscriptionID()
	if subscriptionID == "" {
		return "", fmt.Errorf("Azure subscription ID is empty")
	}
	return subscriptionID, nil
}
