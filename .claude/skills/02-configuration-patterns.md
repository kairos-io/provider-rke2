---
skill: RKE2 Provider Configuration Patterns
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

# RKE2 Provider Configuration Patterns

## Overview

This guide covers configuration patterns for provider-rke2, including cloud-init structure, custom configuration injection, network settings, security hardening, and advanced scenarios for production enterprise edge deployments.

## Key Concepts

### Cloud-Init Cluster Section

Provider-rke2 reads configuration from the `cluster` section of cloud-init:

```yaml
cluster:
  cluster_token: string          # Required: Shared secret for cluster membership
  control_plane_host: string     # Required: Control plane IP/hostname
  role: init|controlplane|worker # Required: Node role
  config: |                      # Optional: RKE2-specific YAML configuration
    key: value
  env:                           # Optional: Environment variables (proxy, etc.)
    HTTP_PROXY: string
  import_local_images: bool      # Optional: Import container images on boot
  local_images_path: string      # Optional: Path to local images
  provider_options:              # Optional: Provider-specific overrides
    key: value
```

### Configuration File Hierarchy

RKE2 configuration is merged from multiple sources in order:

1. **Provider Defaults** (lowest priority)
2. **User config** (90_userdata.yaml - from cluster.config)
3. **Provider overrides** (99_userdata.yaml - from provider logic)
4. **Merged final** (/etc/rancher/rke2/config.yaml)

### RKE2 Configuration Format

RKE2 uses YAML configuration compatible with CLI flags:

```yaml
# Server options (control plane)
server: https://control-plane:9345
token: secrettoken
tls-san:
  - hostname.example.com
write-kubeconfig-mode: "0644"
node-name: my-node

# Network options
cluster-cidr: 10.42.0.0/16
service-cidr: 10.43.0.0/16
cluster-dns: 10.43.0.10

# Security options
selinux: true
profile: cis-1.23
fips: true
secrets-encryption: true

# Agent options (worker)
node-label:
  - "region=us-west"
  - "environment=production"
kubelet-arg:
  - "max-pods=50"
```

## Implementation Patterns

### Basic Init Node Configuration

```yaml
#cloud-config
hostname: rke2-control-plane-01

cluster:
  cluster_token: K10f3e1a2b3c4d5e6f7g8h9i0j1k2l3m4
  control_plane_host: 192.168.1.100
  role: init
  config: |
    write-kubeconfig-mode: "0644"
    disable:
      - rke2-ingress-nginx  # Disable default ingress controller
    tls-san:
      - rke2-api.example.com
      - 192.168.1.100
    selinux: true
    profile: cis-1.23
```

**What happens**:
1. Provider configures RKE2 server mode
2. RKE2 bootstraps embedded etcd cluster
3. TLS certificates include control_plane_host + user tls-san entries
4. Kubeconfig written to /etc/rancher/rke2/rke2.yaml (mode 0644)
5. CIS hardening profile applied
6. SELinux policies enforced

### Control Plane Join Configuration

```yaml
#cloud-config
hostname: rke2-control-plane-02

cluster:
  cluster_token: K10f3e1a2b3c4d5e6f7g8h9i0j1k2l3m4
  control_plane_host: 192.168.1.100
  role: controlplane
  config: |
    write-kubeconfig-mode: "0644"
    tls-san:
      - rke2-api.example.com
      - 192.168.1.100
      - 192.168.1.101
    selinux: true
```

**What happens**:
1. Provider sets `server: https://192.168.1.100:9345`
2. Node joins existing etcd cluster using cluster_token
3. Becomes full control plane with API server, scheduler, controller
4. Shares etcd data with other control plane nodes

### Worker Node Configuration

```yaml
#cloud-config
hostname: rke2-worker-01

cluster:
  cluster_token: K10f3e1a2b3c4d5e6f7g8h9i0j1k2l3m4
  control_plane_host: 192.168.1.100
  role: worker
  config: |
    node-name: edge-worker-site-a-01
    node-label:
      - "site=site-a"
      - "zone=az1"
      - "workload=general"
      - "security=high"
    kubelet-arg:
      - "max-pods=110"
      - "eviction-hard=memory.available<500Mi"
    selinux: true
```

**What happens**:
1. Provider configures rke2-agent service (not rke2-server)
2. Kubelet connects to control plane API at 192.168.1.100:9345 (registration)
3. After registration, uses API at 192.168.1.100:6443
4. Node registers with custom name and labels
5. Kubelet uses custom max-pods and eviction settings
6. SELinux enforcement enabled

### Custom Network Configuration

```yaml
#cloud-config
cluster:
  cluster_token: K10network-custom-rke2-token
  control_plane_host: 10.0.0.10
  role: init
  config: |
    cluster-cidr: 10.52.0.0/16       # Pod network
    service-cidr: 10.53.0.0/16       # Service network
    cluster-dns: 10.53.0.10          # CoreDNS ClusterIP
    cni:
      - canal                         # Explicit Canal CNI
```

**Use case**: Custom network ranges to avoid conflicts with existing infrastructure.

**Important**: If using proxy, ensure these CIDRs are in NO_PROXY.

### FIPS 140-2 Compliance Mode

```yaml
#cloud-config
cluster:
  cluster_token: K10fips-compliant-rke2-token
  control_plane_host: 192.168.1.100
  role: init
  config: |
    fips: true                        # Enable FIPS 140-2 mode
    write-kubeconfig-mode: "0600"     # Stricter permissions
    tls-min-version: "1.3"            # Enforce TLS 1.3+
    tls-cipher-suites:
      - TLS_AES_256_GCM_SHA384
      - TLS_AES_128_GCM_SHA256
    secrets-encryption: true          # Encrypt secrets at rest
```

**Use case**: Government, regulated industries, defense contractors.

**Important**:
- FIPS mode requires FIPS-compliant OS (RHEL, Rocky Linux)
- Container images must use FIPS-compliant crypto libraries
- Performance overhead (10-15% typical)

### CIS Kubernetes Benchmark Hardening

```yaml
#cloud-config
cluster:
  cluster_token: K10cis-hardened-rke2-token
  control_plane_host: 192.168.1.100
  role: init
  config: |
    profile: cis-1.23                 # Apply CIS 1.23 profile
    selinux: true                     # Enable SELinux
    protect-kernel-defaults: true     # Kubelet kernel tuning
    audit-policy-file: /etc/rancher/rke2/audit-policy.yaml
    kube-apiserver-arg:
      - "audit-log-path=/var/log/kube-audit.log"
      - "audit-log-maxage=30"
      - "audit-log-maxbackup=10"
      - "audit-log-maxsize=100"

write_files:
  - path: /etc/rancher/rke2/audit-policy.yaml
    permissions: "0600"
    content: |
      apiVersion: audit.k8s.io/v1
      kind: Policy
      rules:
      - level: RequestResponse
        resources:
        - group: ""
          resources: ["secrets", "configmaps"]
```

**Use case**: Meeting CIS Kubernetes Benchmark compliance for audits.

### Pod Security Standards Configuration

```yaml
#cloud-config
cluster:
  cluster_token: K10pss-enforced-rke2-token
  control_plane_host: 192.168.1.100
  role: init
  config: |
    pod-security-admission-config-file: /etc/rancher/rke2/pss-config.yaml

write_files:
  - path: /etc/rancher/rke2/pss-config.yaml
    permissions: "0600"
    content: |
      apiVersion: apiserver.config.k8s.io/v1
      kind: AdmissionConfiguration
      plugins:
      - name: PodSecurity
        configuration:
          apiVersion: pod-security.admission.config.k8s.io/v1beta1
          kind: PodSecurityConfiguration
          defaults:
            enforce: "restricted"       # Default enforce restricted
            audit: "restricted"
            warn: "restricted"
          exemptions:
            namespaces:
            - kube-system               # Allow privileged in system NS
            - rke2-system
```

**Use case**: Enforce pod security best practices cluster-wide.

### Node Taints and Labels

```yaml
#cloud-config
cluster:
  cluster_token: K10taints-example-rke2-token
  control_plane_host: 192.168.1.100
  role: worker
  config: |
    node-name: gpu-worker-01
    node-label:
      - "gpu=nvidia-a100"
      - "accelerator=true"
      - "workload=ml-training"
      - "compliance=fips"
    node-taint:
      - "gpu=true:NoSchedule"          # Only pods with toleration
      - "fips=required:NoExecute"      # FIPS workloads only
```

**Use case**: Dedicated nodes for specific workloads (GPU, FIPS, high-memory).

### Kubelet Configuration

```yaml
#cloud-config
cluster:
  cluster_token: K10kubelet-tuned-rke2-token
  control_plane_host: 192.168.1.100
  role: worker
  config: |
    kubelet-arg:
      - "max-pods=200"
      - "pod-max-pids=4096"
      - "eviction-hard=memory.available<1Gi,nodefs.available<10%"
      - "eviction-soft=memory.available<2Gi,nodefs.available<15%"
      - "eviction-soft-grace-period=memory.available=2m,nodefs.available=2m"
      - "image-gc-high-threshold=85"
      - "image-gc-low-threshold=80"
      - "system-reserved=cpu=500m,memory=1Gi"
      - "kube-reserved=cpu=500m,memory=1Gi"
    protect-kernel-defaults: true      # CIS requirement
```

**Use case**: Fine-tune kubelet for high-density or resource-constrained nodes.

### Private Registry Configuration

```yaml
#cloud-config
cluster:
  cluster_token: K10registry-example-rke2-token
  control_plane_host: 192.168.1.100
  role: worker
  config: |
    node-name: worker-private-registry

# Containerd registry config (separate from cluster section)
write_files:
  - path: /etc/rancher/rke2/registries.yaml
    permissions: "0644"
    content: |
      mirrors:
        docker.io:
          endpoint:
            - "https://registry.example.com"
        registry.example.com:
          endpoint:
            - "https://registry.example.com"
      configs:
        "registry.example.com":
          auth:
            username: myuser
            password: mypassword
          tls:
            cert_file: /etc/ssl/certs/registry.crt
            key_file: /etc/ssl/certs/registry.key
            ca_file: /etc/ssl/certs/ca.crt
```

**Use case**: Pull images from private registry or mirror public registries.

### Air-Gap Deployment

```yaml
#cloud-config
cluster:
  cluster_token: K10airgap-rke2-deployment-token
  control_plane_host: 192.168.100.10
  role: init
  import_local_images: true
  local_images_path: "/opt/airgap/images"
  config: |
    disable-cloud-controller: true
    node-name: airgap-control-01
    system-default-registry: registry.local:5000

stages:
  boot:
    - name: "Import RKE2 Images"
      commands:
        - "rke2 ctr images import /opt/airgap/rke2-images.tar"
        - "rke2 ctr images import /opt/airgap/app-images.tar"
```

**Use case**: Edge deployments without internet connectivity.

**Prerequisites**:
1. RKE2 images tarball at /opt/airgap/images/
2. Import script at /opt/rke2/scripts/import.sh
3. Provider automatically runs import during boot.before stage

### Proxy Environment Configuration

```yaml
#cloud-config
cluster:
  cluster_token: K10proxy-environment-rke2-token
  control_plane_host: 10.10.1.100
  role: worker
  env:
    HTTP_PROXY: "http://proxy.corp.com:3128"
    HTTPS_PROXY: "http://proxy.corp.com:3128"
    NO_PROXY: "localhost,127.0.0.1,.local,.corp.com,10.0.0.0/8"
  config: |
    node-name: worker-behind-proxy
    cluster-cidr: 10.42.0.0/16
    service-cidr: 10.43.0.0/16
```

**Generated proxy file** (/etc/default/rke2-agent):
```bash
HTTP_PROXY=http://proxy.corp.com:3128
HTTPS_PROXY=http://proxy.corp.com:3128
CONTAINERD_HTTP_PROXY=http://proxy.corp.com:3128
CONTAINERD_HTTPS_PROXY=http://proxy.corp.com:3128
NO_PROXY=10.42.0.0/16,10.43.0.0/16,10.10.1.0/24,.svc,.svc.cluster,.svc.cluster.local,localhost,127.0.0.1,.local,.corp.com,10.0.0.0/8
CONTAINERD_NO_PROXY=10.42.0.0/16,10.43.0.0/16,10.10.1.0/24,.svc,.svc.cluster,.svc.cluster.local,localhost,127.0.0.1,.local,.corp.com,10.0.0.0/8
```

**Provider automatically adds**:
- cluster-cidr
- service-cidr
- Node CIDR (detected from network interface)
- .svc, .svc.cluster, .svc.cluster.local

### Custom Canal CNI Configuration

```yaml
#cloud-config
cluster:
  cluster_token: K10canal-custom-rke2-token
  control_plane_host: 192.168.1.100
  role: init
  config: |
    cni:
      - canal
    kube-proxy-arg:
      - "proxy-mode=ipvs"              # Use IPVS instead of iptables
      - "ipvs-scheduler=rr"            # Round-robin scheduling

write_files:
  - path: /var/lib/rancher/rke2/server/manifests/canal-config.yaml
    permissions: "0600"
    content: |
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: canal-config
        namespace: kube-system
      data:
        canal_iface: "eth1"            # Custom interface for Canal
        masquerade: "true"
        net-conf.json: |
          {
            "Network": "10.42.0.0/16",
            "Backend": {
              "Type": "vxlan",
              "Port": 8472
            }
          }
```

**Use case**: Multi-interface nodes, custom VXLAN port, performance tuning.

### Custom DNS Configuration

```yaml
#cloud-config
cluster:
  cluster_token: K10dns-config-rke2-token
  control_plane_host: 192.168.1.100
  role: init
  config: |
    cluster-dns: 10.43.0.10
    cluster-domain: cluster.local
    resolv-conf: /etc/rke2-resolv.conf  # Custom resolv.conf for kubelet

write_files:
  - path: /etc/rke2-resolv.conf
    permissions: "0644"
    content: |
      nameserver 8.8.8.8
      nameserver 8.8.4.4
      search cluster.local svc.cluster.local corp.example.com
```

**Use case**: Custom DNS resolution for hybrid cloud/on-prem environments.

### Advanced: Multi-Interface Node

```yaml
#cloud-config
cluster:
  cluster_token: K10multi-interface-rke2-token
  control_plane_host: 192.168.1.100
  role: worker
  config: |
    node-ip: 192.168.1.50              # Primary interface for k8s traffic
    node-external-ip: 203.0.113.50     # Public IP for external access
    bind-address: 0.0.0.0              # Listen on all interfaces
```

**Use case**: Nodes with multiple network interfaces (public/private, management/data).

## Common Pitfalls

### 1. Registration Port Confusion

RKE2 uses **port 9345** for registration, but API is on **6443**:

```yaml
# Provider automatically uses 9345 for registration
cluster:
  control_plane_host: 10.0.1.100

# Generated: server: https://10.0.1.100:9345

# After cluster ready, access API:
# kubectl --server=https://10.0.1.100:6443 get nodes
```

### 2. Config Merge Order Confusion

User config (90_userdata.yaml) can be overridden by provider config (99_userdata.yaml):

```yaml
# Use provider_options to override provider settings
cluster:
  provider_options:
    server: ""  # Override provider's server setting
```

### 3. Missing Token Requirements

RKE2 token must be valid format:

```yaml
# BAD - too short
cluster_token: abc

# GOOD - random string 16+ chars
cluster_token: K10abcdef1234567890

# GOOD - bootstrap token format
cluster_token: abcdef.0123456789abcdef
```

### 4. TLS SAN Not Including All Access Methods

If accessing API via multiple hostnames/IPs, all must be in tls-san:

```yaml
cluster:
  control_plane_host: 192.168.1.100  # Automatically added
  config: |
    tls-san:
      - 10.0.1.100                   # VPN IP
      - rke2.example.com             # DNS
      - load-balancer.local          # Load balancer
```

### 5. Proxy Not Excluding Cluster Networks

Provider auto-adds cluster CIDRs, but user should exclude corporate networks:

```yaml
# GOOD - provider adds cluster-cidr, service-cidr, node CIDR
env:
  HTTP_PROXY: http://proxy:3128
  NO_PROXY: localhost,127.0.0.1,.corp.com  # Add corporate domains
```

### 6. FIPS Mode Without FIPS OS

```yaml
# BAD - FIPS enabled on non-FIPS OS
config: |
  fips: true  # Fails on Ubuntu, Debian

# GOOD - FIPS on FIPS-compliant OS
# Use RHEL 8+, Rocky Linux 8+, or other FIPS-validated OS
```

### 7. Canal CNI Conflicts

Canal enforces NetworkPolicy by default. Pods without policies may be unreachable:

```yaml
# Add default allow policy if needed
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-all
  namespace: default
spec:
  podSelector: {}
  ingress:
  - {}
  egress:
  - {}
```

## Integration Points

### Stylus Integration

**Generated Configuration**: Stylus dynamically generates cluster config for edge devices:

```yaml
# Template used by Stylus
cluster:
  cluster_token: "{{.ClusterToken}}"
  control_plane_host: "{{.ControlPlaneEndpoint}}"
  role: "{{.NodeRole}}"
  config: |
    node-name: "{{.DeviceID}}"
    node-label:
      - "stylus.edge/site={{.SiteID}}"
      - "stylus.edge/region={{.Region}}"
      - "stylus.edge/device-type={{.DeviceType}}"
    selinux: true
    profile: cis-1.23
    {{- if .CustomConfig }}
    {{.CustomConfig}}
    {{- end}}
```

**Stylus-Specific Labels**: Device metadata encoded as node labels:

```yaml
node-label:
  - "stylus.edge/site=factory-01"
  - "stylus.edge/region=us-west"
  - "stylus.edge/device-type=gateway"
  - "stylus.edge/firmware-version=2.1.0"
  - "stylus.edge/security=fips"
```

### Kairos Integration

**Provider Options**: Kairos-specific options via provider_options:

```yaml
cluster:
  provider_options:
    cluster-root-path: "/run/initramfs/live"  # Custom root path
```

**File Structure** (/Users/rishi/work/src/provider-rke2/pkg/provider/provider.go):
```go
func parseFiles(cluster clusterplugin.Cluster, systemName string) []yip.File {
    options, proxyOptions, userOptions := parseOptions(cluster)

    files := []yip.File{
        {
            Path:        filepath.Join(configurationPath, "90_userdata.yaml"),
            Permissions: 0400,
            Content:     string(userOptions),  // From cluster.config
        },
        {
            Path:        filepath.Join(configurationPath, "99_userdata.yaml"),
            Permissions: 0400,
            Content:     string(options),      // Provider-generated
        },
    }
    // ... proxy files
}
```

## Reference Examples

### Production Enterprise Edge Deployment

```yaml
#cloud-config
# Production-grade enterprise edge node configuration
hostname: rke2-edge-{{.SiteID}}-{{.NodeIndex}}

users:
  - name: admin
    groups: [sudo]
    ssh_authorized_keys:
      - ssh-rsa AAAA...

cluster:
  cluster_token: K10prod-enterprise-edge-secure-token
  control_plane_host: 10.0.1.10
  role: worker
  env:
    HTTP_PROXY: "http://proxy.corp:3128"
    HTTPS_PROXY: "http://proxy.corp:3128"
  config: |
    node-name: rke2-edge-site-{{.SiteID}}-{{.NodeIndex}}
    node-label:
      - "site={{.SiteID}}"
      - "region={{.Region}}"
      - "environment=production"
      - "workload=edge-inference"
      - "compliance=cis"
    node-taint:
      - "edge=true:NoSchedule"
    kubelet-arg:
      - "max-pods=50"
      - "eviction-hard=memory.available<256Mi,nodefs.available<10%"
      - "system-reserved=cpu=500m,memory=512Mi"
      - "kube-reserved=cpu=500m,memory=512Mi"
      - "protect-kernel-defaults=true"
    selinux: true
    profile: cis-1.23

write_files:
  - path: /etc/rancher/rke2/registries.yaml
    content: |
      mirrors:
        docker.io:
          endpoint:
            - "https://registry.{{.Region}}.corp.com"

stages:
  boot:
    - name: "Configure monitoring"
      commands:
        - "systemctl enable node-exporter"
        - "systemctl start node-exporter"
```

### Configuration File Locations

- **User Config**: `/etc/rancher/rke2/config.d/90_userdata.yaml`
- **Provider Config**: `/etc/rancher/rke2/config.d/99_userdata.yaml`
- **Merged Config**: `/etc/rancher/rke2/config.yaml`
- **Proxy Config**: `/etc/default/rke2-server` or `/etc/default/rke2-agent`
- **Registry Config**: `/etc/rancher/rke2/registries.yaml`
- **Kubeconfig**: `/etc/rancher/rke2/rke2.yaml`
- **Audit Logs**: `/var/log/kube-audit.log` (if audit logging enabled)

## Related Skills

- **01-architecture.md**: Provider architecture and component overview
- **03-cluster-roles.md**: Role-specific configurations (init, controlplane, worker)
- **04-security.md**: FIPS, CIS, SELinux, and Pod Security Standards
- **05-networking.md**: Canal CNI configuration and NetworkPolicy
- **06-proxy-configuration.md**: HTTP/HTTPS proxy setup

## Documentation References

- **Provider Code**: `/Users/rishi/work/src/provider-rke2/pkg/provider/provider.go`
- **Configuration Types**: `/Users/rishi/work/src/provider-rke2/pkg/types/rke2.go`
- **RKE2 Server Docs**: https://docs.rke2.io/reference/server_config
- **RKE2 Agent Docs**: https://docs.rke2.io/reference/linux_agent_config
- **RKE2 Networking**: https://docs.rke2.io/networking/basic_network_options
- **RKE2 Security**: https://docs.rke2.io/security/hardening_guide
