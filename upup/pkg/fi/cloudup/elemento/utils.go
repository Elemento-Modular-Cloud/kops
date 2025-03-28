/*
Copyright 2025 The Kubernetes Authors.

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

package elemento

import (
	"fmt"

	"k8s.io/kops/pkg/apis/kops"
)

// FindRegion determines the region from the zones specified in the cluster
func FindRegion(cluster *kops.Cluster) (string, error) {
	var region string
	// TODO: insert supported regions
	
	for _, subnet := range cluster.Spec.Networking.Subnets {
		var zoneRegion string
		switch subnet.Zone {
		case "fsn1", "nbg1", "hel1":
			zoneRegion = "eu-central"
		case "ash":
			zoneRegion = "us-east"
		case "hil":
			zoneRegion = "us-west"
		default:
			return "", fmt.Errorf("unknown zone %q for elemento cloud, known zones are fsn1, nbg1, hel1, ash, hil", subnet.Zone)
		}

		if region != "" && zoneRegion != region {
			return "", fmt.Errorf("cluster cannot span multiple regions (found zone %q, but region is %q)", subnet.Zone, region)
		}

		region = zoneRegion
	}

	return region, nil
}
