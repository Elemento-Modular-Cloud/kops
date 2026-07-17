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
	"k8s.io/klog/v2"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/elemento"
)

// +kops:fitask
type DNSZone struct {
	Name      *string
	Network   *Network
	IPAddress *string
	Lifecycle fi.Lifecycle
}

var _ fi.CloudupTask = &DNSZone{}
var _ fi.CloudupHasDependencies = &DNSZone{}
var _ fi.HasLifecycle = &DNSZone{}
var _ fi.HasName = &DNSZone{}

func (z *DNSZone) GetDependencies(tasks map[string]fi.CloudupTask) []fi.CloudupTask {
	if z.Network == nil {
		return nil
	}
	return []fi.CloudupTask{z.Network}
}

func (z *DNSZone) GetLifecycle() fi.Lifecycle {
	return z.Lifecycle
}

func (z *DNSZone) SetLifecycle(lifecycle fi.Lifecycle) {
	z.Lifecycle = lifecycle
}

func (z *DNSZone) GetName() *string {
	return z.Name
}

func (z *DNSZone) String() string {
	return fi.CloudupTaskAsString(z)
}

func (z *DNSZone) Find(c *fi.CloudupContext) (*DNSZone, error) {
	cloud := c.T.Cloud.(elemento.ElementoCloud)
	client := cloud.DnsClient()
	zoneName := fi.ValueOf(z.Name)

	fmt.Printf("EKOPS: Finding Elemento DNS zone %q\n", zoneName)

	dns, _, err := client.Get(context.TODO(), zoneName)
	if err != nil {
		if elemento.IsDNSMissing(err) {
			fmt.Printf("EKOPS: Elemento DNS zone %q not found\n", zoneName)
			return nil, nil
		}
		return nil, fmt.Errorf("getting Elemento DNS zone %q: %w", zoneName, err)
	}
	if dns == nil {
		fmt.Printf("EKOPS: Elemento DNS zone %q not found\n", zoneName)
		return nil, nil
	}
	dnsIPAddress := strings.TrimSpace(dns.IPAddress)
	if dnsIPAddress == "" {
		return nil, fmt.Errorf("Elemento DNS zone %q has no service IP address", zoneName)
	}
	z.IPAddress = fi.PtrTo(dnsIPAddress)

	fmt.Printf("EKOPS: Elemento DNS zone %q already exists\n", zoneName)
	return &DNSZone{
		Name:      fi.PtrTo(dns.ZoneName),
		Network:   z.Network,
		IPAddress: z.IPAddress,
		Lifecycle: z.Lifecycle,
	}, nil
}

func (z *DNSZone) Run(c *fi.CloudupContext) error {
	return fi.CloudupDefaultDeltaRunMethod(z, c)
}

func (_ *DNSZone) CheckChanges(actual, expected, changes *DNSZone) error {
	if expected.Name == nil {
		return fi.RequiredField("Name")
	}
	if expected.Network == nil {
		return fi.RequiredField("Network")
	}
	return nil
}

func (_ *DNSZone) RenderElemento(t *elemento.ElementoAPITarget, actual, expected, changes *DNSZone) error {
	zoneName := fi.ValueOf(expected.Name)
	networkUID := strings.TrimSpace(fi.ValueOf(expected.Network.ID))
	if networkUID == "" {
		return fmt.Errorf("Elemento network %q has no UUID for DNS service", fi.ValueOf(expected.Network.Name))
	}

	fmt.Printf("EKOPS: Ensuring Elemento DNS zone %q\n", zoneName)
	dnsService, err := ensureElementoDNSZone(context.TODO(), t.Cloud.DnsClient(), zoneName, networkUID)
	if err != nil {
		return err
	}
	if dnsService == nil || strings.TrimSpace(dnsService.IPAddress) == "" {
		return fmt.Errorf("Elemento DNS zone %q returned no service IP address", zoneName)
	}
	expected.IPAddress = fi.PtrTo(strings.TrimSpace(dnsService.IPAddress))
	fmt.Printf("EKOPS: Elemento DNS zone %q ensured\n", zoneName)
	return nil
}

// +kops:fitask
type DNSRecord struct {
	Name            *string
	Data            *string
	DNSZone         *string
	DNSZoneTask     *DNSZone
	DHCPReservation *DHCPReservation
	DependsOn       *DNSRecord
	Type            *string
	TTL             *int64
	Lifecycle       fi.Lifecycle
	Comment         *string
}

var _ fi.CloudupTask = &DNSRecord{}
var _ fi.CloudupHasDependencies = &DNSRecord{}

func (d *DNSRecord) GetDependencies(tasks map[string]fi.CloudupTask) []fi.CloudupTask {
	var deps []fi.CloudupTask
	if d.DNSZoneTask != nil {
		deps = append(deps, d.DNSZoneTask)
	}
	if d.DHCPReservation != nil {
		deps = append(deps, d.DHCPReservation)
	}
	if d.DependsOn != nil {
		deps = append(deps, d.DependsOn)
	}
	return deps
}

func (d *DNSRecord) Find(c *fi.CloudupContext) (*DNSRecord, error) {
	cloud := c.T.Cloud.(elemento.ElementoCloud)
	client := cloud.DnsClient()
	if d.DHCPReservation != nil {
		d.Data = d.DHCPReservation.IPAddress
	}

	record, _, err := client.GetDnsRecord(context.TODO(), fi.ValueOf(d.DNSZone), fi.ValueOf(d.Name), fi.ValueOf(d.Type))
	if err != nil {
		if elemento.IsDNSMissing(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("getting Elemento DNS record %q in zone %q: %w", fi.ValueOf(d.Name), fi.ValueOf(d.DNSZone), err)
	}
	if record == nil {
		return nil, nil
	}

	return &DNSRecord{
		Name:            fi.PtrTo(record.Name),
		Data:            fi.PtrTo(record.Value),
		DNSZone:         d.DNSZone,
		DNSZoneTask:     d.DNSZoneTask,
		DHCPReservation: d.DHCPReservation,
		DependsOn:       d.DependsOn,
		Type:            fi.PtrTo(record.Type),
		TTL:             fi.PtrTo(int64(record.TTL)),
		Lifecycle:       d.Lifecycle,
		Comment:         d.Comment,
	}, nil
}

func (d *DNSRecord) Run(c *fi.CloudupContext) error {
	return fi.CloudupDefaultDeltaRunMethod(d, c)
}

func (_ *DNSRecord) CheckChanges(actual, expected, changes *DNSRecord) error {
	if expected.Name == nil {
		return fi.RequiredField("Name")
	}
	if expected.DNSZone == nil {
		return fi.RequiredField("DNSZone")
	}
	if expected.Type == nil {
		return fi.RequiredField("Type")
	}
	if fi.ValueOf(expected.Type) != "A" {
		return fmt.Errorf("Elemento DNS currently supports only A records, got %q", fi.ValueOf(expected.Type))
	}
	if expected.Data == nil {
		if expected.DHCPReservation == nil {
			return fi.RequiredField("Data")
		}
	}

	return nil
}

func (_ *DNSRecord) RenderElemento(t *elemento.ElementoAPITarget, actual, expected, changes *DNSRecord) error {
	client := t.Cloud.DnsClient()
	zoneName := fi.ValueOf(expected.DNSZone)
	recordName := fi.ValueOf(expected.Name)
	recordValue := fi.ValueOf(expected.Data)
	if expected.DHCPReservation != nil {
		recordValue = fi.ValueOf(expected.DHCPReservation.IPAddress)
		expected.Data = fi.PtrTo(recordValue)
	}
	if recordValue == "" {
		return fmt.Errorf("Elemento DNS record %q in zone %q has no IP address", recordName, zoneName)
	}

	if err := ensureElementoDNSRecord(context.TODO(), client, zoneName, recordName, recordValue); err != nil {
		return err
	}

	return nil
}

func ensureElementoDNSZone(ctx context.Context, client ecloud.DnsClient, zoneName, networkUID string) (*ecloud.Dns, error) {
	dnsService, _, err := client.Create(ctx, zoneName, networkUID)
	if err != nil {
		if elemento.IsDNSAlreadyExists(err) {
			klog.V(2).Infof("Elemento DNS zone %q already exists", zoneName)
			dnsService, _, err = client.Get(ctx, zoneName)
			if err != nil {
				return nil, fmt.Errorf("getting existing Elemento DNS zone %q: %w", zoneName, err)
			}
			return dnsService, nil
		}
		return nil, fmt.Errorf("creating Elemento DNS zone %q: %w", zoneName, err)
	}

	klog.V(2).Infof("Created Elemento DNS zone %q", zoneName)
	return dnsService, nil
}

func ensureElementoDNSRecord(ctx context.Context, client ecloud.DnsClient, zoneName, recordName, recordValue string) error {
	record, _, err := client.AddDnsRecord(ctx, zoneName, recordName, recordValue)
	if err != nil {
		if elemento.IsDNSAlreadyExists(err) {
			klog.V(2).Infof("Elemento DNS record %q in zone %q already exists", recordName, zoneName)
			return nil
		}
		return fmt.Errorf("creating Elemento DNS record %q in zone %q: %w", recordName, zoneName, err)
	}

	klog.V(2).Infof("Created Elemento DNS record %q in zone %q as %q", recordName, zoneName, record.Name)
	return nil
}
