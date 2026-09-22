/*
Copyright 2019 The Kubernetes Authors.

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

package protokube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/digitalocean/godo"
	"golang.org/x/oauth2"

	"k8s.io/kops/protokube/pkg/gossip"
	gossipdo "k8s.io/kops/protokube/pkg/gossip/do"
)

const (
	dropletNameMetadataURL = "http://169.254.169.254/metadata/v1/hostname"
	dropletIDMetadataTags  = "http://169.254.169.254/metadata/v1/tags"
)

type DOCloudProvider struct {
	ClusterID  string
	godoClient *godo.Client

	dropletName string
	dropletTags []string
}

var _ CloudProvider = &DOCloudProvider{}

func GetClusterID() (string, error) {
	clusterID := ""

	dropletTags, err := getMetadataDropletTags()
	if err != nil {
		return clusterID, fmt.Errorf("GetClusterID failed - unable to retrieve droplet tags: %s", err)
	}

	for _, dropletTag := range dropletTags {
		if strings.Contains(dropletTag, "KubernetesCluster:") {
			clusterID = strings.ReplaceAll(dropletTag, ".", "-")

			tokens := strings.Split(clusterID, ":")
			if len(tokens) != 2 {
				return clusterID, fmt.Errorf("invalid clusterID (expected two tokens): %q", clusterID)
			}

			clusterID := tokens[1]

			return clusterID, nil
		}
	}

	return clusterID, fmt.Errorf("failed to get droplet clusterID")
}

func NewDOCloudProvider() (*DOCloudProvider, error) {
	dropletName, err := getMetadataDropletName()
	if err != nil {
		return nil, fmt.Errorf("failed to get droplet name: %s", err)
	}

	godoClient, err := NewDOCloud()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize digitalocean cloud: %s", err)
	}

	dropletTags, err := getMetadataDropletTags()
	if err != nil {
		return nil, fmt.Errorf("failed to get droplet tags: %s", err)
	}

	clusterID, err := GetClusterID()
	if err != nil {
		return nil, fmt.Errorf("failed to get clusterID: %s", err)
	}

	return &DOCloudProvider{
		godoClient:  godoClient,
		ClusterID:   clusterID,
		dropletName: dropletName,
		dropletTags: dropletTags,
	}, nil
}

func NewDOCloud() (*godo.Client, error) {
	accessToken := os.Getenv("DIGITALOCEAN_ACCESS_TOKEN")
	if accessToken == "" {
		return nil, errors.New("DIGITALOCEAN_ACCESS_TOKEN is required")
	}

	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	return godo.NewClient(oauth2.NewClient(context.TODO(), tokenSource)), nil
}

func (d *DOCloudProvider) GossipSeeds() (gossip.SeedProvider, error) {
	for _, dropletTag := range d.dropletTags {
		if strings.Contains(dropletTag, strings.ReplaceAll(d.ClusterID, ".", "-")) {
			return gossipdo.NewSeedProvider(d.godoClient, dropletTag)
		}
	}

	return nil, fmt.Errorf("could not determine a matching droplet tag for gossip seeding")
}

func (d *DOCloudProvider) InstanceID() string {
	return d.dropletName
}

func getMetadataDropletName() (string, error) {
	return getMetadata(dropletNameMetadataURL)
}

func getMetadataDropletTags() ([]string, error) {
	tagString, err := getMetadata(dropletIDMetadataTags)
	return strings.Split(tagString, "\n"), err
}

func getMetadata(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("droplet metadata returned non-200 status code: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(bodyBytes), nil
}
