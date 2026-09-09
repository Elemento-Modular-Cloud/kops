package elementomodel

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Elemento-Modular-Cloud/ecloud-go/ecloud"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/pkg/model"
	"k8s.io/kops/pkg/model/iam"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/elementotasks"
	"k8s.io/kops/upup/pkg/fi/fitasks"
	"k8s.io/kops/util/pkg/vfs"
)

func placementBuilderFixture(t *testing.T) (*PlacementModelBuilder, *fi.CloudupModelBuilderContext) {
	t.Helper()
	for key, value := range map[string]string{
		"ATOMOS_CONTROL_PLANES": "192.168.1.1", "ATOMOS_WORKERS": "192.168.1.2",
		"PROVIDERS": "", "ATOMOS_K8S_SERVICES": "192.168.1.9",
	} {
		t.Setenv(key, value)
	}
	groups := []*kops.InstanceGroup{newControlPlaneInstanceGroup("control-plane-europe"), {
		ObjectMeta: metav1.ObjectMeta{Name: "nodes-europe"},
		Spec:       kops.InstanceGroupSpec{Role: kops.InstanceGroupRoleNode, MinSize: fi.PtrTo(int32(1))},
	}}
	b := &PlacementModelBuilder{
		ElementoModelContext: &ElementoModelContext{KopsModelContext: &model.KopsModelContext{
			IAMModelContext:   iam.IAMModelContext{Cluster: &kops.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test.k8s"}}},
			AllInstanceGroups: groups, InstanceGroups: groups,
		}},
		ConfigBase: vfs.NewMemFSPath(vfs.NewMemFSContext(), "memfs://state/test.k8s"),
		Lifecycle:  fi.LifecycleSync,
	}
	return b, &fi.CloudupModelBuilderContext{Tasks: map[string]fi.CloudupTask{}}
}

func TestPlacementBuilderPersistsAndReusesPlan(t *testing.T) {
	b, c := placementBuilderFixture(t)
	if err := b.Build(c); err != nil {
		t.Fatal(err)
	}
	task, ok := c.Tasks["ManagedFile/elemento-placement"].(*fitasks.ManagedFile)
	if !ok {
		t.Fatal(c.Tasks)
	}
	data, err := fi.ResourceAsBytes(task.Contents)
	if err != nil {
		t.Fatal(err)
	}
	var plan ecloud.MulticloudPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Nodes) != 2 || plan.ServicesTarget != "192.168.1.9" {
		t.Fatal(plan)
	}
	if err := b.ConfigBase.Join(placementPlanLocation).WriteFile(context.Background(), bytes.NewReader(data), nil); err != nil {
		t.Fatal(err)
	}
	// A filtered update still plans all cluster instance groups.
	b.InstanceGroups = b.InstanceGroups[:1]
	c = &fi.CloudupModelBuilderContext{Tasks: map[string]fi.CloudupTask{}}
	if err := b.Build(c); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ATOMOS_WORKERS", "192.168.1.3")
	c = &fi.CloudupModelBuilderContext{Tasks: map[string]fi.CloudupTask{}}
	if err := b.Build(c); err == nil || !strings.Contains(err.Error(), "refusing to move") {
		t.Fatalf("expected drift rejection: %v", err)
	}
}

func TestPlacementBuilderFailsBeforeAddingTasks(t *testing.T) {
	for _, test := range []struct {
		name    string
		env     map[string]string
		message string
	}{
		{"counts", map[string]string{"ATOMOS_WORKERS": ""}, "count mismatch"},
		{"adapter", map[string]string{"ATOMOS_WORKERS": "", "PROVIDERS": "google", "GOOGLE_WORKERS": "1", "GOOGLE_CONTROL_PLANES": "0"}, "no provisioning adapter"},
	} {
		t.Run(test.name, func(t *testing.T) {
			b, c := placementBuilderFixture(t)
			for k, v := range test.env {
				t.Setenv(k, v)
			}
			if err := b.Build(c); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(c.Tasks) != 0 {
				t.Fatal("tasks added before validation")
			}
		})
	}
}

func TestPlacementNetworkDependency(t *testing.T) {
	_, _ = placementBuilderFixture(t)
	network := &elementotasks.Network{}
	task := &fitasks.ManagedFile{Name: fi.PtrTo("elemento-placement")}
	tasks := map[string]fi.CloudupTask{"ManagedFile/elemento-placement": task, "Network/test.k8s": network}
	deps := network.GetDependencies(tasks)
	if len(deps) != 1 {
		t.Fatal(deps)
	}
	if dep, ok := deps[0].(*fitasks.ManagedFile); !ok || fi.ValueOf(dep.Name) != "elemento-placement" {
		t.Fatal(deps)
	}
	if deps[0] != task {
		t.Fatal("dependency must reference the actual task")
	}
	edges := fi.FindTaskDependencies(tasks)
	if len(edges["Network/test.k8s"]) != 1 || edges["Network/test.k8s"][0] != "ManagedFile/elemento-placement" {
		t.Fatal(edges)
	}
}
