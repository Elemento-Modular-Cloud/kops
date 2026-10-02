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

package elementotasks

import (
	"context"
	"fmt"
	"strings"

	"github.com/Elemento-Modular-Cloud/ecloud-go/ecloud"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/elemento"
)

// +kops:fitask
type DHCPService struct {
	Name        *string
	Lifecycle   fi.Lifecycle
	Network     *Network
	DNSZoneTask *DNSZone
}

var _ fi.CloudupTask = &DHCPService{}
var _ fi.CloudupHasDependencies = &DHCPService{}
var _ fi.HasLifecycle = &DHCPService{}
var _ fi.HasName = &DHCPService{}

func (d *DHCPService) GetDependencies(tasks map[string]fi.CloudupTask) []fi.CloudupTask {
	var deps []fi.CloudupTask
	if d.Network != nil {
		deps = append(deps, d.Network)
	}
	if d.DNSZoneTask != nil {
		deps = append(deps, d.DNSZoneTask)
	}
	return deps
}

func (d *DHCPService) GetLifecycle() fi.Lifecycle {
	return d.Lifecycle
}

func (d *DHCPService) SetLifecycle(lifecycle fi.Lifecycle) {
	d.Lifecycle = lifecycle
}

func (d *DHCPService) GetName() *string {
	return d.Name
}

func (d *DHCPService) String() string {
	return fi.CloudupTaskAsString(d)
}

func (d *DHCPService) Find(c *fi.CloudupContext) (*DHCPService, error) {
	cloud := c.T.Cloud.(elemento.ElementoCloud)
	client := cloud.DhcpClient()
	dhcp, _, err := client.Get(context.TODO())
	if err != nil {
		if isElementoDHCPMissing(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting Elemento DHCP service: %w", err)
	}
	if dhcp == nil {
		return nil, nil
	}

	networkName := fi.ValueOf(d.Name)
	for _, network := range dhcp.Networks {
		if network.NetworkName == networkName {
			return &DHCPService{
				Name:        d.Name,
				Lifecycle:   d.Lifecycle,
				Network:     d.Network,
				DNSZoneTask: d.DNSZoneTask,
			}, nil
		}
	}

	return nil, nil
}

func (d *DHCPService) Run(c *fi.CloudupContext) error {
	return fi.CloudupDefaultDeltaRunMethod(d, c)
}

func (_ *DHCPService) CheckChanges(actual, expected, changes *DHCPService) error {
	if expected.Name == nil {
		return fi.RequiredField("Name")
	}
	if expected.Network == nil {
		return fi.RequiredField("Network")
	}
	if expected.DNSZoneTask == nil {
		return fi.RequiredField("DNSZoneTask")
	}
	return nil
}

func (_ *DHCPService) RenderElemento(t *elemento.ElementoAPITarget, actual, expected, changes *DHCPService) error {
	networkName := fi.ValueOf(expected.Name)
	networkClient := t.Cloud.NetworkClient()
	network, _, err := networkClient.GetByName(context.TODO(), networkName)
	if err != nil {
		return fmt.Errorf("getting Elemento network %q for DHCP: %w", networkName, err)
	}
	if network == nil {
		return fmt.Errorf("Elemento network %q was not found for DHCP", networkName)
	}
	if strings.TrimSpace(network.ID) == "" {
		return fmt.Errorf("Elemento network %q has no UUID for DHCP interface", networkName)
	}

	dhcpClient := t.Cloud.DhcpClient()
	_, _, err = dhcpClient.Create(context.TODO(), ecloud.DhcpCreateOpts{
		NetworkName: network.Name,
		NetworkUID:  network.ID,
		GatewayCIDR: network.GatewayCIDR,
		PoolStart:   network.DHCPPoolStart,
		PoolEnd:     network.DHCPPoolEnd,
	})
	if err != nil {
		return fmt.Errorf("ensuring Elemento DHCP service for network %q: %w", networkName, err)
	}

	fmt.Printf("EKOPS: Elemento DHCP service for network %q ensured\n", networkName)
	return nil
}

// +kops:fitask
type DHCPReservation struct {
	Name        *string
	Lifecycle   fi.Lifecycle
	NetworkName *string
	MACAddress  *string
	IPAddress   *string
	DHCPService *DHCPService
	DependsOn   *DHCPReservation
}

var _ fi.CloudupTask = &DHCPReservation{}
var _ fi.CloudupHasDependencies = &DHCPReservation{}
var _ fi.HasLifecycle = &DHCPReservation{}
var _ fi.HasName = &DHCPReservation{}

func (d *DHCPReservation) GetDependencies(tasks map[string]fi.CloudupTask) []fi.CloudupTask {
	var deps []fi.CloudupTask
	if d.DHCPService != nil {
		deps = append(deps, d.DHCPService)
	}
	if d.DependsOn != nil {
		deps = append(deps, d.DependsOn)
	}
	return deps
}

func (d *DHCPReservation) GetLifecycle() fi.Lifecycle {
	return d.Lifecycle
}

func (d *DHCPReservation) SetLifecycle(lifecycle fi.Lifecycle) {
	d.Lifecycle = lifecycle
}

func (d *DHCPReservation) GetName() *string {
	return d.Name
}

func (d *DHCPReservation) String() string {
	return fi.CloudupTaskAsString(d)
}

func (d *DHCPReservation) Find(c *fi.CloudupContext) (*DHCPReservation, error) {
	cloud := c.T.Cloud.(elemento.ElementoCloud)
	client := cloud.DhcpClient()
	reservation, _, err := client.GetReservation(
		context.TODO(),
		fi.ValueOf(d.NetworkName),
		fi.ValueOf(d.MACAddress),
		fi.ValueOf(d.Name),
	)
	if err != nil {
		if isElementoDHCPMissing(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting Elemento DHCP reservation %q: %w", fi.ValueOf(d.Name), err)
	}
	if reservation == nil {
		return nil, nil
	}

	// Adopt an existing hostname reservation so rerunning kOps does not create
	// a second reservation when the initially generated MAC is no longer known.
	d.MACAddress = fi.PtrTo(reservation.MACAddress)
	d.IPAddress = fi.PtrTo(reservation.IPAddress)

	return &DHCPReservation{
		Name:        d.Name,
		Lifecycle:   d.Lifecycle,
		NetworkName: d.NetworkName,
		MACAddress:  d.MACAddress,
		IPAddress:   d.IPAddress,
		DHCPService: d.DHCPService,
		DependsOn:   d.DependsOn,
	}, nil
}

func (d *DHCPReservation) Run(c *fi.CloudupContext) error {
	return fi.CloudupDefaultDeltaRunMethod(d, c)
}

func (_ *DHCPReservation) CheckChanges(actual, expected, changes *DHCPReservation) error {
	if expected.Name == nil {
		return fi.RequiredField("Name")
	}
	if expected.NetworkName == nil {
		return fi.RequiredField("NetworkName")
	}
	if expected.MACAddress == nil {
		return fi.RequiredField("MACAddress")
	}
	if expected.DHCPService == nil {
		return fi.RequiredField("DHCPService")
	}
	return nil
}

func (_ *DHCPReservation) RenderElemento(t *elemento.ElementoAPITarget, actual, expected, changes *DHCPReservation) error {
	client := t.Cloud.DhcpClient()
	reservation, _, err := client.ReserveIpAddress(context.TODO(), ecloud.DhcpReserveAddressOpts{
		NetworkName: fi.ValueOf(expected.NetworkName),
		MACAddress:  fi.ValueOf(expected.MACAddress),
		Hostname:    fi.ValueOf(expected.Name),
	})
	if err != nil {
		return fmt.Errorf("reserving Elemento DHCP address for %q: %w", fi.ValueOf(expected.Name), err)
	}
	if reservation == nil || strings.TrimSpace(reservation.IPAddress) == "" {
		return fmt.Errorf("Elemento DHCP reservation for %q returned no IP address", fi.ValueOf(expected.Name))
	}

	expected.MACAddress = fi.PtrTo(reservation.MACAddress)
	expected.IPAddress = fi.PtrTo(reservation.IPAddress)
	fmt.Printf("EKOPS: Reserved DHCP address %q for server %q with MAC %q\n",
		reservation.IPAddress, fi.ValueOf(expected.Name), reservation.MACAddress)
	return nil
}

func isElementoDHCPMissing(err error) bool {
	if ecloud.IsError(err, ecloud.ErrorCodeNotFound) {
		return true
	}

	message := strings.ToLower(err.Error())
	return strings.Contains(message, "dhcp service not found") ||
		strings.Contains(message, "dhcp reservation not found") ||
		strings.Contains(message, "api error: 404")
}
