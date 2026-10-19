// Copyright 2015 The Kubernetes Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package gcetokensource is an in-tree copy of the AltTokenSource type from
// k8s.io/cloud-provider-gcp/providers/gce/token_source.go at tag v32.4.0.
// It is forked rather than imported so kops does not pull in the full
// cloud-provider-gcp module just for one OAuth2 helper used by the
// clouddns provider.
//
// Modifications relative to upstream:
//
//   - Prometheus counters and the legacyregistry init() are removed; the
//     k8s.io/component-base/metrics dependency is no longer needed
//   - the build constraint and gce package name are dropped (the file is
//     consumed only as a function helper, not as part of the gce package)
package gcetokensource
