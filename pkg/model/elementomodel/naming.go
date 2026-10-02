package elementomodel

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/upup/pkg/fi"
)

// nodeNamesByInstanceGroup assigns global ordinals per role in the single zone.
// Always use the complete group list so filtered updates keep the same names.
func (b *ElementoModelContext) nodeNamesByInstanceGroup(current *kops.InstanceGroup) (map[string][]string, error) {
	groups := b.AllInstanceGroups
	if groups == nil {
		groups = b.InstanceGroups
	}
	if len(groups) == 0 && current != nil {
		groups = []*kops.InstanceGroup{current}
	}
	zones := map[string]bool{}
	subnetZones := map[string]string{}
	for _, subnet := range b.Cluster.Spec.Networking.Subnets {
		zone := strings.TrimSpace(subnet.Zone)
		if zone == "" {
			zone = strings.TrimSpace(subnet.Name)
		}
		if zone != "" {
			zones[zone] = true
			subnetZones[subnet.Name] = zone
		}
	}
	for _, ig := range groups {
		for _, subnet := range ig.Spec.Subnets {
			zone := subnetZones[subnet]
			if zone == "" {
				if len(subnetZones) != 0 {
					return nil, fmt.Errorf("instance group %q references unknown subnet %q", ig.Name, subnet)
				}
				zone = strings.TrimSpace(subnet)
			}
			if zone != "" {
				zones[zone] = true
			}
		}
	}
	if len(zones) != 1 {
		return nil, fmt.Errorf("Elemento requires exactly one zone for node naming; found %d", len(zones))
	}
	var zone string
	for name := range zones {
		zone = name
	}
	ordered := append([]*kops.InstanceGroup(nil), groups...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	names := map[string][]string{}
	counters := map[kops.InstanceGroupRole]int{}
	for _, ig := range ordered {
		if _, exists := names[ig.Name]; exists {
			return nil, fmt.Errorf("duplicate instance group %q", ig.Name)
		}
		prefix := ""
		switch ig.Spec.Role {
		case kops.InstanceGroupRoleControlPlane:
			prefix = "control-plane"
		case kops.InstanceGroupRoleNode:
			prefix = "nodes"
		default:
			return nil, fmt.Errorf("unsupported Elemento node role %q", ig.Spec.Role)
		}
		names[ig.Name] = []string{}
		for ordinal := int32(0); ordinal < fi.ValueOf(ig.Spec.MinSize); ordinal++ {
			counters[ig.Spec.Role]++
			names[ig.Name] = append(names[ig.Name], fmt.Sprintf("%s-%s-%d", prefix, zone, counters[ig.Spec.Role]))
		}
	}
	return names, nil
}

func (b *ElementoModelContext) nodeNamesForInstanceGroup(ig *kops.InstanceGroup) ([]string, error) {
	names, err := b.nodeNamesByInstanceGroup(ig)
	if err != nil {
		return nil, err
	}
	result, ok := names[ig.Name]
	if !ok {
		return nil, fmt.Errorf("instance group %q is absent from the naming plan", ig.Name)
	}
	return result, nil
}
