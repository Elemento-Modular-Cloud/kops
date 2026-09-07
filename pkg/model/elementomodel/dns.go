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
	"fmt"
	"strings"

	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/cloudup/elementotasks"
)

const elementoDNSRecordTTL int64 = 3600

// DNSModelBuilder is the provider-native integration point for Elemento-managed
// DNS records that must exist before nodeup starts.
type DNSModelBuilder struct {
	*ElementoModelContext
	Lifecycle fi.Lifecycle
}

var _ fi.CloudupModelBuilder = &DNSModelBuilder{}

func (b *DNSModelBuilder) Build(c *fi.CloudupModelBuilderContext) error {
	if err := validateGoogleControlPlaneConfiguration(b.InstanceGroups); err != nil {
		return err
	}
	if !b.Cluster.PublishesDNSRecords() {
		return nil
	}

	dnsZoneTask := &elementotasks.DNSZone{
		Name:      fi.PtrTo(b.ClusterName()),
		Network:   b.LinkToNetwork(),
		Lifecycle: b.Lifecycle,
	}
	c.EnsureTask(dnsZoneTask)

	var previousDNSRecordTask *elementotasks.DNSRecord
	for _, ig := range b.InstanceGroups {
		dnsRecordTasks, err := b.elementoDNSRecordTasksForInstanceGroup(ig, b.Lifecycle, dnsZoneTask)
		if err != nil {
			return err
		}
		for _, dnsRecordTask := range dnsRecordTasks {
			dnsRecordTask.DependsOn = previousDNSRecordTask
			c.EnsureTask(dnsRecordTask)
			previousDNSRecordTask = dnsRecordTask
		}
	}

	return nil
}

func (b *ElementoModelContext) elementoDNSRecordTasksForInstanceGroup(ig *kops.InstanceGroup, lifecycle fi.Lifecycle, dnsZoneTask *elementotasks.DNSZone) ([]*elementotasks.DNSRecord, error) {
	if !b.Cluster.PublishesDNSRecords() {
		return nil, nil
	}

	igSize := fi.ValueOf(ig.Spec.MinSize)
	clusterName := b.ClusterName()
	zoneName := b.ClusterName()
	googleControlPlaneIP, externalControlPlane, err := googleControlPlaneIPForInstanceGroup(ig)
	if err != nil {
		return nil, err
	}
	primaryAPIServer, err := b.elementoPrimaryAPIServerInstanceGroup(ig)
	if err != nil {
		return nil, err
	}

	var tasks []*elementotasks.DNSRecord
	addRecord := func(recordName string, reservation *elementotasks.DHCPReservation) {
		task := &elementotasks.DNSRecord{
			Name:            fi.PtrTo(trimElementoDNSZoneSuffix(recordName, zoneName)),
			DNSZone:         fi.PtrTo(zoneName),
			DNSZoneTask:     dnsZoneTask,
			DHCPReservation: reservation,
			Type:            fi.PtrTo("A"),
			TTL:             fi.PtrTo(elementoDNSRecordTTL),
			Lifecycle:       lifecycle,
		}
		if externalControlPlane {
			task.Data = fi.PtrTo(googleControlPlaneIP)
			task.DHCPReservation = nil
		}
		tasks = append(tasks, task)
	}

	for ordinal := int32(1); ordinal <= igSize; ordinal++ {
		serverName := fmt.Sprintf("%s-%d", ig.Name, ordinal)
		var reservation *elementotasks.DHCPReservation
		if !externalControlPlane {
			reservation = &elementotasks.DHCPReservation{
				Name: fi.PtrTo(serverName),
			}
		}

		addRecord(fmt.Sprintf("%s.%s", serverName, clusterName), reservation)

		if !ig.HasAPIServer() || ordinal != 1 {
			continue
		}

		if primaryAPIServer {
			if !b.UseLoadBalancerForAPI() {
				apiPublicName := b.Cluster.Spec.API.PublicName
				if apiPublicName == "" {
					apiPublicName = "api." + clusterName
				}
				addRecord(apiPublicName, reservation)
			}
			if !b.UseLoadBalancerForInternalAPI() {
				addRecord(b.Cluster.APIInternalName(), reservation)
			}
			addRecord("kops-controller.internal."+clusterName, reservation)
		}

		for _, member := range elementoEtcdMembersForInstanceGroup(b.Cluster.Spec.EtcdClusters, ig.Name) {
			addRecord(fmt.Sprintf("node%d.%s.%s", member.index, member.clusterName, clusterName), reservation)
			addRecord(fmt.Sprintf("%s--%s--%d.internal.%s", clusterName, member.clusterName, member.index, clusterName), reservation)
		}
	}

	return tasks, nil
}

func (b *ElementoModelContext) elementoPrimaryAPIServerInstanceGroup(current *kops.InstanceGroup) (bool, error) {
	instanceGroups := b.InstanceGroups
	if len(instanceGroups) == 0 {
		instanceGroups = []*kops.InstanceGroup{current}
	}

	var firstAPIServer *kops.InstanceGroup
	for _, candidate := range instanceGroups {
		if !candidate.HasAPIServer() || fi.ValueOf(candidate.Spec.MinSize) == 0 {
			continue
		}
		if firstAPIServer == nil {
			firstAPIServer = candidate
		}
		_, external, err := googleControlPlaneIPForInstanceGroup(candidate)
		if err != nil {
			return false, err
		}
		if !external {
			return candidate.Name == current.Name, nil
		}
	}

	return firstAPIServer != nil && firstAPIServer.Name == current.Name, nil
}

type elementoEtcdMember struct {
	clusterName string
	index       int
}

func elementoEtcdMembersForInstanceGroup(etcdClusters []kops.EtcdClusterSpec, instanceGroupName string) []elementoEtcdMember {
	if len(etcdClusters) == 0 {
		return []elementoEtcdMember{
			{clusterName: "main", index: 0},
			{clusterName: "events", index: 0},
		}
	}

	var members []elementoEtcdMember
	for _, etcdCluster := range etcdClusters {
		clusterName := strings.TrimSpace(etcdCluster.Name)
		if clusterName == "" {
			continue
		}
		for index, member := range etcdCluster.Members {
			if fi.ValueOf(member.InstanceGroup) == instanceGroupName {
				members = append(members, elementoEtcdMember{clusterName: clusterName, index: index})
			}
		}
	}
	return members
}

func trimElementoDNSZoneSuffix(name, zone string) string {
	return strings.TrimSuffix(name, "."+strings.TrimSuffix(zone, "."))
}
