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

package azure

import (
	"errors"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

// IsDependencyViolation checks if the error is a transient dependency/conflict
// error that should be retried quietly during resource deletion.
func IsDependencyViolation(err error) bool {
	var azErr *azcore.ResponseError
	if !errors.As(err, &azErr) {
		return false
	}
	switch azErr.ErrorCode {
	// Conflict is returned when deleting multiple VMSS VMs concurrently;
	// Azure only allows one mutating operation per VMSS at a time.
	case "Conflict":
		return true
	case "InUseRouteTableCannotBeDeleted", "InUseNetworkSecurityGroupCannotBeDeleted",
		"InUseSubnetCannotBeDeleted", "NatGatewayInUseBySubnet":
		return true
	default:
		return false
	}
}
