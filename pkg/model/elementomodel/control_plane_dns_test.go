package elementomodel

import (
	"fmt"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/model"
	"k8s.io/kops/pkg/model/iam"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/elementotasks"
)

func newControlPlaneInstanceGroup(name string) *kops.InstanceGroup {
	return &kops.InstanceGroup{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: kops.InstanceGroupSpec{
			Role:    kops.InstanceGroupRoleControlPlane,
			MinSize: fi.PtrTo(int32(1)), Subnets: []string{"europe"},
		},
	}
}

func TestControlPlaneDNSUsesReservationsAndEtcdTopology(t *testing.T) {
	groups := []*kops.InstanceGroup{
		newControlPlaneInstanceGroup("control-plane-europe-1"),
		newControlPlaneInstanceGroup("control-plane-europe-2"),
		newControlPlaneInstanceGroup("control-plane-europe-3"),
	}
	cluster := &kops.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test.k8s"}}
	for _, name := range []string{"main", "events"} {
		etcd := kops.EtcdClusterSpec{Name: name}
		for _, group := range groups {
			etcd.Members = append(etcd.Members, kops.EtcdMemberSpec{InstanceGroup: fi.PtrTo(group.Name)})
		}
		cluster.Spec.EtcdClusters = append(cluster.Spec.EtcdClusters, etcd)
	}
	ctx := &ElementoModelContext{KopsModelContext: &model.KopsModelContext{
		IAMModelContext:   iam.IAMModelContext{Cluster: cluster},
		AllInstanceGroups: []*kops.InstanceGroup{groups[2], groups[0], groups[1]},
		InstanceGroups:    []*kops.InstanceGroup{groups[1]},
	}}
	for i, group := range groups {
		records, err := ctx.elementoDNSRecordTasksForInstanceGroup(group, fi.LifecycleSync,
			&elementotasks.DNSZone{Name: fi.PtrTo(cluster.Name)})
		if err != nil {
			t.Fatal(err)
		}
		want := []string{group.Name}
		if i == 0 {
			want = append(want, "api", "api.internal", "kops-controller.internal")
		}
		for _, name := range []string{"main", "events"} {
			want = append(want, fmt.Sprintf("node%d.%s", i, name),
				fmt.Sprintf("test.k8s--%s--%d.internal", name, i))
		}
		var got []string
		for _, record := range records {
			got = append(got, fi.ValueOf(record.Name))
			if record.Data != nil || record.DHCPReservation == nil ||
				fi.ValueOf(record.DHCPReservation.Name) != group.Name {
				t.Fatalf("record %s must use its node reservation", fi.ValueOf(record.Name))
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("group %s: records %v, want %v", group.Name, got, want)
		}
	}
}
