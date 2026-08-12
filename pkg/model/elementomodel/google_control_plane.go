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

const googleControlPlaneEnv = "GOOGLE_CONTROL_PLANE"

func googleControlPlaneIPForInstanceGroup(ig *kops.InstanceGroup) (string, bool, error) {
	value := strings.TrimSpace(os.Getenv(googleControlPlaneEnv))
	if value == "" || ig.Spec.Role != kops.InstanceGroupRoleControlPlane {
		return "", false, nil
	}

	ip := net.ParseIP(value)
	if ip == nil || ip.To4() == nil {
		return "", false, fmt.Errorf("%s must contain a valid IPv4 address, got %q", googleControlPlaneEnv, value)
	}
	if fi.ValueOf(ig.Spec.MinSize) != 1 {
		return "", false, fmt.Errorf("%s supports exactly one control-plane instance, but instance group %q has minSize %d", googleControlPlaneEnv, ig.Name, fi.ValueOf(ig.Spec.MinSize))
	}

	return ip.String(), true, nil
}
