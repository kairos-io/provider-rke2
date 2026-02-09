# RKE2 Provider Cluster Roles

## Overview

Provider-rke2 supports three cluster roles: init (bootstrap first etcd node), controlplane (additional control plane nodes), and worker (agent-only nodes). Each role has distinct responsibilities, configuration requirements, and operational characteristics for building resilient enterprise edge Kubernetes clusters.

## Key Concepts

### Role Hierarchy

```
┌──────────────────────────────────────────────────────┐
│  Init Role (role: init)                              │
│  • First node in cluster                             │
│  • Bootstraps embedded etcd cluster                  │
│  • Becomes first control plane                        │
│  • Generates cluster certificates                     │
│  • ONE per cluster                                   │
└──────────────────────────────────────────────────────┘
                        │
                        ▼
┌──────────────────────────────────────────────────────┐
│  ControlPlane Role (role: controlplane)              │
│  • Joins existing etcd cluster                       │
│  • Runs API server, scheduler, controller            │
│  • Participates in etcd consensus                    │
│  • Can handle API traffic independently              │
│  • MULTIPLE for HA (typically 3 or 5 total)          │
└──────────────────────────────────────────────────────┘
                        │
                        ▼
┌──────────────────────────────────────────────────────┐
│  Worker Role (role: worker)                          │
│  • Runs rke2-agent service only                      │
│  • No etcd, API server, or controllers               │
│  • Runs kubelet and container runtime                │
│  • Connects to control plane for API requests        │
│  • MULTIPLE for workload capacity                    │
└──────────────────────────────────────────────────────┘
```

### Init Role Specifics

**Purpose**: Bootstrap the first node and initialize the etcd cluster.

**Components Running**:
- etcd (cluster datastore)
- kube-apiserver (Kubernetes API)
- kube-scheduler (pod scheduling)
- kube-controller-manager (reconciliation loops)
- kubelet (node agent)
- containerd (container runtime)
- CoreDNS (cluster DNS)
- Canal CNI (Calico + Flannel for networking and policy)
- Ingress-NGINX (ingress - if not disabled)

**Provider Configuration Logic**:
```go
// /Users/rishi/work/src/provider-rke2/pkg/provider/provider.go
case clusterplugin.RoleInit:
    rke2Config.Server = ""            // No upstream server
    rke2Config.TLSSan = []string{cluster.ControlPlaneHost}
    systemName = "rke2-server"        // Server service
```

**Key Configuration Options**:
- No `server` field - This is the first node
- TLS SANs include control_plane_host for certificate
- Listens on port 9345 for new nodes to join
- API server on port 6443 after bootstrap

### ControlPlane Role Specifics

**Purpose**: Add additional control plane nodes for high availability.

**Components Running**: Same as init role (etcd, API server, scheduler, controller, kubelet, Canal, etc.)

**Provider Configuration Logic**:
```go
case clusterplugin.RoleControlPlane:
    rke2Config.Server = fmt.Sprintf("https://%s:9345", cluster.ControlPlaneHost)
    rke2Config.TLSSan = []string{cluster.ControlPlaneHost}
    systemName = "rke2-server"        // Server service
```

**Key Configuration Options**:
- `server: https://<init-node>:9345` - Points to init node registration endpoint
- Uses cluster_token to authenticate and join etcd
- After join, becomes full control plane with its own API server

**Joining Process**:
1. Contacts init node registration endpoint (port 9345) using cluster_token
2. Downloads cluster CA and certificates
3. Joins etcd cluster as new member
4. Starts API server, scheduler, controller
5. Begins serving API requests on port 6443

### Worker Role Specifics

**Purpose**: Run application workloads without control plane overhead.

**Components Running**:
- kubelet (node agent)
- kube-proxy (service networking)
- containerd (container runtime)
- Canal CNI agent (Flannel for networking, Calico for policy)

**Components NOT Running**:
- etcd (no datastore)
- kube-apiserver (no API serving)
- kube-scheduler (no scheduling logic)
- kube-controller-manager (no reconciliation)

**Provider Configuration Logic**:
```go
case clusterplugin.RoleWorker:
    systemName = "rke2-agent"         // Agent service (not rke2-server)
    rke2Config.Server = fmt.Sprintf("https://%s:9345", cluster.ControlPlaneHost)
    // No TLSSan needed for workers
```

**Key Configuration Options**:
- `server: https://<control-plane>:9345` - Registration endpoint
- Uses cluster_token to authenticate kubelet
- After registration, uses API at port 6443
- Significantly lower resource consumption than control plane

## Implementation Patterns

### Single-Node Development Cluster

```yaml
#cloud-config
# Simple single-node cluster for testing

cluster:
  cluster_token: K10dev-single-node-rke2-token
  control_plane_host: 192.168.1.100
  role: init
  config: |
    write-kubeconfig-mode: "0644"
    node-name: rke2-dev-all-in-one
    selinux: true
```

**Use case**: Development, testing, CI/CD environments
**Pros**: Simple, low resource usage, quick setup
**Cons**: No HA, single point of failure

### 3-Node HA Control Plane Cluster

```yaml
# Node 1 (init - bootstrap)
cluster:
  cluster_token: K10ha-rke2-cluster-shared-token
  control_plane_host: 10.0.1.10
  role: init
  config: |
    tls-san:
      - 10.0.1.10
      - 10.0.1.11
      - 10.0.1.12
      - rke2-api.example.com
    selinux: true
    profile: cis-1.23

# Node 2 (controlplane - join)
cluster:
  cluster_token: K10ha-rke2-cluster-shared-token
  control_plane_host: 10.0.1.10
  role: controlplane
  config: |
    tls-san:
      - 10.0.1.10
      - 10.0.1.11
      - 10.0.1.12
      - rke2-api.example.com
    selinux: true

# Node 3 (controlplane - join)
cluster:
  cluster_token: K10ha-rke2-cluster-shared-token
  control_plane_host: 10.0.1.10
  role: controlplane
  config: |
    tls-san:
      - 10.0.1.10
      - 10.0.1.11
      - 10.0.1.12
      - rke2-api.example.com
    selinux: true
```

**Why 3 nodes**: etcd requires (n/2)+1 for quorum. 3 nodes tolerate 1 failure.

**Use case**: Production enterprise edge sites requiring HA
**Pros**: Tolerates 1 node failure, distributed API load, meets enterprise SLAs
**Cons**: Higher resource usage, more complex deployment

### Multi-Worker Enterprise Edge Deployment

```yaml
# Control plane (init)
cluster:
  cluster_token: K10enterprise-edge-deployment-token
  control_plane_host: 10.0.1.10
  role: init
  config: |
    selinux: true
    profile: cis-1.23

# Workers (multiple)
cluster:
  cluster_token: K10enterprise-edge-deployment-token
  control_plane_host: 10.0.1.10
  role: worker
  config: |
    node-name: rke2-worker-{{.DeviceID}}
    node-label:
      - "site={{.SiteID}}"
      - "region={{.Region}}"
      - "workload=edge-inference"
      - "compliance=cis"
    selinux: true
```

**Use case**: Edge sites with centralized control plane, distributed workers
**Pros**: Scalable workload capacity, efficient resource use, security hardening
**Cons**: Workers depend on control plane connectivity

### 5-Node HA with Dedicated Workers

```yaml
# 3 control plane nodes (init + 2 controlplane)
# ... (as in 3-node HA example)

# Worker nodes (dedicated for workloads)
cluster:
  cluster_token: K10ha-dedicated-workers-rke2-token
  control_plane_host: 10.0.1.10
  role: worker
  config: |
    node-name: rke2-worker-{{.Index}}
    node-label:
      - "node-role.kubernetes.io/worker=true"
      - "workload=production"
    kubelet-arg:
      - "max-pods=200"
    selinux: true
```

**Why separate workers**: Keep control plane lightweight, scale workload capacity independently

**Use case**: Production clusters with heavy workloads, strict SLAs
**Pros**: Control plane isolation, independent scaling, better resource management
**Cons**: More nodes, higher operational complexity

### FIPS-Compliant Edge Deployment

```yaml
# Control plane with FIPS
cluster:
  cluster_token: K10fips-edge-rke2-token
  control_plane_host: 10.0.1.10
  role: init
  config: |
    fips: true
    selinux: true
    profile: cis-1.23
    secrets-encryption: true

# FIPS workers
cluster:
  cluster_token: K10fips-edge-rke2-token
  control_plane_host: 10.0.1.10
  role: worker
  config: |
    node-label:
      - "compliance=fips"
      - "security=high"
    node-taint:
      - "fips=required:NoExecute"
    selinux: true
```

**Use case**: Government, defense, regulated industries requiring FIPS 140-2
**Pros**: Meets compliance requirements, enhanced security
**Cons**: Performance overhead, requires FIPS-validated OS

## Common Pitfalls

### 1. Multiple Init Nodes

**Problem**: Only ONE init node per cluster. Multiple init nodes create separate clusters.

```yaml
# BAD - Node 1
role: init

# BAD - Node 2 (creates second cluster!)
role: init

# GOOD - Node 1
role: init

# GOOD - Node 2
role: controlplane
```

### 2. Worker Node Trying to Init

**Problem**: Workers can't bootstrap etcd or run control plane.

```yaml
# BAD
cluster:
  role: worker
  config: |
    # Worker can't bootstrap - will fail
```

### 3. ControlPlane Without Init

**Problem**: ControlPlane nodes need existing cluster to join.

```bash
# Deploy sequence:
# WRONG: Deploy controlplane first
kubectl apply -f controlplane-node.yaml  # Fails - no cluster yet

# RIGHT: Deploy init first, then controlplane
kubectl apply -f init-node.yaml          # Bootstraps cluster
# Wait for cluster ready
kubectl apply -f controlplane-node.yaml  # Joins successfully
```

### 4. Inconsistent cluster_token

**Problem**: All nodes must use same cluster_token.

```yaml
# Init node
cluster_token: K10token-abc123

# ControlPlane node - BAD
cluster_token: K10token-xyz789  # Different token - won't join!

# Worker node - GOOD
cluster_token: K10token-abc123  # Same token - joins successfully
```

### 5. Wrong control_plane_host for Workers

**Problem**: Workers must point to actual control plane IP, not their own IP.

```yaml
# Control plane
cluster:
  control_plane_host: 10.0.1.10
  role: init

# Worker - BAD
cluster:
  control_plane_host: 10.0.1.50  # Worker's own IP - wrong!
  role: worker

# Worker - GOOD
cluster:
  control_plane_host: 10.0.1.10  # Control plane IP - correct!
  role: worker
```

### 6. Port Confusion (9345 vs 6443)

**Problem**: Registration uses 9345, API uses 6443.

```yaml
# Registration (automatic)
cluster:
  control_plane_host: 10.0.1.10  # Provider uses :9345

# API access (after cluster ready)
# kubectl --server=https://10.0.1.10:6443 get nodes
```

### 7. etcd Quorum Loss in HA

**Problem**: Need (n/2)+1 nodes for quorum. Losing majority breaks cluster.

```
3-node cluster: Tolerates 1 failure (2/3 = quorum)
5-node cluster: Tolerates 2 failures (3/5 = quorum)
```

**Avoid**:
- Even-numbered control plane (4 nodes = same fault tolerance as 3)
- Deploying control planes without network redundancy
- Not monitoring etcd health

### 8. Role Confusion in Config

**Problem**: Setting wrong role for node's intended purpose.

```yaml
# Node should be worker but configured as controlplane
# Results in unexpected etcd/API server running
cluster:
  role: controlplane  # Should be: worker
  config: |
    node-label:
      - "node-role.kubernetes.io/worker=true"  # Contradicts role!
```

## Role-Specific Configuration

### Init Node Best Practices

```yaml
cluster:
  role: init
  config: |
    # Required: TLS SANs for all control plane IPs
    tls-san:
      - 10.0.1.10
      - 10.0.1.11
      - 10.0.1.12
      - api.rke2.example.com

    # Recommended: Security hardening
    selinux: true
    profile: cis-1.23
    secrets-encryption: true

    # Recommended: Kubeconfig access
    write-kubeconfig-mode: "0644"

    # Optional: Disable unneeded components
    disable:
      - rke2-ingress-nginx

    # Optional: Custom network CIDRs
    cluster-cidr: 10.42.0.0/16
    service-cidr: 10.43.0.0/16
```

### ControlPlane Node Best Practices

```yaml
cluster:
  role: controlplane
  config: |
    # Required: Must match init node TLS SANs
    tls-san:
      - 10.0.1.10
      - 10.0.1.11
      - 10.0.1.12
      - api.rke2.example.com

    # Recommended: Security hardening
    selinux: true

    # Recommended: Kubeconfig access
    write-kubeconfig-mode: "0644"

    # Note: cluster-cidr and service-cidr inherited from init node
```

### Worker Node Best Practices

```yaml
cluster:
  role: worker
  config: |
    # Recommended: Custom node name
    node-name: rke2-worker-{{.SiteID}}-{{.Index}}

    # Recommended: Node labels for scheduling
    node-label:
      - "node-role.kubernetes.io/worker=true"
      - "workload=general"
      - "region={{.Region}}"
      - "compliance=cis"

    # Optional: Node taints for dedicated workloads
    node-taint:
      - "workload=ml:NoSchedule"

    # Optional: Kubelet tuning
    kubelet-arg:
      - "max-pods=110"
      - "eviction-hard=memory.available<500Mi"

    # Recommended: Security
    selinux: true
```

## Multi-Node Cluster Setup Patterns

### Pattern 1: Rolling Deployment

```bash
# Step 1: Deploy init node
deploy init-node.yaml
wait_for_api_ready

# Step 2: Deploy controlplane nodes (one at a time)
deploy controlplane-1.yaml
wait_for_node_ready
deploy controlplane-2.yaml
wait_for_node_ready

# Step 3: Deploy workers (parallel or sequential)
deploy worker-1.yaml
deploy worker-2.yaml
deploy worker-3.yaml
```

### Pattern 2: Phased Deployment

```bash
# Phase 1: Bootstrap control plane HA
deploy init-node.yaml
deploy controlplane-1.yaml
deploy controlplane-2.yaml
wait_for_cluster_stable

# Phase 2: Add workers in batches
for batch in $(seq 1 10); do
  deploy worker-batch-${batch}.yaml
  wait_for_batch_ready
done
```

### Pattern 3: Site-Local Deployment

```bash
# Each edge site gets own cluster
for site in site-a site-b site-c; do
  deploy ${site}-init.yaml
  wait_for_api_ready

  for worker in $(seq 1 5); do
    deploy ${site}-worker-${worker}.yaml
  done
done
```

## Integration Points

### Stylus Integration

**Role Assignment**: Stylus determines node role based on device profile:

```go
// Stylus edge controller logic (conceptual)
func assignRole(device Device) string {
    if device.IsFirstInSite() {
        return "init"
    } else if device.HasControlPlaneCapability() && needsHA() {
        return "controlplane"
    } else {
        return "worker"
    }
}
```

**Dynamic Role Configuration**:
```yaml
# Stylus-generated config template
cluster:
  cluster_token: "{{.ClusterToken}}"
  control_plane_host: "{{.ControlPlaneIP}}"
  role: "{{.AssignedRole}}"  # Dynamically determined
  config: |
    node-name: "{{.DeviceID}}"
    selinux: true
    profile: cis-1.23
```

### Kairos Integration

**Service Management** (/Users/rishi/work/src/provider-rke2/pkg/provider/provider.go):

```go
// Init and ControlPlane roles
systemName := "rke2-server"  // Server service

// Worker role
systemName := "rke2-agent"  // Agent service

// Service enable/start stages
yip.Stage{
    Name: "Enable Systemd Services",
    If:   "[ -x /bin/systemctl ]",
    Commands: []string{
        fmt.Sprintf("systemctl enable %s", systemName),
        fmt.Sprintf("systemctl restart %s", systemName),
    },
}
```

## Related Skills

- **01-architecture.md**: Overall provider architecture and components
- **02-configuration-patterns.md**: Configuration examples for each role
- **04-security.md**: FIPS, CIS, SELinux security features
- **05-networking.md**: Canal CNI and NetworkPolicy

## Documentation References

- **Provider Logic**: `/Users/rishi/work/src/provider-rke2/pkg/provider/provider.go`
- **Role Handling**: Role-specific logic in parseOptions function
- **RKE2 Server Docs**: https://docs.rke2.io/architecture#server-nodes
- **RKE2 Agent Docs**: https://docs.rke2.io/architecture#agent-worker-nodes
- **RKE2 HA Setup**: https://docs.rke2.io/install/ha
