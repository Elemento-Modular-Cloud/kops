package elementotasks

import (
	"reflect"
	"testing"

	"github.com/Elemento-Modular-Cloud/ecloud-go/ecloud"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/elemento"
)

func TestServerGroupGlobalNamesAndOwnership(t *testing.T) {
	group := &ServerGroup{Name: fi.PtrTo("control-plane-europe-2"), ServerNames: []string{"control-plane-europe-2"}}
	server := &ecloud.Server{Name: "control-plane-europe-2", Labels: map[string]string{
		elemento.TagKubernetesClusterName: "test.k8s", elemento.TagKubernetesInstanceGroup: "control-plane-europe-2",
	}}
	got, err := group.matchingServers([]*ecloud.Server{server}, "test.k8s")
	if err != nil || len(got) != 1 {
		t.Fatalf("global name not recognized: %v", err)
	}
	server.Labels[elemento.TagKubernetesInstanceGroup] = "other"
	if _, err := group.matchingServers([]*ecloud.Server{server}, "test.k8s"); err == nil {
		t.Fatal("ownership conflict accepted")
	}
	server.Labels[elemento.TagKubernetesInstanceGroup] = "control-plane-europe-2"
	server.Name = "control-plane-europe-2-1"
	if _, err := group.matchingServers([]*ecloud.Server{server}, "test.k8s"); err == nil {
		t.Fatal("old name accepted for automatic rename")
	}
}

func TestMissingServerNamesHandlesHoles(t *testing.T) {
	wanted := []string{"nodes-europe-1", "nodes-europe-2", "nodes-europe-3"}
	got := missingServerNames(wanted, []string{"nodes-europe-2"})
	if !reflect.DeepEqual(got, []string{"nodes-europe-1", "nodes-europe-3"}) {
		t.Fatal(got)
	}
	if len(missingServerNames(wanted, wanted)) != 0 {
		t.Fatal("existing servers would be recreated")
	}
}
