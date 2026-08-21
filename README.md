# C3OS RKE2 Cluster Plugin

---

> **Found a bug, or want to request a feature?** Open it on
> [kairos-io/kairos](https://github.com/kairos-io/kairos/issues), including
> issues about this repository. Every Kairos issue lives in one place, so you
> never have to work out which repository to file against.

This provider will configure a RKE2 installation based on the cluster section of cloud init.

## Configuration

`cluster_token`: a token all members of the cluster must have to join the cluster.

`control_plane_host`: the host of the cluster control plane.  This is used to join nodes to a cluster.  If this is a single node cluster this is not required.

`role`: defines what operations is this device responsible for. The roles are described in detail below.
- `init` This role denotes a device that should initialize the etcd cluster and operate as a RKE2 server.  There should only be one device with this role per cluster.
- `controlplane`: runs the RKE2 server.
- `worker`: runs the RKE2 agent.

### Example
```yaml
#cloud-config

cluster:
  cluster_token: randomstring
  control_plane_host: cluster.example.com
  role: init
  config: |
    node-name: example-node
```