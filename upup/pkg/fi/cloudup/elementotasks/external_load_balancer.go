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
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/elemento"
)

// ExternalLoadBalancer is infrastructure outside Kubernetes. It must be
// available before internal API DNS records and control-plane bootstrap.
type ExternalLoadBalancer struct {
	Name        *string
	Plan        *ecloud.MulticloudPlan
	DNSZoneTask *DNSZone
	Lifecycle   fi.Lifecycle
}

var _ fi.CloudupTask = &ExternalLoadBalancer{}
var _ fi.CloudupHasDependencies = &ExternalLoadBalancer{}
var _ fi.HasLifecycle = &ExternalLoadBalancer{}
var _ fi.HasName = &ExternalLoadBalancer{}

func (l *ExternalLoadBalancer) GetDependencies(tasks map[string]fi.CloudupTask) []fi.CloudupTask {
	if l.DNSZoneTask == nil {
		return nil
	}
	return []fi.CloudupTask{l.DNSZoneTask}
}

func (l *ExternalLoadBalancer) GetLifecycle() fi.Lifecycle {
	return l.Lifecycle
}

func (l *ExternalLoadBalancer) SetLifecycle(lifecycle fi.Lifecycle) {
	l.Lifecycle = lifecycle
}

func (l *ExternalLoadBalancer) GetName() *string {
	return l.Name
}

func (l *ExternalLoadBalancer) String() string {
	return fi.CloudupTaskAsString(l)
}

func (l *ExternalLoadBalancer) Find(c *fi.CloudupContext) (*ExternalLoadBalancer, error) {
	if l.Plan == nil || l.Plan.LoadBalancer == nil {
		return nil, nil
	}
	cloud := c.T.Cloud.(elemento.ElementoCloud)
	result, err := cloud.MulticloudClient().FindExternalLoadBalancer(context.TODO(), l.Plan)
	if err != nil {
		return nil, fmt.Errorf("finding external load balancer %q: %w", fi.ValueOf(l.Name), err)
	}
	if result == nil {
		return nil, nil
	}
	return &ExternalLoadBalancer{
		Name:        l.Name,
		Plan:        l.Plan,
		DNSZoneTask: l.DNSZoneTask,
		Lifecycle:   l.Lifecycle,
	}, nil
}

func (l *ExternalLoadBalancer) Run(c *fi.CloudupContext) error {
	return fi.CloudupDefaultDeltaRunMethod(l, c)
}

func (_ *ExternalLoadBalancer) CheckChanges(actual, expected, changes *ExternalLoadBalancer) error {
	if expected.Name == nil || strings.TrimSpace(fi.ValueOf(expected.Name)) == "" {
		return fi.RequiredField("Name")
	}
	if expected.Plan == nil || expected.Plan.LoadBalancer == nil {
		return fi.RequiredField("Plan.LoadBalancer")
	}
	if expected.DNSZoneTask == nil {
		return fi.RequiredField("DNSZoneTask")
	}
	return nil
}

func (_ *ExternalLoadBalancer) RenderElemento(t *elemento.ElementoAPITarget, actual, expected, changes *ExternalLoadBalancer) error {
	dnsIPAddress := strings.TrimSpace(fi.ValueOf(expected.DNSZoneTask.IPAddress))
	if dnsIPAddress == "" {
		dnsClient := t.Cloud.DnsClient()
		dnsService, _, err := dnsClient.Get(context.TODO(), fi.ValueOf(expected.DNSZoneTask.Name))
		if err != nil {
			return fmt.Errorf("getting DNS service IP for external load balancer: %w", err)
		}
		if dnsService != nil {
			dnsIPAddress = strings.TrimSpace(dnsService.IPAddress)
		}
	}
	if dnsIPAddress == "" {
		return fmt.Errorf("DNS zone task for external load balancer has no service IP address")
	}
	expected.DNSZoneTask.IPAddress = fi.PtrTo(dnsIPAddress)

	result, _, err := t.Cloud.MulticloudClient().CreateExternalLoadBalancer(context.TODO(), expected.Plan, ecloud.ExternalLoadBalancerCreateOpts{
		DNSIPAddress: dnsIPAddress,
	})
	if err != nil {
		if result != nil && result.Server != nil {
			return fmt.Errorf("creating external load balancer %q (%s): %w", fi.ValueOf(expected.Name), result.Server.ID, err)
		}
		return fmt.Errorf("creating external load balancer %q: %w", fi.ValueOf(expected.Name), err)
	}
	if result == nil || result.Server == nil {
		return fmt.Errorf("external load balancer %q returned no server", fi.ValueOf(expected.Name))
	}
	fmt.Printf("EKOPS: External load balancer %q is running as %s\n", fi.ValueOf(expected.Name), result.Server.ID)
	return nil
}
