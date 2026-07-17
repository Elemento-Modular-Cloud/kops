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
	"testing"

	"k8s.io/kops/upup/pkg/fi"
)

func TestDNSRecordCheckChangesAllowsPendingDHCPReservation(t *testing.T) {
	expected := &DNSRecord{
		Name:    fi.PtrTo("control-plane-europe-1"),
		DNSZone: fi.PtrTo("test.k8s"),
		Type:    fi.PtrTo("A"),
		DHCPReservation: &DHCPReservation{
			Name:       fi.PtrTo("control-plane-europe-1"),
			MACAddress: fi.PtrTo("00:FF:A6:01:AA:BB"),
		},
	}

	if err := expected.CheckChanges(nil, expected, &DNSRecord{}); err != nil {
		t.Fatalf("expected pending DHCP reservation to be valid during planning: %v", err)
	}
}

func TestDNSZoneDependsOnNetwork(t *testing.T) {
	network := &Network{Name: fi.PtrTo("test.k8s")}
	zone := &DNSZone{
		Name:    fi.PtrTo("test.k8s"),
		Network: network,
	}

	dependencies := zone.GetDependencies(nil)
	if len(dependencies) != 1 || dependencies[0] != network {
		t.Fatalf("expected DNS zone to depend on its network, got %#v", dependencies)
	}
}

func TestDNSZoneCheckChangesRequiresNetwork(t *testing.T) {
	expected := &DNSZone{Name: fi.PtrTo("test.k8s")}
	if err := expected.CheckChanges(nil, expected, &DNSZone{}); err == nil {
		t.Fatal("expected DNS zone without a network to be rejected")
	}
}
