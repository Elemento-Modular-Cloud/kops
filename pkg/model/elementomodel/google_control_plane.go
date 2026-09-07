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
	"net"
	"os"
	"strings"

	"k8s.io/kops/pkg/apis/kops"
	"k8s.io/kops/upup/pkg/fi"
)

const (
	googleControlPlaneEnv              = "GOOGLE_CONTROL_PLANE"
	googleControlPlaneInstanceGroupEnv = "GOOGLE_CONTROL_PLANE_IG"
	googleControlPlanesEnv             = "GOOGLE_CONTROL_PLANES"
)

// Leave the external control-plane environment variables unset for an
// all-AtomOS cluster. The implementation below remains available for future
// Google or other vanilla-VM scenarios.

func googleControlPlaneIPForInstanceGroup(ig *kops.InstanceGroup) (string, bool, error) {
	if ig.Spec.Role != kops.InstanceGroupRoleControlPlane {
		return "", false, nil
	}

	controlPlanes, err := googleControlPlaneIPs()
	if err != nil {
		return "", false, err
	}
	value, found := controlPlanes[ig.Name]
	if !found {
		legacyIP := strings.TrimSpace(os.Getenv(googleControlPlaneEnv))
		legacyInstanceGroup := strings.TrimSpace(os.Getenv(googleControlPlaneInstanceGroupEnv))
		if legacyIP == "" || legacyInstanceGroup != "" || len(controlPlanes) != 0 {
			return "", false, nil
		}
		value = legacyIP
	}

	if fi.ValueOf(ig.Spec.MinSize) != 1 {
		return "", false, fmt.Errorf("external Google control-plane instance group %q must have minSize 1, got %d", ig.Name, fi.ValueOf(ig.Spec.MinSize))
	}

	return value, true, nil
}

func googleControlPlaneIPs() (map[string]string, error) {
	value := strings.TrimSpace(os.Getenv(googleControlPlanesEnv))
	legacyIP := strings.TrimSpace(os.Getenv(googleControlPlaneEnv))
	legacyInstanceGroup := strings.TrimSpace(os.Getenv(googleControlPlaneInstanceGroupEnv))
	if value != "" && (legacyIP != "" || legacyInstanceGroup != "") {
		return nil, fmt.Errorf("%s cannot be combined with %s or %s", googleControlPlanesEnv, googleControlPlaneEnv, googleControlPlaneInstanceGroupEnv)
	}

	controlPlanes := make(map[string]string)
	if value == "" {
		if legacyIP == "" || legacyInstanceGroup == "" {
			return controlPlanes, nil
		}
		ip, err := parseGoogleControlPlaneIP(googleControlPlaneEnv, legacyIP)
		if err != nil {
			return nil, err
		}
		controlPlanes[legacyInstanceGroup] = ip
		return controlPlanes, nil
	}

	usedIPs := make(map[string]string)
	for _, entry := range strings.Split(value, ",") {
		entry = strings.TrimSpace(entry)
		instanceGroup, rawIP, found := strings.Cut(entry, "=")
		instanceGroup = strings.TrimSpace(instanceGroup)
		rawIP = strings.TrimSpace(rawIP)
		if !found || instanceGroup == "" || rawIP == "" {
			return nil, fmt.Errorf("%s entries must use instance-group=IPv4 format, got %q", googleControlPlanesEnv, entry)
		}
		if _, duplicate := controlPlanes[instanceGroup]; duplicate {
			return nil, fmt.Errorf("%s contains duplicate instance group %q", googleControlPlanesEnv, instanceGroup)
		}
		ip, err := parseGoogleControlPlaneIP(googleControlPlanesEnv, rawIP)
		if err != nil {
			return nil, err
		}
		if previous, duplicate := usedIPs[ip]; duplicate {
			return nil, fmt.Errorf("%s assigns IPv4 address %s to both %q and %q", googleControlPlanesEnv, ip, previous, instanceGroup)
		}
		controlPlanes[instanceGroup] = ip
		usedIPs[ip] = instanceGroup
	}
	return controlPlanes, nil
}

func parseGoogleControlPlaneIP(environmentVariable, value string) (string, error) {
	ip := net.ParseIP(value)
	if ip == nil || ip.To4() == nil {
		return "", fmt.Errorf("%s must contain valid IPv4 addresses, got %q", environmentVariable, value)
	}
	return ip.String(), nil
}

func validateGoogleControlPlaneConfiguration(instanceGroups []*kops.InstanceGroup) error {
	controlPlaneIPs, err := googleControlPlaneIPs()
	if err != nil {
		return err
	}
	legacyIP := strings.TrimSpace(os.Getenv(googleControlPlaneEnv))
	legacyInstanceGroup := strings.TrimSpace(os.Getenv(googleControlPlaneInstanceGroupEnv))
	if len(controlPlaneIPs) == 0 && legacyIP == "" {
		return nil
	}

	activeControlPlanes := make(map[string]*kops.InstanceGroup)
	for _, ig := range instanceGroups {
		if ig.Spec.Role == kops.InstanceGroupRoleControlPlane && fi.ValueOf(ig.Spec.MinSize) > 0 {
			activeControlPlanes[ig.Name] = ig
		}
	}

	if legacyIP != "" && legacyInstanceGroup == "" {
		if _, err := parseGoogleControlPlaneIP(googleControlPlaneEnv, legacyIP); err != nil {
			return err
		}
		if len(activeControlPlanes) != 1 {
			return fmt.Errorf("%s must name the external control-plane instance group when %s is used with multiple control planes", googleControlPlaneInstanceGroupEnv, googleControlPlaneEnv)
		}
		return nil
	}

	for instanceGroup := range controlPlaneIPs {
		ig, found := activeControlPlanes[instanceGroup]
		if !found {
			return fmt.Errorf("external Google control-plane instance group %q does not match an active control-plane instance group", instanceGroup)
		}
		if fi.ValueOf(ig.Spec.MinSize) != 1 {
			return fmt.Errorf("external Google control-plane instance group %q must have minSize 1, got %d", instanceGroup, fi.ValueOf(ig.Spec.MinSize))
		}
	}

	if len(controlPlaneIPs) >= len(activeControlPlanes) {
		return fmt.Errorf("at least one active control-plane instance group must remain managed by AtomOS")
	}
	return nil
}
