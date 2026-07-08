/*
Copyright 2019 The Kubernetes Authors.

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

package protokube

import (
	"fmt"
	"strings"

	"cloud.google.com/go/compute/metadata"
	"k8s.io/klog/v2"
	"k8s.io/kops/protokube/pkg/gossip"
	"k8s.io/kops/upup/pkg/fi/cloudup/gce/gcediscovery"
)

// GCECloudProvider is the CloudProvider implementation for GCE
type GCECloudProvider struct {
	discovery    *gcediscovery.Discovery
	instanceName string
}

var _ CloudProvider = &GCECloudProvider{}

// NewGCECloudProvider builds a GCECloudProvider
func NewGCECloudProvider() (*GCECloudProvider, error) {
	discovery, err := gcediscovery.New()
	if err != nil {
		return nil, err
	}

	a := &GCECloudProvider{
		discovery: discovery,
	}

	err = a.discoverTags()
	if err != nil {
		return nil, err
	}

	return a, nil
}

func (a *GCECloudProvider) discoverTags() error {
	if a.discovery.ClusterName() == "" {
		return fmt.Errorf("cluster-name metadata was empty")
	}

	project := a.discovery.ProjectID()
	if project == "" {
		return fmt.Errorf("project metadata was empty")
	}
	klog.Infof("Found project=%q", project)

	zone := a.discovery.Zone()
	if zone == "" {
		return fmt.Errorf("zone metadata was empty")
	}
	klog.Infof("Found zone=%q", zone)
	klog.Infof("Found region=%q", a.discovery.Region())

	instanceName, err := metadata.InstanceName()
	if err != nil {
		return fmt.Errorf("error reading instance name from GCE: %v", err)
	}
	a.instanceName = strings.TrimSpace(instanceName)
	if a.instanceName == "" {
		return fmt.Errorf("instance name metadata was empty")
	}
	klog.Infof("Found instanceName=%q", a.instanceName)

	return nil
}

func (g *GCECloudProvider) GossipSeeds() (gossip.SeedProvider, error) {
	return g.discovery, nil
}

func (g *GCECloudProvider) InstanceID() string {
	return g.instanceName
}
