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

import "testing"

func TestKubernetesAuthTailnetEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		ip      string
		port    int
		want    string
		wantErr bool
	}{
		{name: "IPv4", ip: "10.0.18.252", port: 8080, want: "http://10.0.18.252:8080"},
		{name: "IPv6", ip: "2001:db8::1", port: 8080, want: "http://[2001:db8::1]:8080"},
		{name: "invalid IP", ip: "auth.internal", port: 8080, wantErr: true},
		{name: "invalid port", ip: "10.0.18.252", port: 0, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := kubernetesAuthTailnetEndpoint(test.ip, test.port)
			if (err != nil) != test.wantErr {
				t.Fatalf("kubernetesAuthTailnetEndpoint() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Errorf("kubernetesAuthTailnetEndpoint() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPrependElementoAuthEnvironment(t *testing.T) {
	userData := "#!/bin/bash\necho ready\n"
	got := prependElementoAuthEnvironment(userData, "http://10.0.18.252:8080", "secret with spaces")

	want := "#!/bin/bash\n" +
		"export ELEMENTO_AUTH_URL=$(printf %s aHR0cDovLzEwLjAuMTguMjUyOjgwODA= | base64 -d)\n" +
		"export ELEMENTO_AUTH_VERIFIER_API_KEY=$(printf %s c2VjcmV0IHdpdGggc3BhY2Vz | base64 -d)\n" +
		"echo ready\n"
	if got != want {
		t.Fatalf("prependElementoAuthEnvironment() = %q, want %q", got, want)
	}
}
