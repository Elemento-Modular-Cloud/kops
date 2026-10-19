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

// DHCPModelBuilder creates reservations for AtomOS nodes before their DNS
// records and VM registration. External nodes use the placement plan's IPs.
type DHCPModelBuilder struct {
	*ElementoModelContext
	Lifecycle fi.Lifecycle
}

var _ fi.CloudupModelBuilder = &DHCPModelBuilder{}

func (b *DHCPModelBuilder) Build(c *fi.CloudupModelBuilderContext) error {
	networkName := b.ClusterName()
	network := b.LinkToNetwork()
	dnsZoneTask := &elementotasks.DNSZone{
		Name:      new(b.ClusterName()),
		Network:   network,
		Lifecycle: b.Lifecycle,
	}
	c.EnsureTask(dnsZoneTask)

	service := &elementotasks.DHCPService{
		Name:        new(networkName),
		Lifecycle:   b.Lifecycle,
		Network:     network,
		DNSZoneTask: dnsZoneTask,
	}
	serviceAdded := false

	var previous *elementotasks.DHCPReservation
	for _, ig := range b.InstanceGroups {
		names, err := b.nodeNamesForInstanceGroup(ig)
		if err != nil {
			return err
		}
		for _, serverName := range names {
			if b.externalNodeIPs[serverName] != "" {
				continue
			}
			if !serviceAdded {
				c.AddTask(service)
				serviceAdded = true
			}
			// The reservation's MAC must also be attached to the VM, otherwise
			// DHCP cannot hand the node the address already published in DNS.
			macAddress, err := ecloud.GenerateElementoDHCPMACAddress()
			if err != nil {
				return fmt.Errorf("generating DHCP MAC address for server %q: %w", serverName, err)
			}

			reservation := &elementotasks.DHCPReservation{
				Name:        new(serverName),
				Lifecycle:   b.Lifecycle,
				NetworkName: new(networkName),
				MACAddress:  new(macAddress),
				DHCPService: service,
				DependsOn:   previous,
			}
			c.AddTask(reservation)
			previous = reservation
		}
	}

	return nil
}
