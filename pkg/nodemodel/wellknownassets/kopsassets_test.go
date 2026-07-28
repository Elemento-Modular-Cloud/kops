/*
Copyright 2020 The Kubernetes Authors.

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

package wellknownassets

import (
	"fmt"
	"net/url"
	"reflect"
	"testing"

	"k8s.io/kops"
	"k8s.io/kops/pkg/assets"
	"k8s.io/kops/util/pkg/hashing"
)

func Test_BuildMirroredAsset(t *testing.T) {
	tests := []struct {
		url      string
		hash     string
		expected []string
	}{
		{
			url: "https://artifacts.k8s.io/binaries/kops/%s/linux/amd64/nodeup",
			expected: []string{
				"https://artifacts.k8s.io/binaries/kops/1.32.0-beta.1/linux/amd64/nodeup",
				"https://github.com/kubernetes/kops/releases/download/v1.32.0-beta.1/nodeup-linux-amd64",
			},
		},
		{
			url: "https://artifacts.k8s.io/binaries/kops/%s/linux/arm64/nodeup",
			expected: []string{
				"https://artifacts.k8s.io/binaries/kops/1.32.0-beta.1/linux/arm64/nodeup",
				"https://github.com/kubernetes/kops/releases/download/v1.32.0-beta.1/nodeup-linux-arm64",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			h := hashing.MustFromString("0000000000000000000000000000000000000000000000000000000000000000")
			u, err := url.Parse(fmt.Sprintf(tc.url, kops.Version))
			if err != nil {
				t.Errorf("cannot parse URL: %s", fmt.Sprintf(tc.url, kops.Version))
				return
			}
			asset := &assets.FileAsset{
				DownloadURL:  u,
				CanonicalURL: u,
				SHAValue:     h,
			}
			actual := assets.BuildMirroredAsset(asset)

			if !reflect.DeepEqual(actual.Locations, tc.expected) {
				t.Errorf("Locations differ:\nActual: %+v\nExpect: %+v", actual.Locations, tc.expected)
				return
			}
		})
	}
}

func TestKopsAssetPath(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		file     string
		expected string
	}{
		{
			name:     "regular base URL preserves binary hierarchy",
			baseURL:  "https://assets.example.com/kops/v1/",
			file:     "linux/amd64/nodeup",
			expected: "linux/amd64/nodeup",
		},
		{
			name:     "regular base URL preserves image hierarchy",
			baseURL:  "https://assets.example.com/kops/v1/",
			file:     "images/kops-controller-amd64.tar.gz",
			expected: "images/kops-controller-amd64.tar.gz",
		},
		{
			name:     "GitHub release flattens binary name",
			baseURL:  "https://github.com/Elemento-Modular-Cloud/kops/releases/download/v1/",
			file:     "linux/amd64/nodeup",
			expected: "nodeup-linux-amd64",
		},
		{
			name:     "GitHub release flattens arm64 binary name",
			baseURL:  "https://github.com/Elemento-Modular-Cloud/kops/releases/download/v1/",
			file:     "linux/arm64/protokube",
			expected: "protokube-linux-arm64",
		},
		{
			name:     "GitHub release flattens image path",
			baseURL:  "https://github.com/Elemento-Modular-Cloud/kops/releases/download/v1/",
			file:     "images/kops-controller-amd64.tar.gz",
			expected: "kops-controller-amd64.tar.gz",
		},
		{
			name:     "GitHub release preserves unknown hierarchy",
			baseURL:  "https://github.com/Elemento-Modular-Cloud/kops/releases/download/v1/",
			file:     "other/example/file",
			expected: "other/example/file",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base, err := url.Parse(tc.baseURL)
			if err != nil {
				t.Fatalf("parsing base URL: %v", err)
			}

			if actual := kopsAssetPath(base, tc.file); actual != tc.expected {
				t.Fatalf("unexpected asset path: got %q, want %q", actual, tc.expected)
			}
		})
	}
}
