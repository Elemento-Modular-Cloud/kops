package elementomodel

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Elemento-Modular-Cloud/ecloud-go/ecloud"
	"k8s.io/kops/upup/pkg/fi"
	"k8s.io/kops/upup/pkg/fi/fitasks"
	"k8s.io/kops/util/pkg/vfs"
)

const placementPlanLocation = "elemento/placement-plan.json"

// PlacementModelBuilder validates the entire cluster before cloud tasks run.
type PlacementModelBuilder struct {
	*ElementoModelContext
	Lifecycle  fi.Lifecycle
	ConfigBase vfs.Path
}

func (b *PlacementModelBuilder) Build(c *fi.CloudupModelBuilderContext) error {
	cfg, err := ecloud.LoadMulticloudConfig()
	if err != nil {
		return err
	}
	groups := b.AllInstanceGroups
	if groups == nil {
		groups = b.InstanceGroups
	}
	var nodes []ecloud.PlacementNode
	names, err := b.nodeNamesByInstanceGroup(nil)
	if err != nil {
		return err
	}
	for _, ig := range groups {
		for _, name := range names[ig.Name] {
			nodes = append(nodes, ecloud.PlacementNode{Name: name, Role: string(ig.Spec.Role)})
		}
	}
	if b.ConfigBase == nil {
		return fmt.Errorf("config store is required for the Elemento placement plan")
	}
	var previous *ecloud.MulticloudPlan
	data, err := b.ConfigBase.Join(placementPlanLocation).ReadFile(c.Context())
	if err == nil {
		previous = &ecloud.MulticloudPlan{}
		if err := json.Unmarshal(data, previous); err != nil {
			return fmt.Errorf("reading saved Elemento placement plan: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading Elemento placement plan: %w", err)
	}
	plan, err := ecloud.BuildMulticloudPlan(b.ClusterName(), cfg, nodes, previous)
	if err != nil {
		return err
	}
	if err := plan.ValidateProvisioning(); err != nil {
		return err
	}
	data, err = json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}
	c.AddTask(&fitasks.ManagedFile{
		Name: fi.PtrTo("elemento-placement"), Lifecycle: b.Lifecycle,
		Location: fi.PtrTo(placementPlanLocation), Contents: fi.NewBytesResource(data),
		PublicACL: fi.PtrTo(false),
	})
	ecloud.SetMulticloudPlan(plan)
	return nil
}
