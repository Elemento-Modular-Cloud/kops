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
	"os"
	"strings"

	// "github.com/Elemento-Modular-Cloud/ecloud-go/ecloud/metadata"
	"k8s.io/kops/pkg/bootstrap"
)

const (
	ElementoAuthenticationTokenPrefix = "x-elemento-id "
	ElementoBootstrapTokenFile        = "/etc/elemento/bootstrap-token"
)

type elementoAuthenticator struct {
}

var _ bootstrap.Authenticator = &elementoAuthenticator{}

func NewElementoAuthenticator() (bootstrap.Authenticator, error) {
	return &elementoAuthenticator{}, nil
}

func (h *elementoAuthenticator) CreateToken(body []byte) (string, error) {
	token, err := readElementoBootstrapToken(ElementoBootstrapTokenFile)
	if err != nil {
		return "", err
	}
	return ElementoAuthenticationTokenPrefix + token, nil
}

func readElementoBootstrapToken(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading Elemento bootstrap token file %q: %w", path, err)
	}
	token := strings.TrimSpace(string(b))
	if token == "" {
		return "", fmt.Errorf("Elemento bootstrap token file %q is empty", path)
	}
	return token, nil
}
