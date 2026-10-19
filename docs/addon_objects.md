# Passing additional configuration objects

kOps supports passing additional objects to the cluster, and recognizes certain "well known" objects.

Objects that are not well-known will be applied directly to the cluster's k8s api.

## Adding objects to a cluster

To add an additional object to an existing cluster, use `kops create -f`:

```
kops create -f scheduler-config.yaml
kops update cluster --name=my.cluster.k8s.local --yes --admin
kops rolling-update cluster --name=my.cluster.k8s.local --yes
```

When creating a new cluster, you can use the `--add` flag (requires the `ClusterAddons` feature flag):

```
export KOPS_FEATURE_FLAGS=ClusterAddons
kops create cluster --name=my.cluster.k8s.local --zones us-east-2a --add scheduler-config.yaml
kops update cluster --name=my.cluster.k8s.local --yes --admin
```

To view the current additional objects for a cluster:

```
kops get all -o yaml
```

# Well-Known Objects

Well-known objects receive special handling from kOps instead of being applied directly to the cluster.

## KubeSchedulerConfiguration (group: kubescheduler.config.k8s.io)

KubeSchedulerConfiguration objects allow for custom configuration of
kube-scheduler, the component responsible for assigning Pods to Nodes.

Fields set in `spec.kubeScheduler` (see [cluster_spec.md](cluster_spec.md#kubescheduler)) are merged on top of the KubeSchedulerConfiguration object. 
This allows you to use KubeSchedulerConfiguration for advanced settings like scheduler profiles and plugins, while using `spec.kubeScheduler` for simpler fields.

Note: the `clientConnection.kubeconfig` field is managed by kOps and must not be set manually.

Example KubeSchedulerConfiguration file:

```yaml
# scheduler-config.yaml
apiVersion: kubescheduler.config.k8s.io/v1
kind: KubeSchedulerConfiguration
profiles:
  - schedulerName: default-scheduler
    plugins:
      score:
        disabled:
          - name: NodeResourcesBalancedAllocation
```

Example usage with an existing cluster:

```
kops create -f scheduler-config.yaml
kops update cluster --name=my.cluster.k8s.local --yes --admin
kops rolling-update cluster --name=my.cluster.k8s.local --yes
```

# Other Kubernetes Objects

Any Kubernetes object that is not a well-known type (such as ConfigMaps, Deployments,
DaemonSets, Services, RBAC resources, etc.) will be applied directly to the cluster
as a standard Kubernetes resource during `kops update cluster`.
