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

package nodemodel

import (
	"reflect"
	"testing"
)

func TestBuildConfigServerOptionsUsesTLSServerNameForIPServers(t *testing.T) {
	options := buildConfigServerOptions("cluster.k8s.local", "ca-data", []string{"10.0.1.2"})

	if got, want := options.TLSServerName, "kops-controller.internal.cluster.k8s.local"; got != want {
		t.Fatalf("TLSServerName = %q, want %q", got, want)
	}
	if got, want := options.Servers, []string{"https://10.0.1.2:3988/"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Servers = %v, want %v", got, want)
	}
}

func TestBuildConfigServerOptionsUsesDNSNameByDefault(t *testing.T) {
	options := buildConfigServerOptions("cluster.k8s.local", "ca-data", nil)

	if options.TLSServerName != "" {
		t.Fatalf("TLSServerName = %q, want empty", options.TLSServerName)
	}
	if got, want := options.Servers, []string{"https://kops-controller.internal.cluster.k8s.local:3988/"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Servers = %v, want %v", got, want)
	}
}
