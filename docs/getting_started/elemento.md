# Getting Started with kOps on Elemento Cloud

## DNS Topology

Elemento currently supports `--dns=private`, which is also its default when
`--dns` is omitted. Its bootstrap records are provisioned in an Elemento-managed
private BIND zone rather than by the in-cluster dns-controller addon.

`--dns=none` is not supported by the current integration because it would skip
DNS tasks required for node bootstrap and direct etcd peer discovery.
`--dns=public` is not supported because authoritative public DNS publication
has not been implemented.

Private DNS does not prevent exposing the API through the public frontend of
the Elemento load balancer. Clients outside the private DNS network must resolve
the API hostname separately, for example through a public DNS record managed
outside kOps or a local hosts entry. Kubernetes TLS and authentication still
apply to requests forwarded by the load balancer.

For existing Elemento state created with `--dns=public`, change
`spec.topology.dns.type` to `Private` in the stored `v1alpha2` YAML before using
this version of kOps.
Changing this field does not publish or remove records in an external public
DNS service.

## Development Verification

This branch requires Go 1.27.1. Select it per command without changing the global
Go configuration:

```bash
GOTOOLCHAIN=go1.27.1 go version
GOTOOLCHAIN=go1.27.1 make kops
```

The development `go.mod` uses the sibling `../ecloud-go` checkout. The bootstrap
assets and controller images must be built from the integrated code before
provisioning a cluster; old Elemento bootstrap releases are not compatible with
the new `kops-channels` static pod.
