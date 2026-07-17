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
	"fmt"

	"github.com/Elemento-Modular-Cloud/ecloud-go/ecloud"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/elementotasks"
)

// DHCPModelBuilder creates the DHCP service and all node reservations before
// any Elemento VM is registered.
type DHCPModelBuilder struct {
	*ElementoModelContext
	Lifecycle fi.Lifecycle
}

var _ fi.CloudupModelBuilder = &DHCPModelBuilder{}

func (b *DHCPModelBuilder) Build(c *fi.CloudupModelBuilderContext) error {
	networkName := b.ClusterName()
	network := b.LinkToNetwork()
	dnsZoneTask := &elementotasks.DNSZone{
		Name:      fi.PtrTo(b.ClusterName()),
		Network:   network,
		Lifecycle: b.Lifecycle,
	}
	c.EnsureTask(dnsZoneTask)

	service := &elementotasks.DHCPService{
		Name:        fi.PtrTo(networkName),
		Lifecycle:   b.Lifecycle,
		Network:     network,
		DNSZoneTask: dnsZoneTask,
	}
	c.AddTask(service)

	var previous *elementotasks.DHCPReservation
	for _, ig := range b.InstanceGroups {
		for ordinal := int32(1); ordinal <= fi.ValueOf(ig.Spec.MinSize); ordinal++ {
			serverName := fmt.Sprintf("%s-%d", ig.Name, ordinal)
			macAddress, err := ecloud.GenerateElementoDHCPMACAddress()
			if err != nil {
				return fmt.Errorf("generating DHCP MAC address for server %q: %w", serverName, err)
			}

			reservation := &elementotasks.DHCPReservation{
				Name:        fi.PtrTo(serverName),
				Lifecycle:   b.Lifecycle,
				NetworkName: fi.PtrTo(networkName),
				MACAddress:  fi.PtrTo(macAddress),
				DHCPService: service,
				DependsOn:   previous,
			}
			c.AddTask(reservation)
			previous = reservation
		}
	}

	return nil
}
