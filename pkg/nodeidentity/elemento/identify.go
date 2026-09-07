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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Elemento-Modular-Cloud/ecloud-go/ecloud"
	corev1 "k8s.io/api/core/v1"
	k8svalidation "k8s.io/apimachinery/pkg/util/validation"
	expirationcache "k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/kops/pkg/apis/kops"

	// "k8s.io/kops/pkg/cloudinstances"
	"k8s.io/kops/pkg/nodeidentity"
	"k8s.io/kops/pkg/nodelabels"
	"k8s.io/kops/upup/pkg/fi/cloudup/elemento"
)

const (
	cacheTTL                              = 60 * time.Minute
	maxIdentityResponseSize               = 1 << 20
	clusterAutoscalerNodeTemplateLabelKey = "k8s.io/cluster-autoscaler/node-template/label/"
	cloudInstanceRoleLabelKey             = "k8s.io/role/"
)

// nodeIdentifier identifies a node from Elemento
type nodeIdentifier struct {
	client         *ecloud.Client
	clusterName    string
	authServiceURL string
	verifierAPIKey string
	httpClient     *http.Client
	cache          expirationcache.Store
	cacheEnabled   bool
}

type authNodeIdentity struct {
	ID                 string            `json:"id"`
	ClusterName        string            `json:"cluster_name"`
	NodeName           string            `json:"node_name"`
	InstanceGroup      string            `json:"instance_group"`
	Role               string            `json:"role"`
	InternalIP         string            `json:"internal_ip"`
	Provider           string            `json:"provider"`
	ProviderInstanceID string            `json:"provider_instance_id"`
	Labels             map[string]string `json:"labels"`
	State              string            `json:"state"`
}

type staticNodeInfo struct {
	InstanceID string
	ProviderID string
	Labels     map[string]string
}

var staticNodesByName = map[string]staticNodeInfo{
	// Static identities from the original three-node development scenario are
	// intentionally disabled. Uncomment them only when reproducing that exact
	// environment; normal provisioning must resolve the UUID from ecloud-go.
	// "control-plane-europe-1": {
	// 	InstanceID: "fc72216e-6fb0-4cbf-a2be-3973da79f955",
	// 	Labels: map[string]string{
	// 		nodelabels.RoleLabelControlPlane20: "",
	// 	},
	// },
	// "nodes-europe-1": {
	// 	InstanceID: "e4ff7b13-51c1-48bf-9ba9-c5fb5839c358",
	// 	Labels: map[string]string{
	// 		nodelabels.RoleLabelNode16: "",
	// 	},
	// },
	// "nodes-europe-2": {
	// 	InstanceID: "f1dc002b-a660-423f-8850-8b3fc28c1625",
	// 	Labels: map[string]string{
	// 		nodelabels.RoleLabelNode16: "",
	// 	},
	// },
}

// New creates and returns a nodeidentity.Identifier for Nodes running on Elemento
func New(CacheNodeidentityInfo bool, clusterName string, verifierOptions *elemento.ElementoVerifierOptions) (nodeidentity.Identifier, error) {
	elementoClient, err := ecloud.NewClient("kops-elemento", "1.0")

	if err != nil {
		return nil, fmt.Errorf("creating client for Elemento Cloud: %w", err)
	}

	identifier := &nodeIdentifier{
		client:       elementoClient,
		clusterName:  strings.TrimSpace(clusterName),
		httpClient:   &http.Client{Timeout: 15 * time.Second},
		cache:        expirationcache.NewTTLStore(stringKeyFunc, cacheTTL),
		cacheEnabled: CacheNodeidentityInfo,
	}

	if verifierOptions != nil {
		identifier.authServiceURL, err = readRequiredFile(verifierOptions.AuthServiceURLFile, "auth service URL")
		if err != nil {
			return nil, err
		}
		parsedURL, parseErr := url.Parse(identifier.authServiceURL)
		if parseErr != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
			return nil, fmt.Errorf("invalid Elemento auth service URL in %q", verifierOptions.AuthServiceURLFile)
		}
		identifier.authServiceURL = strings.TrimRight(identifier.authServiceURL, "/")
		identifier.verifierAPIKey, err = readRequiredFile(verifierOptions.VerifierAPIKeyFile, "verifier API key")
		if err != nil {
			return nil, err
		}
	}

	return identifier, nil
}

// IdentifyNode queries Elemento for the node identity information
func (i *nodeIdentifier) IdentifyNode(ctx context.Context, node *corev1.Node) (*nodeidentity.Info, error) {
	if info, ok := staticNodeIdentity(node.Name); ok {
		return info, nil
	}
	if i.authServiceURL != "" {
		return i.identifyNodeFromAuthService(ctx, node)
	}

	providerID := node.Spec.ProviderID
	var serverID string
	var server *ecloud.Server
	if providerID != "" {
		if !strings.HasPrefix(providerID, "elemento://") {
			return nil, fmt.Errorf("providerID %q not recognized for node %s", providerID, node.Name)
		}
		serverID = strings.TrimPrefix(providerID, "elemento://")

		// If cache is enabled, check if the node information is already cached
		if i.cacheEnabled {
			obj, exists, err := i.cache.GetByKey(serverID)
			if err != nil {
				klog.Warningf("Nodeidentity info cache lookup failure: %v", err)
			}
			if exists {
				return obj.(*nodeidentity.Info), nil
			}
		}

		var err error
		server, err = i.getServer(ctx, serverID)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		server, err = i.getServerByNodeName(ctx, node.Name)
		if err != nil {
			return nil, err
		}
		serverID = server.ID
	}

	if server.Status != "running" {
		return nil, fmt.Errorf("server %s is not running", serverID)
	}

	labels := map[string]string{}
	for key, value := range server.Labels {
		switch {
		case key == elemento.TagKubernetesInstanceRole:
			switch kops.InstanceGroupRole(value) {
			case kops.InstanceGroupRoleControlPlane:
				labels[nodelabels.RoleLabelControlPlane20] = ""
			case kops.InstanceGroupRoleNode:
				labels[nodelabels.RoleLabelNode16] = ""
			case kops.InstanceGroupRoleAPIServer:
				labels[nodelabels.RoleLabelAPIServer16] = ""
			default:
				klog.Warningf("Unknown node role %q for server %s(%s)", value, server.Name, server.ID)
			}
		case strings.HasPrefix(key, elemento.TagKubernetesNodeLabelPrefix):
			labels[strings.TrimPrefix(key, elemento.TagKubernetesNodeLabelPrefix)] = value
		}
	}
	addRoleLabelFallback(labels, server.Name)

	info := &nodeidentity.Info{
		InstanceID:  serverID,
		ProviderID:  "elemento://" + serverID,
		Labels:      labels,
		Initialized: true,
	}

	// If cache is enabled, store the node information in the cache
	if i.cacheEnabled {
		if err := i.cache.Add(info); err != nil {
			klog.Warningf("Failed to add node identity info to cache: %v", err)
		}
	}

	return info, nil
}

func (i *nodeIdentifier) identifyNodeFromAuthService(ctx context.Context, node *corev1.Node) (*nodeidentity.Info, error) {
	if i.clusterName == "" {
		return nil, fmt.Errorf("Elemento cluster name is required for node identity lookup")
	}

	if node.Spec.ProviderID != "" {
		if !strings.HasPrefix(node.Spec.ProviderID, "elemento://") {
			return nil, fmt.Errorf("providerID %q not recognized for node %s", node.Spec.ProviderID, node.Name)
		}
		if i.cacheEnabled {
			serverID := strings.TrimPrefix(node.Spec.ProviderID, "elemento://")
			obj, exists, err := i.cache.GetByKey(serverID)
			if err != nil {
				klog.Warningf("Nodeidentity info cache lookup failure: %v", err)
			}
			if exists {
				return obj.(*nodeidentity.Info), nil
			}
		}
	}

	requestURL := i.authServiceURL + "/v1/clusters/" + url.PathEscape(i.clusterName) + "/nodes/by-name/" + url.PathEscape(node.Name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating Elemento node identity request: %w", err)
	}
	req.Header.Set("X-Verifier-API-Key", i.verifierAPIKey)

	response, err := i.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying Elemento node identity for %q: %w", node.Name, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Elemento auth service returned status %d for node %q", response.StatusCode, node.Name)
	}

	var identity authNodeIdentity
	if err := json.NewDecoder(io.LimitReader(response.Body, maxIdentityResponseSize)).Decode(&identity); err != nil {
		return nil, fmt.Errorf("decoding Elemento node identity for %q: %w", node.Name, err)
	}
	if identity.ClusterName != i.clusterName || identity.NodeName != node.Name {
		return nil, fmt.Errorf("Elemento auth service returned mismatched identity for node %q", node.Name)
	}
	if identity.Provider != "elemento" {
		return nil, fmt.Errorf("Elemento auth service returned provider %q for node %q", identity.Provider, node.Name)
	}
	if strings.TrimSpace(identity.ProviderInstanceID) == "" {
		return nil, fmt.Errorf("Elemento auth service returned no provider instance ID for node %q", node.Name)
	}
	if identity.State == "revoked" {
		return nil, fmt.Errorf("Elemento identity for node %q has been revoked", node.Name)
	}
	providerID := "elemento://" + identity.ProviderInstanceID
	if node.Spec.ProviderID != "" && node.Spec.ProviderID != providerID {
		return nil, fmt.Errorf("providerID %q does not match authenticated providerID %q for node %s", node.Spec.ProviderID, providerID, node.Name)
	}

	labels := kubernetesNodeLabels(identity.Labels)
	switch kops.InstanceGroupRole(identity.Role) {
	case kops.InstanceGroupRoleControlPlane:
		labels[nodelabels.RoleLabelControlPlane20] = ""
	case kops.InstanceGroupRoleNode:
		labels[nodelabels.RoleLabelNode16] = ""
	case kops.InstanceGroupRoleAPIServer:
		labels[nodelabels.RoleLabelAPIServer16] = ""
	default:
		return nil, fmt.Errorf("Elemento auth service returned unknown role %q for node %q", identity.Role, node.Name)
	}

	info := &nodeidentity.Info{
		InstanceID:  identity.ProviderInstanceID,
		ProviderID:  providerID,
		Labels:      labels,
		Initialized: true,
	}
	if i.cacheEnabled {
		if err := i.cache.Add(info); err != nil {
			klog.Warningf("Failed to add node identity info to cache: %v", err)
		}
	}
	return info, nil
}

func kubernetesNodeLabels(source map[string]string) map[string]string {
	labels := make(map[string]string, len(source))
	for key, value := range source {
		switch {
		case strings.HasPrefix(key, clusterAutoscalerNodeTemplateLabelKey):
			key = strings.TrimPrefix(key, clusterAutoscalerNodeTemplateLabelKey)
		case strings.HasPrefix(key, elemento.TagKubernetesNodeLabelPrefix):
			key = strings.TrimPrefix(key, elemento.TagKubernetesNodeLabelPrefix)
		case strings.HasPrefix(key, cloudInstanceRoleLabelKey):
			// This is an instance tag used by kOps, not a Kubernetes node label.
			continue
		}

		if errors := k8svalidation.IsQualifiedName(key); len(errors) != 0 {
			klog.Warningf("Ignoring invalid Kubernetes node label %q from Elemento identity: %s", key, strings.Join(errors, "; "))
			continue
		}
		if errors := k8svalidation.IsValidLabelValue(value); len(errors) != 0 {
			klog.Warningf("Ignoring invalid value for Kubernetes node label %q from Elemento identity: %s", key, strings.Join(errors, "; "))
			continue
		}
		labels[key] = value
	}
	return labels
}

func readRequiredFile(path, description string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("Elemento %s file path is required", description)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading Elemento %s file %q: %w", description, path, err)
	}
	value := strings.TrimSpace(string(contents))
	if value == "" {
		return "", fmt.Errorf("Elemento %s file %q is empty", description, path)
	}
	return value, nil
}

func staticNodeIdentity(nodeName string) (*nodeidentity.Info, bool) {
	static, ok := staticNodesByName[nodeName]
	if !ok {
		return nil, false
	}
	providerID := static.ProviderID
	if providerID == "" {
		providerID = "elemento://" + static.InstanceID
	}
	labels := map[string]string{}
	for key, value := range static.Labels {
		labels[key] = value
	}
	return &nodeidentity.Info{
		InstanceID:  static.InstanceID,
		ProviderID:  providerID,
		Labels:      labels,
		Initialized: true,
	}, true
}

// stringKeyFunc is a string as cache key function
func stringKeyFunc(obj interface{}) (string, error) {
	key := obj.(*nodeidentity.Info).InstanceID
	return key, nil
}

// getServer retrieves the server information from Elemento for the given server ID
func (i *nodeIdentifier) getServer(ctx context.Context, id string) (*ecloud.Server, error) {
	server, _, err := i.client.Server.GetByID(ctx, id)
	if err != nil || server == nil {
		return nil, fmt.Errorf("failed to get info for server %q: %w", id, err)
	}

	return server, nil
}

// getServerByNodeName retrieves the Elemento server that registered the given Kubernetes node name.
func (i *nodeIdentifier) getServerByNodeName(ctx context.Context, nodeName string) (*ecloud.Server, error) {
	servers, _, err := i.client.Server.List(ctx, ecloud.ServerListOpts{Name: nodeName})
	if err != nil {
		return nil, fmt.Errorf("failed to list servers for node %q: %w", nodeName, err)
	}

	var matches []*ecloud.Server
	var clusterMatches []*ecloud.Server
	for _, server := range servers {
		if server.Name != nodeName {
			continue
		}
		matches = append(matches, server)
		if i.clusterName != "" && server.Labels[elemento.TagKubernetesClusterName] == i.clusterName {
			clusterMatches = append(clusterMatches, server)
		}
	}

	if len(clusterMatches) > 0 {
		matches = clusterMatches
	}

	switch len(matches) {
	case 0:
		if i.clusterName != "" {
			return nil, fmt.Errorf("no Elemento server found for node %q in cluster %q", nodeName, i.clusterName)
		}
		return nil, fmt.Errorf("no Elemento server found for node %q", nodeName)
	case 1:
		return matches[0], nil
	default:
		return nil, fmt.Errorf("found multiple Elemento servers for node %q", nodeName)
	}
}

func addRoleLabelFallback(labels map[string]string, serverName string) {
	for _, key := range []string{
		nodelabels.RoleLabelControlPlane20,
		nodelabels.RoleLabelNode16,
		nodelabels.RoleLabelAPIServer16,
	} {
		if _, found := labels[key]; found {
			return
		}
	}

	switch {
	case strings.HasPrefix(serverName, "control-plane-"):
		labels[nodelabels.RoleLabelControlPlane20] = ""
	case strings.HasPrefix(serverName, "nodes-"):
		labels[nodelabels.RoleLabelNode16] = ""
	}
}
