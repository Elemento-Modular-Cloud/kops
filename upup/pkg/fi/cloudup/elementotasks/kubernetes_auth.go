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
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/Elemento-Modular-Cloud/ecloud-go/ecloud"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/elemento"
)

// +kops:fitask
type KubernetesAuthService struct {
	Name         *string
	Lifecycle    fi.Lifecycle
	Network      *Network
	AtomOSTarget string
	TailnetCIDR  string

	NetworkUID      *string
	TailnetEndpoint *string
}

var _ fi.CloudupTask = &KubernetesAuthService{}
var _ fi.CloudupHasDependencies = &KubernetesAuthService{}
var _ fi.HasLifecycle = &KubernetesAuthService{}
var _ fi.HasName = &KubernetesAuthService{}

func (s *KubernetesAuthService) GetDependencies(tasks map[string]fi.CloudupTask) []fi.CloudupTask {
	if s.Network == nil {
		return nil
	}
	return []fi.CloudupTask{s.Network}
}

func (s *KubernetesAuthService) GetLifecycle() fi.Lifecycle { return s.Lifecycle }

func (s *KubernetesAuthService) SetLifecycle(lifecycle fi.Lifecycle) { s.Lifecycle = lifecycle }

func (s *KubernetesAuthService) GetName() *string { return s.Name }

func (s *KubernetesAuthService) String() string { return fi.CloudupTaskAsString(s) }

func (s *KubernetesAuthService) Find(c *fi.CloudupContext) (*KubernetesAuthService, error) {
	cloud := c.T.Cloud.(elemento.ElementoCloud)
	status, _, err := cloud.KubernetesAuthClient().GetStatus(
		context.TODO(),
		ecloud.KubernetesAuthServiceStatusOpts{AtomOsTarget: s.AtomOSTarget},
	)
	if err != nil {
		var apiErr *ecloud.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("getting Elemento Kubernetes authentication service: %w", err)
	}
	if status == nil {
		return nil, nil
	}

	expectedNetworkUID, err := s.resolveNetworkUID(cloud)
	if err != nil {
		return nil, err
	}
	if status.NetworkUID != expectedNetworkUID {
		return nil, fmt.Errorf(
			"Elemento Kubernetes authentication service uses network_uid %q, expected %q",
			status.NetworkUID,
			expectedNetworkUID,
		)
	}
	if !strings.EqualFold(strings.TrimSpace(status.Status), "running") {
		return nil, fmt.Errorf(
			"Elemento Kubernetes authentication service is %q; automatic service reconciliation is not enabled",
			status.Status,
		)
	}

	endpoint, err := kubernetesAuthTailnetEndpoint(status.TailnetIP, status.TailnetPort)
	if err != nil {
		return nil, err
	}
	s.NetworkUID = fi.PtrTo(expectedNetworkUID)
	s.TailnetEndpoint = fi.PtrTo(endpoint)

	actual := *s
	return &actual, nil
}

func (s *KubernetesAuthService) Run(c *fi.CloudupContext) error {
	return fi.CloudupDefaultDeltaRunMethod(s, c)
}

func (*KubernetesAuthService) CheckChanges(actual, expected, changes *KubernetesAuthService) error {
	if expected.Name == nil {
		return fi.RequiredField("Name")
	}
	if expected.Network == nil {
		return fi.RequiredField("Network")
	}
	return nil
}

func (*KubernetesAuthService) RenderElemento(t *elemento.ElementoAPITarget, actual, expected, changes *KubernetesAuthService) error {
	if actual != nil {
		return nil
	}

	networkUID, err := expected.resolveNetworkUID(t.Cloud)
	if err != nil {
		return err
	}
	service, _, err := t.Cloud.KubernetesAuthClient().CreateService(
		context.TODO(),
		ecloud.KubernetesAuthServiceCreateOpts{
			AtomOsTarget: expected.AtomOSTarget,
			NetworkUID:   networkUID,
			TailnetCIDR:  expected.TailnetCIDR,
		},
	)
	if err != nil {
		return fmt.Errorf("creating Elemento Kubernetes authentication service: %w", err)
	}
	if service.NetworkUID != networkUID {
		return fmt.Errorf(
			"created Elemento Kubernetes authentication service uses network_uid %q, expected %q",
			service.NetworkUID,
			networkUID,
		)
	}
	if !strings.EqualFold(strings.TrimSpace(service.Status), "running") {
		return fmt.Errorf("created Elemento Kubernetes authentication service is %q, expected running", service.Status)
	}
	if strings.TrimSpace(service.TailnetEndpoint) == "" {
		return fmt.Errorf("created Elemento Kubernetes authentication service has no tailnet endpoint")
	}

	expected.NetworkUID = fi.PtrTo(networkUID)
	expected.TailnetEndpoint = fi.PtrTo(strings.TrimRight(service.TailnetEndpoint, "/"))
	fmt.Printf("EKOPS: Elemento Kubernetes authentication service is running at %s\n", fi.ValueOf(expected.TailnetEndpoint))
	return nil
}

func (s *KubernetesAuthService) resolveNetworkUID(cloud elemento.ElementoCloud) (string, error) {
	if s.Network == nil {
		return "", fi.RequiredField("Network")
	}
	if networkUID := strings.TrimSpace(fi.ValueOf(s.Network.ID)); networkUID != "" {
		return networkUID, nil
	}

	networkName := strings.TrimSpace(fi.ValueOf(s.Network.Name))
	networkClient := cloud.NetworkClient()
	network, _, err := networkClient.GetByName(context.TODO(), networkName)
	if err != nil {
		return "", fmt.Errorf("getting Elemento network %q for Kubernetes authentication: %w", networkName, err)
	}
	if network == nil || strings.TrimSpace(network.ID) == "" {
		return "", fmt.Errorf("Elemento network %q has no UUID for Kubernetes authentication", networkName)
	}
	s.Network.ID = fi.PtrTo(network.ID)
	return network.ID, nil
}

func kubernetesAuthTailnetEndpoint(ip string, port int) (string, error) {
	ip = strings.TrimSpace(ip)
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("Elemento Kubernetes authentication service returned invalid tailnet IP %q", ip)
	}
	if port <= 0 || port > 65535 {
		return "", fmt.Errorf("Elemento Kubernetes authentication service returned invalid tailnet port %d", port)
	}
	return "http://" + net.JoinHostPort(ip, fmt.Sprintf("%d", port)), nil
}

// +kops:fitask
type KubernetesAuthCluster struct {
	Name         *string
	Lifecycle    fi.Lifecycle
	AuthService  *KubernetesAuthService
	AtomOSTarget string
	Description  string
}

var _ fi.CloudupTask = &KubernetesAuthCluster{}
var _ fi.CloudupHasDependencies = &KubernetesAuthCluster{}
var _ fi.HasLifecycle = &KubernetesAuthCluster{}
var _ fi.HasName = &KubernetesAuthCluster{}

func (c *KubernetesAuthCluster) GetDependencies(tasks map[string]fi.CloudupTask) []fi.CloudupTask {
	if c.AuthService == nil {
		return nil
	}
	return []fi.CloudupTask{c.AuthService}
}

func (c *KubernetesAuthCluster) GetLifecycle() fi.Lifecycle { return c.Lifecycle }

func (c *KubernetesAuthCluster) SetLifecycle(lifecycle fi.Lifecycle) { c.Lifecycle = lifecycle }

func (c *KubernetesAuthCluster) GetName() *string { return c.Name }

func (c *KubernetesAuthCluster) String() string { return fi.CloudupTaskAsString(c) }

func (*KubernetesAuthCluster) Find(c *fi.CloudupContext) (*KubernetesAuthCluster, error) {
	// EnsureCluster is idempotent; always render until a read endpoint is available.
	return nil, nil
}

func (c *KubernetesAuthCluster) Run(ctx *fi.CloudupContext) error {
	return fi.CloudupDefaultDeltaRunMethod(c, ctx)
}

func (*KubernetesAuthCluster) CheckChanges(actual, expected, changes *KubernetesAuthCluster) error {
	if expected.Name == nil {
		return fi.RequiredField("Name")
	}
	if expected.AuthService == nil {
		return fi.RequiredField("AuthService")
	}
	return nil
}

func (*KubernetesAuthCluster) RenderElemento(t *elemento.ElementoAPITarget, actual, expected, changes *KubernetesAuthCluster) error {
	_, _, err := t.Cloud.KubernetesAuthClient().EnsureCluster(
		context.TODO(),
		ecloud.KubernetesAuthClusterCreateOpts{
			AtomOsTarget: expected.AtomOSTarget,
			Name:         fi.ValueOf(expected.Name),
			Description:  expected.Description,
		},
	)
	if err != nil {
		return fmt.Errorf("ensuring Elemento Kubernetes authentication cluster %q: %w", fi.ValueOf(expected.Name), err)
	}
	return nil
}

// +kops:fitask
type KubernetesAuthInstanceGroup struct {
	Name         *string
	Lifecycle    fi.Lifecycle
	AuthCluster  *KubernetesAuthCluster
	AtomOSTarget string
	ClusterName  string
	Role         string
}

var _ fi.CloudupTask = &KubernetesAuthInstanceGroup{}
var _ fi.CloudupHasDependencies = &KubernetesAuthInstanceGroup{}
var _ fi.HasLifecycle = &KubernetesAuthInstanceGroup{}
var _ fi.HasName = &KubernetesAuthInstanceGroup{}

func (g *KubernetesAuthInstanceGroup) GetDependencies(tasks map[string]fi.CloudupTask) []fi.CloudupTask {
	if g.AuthCluster == nil {
		return nil
	}
	return []fi.CloudupTask{g.AuthCluster}
}

func (g *KubernetesAuthInstanceGroup) GetLifecycle() fi.Lifecycle { return g.Lifecycle }

func (g *KubernetesAuthInstanceGroup) SetLifecycle(lifecycle fi.Lifecycle) { g.Lifecycle = lifecycle }

func (g *KubernetesAuthInstanceGroup) GetName() *string { return g.Name }

func (g *KubernetesAuthInstanceGroup) String() string { return fi.CloudupTaskAsString(g) }

func (*KubernetesAuthInstanceGroup) Find(c *fi.CloudupContext) (*KubernetesAuthInstanceGroup, error) {
	// EnsureInstanceGroup is idempotent; always render until a read endpoint is available.
	return nil, nil
}

func (g *KubernetesAuthInstanceGroup) Run(ctx *fi.CloudupContext) error {
	return fi.CloudupDefaultDeltaRunMethod(g, ctx)
}

func (*KubernetesAuthInstanceGroup) CheckChanges(actual, expected, changes *KubernetesAuthInstanceGroup) error {
	if expected.Name == nil {
		return fi.RequiredField("Name")
	}
	if expected.AuthCluster == nil {
		return fi.RequiredField("AuthCluster")
	}
	if expected.ClusterName == "" {
		return fi.RequiredField("ClusterName")
	}
	if expected.Role == "" {
		return fi.RequiredField("Role")
	}
	return nil
}

func (*KubernetesAuthInstanceGroup) RenderElemento(t *elemento.ElementoAPITarget, actual, expected, changes *KubernetesAuthInstanceGroup) error {
	_, _, err := t.Cloud.KubernetesAuthClient().EnsureInstanceGroup(
		context.TODO(),
		ecloud.KubernetesAuthInstanceGroupCreateOpts{
			AtomOsTarget: expected.AtomOSTarget,
			ClusterName:  expected.ClusterName,
			Name:         fi.ValueOf(expected.Name),
			Role:         expected.Role,
		},
	)
	if err != nil {
		return fmt.Errorf("ensuring Elemento Kubernetes authentication instance group %q: %w", fi.ValueOf(expected.Name), err)
	}
	return nil
}
