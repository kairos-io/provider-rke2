---
skill: RKE2 Provider Networking
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

# RKE2 Provider Networking

## Overview

RKE2 includes **Canal CNI** by default, which combines Flannel for simple pod networking with Calico for advanced NetworkPolicy enforcement. This provides the best of both worlds: Flannel's simplicity and performance with Calico's policy engine and observability features.

## Key Concepts

### Canal CNI Architecture

Canal is a combination of two CNI plugins:

```
┌────────────────────────────────────────────────┐
│              Canal CNI                         │
│  ┌──────────────────┐  ┌──────────────────┐   │
│  │    Flannel       │  │     Calico       │   │
│  │  (Networking)    │  │ (NetworkPolicy)  │   │
│  │                  │  │                  │   │
│  │ • VXLAN overlay  │  │ • Policy engine  │   │
│  │ • Pod-to-pod     │  │ • Label-based    │   │
│  │ • Simple config  │  │ • Ingress/egress │   │
│  │ • Port 8472      │  │ • Observability  │   │
│  └──────────────────┘  └──────────────────┘   │
└────────────────────────────────────────────────┘
```

**Why Canal?**:
- **Flannel**: Proven, stable, simple L3 networking
- **Calico**: Industry-leading NetworkPolicy implementation
- **Combined**: Network simplicity + policy enforcement

### Canal vs Other CNIs

| Feature | Canal (RKE2) | Flannel (K3s) | Calico | Cilium |
|---------|-------------|---------------|---------|---------|
| **NetworkPolicy** | ✅ Yes | ❌ No | ✅ Yes | ✅ Yes |
| **BGP Support** | ✅ Optional | ❌ No | ✅ Yes | ❌ No |
| **Complexity** | Low | Very Low | Medium | High |
| **Performance** | High | High | High | Very High |
| **VXLAN Port** | 8472 | 8472 | 8472/4789 | 8472 |
| **eBPF** | ❌ No | ❌ No | ⚠️ Partial | ✅ Yes |

### Default Network Configuration

```yaml
# RKE2 defaults (same as K3s)
cluster-cidr: 10.42.0.0/16       # Pod network
service-cidr: 10.43.0.0/16       # Service network
cluster-dns: 10.43.0.10          # CoreDNS ClusterIP
```

### VXLAN Overlay Networking

Canal uses Flannel's VXLAN backend by default:

**How VXLAN Works**:
```
Node A (10.0.1.10)              Node B (10.0.1.20)
┌──────────────────┐            ┌──────────────────┐
│ Pod: 10.42.0.5   │            │ Pod: 10.42.1.8   │
│     ▼            │            │     ▲            │
│ flannel.1        │            │ flannel.1        │
│ (VXLAN tunnel)   │            │ (VXLAN tunnel)   │
│     ▼            │            │     ▲            │
│ eth0: 10.0.1.10  │═══════════▶│ eth0: 10.0.1.20  │
└──────────────────┘  UDP 8472  └──────────────────┘
```

**VXLAN Characteristics**:
- **Protocol**: UDP
- **Port**: 8472
- **Encapsulation**: Ethernet frames in UDP packets
- **MTU**: Typically 1450 (1500 - 50 byte overhead)
- **Performance**: ~90-95% of native networking

## Implementation Patterns

### Basic Canal Configuration

```yaml
#cloud-config
cluster:
  cluster_token: K10basic-canal-cluster-token
  control_plane_host: 10.0.1.10
  role: init
  config: |
    # Canal is enabled by default
    cluster-cidr: 10.42.0.0/16
    service-cidr: 10.43.0.0/16
```

**What's included automatically**:
- Flannel VXLAN for pod networking
- Calico for NetworkPolicy enforcement
- CoreDNS for service discovery
- kube-proxy for service load balancing

### Custom Network CIDRs

```yaml
#cloud-config
cluster:
  cluster_token: K10custom-cidr-cluster-token
  control_plane_host: 10.0.1.10
  role: init
  config: |
    cluster-cidr: 10.52.0.0/16       # Custom pod network
    service-cidr: 10.53.0.0/16       # Custom service network
    cluster-dns: 10.53.0.10          # Must be within service-cidr
```

**Use case**: Avoid conflicts with existing network infrastructure.

### Multi-Interface Node Configuration

```yaml
#cloud-config
cluster:
  cluster_token: K10multi-interface-cluster-token
  control_plane_host: 10.0.1.10
  role: worker
  config: |
    node-ip: 192.168.1.50            # Primary interface for k8s
    node-external-ip: 203.0.113.50   # Public IP
    bind-address: 0.0.0.0            # Listen on all interfaces

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
        canal_iface: "eth1"            # Specify interface for Canal
```

**Use case**: Nodes with separate management and data plane networks.

### IPVS Mode for kube-proxy

```yaml
#cloud-config
cluster:
  cluster_token: K10ipvs-mode-cluster-token
  control_plane_host: 10.0.1.10
  role: init
  config: |
    kube-proxy-arg:
      - "proxy-mode=ipvs"              # Use IPVS instead of iptables
      - "ipvs-scheduler=rr"            # Round-robin load balancing
      - "ipvs-strict-arp=true"         # Required for MetalLB
      - "ipvs-sync-period=30s"
      - "ipvs-min-sync-period=5s"
```

**Benefits of IPVS**:
- Better performance with many services (1000+)
- More load balancing algorithms
- Connection-level load balancing
- Better for large clusters

**Requirements**:
```bash
# Load IPVS kernel modules
modprobe ip_vs
modprobe ip_vs_rr
modprobe ip_vs_wrr
modprobe ip_vs_sh
modprobe nf_conntrack
```

### Custom MTU Configuration

```yaml
#cloud-config
cluster:
  cluster_token: K10custom-mtu-cluster-token
  control_plane_host: 10.0.1.10
  role: init
  config: |
    cluster-cidr: 10.42.0.0/16

write_files:
  - path: /var/lib/rancher/rke2/server/manifests/canal-mtu.yaml
    permissions: "0600"
    content: |
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: canal-config
        namespace: kube-system
      data:
        mtu: "1400"                    # Adjust for nested VXLAN
```

**When to customize MTU**:
- Jumbo frames (MTU > 1500)
- Nested VXLAN (reduce to 1400)
- IPsec overhead
- Cloud provider requirements

## NetworkPolicy

### NetworkPolicy Overview

Canal (Calico) enforces Kubernetes NetworkPolicy:

```yaml
# Default: All ingress and egress allowed
# With NetworkPolicy: Default deny, explicit allow
```

**Policy Types**:
- **Ingress**: Controls incoming traffic to pods
- **Egress**: Controls outgoing traffic from pods

### Basic NetworkPolicy Examples

**Deny All Ingress**:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-all-ingress
  namespace: default
spec:
  podSelector: {}                      # Applies to all pods
  policyTypes:
  - Ingress
  # No ingress rules = deny all
```

**Allow Specific Ingress**:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-nginx-ingress
  namespace: default
spec:
  podSelector:
    matchLabels:
      app: nginx
  policyTypes:
  - Ingress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          app: frontend              # Only from frontend pods
    ports:
    - protocol: TCP
      port: 80
```

**Allow from Specific Namespace**:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-from-monitoring
  namespace: app-namespace
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring           # Only from monitoring namespace
    ports:
    - protocol: TCP
      port: 9090                     # Prometheus metrics
```

**Deny All Egress**:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-all-egress
  namespace: default
spec:
  podSelector: {}
  policyTypes:
  - Egress
  # No egress rules = deny all (except DNS)
```

**Allow DNS and Specific Egress**:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-dns-and-api
  namespace: default
spec:
  podSelector:
    matchLabels:
      app: backend
  policyTypes:
  - Egress
  egress:
  # Allow DNS
  - to:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: kube-system
      podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53

  # Allow to database
  - to:
    - podSelector:
        matchLabels:
          app: database
    ports:
    - protocol: TCP
      port: 5432
```

### Advanced NetworkPolicy Patterns

**Allow Internet Egress (Block Internal)**:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-internet-only
  namespace: default
spec:
  podSelector:
    matchLabels:
      internet-access: "true"
  policyTypes:
  - Egress
  egress:
  # Allow DNS
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53

  # Allow external only (not pod or service CIDRs)
  - to:
    - ipBlock:
        cidr: 0.0.0.0/0
        except:
          - 10.42.0.0/16             # Block pod CIDR
          - 10.43.0.0/16             # Block service CIDR
          - 10.0.0.0/8               # Block internal networks
          - 172.16.0.0/12
          - 192.168.0.0/16
```

**Multi-Tier Application**:
```yaml
# Frontend: Allow from internet
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: frontend-policy
  namespace: app
spec:
  podSelector:
    matchLabels:
      tier: frontend
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from: []                         # Allow all
    ports:
    - protocol: TCP
      port: 80
  egress:
  - to:
    - podSelector:
        matchLabels:
          tier: backend
    ports:
    - protocol: TCP
      port: 8080

---
# Backend: Allow only from frontend
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: backend-policy
  namespace: app
spec:
  podSelector:
    matchLabels:
      tier: backend
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          tier: frontend
    ports:
    - protocol: TCP
      port: 8080
  egress:
  - to:
    - podSelector:
        matchLabels:
          tier: database
    ports:
    - protocol: TCP
      port: 5432

---
# Database: Allow only from backend
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: database-policy
  namespace: app
spec:
  podSelector:
    matchLabels:
      tier: database
  policyTypes:
  - Ingress
  ingress:
  - from:
    - podSelector:
        matchLabels:
          tier: backend
    ports:
    - protocol: TCP
      port: 5432
```

## Troubleshooting

### Check Canal Status

```bash
# Check Canal pods
kubectl get pods -n kube-system | grep canal

# Expected output:
# canal-xxxxx   2/2   Running   0   10m

# Each Canal pod runs 2 containers:
# 1. calico-node (policy enforcement)
# 2. kube-flannel (VXLAN networking)
```

### Verify VXLAN Tunnel

```bash
# Check Flannel interface
ip addr show flannel.1

# Expected output:
# flannel.1: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1450
#     inet 10.42.0.0/32 scope global flannel.1

# Check VXLAN port
ss -ulpn | grep 8472

# Expected output:
# UNCONN  0  0  0.0.0.0:8472  *:*
```

### Test Pod-to-Pod Connectivity

```bash
# Create test pods
kubectl run test-1 --image=nicolaka/netshoot -- sleep 3600
kubectl run test-2 --image=nicolaka/netshoot -- sleep 3600

# Get pod IPs
POD1_IP=$(kubectl get pod test-1 -o jsonpath='{.status.podIP}')
POD2_IP=$(kubectl get pod test-2 -o jsonpath='{.status.podIP}')

# Test connectivity
kubectl exec test-1 -- ping -c 3 $POD2_IP

# Expected: 3 packets transmitted, 3 received
```

### Test NetworkPolicy

```bash
# Deploy nginx
kubectl run nginx --image=nginx --labels=app=nginx

# Create deny-all policy
kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: deny-all
spec:
  podSelector: {}
  policyTypes:
  - Ingress
EOF

# Test (should fail)
kubectl run test --rm -it --image=busybox -- wget -O- http://nginx

# Create allow policy
kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-test
spec:
  podSelector:
    matchLabels:
      app: nginx
  ingress:
  - from:
    - podSelector:
        matchLabels:
          run: test
EOF

# Test again (should succeed)
kubectl run test --rm -it --image=busybox --labels=run=test -- wget -O- http://nginx
```

### Check Calico Policy Status

```bash
# Install calicoctl
kubectl apply -f https://raw.githubusercontent.com/projectcalico/calico/master/manifests/calicoctl.yaml

# List Calico NetworkPolicy
kubectl exec -n kube-system calicoctl -- calicoctl get networkpolicy --all-namespaces

# View detailed policy
kubectl exec -n kube-system calicoctl -- calicoctl get networkpolicy <policy-name> -o yaml
```

## Common Pitfalls

### 1. VXLAN Port Blocked by Firewall

```bash
# Problem: Pods on different nodes can't communicate

# Solution: Open UDP port 8472
firewall-cmd --permanent --add-port=8472/udp
firewall-cmd --reload

# Or for iptables
iptables -A INPUT -p udp --dport 8472 -j ACCEPT
```

### 2. MTU Mismatch

```bash
# Problem: Large packets dropped, intermittent connectivity

# Diagnose
kubectl exec test-pod -- ping -M do -s 1400 <other-pod-ip>
# If fails, reduce MTU

# Solution: Set Canal MTU to 1400 (see Custom MTU section)
```

### 3. NetworkPolicy Blocking DNS

```bash
# Problem: Pods can't resolve DNS after NetworkPolicy applied

# Solution: Always allow DNS in egress
egress:
- to:
  - namespaceSelector:
      matchLabels:
        kubernetes.io/metadata.name: kube-system
    podSelector:
      matchLabels:
        k8s-app: kube-dns
  ports:
  - protocol: UDP
    port: 53
```

### 4. Canal Pods CrashLooping

```bash
# Check logs
kubectl logs -n kube-system <canal-pod> -c calico-node
kubectl logs -n kube-system <canal-pod> -c kube-flannel

# Common causes:
# - SELinux denials (check ausearch -m avc)
# - Kernel modules missing (modprobe iptable_nat)
# - Network interface name issues
```

### 5. Service Not Accessible

```bash
# Check kube-proxy mode
kubectl logs -n kube-system <kube-proxy-pod> | grep "proxy mode"

# Check iptables rules (if using iptables mode)
iptables -t nat -L -n | grep <service-ip>

# Check IPVS rules (if using ipvs mode)
ipvsadm -Ln | grep <service-ip>
```

## Performance Tuning

### Optimize for Large Clusters

```yaml
cluster:
  config: |
    kube-proxy-arg:
      - "proxy-mode=ipvs"
      - "ipvs-scheduler=rr"
      - "conntrack-max-per-core=131072"
      - "conntrack-tcp-timeout-established=86400s"
```

### Reduce NetworkPolicy Latency

```yaml
# Minimize policy complexity
# Use namespace selectors instead of individual pod selectors
# Consolidate multiple policies into fewer broader policies
```

## Integration Points

### Stylus Integration

Stylus configures Canal networking for edge clusters:

```yaml
# Stylus-generated config
cluster:
  config: |
    cluster-cidr: {{.ClusterCIDR}}
    service-cidr: {{.ServiceCIDR}}
    # Canal enabled by default
```

### External Load Balancer Integration

Canal works with MetalLB for bare-metal load balancing:

```yaml
# Requires IPVS strict ARP
cluster:
  config: |
    kube-proxy-arg:
      - "proxy-mode=ipvs"
      - "ipvs-strict-arp=true"
```

## Related Skills

- **01-architecture.md**: RKE2 architecture overview
- **02-configuration-patterns.md**: Network configuration examples
- **04-security.md**: NetworkPolicy for security hardening
- **06-proxy-configuration.md**: Proxy integration with Canal

## Documentation References

- **RKE2 Networking**: https://docs.rke2.io/networking/basic_network_options
- **Canal CNI**: https://projectcalico.docs.tigera.io/getting-started/kubernetes/flannel/flannel
- **NetworkPolicy**: https://kubernetes.io/docs/concepts/services-networking/network-policies/
- **Calico**: https://projectcalico.docs.tigera.io/
- **Flannel**: https://github.com/flannel-io/flannel
