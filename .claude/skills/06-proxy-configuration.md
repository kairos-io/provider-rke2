# RKE2 Provider Proxy Configuration

## Overview

Provider-rke2 provides comprehensive HTTP/HTTPS proxy support for enterprise edge deployments behind corporate firewalls. The provider automatically handles proxy configuration for RKE2, containerd, and Kubernetes components, with intelligent NO_PROXY merging to ensure cluster networking functions correctly.

## Key Concepts

### Proxy Environment Variables

Standard proxy environment variables:

```bash
HTTP_PROXY=http://proxy.example.com:8080
HTTPS_PROXY=http://proxy.example.com:8080
NO_PROXY=localhost,127.0.0.1,.local
```

**Applied to**:
- RKE2 server/agent processes
- Containerd (for image pulls)
- Kubernetes components (kubelet, kube-proxy)
- System package managers (for updates)

### Automatic NO_PROXY Merging

Provider-rke2 automatically adds these to NO_PROXY:

1. **Cluster CIDR** (pod network): `10.42.0.0/16` (default)
2. **Service CIDR** (service network): `10.43.0.0/16` (default)
3. **Node CIDR** (auto-detected): Node's network subnet
4. **k8sNoProxy constant**: `.svc,.svc.cluster,.svc.cluster.local`
5. **User NO_PROXY**: Values from user configuration

**Why automatic merging?**
- Prevents proxying internal cluster traffic
- Ensures pod-to-pod communication
- Maintains service discovery
- Avoids DNS resolution issues

### Proxy Configuration Files

Provider-rke2 generates proxy config files:

| Component | Config File | Purpose |
|-----------|------------|---------|
| RKE2 Server | `/etc/default/rke2-server` | Control plane proxy settings |
| RKE2 Agent | `/etc/default/rke2-agent` | Worker node proxy settings |
| Containerd | Embedded in above | Image pull proxy settings |

## Implementation Patterns

### Basic Proxy Configuration

```yaml
#cloud-config
cluster:
  cluster_token: K10basic-proxy-cluster-token
  control_plane_host: 10.0.1.10
  role: worker
  env:
    HTTP_PROXY: "http://proxy.corp.com:3128"
    HTTPS_PROXY: "http://proxy.corp.com:3128"
    NO_PROXY: "localhost,127.0.0.1,.local"
  config: |
    node-name: worker-01
```

**Generated file** (`/etc/default/rke2-agent`):
```bash
HTTP_PROXY=http://proxy.corp.com:3128
HTTPS_PROXY=http://proxy.corp.com:3128
CONTAINERD_HTTP_PROXY=http://proxy.corp.com:3128
CONTAINERD_HTTPS_PROXY=http://proxy.corp.com:3128
NO_PROXY=10.42.0.0/16,10.43.0.0/16,10.0.1.0/24,.svc,.svc.cluster,.svc.cluster.local,localhost,127.0.0.1,.local
CONTAINERD_NO_PROXY=10.42.0.0/16,10.43.0.0/16,10.0.1.0/24,.svc,.svc.cluster,.svc.cluster.local,localhost,127.0.0.1,.local
```

**Provider added automatically**:
- `10.42.0.0/16` - Cluster CIDR (pods)
- `10.43.0.0/16` - Service CIDR (services)
- `10.0.1.0/24` - Node CIDR (auto-detected)
- `.svc,.svc.cluster,.svc.cluster.local` - k8sNoProxy constant

### Authenticated Proxy

```yaml
#cloud-config
cluster:
  cluster_token: K10auth-proxy-cluster-token
  control_plane_host: 10.0.1.10
  role: worker
  env:
    HTTP_PROXY: "http://proxyuser:proxypass@proxy.corp.com:3128"
    HTTPS_PROXY: "http://proxyuser:proxypass@proxy.corp.com:3128"
    NO_PROXY: "localhost,127.0.0.1"
  config: |
    node-name: worker-auth-proxy-01
```

**Security note**: Credentials visible in systemd environment. For production, consider:
- Transparent proxy (no auth required from nodes)
- Certificate-based proxy authentication
- Network-based authentication (IP allowlisting)

### HTTPS Proxy with Custom CA

```yaml
#cloud-config
cluster:
  cluster_token: K10https-proxy-ca-cluster-token
  control_plane_host: 10.0.1.10
  role: worker
  env:
    HTTP_PROXY: "http://proxy.corp.com:3128"
    HTTPS_PROXY: "https://secure-proxy.corp.com:3129"
    NO_PROXY: "localhost,127.0.0.1,.corp.com"
  config: |
    node-name: worker-https-proxy-01

write_files:
  - path: /etc/pki/ca-trust/source/anchors/proxy-ca.crt
    permissions: "0644"
    content: |
      -----BEGIN CERTIFICATE-----
      MIIDXTCCAkWgAwIBAgIJAKZ...
      -----END CERTIFICATE-----

stages:
  boot:
    - name: "Update CA trust"
      commands:
        - "update-ca-trust extract"
```

### Proxy with Custom Network CIDRs

```yaml
#cloud-config
cluster:
  cluster_token: K10custom-cidr-proxy-cluster-token
  control_plane_host: 10.0.1.10
  role: init
  env:
    HTTP_PROXY: "http://proxy.corp.com:3128"
    HTTPS_PROXY: "http://proxy.corp.com:3128"
    NO_PROXY: "localhost,127.0.0.1,.corp.com,10.0.0.0/8"
  config: |
    cluster-cidr: 10.52.0.0/16       # Custom pod CIDR
    service-cidr: 10.53.0.0/16       # Custom service CIDR
```

**Generated NO_PROXY**:
```
10.52.0.0/16,10.53.0.0/16,10.0.1.0/24,.svc,.svc.cluster,.svc.cluster.local,localhost,127.0.0.1,.corp.com,10.0.0.0/8
```

Provider automatically detects custom CIDRs and includes them.

### Multi-Interface Proxy Configuration

```yaml
#cloud-config
cluster:
  cluster_token: K10multi-interface-proxy-token
  control_plane_host: 10.0.1.10
  role: worker
  env:
    HTTP_PROXY: "http://proxy.corp.com:3128"
    HTTPS_PROXY: "http://proxy.corp.com:3128"
    NO_PROXY: "localhost,127.0.0.1,.corp.com,192.168.0.0/16"
  config: |
    node-ip: 192.168.1.50            # Primary interface
    node-external-ip: 203.0.113.50   # Public interface (via proxy)
```

**Generated NO_PROXY includes**:
- Node CIDR from primary interface (`192.168.1.0/24`)
- User-specified ranges (`192.168.0.0/16`)
- Cluster and service CIDRs

### No Proxy Configuration

If no proxy is configured, provider-rke2 does not create proxy files:

```yaml
#cloud-config
cluster:
  cluster_token: K10no-proxy-cluster-token
  control_plane_host: 10.0.1.10
  role: worker
  # No env section = no proxy
  config: |
    node-name: worker-no-proxy-01
```

**Result**: No `/etc/default/rke2-agent` file created.

## Advanced Patterns

### Proxy Bypass for Internal Domains

```yaml
#cloud-config
cluster:
  cluster_token: K10proxy-bypass-cluster-token
  control_plane_host: 10.0.1.10
  role: worker
  env:
    HTTP_PROXY: "http://proxy.corp.com:3128"
    HTTPS_PROXY: "http://proxy.corp.com:3128"
    NO_PROXY: "localhost,127.0.0.1,.corp.com,.internal,.local,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"
  config: |
    node-name: worker-bypass-01
```

**Common bypass patterns**:
- `.corp.com` - Corporate domain
- `.internal` - Internal services
- `.local` - Local domain
- `10.0.0.0/8` - Private network (Class A)
- `172.16.0.0/12` - Private network (Class B)
- `192.168.0.0/16` - Private network (Class C)

### Proxy for Air-Gap Image Sync

```yaml
#cloud-config
cluster:
  cluster_token: K10airgap-sync-proxy-token
  control_plane_host: 10.0.1.10
  role: init
  env:
    HTTP_PROXY: "http://proxy.corp.com:3128"
    HTTPS_PROXY: "http://proxy.corp.com:3128"
  config: |
    system-default-registry: registry.corp.com:5000

write_files:
  - path: /etc/rancher/rke2/registries.yaml
    permissions: "0644"
    content: |
      mirrors:
        docker.io:
          endpoint:
            - "https://registry.corp.com:5000"
      configs:
        "registry.corp.com:5000":
          tls:
            insecure_skip_verify: false
```

**Use case**: Use proxy to sync images to internal registry, then deploy air-gapped.

### Conditional Proxy (Development vs Production)

```yaml
#cloud-config
# Template for environment-specific deployments

cluster:
  cluster_token: K10conditional-proxy-token
  control_plane_host: 10.0.1.10
  role: worker
  {{- if eq .Environment "production" }}
  env:
    HTTP_PROXY: "http://prod-proxy.corp.com:3128"
    HTTPS_PROXY: "http://prod-proxy.corp.com:3128"
    NO_PROXY: "localhost,127.0.0.1,.corp.com"
  {{- else if eq .Environment "development" }}
  env:
    HTTP_PROXY: "http://dev-proxy.corp.com:8080"
    HTTPS_PROXY: "http://dev-proxy.corp.com:8080"
    NO_PROXY: "localhost,127.0.0.1,.dev.corp.com"
  {{- end }}
  config: |
    node-name: worker-{{.Environment}}-01
```

## Proxy Configuration Code

### Provider Logic

From `/Users/rishi/work/src/provider-rke2/pkg/provider/provider.go`:

```go
// proxyEnv generates proxy environment configuration
func proxyEnv(userOptions []byte, proxyMap map[string]string) string {
    // Extract cluster-cidr and service-cidr from user config
    clusterCIDR := extractCIDR(userOptions, "cluster-cidr")
    serviceCIDR := extractCIDR(userOptions, "service-cidr")

    // Auto-detect node CIDR
    nodeCIDR := getNodeCIDR()

    // Build NO_PROXY
    noProxy := proxyMap["NO_PROXY"]
    if clusterCIDR != "" {
        noProxy = appendCIDR(noProxy, clusterCIDR)
    }
    if serviceCIDR != "" {
        noProxy = appendCIDR(noProxy, serviceCIDR)
    }
    if nodeCIDR != "" {
        noProxy = appendCIDR(noProxy, nodeCIDR)
    }

    // Append k8sNoProxy constant
    noProxy = noProxy + "," + constants.K8SNoProxy
    // constants.K8SNoProxy = ".svc,.svc.cluster,.svc.cluster.local"

    // Generate environment file
    return formatProxyEnv(proxyMap, noProxy)
}
```

### Node CIDR Auto-Detection

```go
// getNodeCIDR auto-detects the node's network CIDR
func getNodeCIDR() string {
    // Gets primary interface IPv4 address
    // Calculates /24 subnet
    // Example: 10.0.1.50 → 10.0.1.0/24
}
```

### k8sNoProxy Constant

From `/Users/rishi/work/src/provider-rke2/pkg/constants/constants.go`:

```go
const K8SNoProxy = ".svc,.svc.cluster,.svc.cluster.local"
```

**Why these domains?**:
- `.svc` - Short service name (e.g., `mysql.default.svc`)
- `.svc.cluster` - Cluster-qualified (e.g., `mysql.default.svc.cluster`)
- `.svc.cluster.local` - Fully qualified (e.g., `mysql.default.svc.cluster.local`)

## Troubleshooting

### Test Proxy Connectivity

```bash
# From node, test proxy
curl -x http://proxy.corp.com:3128 https://www.google.com
# Should return HTML

# Test without proxy (should fail if firewall blocks)
curl --noproxy '*' https://www.google.com
# Should timeout or fail
```

### Verify Proxy Configuration

```bash
# Check RKE2 service environment
systemctl show rke2-server --property=Environment
# or
systemctl show rke2-agent --property=Environment

# Check proxy file
cat /etc/default/rke2-server
# or
cat /etc/default/rke2-agent
```

### Test Image Pull Through Proxy

```bash
# Pull image manually
rke2 ctr image pull docker.io/library/nginx:latest

# Check containerd proxy settings
rke2 ctr info | grep -A 10 proxy
```

### Check Pod DNS Resolution

```bash
# DNS should not be proxied
kubectl run test --rm -it --image=busybox -- nslookup kubernetes.default.svc.cluster.local

# Should resolve to service IP (10.43.0.1)
```

### Debug NO_PROXY Issues

```bash
# Check actual NO_PROXY in use
kubectl exec <pod> -- env | grep NO_PROXY

# Test connectivity to another pod
kubectl exec <pod> -- curl http://<other-pod-ip>

# If fails, check if pod IP in NO_PROXY
echo $NO_PROXY | grep <pod-ip>
```

## Common Pitfalls

### 1. Missing Cluster CIDRs in NO_PROXY

**Problem**: Pods can't communicate after proxy configured.

**Cause**: User overrides NO_PROXY without cluster CIDRs.

**Solution**: Provider automatically adds cluster CIDRs. Don't manually override.

### 2. Service Discovery Fails

**Problem**: Pods can't resolve service names (e.g., `mysql.default.svc`).

**Cause**: Missing `.svc` domains in NO_PROXY.

**Solution**: Provider adds k8sNoProxy constant automatically.

### 3. Node CIDR Not Auto-Detected

**Problem**: Nodes on multi-interface hosts use wrong interface.

**Solution**: Specify `node-ip` explicitly:
```yaml
config: |
  node-ip: 192.168.1.50  # Correct interface
```

### 4. Proxy Credentials Exposed

**Problem**: Proxy username/password visible in systemd.

**Solution**: Use transparent proxy or certificate authentication instead of basic auth.

### 5. HTTPS Proxy with Self-Signed CA

**Problem**: Image pulls fail with TLS errors.

**Solution**: Install proxy CA certificate:
```yaml
write_files:
  - path: /etc/pki/ca-trust/source/anchors/proxy-ca.crt
    content: |
      -----BEGIN CERTIFICATE-----
      ...
      -----END CERTIFICATE-----

stages:
  boot:
    - commands:
        - "update-ca-trust extract"
```

### 6. Proxy Configuration Not Applied

**Problem**: RKE2 not using proxy despite configuration.

**Cause**: Service started before proxy file created.

**Solution**: Restart RKE2:
```bash
systemctl restart rke2-server
# or
systemctl restart rke2-agent
```

### 7. Private Registry Not Bypassed

**Problem**: Pulls to internal registry go through proxy.

**Solution**: Add registry to NO_PROXY:
```yaml
env:
  NO_PROXY: "localhost,127.0.0.1,registry.corp.com,.corp.com"
```

## Integration Points

### Stylus Integration

Stylus generates proxy configuration from edge cluster settings:

```yaml
# Stylus-generated proxy config
cluster:
  env:
    HTTP_PROXY: "{{.ProxyHTTP}}"
    HTTPS_PROXY: "{{.ProxyHTTPS}}"
    NO_PROXY: "{{.ProxyBypass}}"
```

**Stylus automatically handles**:
- Proxy settings from cluster profile
- NO_PROXY from edge cluster configuration
- Provider adds cluster/service CIDRs automatically

### Corporate Firewall Checklist

When deploying RKE2 behind corporate firewall:

**Required Outbound Access** (if using proxy):
- Container registries (docker.io, gcr.io, etc.)
- RKE2 release downloads (github.com)
- Operating system updates (distro repos)

**Required Internal Access** (NO_PROXY):
- Cluster CIDR (pod-to-pod)
- Service CIDR (service discovery)
- Node network (node-to-node)
- Control plane endpoint
- Private registries
- Corporate domains

**Firewall Ports**:
- **8472/UDP**: VXLAN (inter-node, NO_PROXY)
- **6443/TCP**: Kubernetes API (inter-node, NO_PROXY)
- **9345/TCP**: RKE2 registration (inter-node, NO_PROXY)
- **2379-2380/TCP**: etcd (inter-node, NO_PROXY)

## Related Skills

- **01-architecture.md**: RKE2 architecture overview
- **02-configuration-patterns.md**: Configuration examples
- **05-networking.md**: Canal CNI and networking
- **08-troubleshooting.md**: Troubleshooting connectivity issues

## Documentation References

- **Provider Code**: `/Users/rishi/work/src/provider-rke2/pkg/provider/provider.go` (proxyEnv function)
- **Constants**: `/Users/rishi/work/src/provider-rke2/pkg/constants/constants.go` (K8SNoProxy)
- **RKE2 Proxy Docs**: https://docs.rke2.io/install/airgap#configuring-an-http-proxy
- **Containerd Proxy**: https://github.com/containerd/containerd/blob/main/docs/cri/config.md
