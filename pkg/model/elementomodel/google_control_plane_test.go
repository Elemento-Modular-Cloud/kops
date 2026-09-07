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
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/model"
	"k8s.io/kops/pkg/model/iam"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/elementotasks"
)

func TestGoogleControlPlaneIPForInstanceGroup(t *testing.T) {
	t.Setenv(googleControlPlaneEnv, "10.0.255.1")
	t.Setenv(googleControlPlaneInstanceGroupEnv, "control-plane-europe-2")

	controlPlane := &kops.InstanceGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "control-plane-europe-2"},
		Spec: kops.InstanceGroupSpec{
			Role:    kops.InstanceGroupRoleControlPlane,
			MinSize: fi.PtrTo(int32(1)),
		},
	}
	ip, enabled, err := googleControlPlaneIPForInstanceGroup(controlPlane)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enabled || ip != "10.0.255.1" {
		t.Fatalf("expected external control-plane IP 10.0.255.1, got ip=%q enabled=%t", ip, enabled)
	}

	atomosControlPlane := controlPlane.DeepCopy()
	atomosControlPlane.Name = "control-plane-europe-1"
	ip, enabled, err = googleControlPlaneIPForInstanceGroup(atomosControlPlane)
	if err != nil {
		t.Fatalf("unexpected AtomOS control-plane error: %v", err)
	}
	if enabled || ip != "" {
		t.Fatalf("expected AtomOS control-plane provisioning to remain enabled, got ip=%q enabled=%t", ip, enabled)
	}

	worker := &kops.InstanceGroup{
		Spec: kops.InstanceGroupSpec{
			Role:    kops.InstanceGroupRoleNode,
			MinSize: fi.PtrTo(int32(2)),
		},
	}
	ip, enabled, err = googleControlPlaneIPForInstanceGroup(worker)
	if err != nil {
		t.Fatalf("unexpected worker error: %v", err)
	}
	if enabled || ip != "" {
		t.Fatalf("expected worker provisioning to remain unchanged, got ip=%q enabled=%t", ip, enabled)
	}
}

func TestGoogleControlPlaneIPsForMultipleInstanceGroups(t *testing.T) {
	t.Setenv(googleControlPlanesEnv, "control-plane-europe-2=10.0.255.1, control-plane-europe-3=10.0.254.1")

	tests := []struct {
		name     string
		expected string
		external bool
	}{
		{name: "control-plane-europe-1"},
		{name: "control-plane-europe-2", expected: "10.0.255.1", external: true},
		{name: "control-plane-europe-3", expected: "10.0.254.1", external: true},
	}
	for _, test := range tests {
		ig := &kops.InstanceGroup{
			ObjectMeta: metav1.ObjectMeta{Name: test.name},
			Spec: kops.InstanceGroupSpec{
				Role:    kops.InstanceGroupRoleControlPlane,
				MinSize: fi.PtrTo(int32(1)),
			},
		}
		ip, external, err := googleControlPlaneIPForInstanceGroup(ig)
		if err != nil {
			t.Fatalf("resolving %q: %v", test.name, err)
		}
		if external != test.external || ip != test.expected {
			t.Errorf("instance group %q: got ip=%q external=%t, expected ip=%q external=%t", test.name, ip, external, test.expected, test.external)
		}
	}
}

func TestGoogleControlPlaneIPForInstanceGroupRejectsInvalidConfiguration(t *testing.T) {
	controlPlane := &kops.InstanceGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "control-plane-europe"},
		Spec: kops.InstanceGroupSpec{
			Role:    kops.InstanceGroupRoleControlPlane,
			MinSize: fi.PtrTo(int32(1)),
		},
	}

	t.Setenv(googleControlPlaneEnv, "not-an-ip")
	if _, _, err := googleControlPlaneIPForInstanceGroup(controlPlane); err == nil {
		t.Fatal("expected invalid IPv4 address to be rejected")
	}

	t.Setenv(googleControlPlaneEnv, "10.0.255.1")
	controlPlane.Spec.MinSize = fi.PtrTo(int32(2))
	if _, _, err := googleControlPlaneIPForInstanceGroup(controlPlane); err == nil {
		t.Fatal("expected multiple control-plane instances to be rejected")
	}
}

func TestValidateGoogleControlPlaneConfiguration(t *testing.T) {
	t.Setenv(googleControlPlaneEnv, "10.0.255.1")
	controlPlanes := []*kops.InstanceGroup{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "control-plane-europe-1"},
			Spec: kops.InstanceGroupSpec{
				Role:    kops.InstanceGroupRoleControlPlane,
				MinSize: fi.PtrTo(int32(1)),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "control-plane-europe-2"},
			Spec: kops.InstanceGroupSpec{
				Role:    kops.InstanceGroupRoleControlPlane,
				MinSize: fi.PtrTo(int32(1)),
			},
		},
	}

	if err := validateGoogleControlPlaneConfiguration(controlPlanes); err == nil {
		t.Fatalf("expected multiple control planes without %s to be rejected", googleControlPlaneInstanceGroupEnv)
	}
	t.Setenv(googleControlPlaneInstanceGroupEnv, "control-plane-does-not-exist")
	if err := validateGoogleControlPlaneConfiguration(controlPlanes); err == nil {
		t.Fatal("expected an unknown Google control-plane instance group to be rejected")
	}
	t.Setenv(googleControlPlaneInstanceGroupEnv, "control-plane-europe-2")
	if err := validateGoogleControlPlaneConfiguration(controlPlanes); err != nil {
		t.Fatalf("expected a matching Google control-plane instance group to pass validation: %v", err)
	}
}

func TestValidateMultipleGoogleControlPlanes(t *testing.T) {
	t.Setenv(googleControlPlanesEnv, "control-plane-europe-2=10.0.255.1,control-plane-europe-3=10.0.254.1")
	controlPlanes := []*kops.InstanceGroup{
		newControlPlaneInstanceGroup("control-plane-europe-1"),
		newControlPlaneInstanceGroup("control-plane-europe-2"),
		newControlPlaneInstanceGroup("control-plane-europe-3"),
	}
	if err := validateGoogleControlPlaneConfiguration(controlPlanes); err != nil {
		t.Fatalf("expected two Google control planes and one AtomOS control plane to pass validation: %v", err)
	}

	t.Setenv(googleControlPlanesEnv, "control-plane-europe-2=10.0.255.1,control-plane-europe-3=10.0.255.1")
	if err := validateGoogleControlPlaneConfiguration(controlPlanes); err == nil {
		t.Fatal("expected duplicate Google control-plane IPs to be rejected")
	}

	t.Setenv(googleControlPlanesEnv, "control-plane-europe-1=10.0.253.1,control-plane-europe-2=10.0.255.1,control-plane-europe-3=10.0.254.1")
	if err := validateGoogleControlPlaneConfiguration(controlPlanes); err == nil {
		t.Fatal("expected a configuration without an AtomOS control plane to be rejected")
	}
}

func TestGoogleControlPlaneDNSRecordsUseStaticIP(t *testing.T) {
	t.Setenv(googleControlPlaneEnv, "10.0.255.1")

	cluster := &kops.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test.k8s"},
	}
	context := &ElementoModelContext{
		KopsModelContext: &model.KopsModelContext{
			IAMModelContext: iam.IAMModelContext{Cluster: cluster},
		},
	}
	controlPlane := &kops.InstanceGroup{
		ObjectMeta: metav1.ObjectMeta{Name: "control-plane-europe"},
		Spec: kops.InstanceGroupSpec{
			Role:    kops.InstanceGroupRoleControlPlane,
			MinSize: fi.PtrTo(int32(1)),
		},
	}
	zone := &elementotasks.DNSZone{Name: fi.PtrTo("test.k8s")}

	records, err := context.elementoDNSRecordTasksForInstanceGroup(controlPlane, fi.LifecycleSync, zone)
	if err != nil {
		t.Fatalf("building control-plane DNS records: %v", err)
	}
	if len(records) != 8 {
		t.Fatalf("expected 8 control-plane DNS records, got %d", len(records))
	}
	for _, record := range records {
		if got := fi.ValueOf(record.Data); got != "10.0.255.1" {
			t.Errorf("record %q has IP %q, expected 10.0.255.1", fi.ValueOf(record.Name), got)
		}
		if record.DHCPReservation != nil {
			t.Errorf("record %q unexpectedly depends on DHCP", fi.ValueOf(record.Name))
		}
	}
}

func TestElementoControlPlaneDNSRecordsForMixedPlacement(t *testing.T) {
	t.Setenv(googleControlPlanesEnv, "control-plane-europe-2=10.0.255.1,control-plane-europe-3=10.0.254.1")

	atomos := newControlPlaneInstanceGroup("control-plane-europe-1")
	google1 := newControlPlaneInstanceGroup("control-plane-europe-2")
	google2 := newControlPlaneInstanceGroup("control-plane-europe-3")
	cluster := &kops.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test.k8s"},
		Spec: kops.ClusterSpec{
			EtcdClusters: []kops.EtcdClusterSpec{
				{
					Name: "main",
					Members: []kops.EtcdMemberSpec{
						{InstanceGroup: fi.PtrTo(atomos.Name)},
						{InstanceGroup: fi.PtrTo(google1.Name)},
						{InstanceGroup: fi.PtrTo(google2.Name)},
					},
				},
				{
					Name: "events",
					Members: []kops.EtcdMemberSpec{
						{InstanceGroup: fi.PtrTo(atomos.Name)},
						{InstanceGroup: fi.PtrTo(google1.Name)},
						{InstanceGroup: fi.PtrTo(google2.Name)},
					},
				},
			},
		},
	}
	context := &ElementoModelContext{
		KopsModelContext: &model.KopsModelContext{
			IAMModelContext: iam.IAMModelContext{Cluster: cluster},
			InstanceGroups:  []*kops.InstanceGroup{atomos, google1, google2},
		},
	}
	zone := &elementotasks.DNSZone{Name: fi.PtrTo("test.k8s")}

	atomosRecords, err := context.elementoDNSRecordTasksForInstanceGroup(atomos, fi.LifecycleSync, zone)
	if err != nil {
		t.Fatalf("building AtomOS control-plane DNS records: %v", err)
	}
	google1Records, err := context.elementoDNSRecordTasksForInstanceGroup(google1, fi.LifecycleSync, zone)
	if err != nil {
		t.Fatalf("building first Google control-plane DNS records: %v", err)
	}
	google2Records, err := context.elementoDNSRecordTasksForInstanceGroup(google2, fi.LifecycleSync, zone)
	if err != nil {
		t.Fatalf("building second Google control-plane DNS records: %v", err)
	}

	assertDNSRecordNames(t, atomosRecords, []string{
		"control-plane-europe-1-1",
		"api",
		"api.internal",
		"kops-controller.internal",
		"node0.main",
		"test.k8s--main--0.internal",
		"node0.events",
		"test.k8s--events--0.internal",
	})
	assertDNSRecordNames(t, google1Records, []string{
		"control-plane-europe-2-1",
		"node1.main",
		"test.k8s--main--1.internal",
		"node1.events",
		"test.k8s--events--1.internal",
	})
	assertDNSRecordNames(t, google2Records, []string{
		"control-plane-europe-3-1",
		"node2.main",
		"test.k8s--main--2.internal",
		"node2.events",
		"test.k8s--events--2.internal",
	})
	for _, record := range google1Records {
		if got := fi.ValueOf(record.Data); got != "10.0.255.1" {
			t.Errorf("record %q has IP %q, expected 10.0.255.1", fi.ValueOf(record.Name), got)
		}
	}
	for _, record := range google2Records {
		if got := fi.ValueOf(record.Data); got != "10.0.254.1" {
			t.Errorf("record %q has IP %q, expected 10.0.254.1", fi.ValueOf(record.Name), got)
		}
	}
}

func newControlPlaneInstanceGroup(name string) *kops.InstanceGroup {
	return &kops.InstanceGroup{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: kops.InstanceGroupSpec{
			Role:    kops.InstanceGroupRoleControlPlane,
			MinSize: fi.PtrTo(int32(1)),
		},
	}
}

func assertDNSRecordNames(t *testing.T, records []*elementotasks.DNSRecord, expected []string) {
	t.Helper()
	if len(records) != len(expected) {
		t.Fatalf("expected %d DNS records, got %d", len(expected), len(records))
	}
	for index, record := range records {
		if got := fi.ValueOf(record.Name); got != expected[index] {
			t.Errorf("record %d has name %q, expected %q", index, got, expected[index])
		}
	}
}
