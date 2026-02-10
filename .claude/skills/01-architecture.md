---
skill: RKE2 Provider Architecture
description: Skill documentation for provider-rke2
type: general
repository: provider-rke2
team: edge
topics: [kubernetes, provider, edge, cluster]
difficulty: intermediate
last_updated: 2026-02-09
related_skills: []
memory_references: []
---

# RKE2 Provider Architecture

## Overview

The provider-rke2 is a Kairos/C3OS cluster provider that configures RKE2 (Rancher Kubernetes Engine 2) installations for enterprise edge deployments. RKE2 is a security-focused, enterprise-grade Kubernetes distribution that combines Kubernetes security best practices with an edge-optimized architecture, making it ideal for production edge computing, government/regulated industries, and environments requiring FIPS 140-2 or CIS Kubernetes Benchmark compliance.

## Key Concepts

### Kubernetes Distribution: RKE2

RKE2 is Rancher's next-generation Kubernetes distribution built for security and compliance:

- **Security-Focused**: FIPS 140-2 compliant, CIS Kubernetes Benchmark certified
- **Enterprise-Grade**: Production-ready with enhanced security defaults
- **SELinux Support**: Built-in SELinux policies for hardened environments
- **Conformant**: Fully certified Kubernetes with CNCF certification
- **Embedded Components**: Includes containerd, CoreDNS, Canal CNI, and Ingress-NGINX by default
- **Edge-Optimized**: Designed for distributed edge deployments with central management
- **Pod Security Standards**: PSS (Pod Security Standards) enforced by default

### RKE2 vs K3s

| Feature | K3s | RKE2 |
|---------|-----|------|
| **Target Use Case** | Edge, IoT, dev/test | Enterprise edge, production |
| **Binary Size** | ~100MB | ~150MB |
| **CNI** | Flannel | Canal (Calico + Flannel) |
| **NetworkPolicy** | Not by default | Yes (via Calico) |
| **Security Certifications** | General | FIPS 140-2, CIS certified |
| **SELinux** | Optional | Built-in policies |
| **Complexity** | Lower | Higher |
| **Registration Port** | 6443 | 9345 |

### Kairos/C3OS Integration

Provider-rke2 integrates with Kairos immutable Linux distribution:

- **Cloud-init Configuration**: Declarative cluster setup via cluster section
- **Immutable OS**: A/B partition updates with atomic upgrades
- **Boot Stages**: Yip-based stage execution during boot.before phase
- **Service Management**: OpenRC and systemd service orchestration
- **Image Import**: Local container image preloading for air-gap deployments

### Component Architecture

```
┌─────────────────────────────────────────────────────┐
│              Cloud-Init (User Configuration)         │
│  cluster:                                           │
│    cluster_token: token123                         │
│    control_plane_host: 10.0.1.100                  │
│    role: init|controlplane|worker                  │
│    config: |                                       │
│      node-name: edge-node-1                        │
└─────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────┐
│         Provider-RKE2 (Cluster Plugin)              │
│  • Parse cluster configuration                      │
│  • Generate RKE2 config files                       │
│  • Configure proxy settings                         │
│  • Manage TLS SANs                                  │
│  • Handle role-specific setup                       │
└─────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────┐
│               Yip Stage Execution                   │
│  boot.before:                                       │
│    1. Disable swap                                  │
│    2. Install config files                          │
│    3. Import local images (optional)                │
│    4. Enable rke2-server/rke2-agent service         │
│    5. Start RKE2                                    │
└─────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────┐
│                RKE2 Installation                    │
│  ┌───────────────┐  ┌────────────────┐             │
│  │ RKE2 Server   │  │  RKE2 Agent    │             │
│  │ (Control      │  │  (Worker)      │             │
│  │  Plane)       │  │                │             │
│  │               │  │                │             │
│  │ • API Server  │  │ • Kubelet      │             │
│  │ • Scheduler   │  │ • Kube-proxy   │             │
│  │ • Controller  │  │ • Container    │             │
│  │ • etcd        │  │   Runtime      │             │
│  │ • Canal CNI   │  │ • Canal CNI    │             │
│  └───────────────┘  └────────────────┘             │
└─────────────────────────────────────────────────────┘
```

### Configuration Flow

1. **Cluster Definition**: User defines cluster configuration in cloud-init
2. **Provider Execution**: Provider-rke2 processes configuration via ClusterProvider()
3. **Config Generation**: Creates RKE2 YAML configs in /etc/rancher/rke2/config.d/
4. **Config Merging**: jq merges multiple config files into /etc/rancher/rke2/config.yaml
5. **Service Start**: systemd/OpenRC starts rke2-server or rke2-agent service
6. **Cluster Join**: Node joins cluster using cluster_token and control_plane_host (port 9345)

### File Structure

```
/etc/rancher/rke2/
├── config.d/
│   ├── 90_userdata.yaml          # User-provided configuration
│   └── 99_userdata.yaml          # Provider-generated configuration
├── config.yaml                    # Merged final configuration
└── ...

/etc/default/
├── rke2-server                    # Server proxy environment variables
└── rke2-agent                     # Agent proxy environment variables

/var/log/
├── rke2-import-images.log        # Image import logs (if enabled)
└── ...
```

## Implementation Patterns

### Role-Based Configuration

The provider handles three distinct roles with different configurations:

**Init Role (Bootstrap First Node)**:
```go
// pkg/provider/provider.go
case clusterplugin.RoleInit:
    rke2Config.Server = ""  // No upstream server - this IS the server
    rke2Config.TLSSan = []string{cluster.ControlPlaneHost}
    // Initializes embedded etcd cluster
```

**ControlPlane Role (Additional Control Plane)**:
```go
case clusterplugin.RoleControlPlane:
    rke2Config.Server = fmt.Sprintf("https://%s:9345", cluster.ControlPlaneHost)
    rke2Config.TLSSan = []string{cluster.ControlPlaneHost}
    // Joins existing etcd cluster
```

**Worker Role (Agent Only)**:
```go
case clusterplugin.RoleWorker:
    systemName = "rke2-agent"
    rke2Config.Server = fmt.Sprintf("https://%s:9345", cluster.ControlPlaneHost)
    // Runs kubelet and container runtime only
```

**Key Difference from K3s**: RKE2 uses port **9345** for registration (K3s uses 6443). The API server still listens on 6443 after cluster is bootstrapped.

### Configuration File Generation

```go
// pkg/provider/provider.go:parseFiles()
files := []yip.File{
    {
        Path:        "/etc/rancher/rke2/config.d/90_userdata.yaml",
        Permissions: 0400,
        Content:     string(userOptions),  // User YAML config
    },
    {
        Path:        "/etc/rancher/rke2/config.d/99_userdata.yaml",
        Permissions: 0400,
        Content:     string(options),       // Provider-generated config
    },
}
```

Configuration merging happens via jq during boot:
```bash
jq -s 'def flatten: reduce .[] as $i([]; if $i | type == "array" then . + ($i | flatten) else . + [$i] end); [.[] | to_entries] | flatten | reduce .[] as $dot ({}; .[$dot.key] += $dot.value)' /etc/rancher/rke2/config.d/*.yaml > /etc/rancher/rke2/config.yaml
```

### Proxy Configuration

Provider-rke2 handles HTTP/HTTPS proxy with automatic NO_PROXY calculation:

```go
// pkg/provider/provider.go:proxyEnv()
func proxyEnv(proxyOptions []byte, proxyMap map[string]string) string {
    // Extracts cluster-cidr and service-cidr from user config
    // Adds node CIDR automatically via getNodeCIDR()
    // Appends .svc,.svc.cluster,.svc.cluster.local
    // Generates /etc/default/rke2-server or /etc/default/rke2-agent
}
```

Example generated proxy file:
```bash
HTTP_PROXY=http://proxy.example.com:8080
HTTPS_PROXY=http://proxy.example.com:8080
CONTAINERD_HTTP_PROXY=http://proxy.example.com:8080
CONTAINERD_HTTPS_PROXY=http://proxy.example.com:8080
NO_PROXY=10.42.0.0/16,10.43.0.0/16,192.168.1.0/24,.svc,.svc.cluster,.svc.cluster.local
CONTAINERD_NO_PROXY=10.42.0.0/16,10.43.0.0/16,192.168.1.0/24,.svc,.svc.cluster,.svc.cluster.local
```

### Image Import for Air-Gap

```go
// pkg/provider/provider.go:parseStages()
if cluster.ImportLocalImages {
    if cluster.LocalImagesPath == "" {
        cluster.LocalImagesPath = "/opt/content/images"
    }
    importStage := yip.Stage{
        Name: constants.ImportRKE2Images,
        Commands: []string{
            "chmod +x /opt/rke2/scripts/import.sh",
            "/bin/sh /opt/rke2/scripts/import.sh /opt/content/images > /var/log/rke2-import-images.log",
        },
    }
}
```

## Security Features

### FIPS 140-2 Compliance

RKE2 supports FIPS 140-2 mode for cryptographic operations:

```yaml
cluster:
  config: |
    fips: true  # Enable FIPS 140-2 mode
```

When enabled:
- Uses FIPS-validated cryptographic libraries
- Enforces secure cipher suites
- Required for government/regulated workloads

### CIS Kubernetes Benchmark

RKE2 is hardened to meet CIS Kubernetes Benchmark standards:

- Minimal privilege configurations
- Restricted service account permissions
- Network policy defaults
- Audit logging enabled
- Pod Security Standards enforced

```yaml
cluster:
  config: |
    profile: cis-1.23  # Apply CIS profile
```

### SELinux Integration

Built-in SELinux policies for hardened Linux environments:

```yaml
cluster:
  config: |
    selinux: true  # Enable SELinux enforcement
```

### Pod Security Standards

RKE2 enforces Pod Security Standards by default:

```yaml
cluster:
  config: |
    pod-security-admission-config-file: /etc/rancher/rke2/pss.yaml
```

Default PSS configuration:
- Privileged: Allow all (system namespaces only)
- Baseline: Enforce (default for user namespaces)
- Restricted: Warn and audit

## Common Pitfalls

### 1. Incorrect Port for Registration

RKE2 uses port **9345** for registration, not 6443:

```yaml
# CORRECT - RKE2 registration
cluster:
  control_plane_host: 10.0.1.100  # Provider adds :9345 automatically

# API server still on 6443 after bootstrap
# Access API: https://10.0.1.100:6443
```

### 2. Cluster Token Format

RKE2 tokens must be at least 16 characters. Short tokens cause authentication failures:

```yaml
# BAD
cluster_token: abc123

# GOOD
cluster_token: K10abcdef1234567890abcdef1234567890
```

### 3. Canal CNI NetworkPolicy Conflicts

Canal (Calico) enforces NetworkPolicy by default. Ensure workloads have proper policies:

```yaml
# Example: Allow ingress to nginx pods
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-nginx
spec:
  podSelector:
    matchLabels:
      app: nginx
  ingress:
  - from: []  # Allow from all
```

### 4. FIPS Mode Compatibility

When FIPS mode is enabled, some container images may not work (non-FIPS crypto):

```yaml
# Test before production
cluster:
  config: |
    fips: true

# Verify workloads are FIPS-compatible
```

### 5. Control Plane Host Mismatch

Workers must use the same control_plane_host as the init node's IP:

```yaml
# Init node config (IP: 10.0.1.100)
cluster:
  role: init
  control_plane_host: 10.0.1.100

# Worker config - MUST match
cluster:
  role: worker
  control_plane_host: 10.0.1.100  # Same IP
```

### 6. TLS SAN Issues

If accessing cluster via hostname/load balancer, add to tls-san:

```yaml
cluster:
  config: |
    tls-san:
      - cluster.example.com
      - 10.0.1.100
      - load-balancer.local
```

### 7. Swap Not Disabled

RKE2 requires swap disabled. Provider handles this, but custom kernel configs may re-enable:

```bash
# Verify swap is off
swapon --show  # Should return empty

# Provider runs these commands:
sed -i '/ swap / s/^\(.*\)$/#\1/g' /etc/fstab
swapoff -a
```

### 8. Port Conflicts

Ensure these ports are available:
- **9345**: RKE2 registration (control plane)
- **6443**: Kubernetes API (all control plane nodes)
- **10250**: Kubelet metrics (all nodes)
- **2379-2380**: etcd (control plane only)
- **8472**: Flannel VXLAN (all nodes - Canal uses Flannel for networking)
- **179**: Calico BGP (if using BGP mode instead of VXLAN)

## Integration Points

### Stylus Integration

**Provider Selection**: Stylus edge agent selects provider-rke2 for enterprise deployments:
- Used when cluster spec specifies rke2 distribution
- Automatically installed in Kairos image with provider-kairos package
- Preferred for production edge sites requiring compliance

**Edge Device Provisioning**:
1. Stylus generates cloud-init with cluster configuration
2. Device boots with Kairos + provider-rke2
3. Provider-rke2 processes cluster section during boot.before stage
4. RKE2 starts and registers with cluster (port 9345)
5. Stylus monitors cluster health via Kubernetes API (port 6443)

**Device Registration**:
```yaml
# Generated by Stylus for edge device
cluster:
  cluster_token: "${STYLUS_CLUSTER_TOKEN}"
  control_plane_host: "${STYLUS_CONTROL_PLANE_IP}"
  role: worker
  config: |
    node-name: "${DEVICE_ID}"
    node-label:
      - "stylus.edge/site=${SITE_ID}"
      - "stylus.edge/region=${REGION}"
    selinux: true  # Enable SELinux for edge security
    profile: cis-1.23  # Apply CIS hardening
```

### Kairos Integration

**Provider Plugin System**:
```go
// main.go
plugin := clusterplugin.ClusterPlugin{
    Provider: provider.ClusterProvider,
}

plugin.Run(
    pluggable.FactoryPlugin{
        EventType:     clusterplugin.EventClusterReset,
        PluginHandler: handleClusterReset,
    },
)
```

**Cluster Reset Event**:
```go
// pkg/provider/reset.go
func handleClusterReset(event *pluggable.Event) pluggable.EventResponse {
    // Runs rke2-uninstall.sh or rke2-agent-uninstall.sh
    // Removes /etc/rancher/rke2
    // Cleans up /var/lib/rancher/rke2
}
```

**Yip Stage Execution**:
```go
// pkg/provider/provider.go:ClusterProvider()
cfg := yip.YipConfig{
    Name: "RKE2 Kairos Cluster Provider",
    Stages: map[string][]yip.Stage{
        "boot.before": parseStages(...),  // Executed before boot
    },
}
```

## Reference Examples

### Basic Single-Node Cluster

```yaml
#cloud-config
# Simple single-node cluster for testing

cluster:
  cluster_token: K10dev-single-node-rke2-token
  control_plane_host: 192.168.1.100
  role: init
  config: |
    write-kubeconfig-mode: "0644"
    node-name: rke2-controller
```

### Multi-Node HA Cluster with Security

```yaml
# Init node (first control plane)
cluster:
  cluster_token: K10ha-secure-rke2-cluster-token
  control_plane_host: 10.0.1.100
  role: init
  config: |
    tls-san:
      - rke2.example.com
      - 10.0.1.100
      - 10.0.1.101
      - 10.0.1.102
    selinux: true
    profile: cis-1.23
    secrets-encryption: true

# Additional control plane nodes
cluster:
  cluster_token: K10ha-secure-rke2-cluster-token
  control_plane_host: 10.0.1.100
  role: controlplane
  config: |
    tls-san:
      - rke2.example.com
      - 10.0.1.100
      - 10.0.1.101
      - 10.0.1.102
    selinux: true

# Worker nodes
cluster:
  cluster_token: K10ha-secure-rke2-cluster-token
  control_plane_host: 10.0.1.100
  role: worker
  config: |
    selinux: true
```

### Air-Gap Deployment with FIPS

```yaml
cluster:
  cluster_token: K10airgap-fips-rke2-token
  control_plane_host: 192.168.100.10
  role: init
  import_local_images: true
  local_images_path: "/opt/content/images"
  config: |
    fips: true
    airgap-extra-registry: registry.local:5000
    selinux: true
```

### Proxy Environment

```yaml
cluster:
  cluster_token: K10proxy-rke2-token
  control_plane_host: 10.10.1.50
  role: worker
  env:
    HTTP_PROXY: "http://proxy.corp.com:3128"
    HTTPS_PROXY: "http://proxy.corp.com:3128"
    NO_PROXY: "localhost,127.0.0.1,.local,.corp.com"
  config: |
    node-name: rke2-worker-01
```

## Networking: Canal CNI

RKE2 includes **Canal CNI** by default, which combines:
- **Flannel**: Provides networking (VXLAN overlay by default)
- **Calico**: Provides NetworkPolicy enforcement

**Benefits**:
- Simple networking like Flannel
- Advanced NetworkPolicy support like Calico
- VXLAN backend (UDP port 8472)
- Optional BGP mode for performance

**Default Network CIDRs**:
```yaml
cluster-cidr: 10.42.0.0/16      # Pod network
service-cidr: 10.43.0.0/16      # Service network
```

**NetworkPolicy Example**:
```yaml
# Canal enforces this automatically
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-all-ingress
spec:
  podSelector: {}
  policyTypes:
  - Ingress
```

## Related Skills

- **02-configuration-patterns.md**: Detailed configuration examples and patterns
- **03-cluster-roles.md**: Deep dive into init, controlplane, and worker roles
- **04-security.md**: FIPS, CIS, SELinux, and Pod Security Standards
- **05-networking.md**: Canal CNI, NetworkPolicy, and VXLAN configuration
- **06-proxy-configuration.md**: HTTP/HTTPS proxy setup and NO_PROXY handling

## Documentation References

- **Provider README**: `/Users/rishi/work/src/provider-rke2/README.md`
- **Main Provider Logic**: `/Users/rishi/work/src/provider-rke2/pkg/provider/provider.go`
- **Configuration Types**: `/Users/rishi/work/src/provider-rke2/pkg/types/rke2.go`
- **Reset Handler**: `/Users/rishi/work/src/provider-rke2/pkg/provider/reset.go`
- **Constants**: `/Users/rishi/work/src/provider-rke2/pkg/constants/constants.go`
- **RKE2 Official Docs**: https://docs.rke2.io/
- **Kairos Documentation**: https://kairos.io/docs/
- **Rancher Documentation**: https://rancher.com/docs/rke2/latest/en/
