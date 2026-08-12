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

	controlPlane := &kops.InstanceGroup{
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
