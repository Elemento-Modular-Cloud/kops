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
		{"adapter", map[string]string{"ATOMOS_WORKERS": "", "PROVIDERS": "unsupported", "UNSUPPORTED_WORKERS": "1", "UNSUPPORTED_CONTROL_PLANES": "0"}, "no provisioning adapter"},
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

func TestExternalLoadBalancerDNSDependencies(t *testing.T) {
	b, c := placementBuilderFixture(t)
	t.Setenv("EXTERNAL_LOAD_BALANCER_PROVIDER", "google")
	t.Setenv("EXTERNAL_LOAD_BALANCER_PUBLIC_API_CIDRS", "198.51.100.42/32")
	if err := b.Build(c); err != nil {
		t.Fatal(err)
	}
	dns := &DNSModelBuilder{ElementoModelContext: b.ElementoModelContext, Lifecycle: fi.LifecycleSync}
	if err := dns.Build(c); err != nil {
		t.Fatal(err)
	}

	var loadBalancer *elementotasks.ExternalLoadBalancer
	var records []*elementotasks.DNSRecord
	for _, task := range c.Tasks {
		switch task := task.(type) {
		case *elementotasks.ExternalLoadBalancer:
			loadBalancer = task
		case *elementotasks.DNSRecord:
			if task.ExternalLoadBalancer != nil {
				records = append(records, task)
			}
		}
	}
	if loadBalancer == nil || loadBalancer.Plan == nil || loadBalancer.Plan.LoadBalancer == nil {
		t.Fatalf("external load balancer task was not built: %#v", loadBalancer)
	}
	if len(records) != 3 {
		t.Fatalf("got %d load balancer DNS records, want 3", len(records))
	}
	seen := map[string]bool{}
	for _, record := range records {
		seen[fi.ValueOf(record.Name)] = true
		if record.ExternalLoadBalancer != loadBalancer {
			t.Fatalf("DNS record %q does not depend on the load balancer", fi.ValueOf(record.Name))
		}
	}
	if !seen["api"] || !seen["api.internal"] || !seen["kops-controller.internal"] {
		t.Fatalf("unexpected load balancer DNS records: %v", seen)
	}
	apiInternal := c.Tasks["DNSRecord/api.internal"].(*elementotasks.DNSRecord)
	kopsControllerInternal := c.Tasks["DNSRecord/kops-controller.internal"].(*elementotasks.DNSRecord)
	publicAPI := c.Tasks["DNSRecord/api"].(*elementotasks.DNSRecord)
	if kopsControllerInternal.DependsOn != apiInternal || publicAPI.DependsOn != kopsControllerInternal {
		t.Fatal("load balancer DNS records must be written in order")
	}

	controlPlane := b.InstanceGroups[0]
	legacyRecords, err := b.elementoDNSRecordTasksForInstanceGroup(controlPlane, fi.LifecycleSync,
		&elementotasks.DNSZone{Name: fi.PtrTo("test.k8s")})
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range legacyRecords {
		if name := fi.ValueOf(record.Name); name == "api" || name == "api.internal" || name == "kops-controller.internal" {
			t.Fatalf("legacy internal DNS record %q was still generated", name)
		}
	}
}

func TestExternalPlacementDNSAndDHCPSources(t *testing.T) {
	for _, allExternal := range []bool{false, true} {
		b, c := placementBuilderFixture(t)
		b.externalNodeIPs = map[string]string{"nodes-europe-1": "10.0.253.1"}
		if allExternal {
			b.externalNodeIPs["control-plane-europe-1"] = "10.0.254.1"
		}
		// Exercise prepared models directly; the provisioning guard stays enabled.
		dhcp := &DHCPModelBuilder{ElementoModelContext: b.ElementoModelContext, Lifecycle: fi.LifecycleSync}
		if err := dhcp.Build(c); err != nil {
			t.Fatal(err)
		}
		if c.Tasks["DHCPReservation/nodes-europe-1"] != nil {
			t.Fatal("external worker received DHCP reservation")
		}
		if (c.Tasks["DHCPService/test.k8s"] == nil) != allExternal {
			t.Fatal("unexpected DHCP service")
		}
		if (c.Tasks["DHCPReservation/control-plane-europe-1"] == nil) != allExternal {
			t.Fatal("unexpected control-plane reservation")
		}
		for _, group := range b.InstanceGroups {
			records, err := b.elementoDNSRecordTasksForInstanceGroup(group, fi.LifecycleSync,
				&elementotasks.DNSZone{Name: fi.PtrTo("test.k8s")})
			if err != nil {
				t.Fatal(err)
			}
			if len(records) == 0 {
				t.Fatal("missing DNS records")
			}
			names, err := b.nodeNamesForInstanceGroup(group)
			if err != nil {
				t.Fatal(err)
			}
			ip := b.externalNodeIPs[names[0]]
			for _, record := range records {
				if ip != "" {
					if fi.ValueOf(record.Data) != ip || record.DHCPReservation != nil {
						t.Fatal("external DNS must use placement IP")
					}
				} else if record.Data != nil || record.DHCPReservation == nil {
					t.Fatal("AtomOS DNS must use DHCP")
				}
			}
		}
	}
}
