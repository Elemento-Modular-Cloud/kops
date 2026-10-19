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

package vfs

import "testing"

func Test_GSPath_Parse(t *testing.T) {
	grid := []struct {
		Input          string
		ExpectError    bool
		ExpectedBucket string
		ExpectedPath   string
	}{
		{
			Input:          "gs://bucket",
			ExpectedBucket: "bucket",
			ExpectedPath:   "",
		},
		{
			Input:          "gs://bucket/path",
			ExpectedBucket: "bucket",
			ExpectedPath:   "path",
		},
		{
			Input:          "gs://bucket2/path/subpath",
			ExpectedBucket: "bucket2",
			ExpectedPath:   "path/subpath",
		},
		{
			Input:       "gs:///bucket/path/subpath",
			ExpectError: true,
		},
		{
			Input:       "gs://",
			ExpectError: true,
		},
	}
	for _, g := range grid {
		gspath, err := Context.buildGCSPath(g.Input)
		if !g.ExpectError {
			if err != nil {
				t.Fatalf("unexpected error parsing gs path: %v", err)
			}
			if gspath.bucket != g.ExpectedBucket {
				t.Fatalf("unexpected gs path: %v", gspath)
			}
			if gspath.key != g.ExpectedPath {
				t.Fatalf("unexpected gs path: %v", gspath)
			}
		} else {
			if err == nil {
				t.Fatalf("unexpected success parsing %q", g.Input)
			}
		}
	}
}
