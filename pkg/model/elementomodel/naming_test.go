package elementomodel

import (
	"reflect"
	"strings"
	"testing"

	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/elementotasks"
)

func TestSingleZoneNamesAcrossGroupsAndFilteredUpdates(t *testing.T) {
	b, _ := placementBuilderFixture(t)
	cp1 := newControlPlaneInstanceGroup("control-plane-europe")
	cp2 := newControlPlaneInstanceGroup("control-plane-europe-2")
	cp3 := newControlPlaneInstanceGroup("control-plane-europe-3")
	w1 := newControlPlaneInstanceGroup("workers-a")
	w1.Spec.Role = kops.InstanceGroupRoleNode
	w1.Spec.MinSize = fi.PtrTo(int32(2))
	w2 := newControlPlaneInstanceGroup("workers-b")
	w2.Spec.Role = kops.InstanceGroupRoleNode
	b.AllInstanceGroups = []*kops.InstanceGroup{cp3, w2, cp2, w1, cp1}
	b.InstanceGroups = []*kops.InstanceGroup{cp2}
	got, err := b.nodeNamesByInstanceGroup(nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		cp1.Name: {"control-plane-europe-1"}, cp2.Name: {"control-plane-europe-2"}, cp3.Name: {"control-plane-europe-3"},
		w1.Name: {"nodes-europe-1", "nodes-europe-2"}, w2.Name: {"nodes-europe-3"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("names = %#v", got)
	}
	names, err := b.nodeNamesForInstanceGroup(cp2)
	if err != nil || !reflect.DeepEqual(names, want[cp2.Name]) {
		t.Fatalf("filtered names = %v, %v", names, err)
	}
	records, err := b.elementoDNSRecordTasksForInstanceGroup(cp2, fi.LifecycleSync, &elementotasks.DNSZone{Name: fi.PtrTo("test.k8s")})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 || fi.ValueOf(records[0].Name) != names[0] || fi.ValueOf(records[0].DHCPReservation.Name) != names[0] {
		t.Fatal("DNS and DHCP naming mismatch")
	}
}

func TestSingleZoneNamingRejectsMultipleZonesBeforeTasks(t *testing.T) {
	b, c := placementBuilderFixture(t)
	b.AllInstanceGroups[1].Spec.Subnets = []string{"america"}
	err := b.Build(c)
	if err == nil || !strings.Contains(err.Error(), "exactly one zone") {
		t.Fatalf("expected zone error, got %v", err)
	}
	if len(c.Tasks) != 0 {
		t.Fatal("tasks created before validation")
	}
}

func TestSingleZoneMultipleSubnetsInSameZone(t *testing.T) {
	b, _ := placementBuilderFixture(t)
	b.Cluster.Spec.Networking.Subnets = []kops.ClusterSubnetSpec{{Name: "private", Zone: "europe"}, {Name: "public", Zone: "europe"}}
	b.AllInstanceGroups[0].Spec.Subnets = []string{"private"}
	b.AllInstanceGroups[1].Spec.Subnets = []string{"public"}
	if _, err := b.nodeNamesByInstanceGroup(nil); err != nil {
		t.Fatal(err)
	}
	b.Cluster.Spec.Networking.Subnets[1].Zone = "america"
	if _, err := b.nodeNamesByInstanceGroup(nil); err == nil {
		t.Fatal("multiple zones accepted")
	}
}
