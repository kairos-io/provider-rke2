---
skill: RKE2 Provider Security Features
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

# RKE2 Provider Security Features

## Overview

RKE2 is designed with security as a primary focus, providing enterprise-grade security features including FIPS 140-2 compliance, CIS Kubernetes Benchmark hardening, SELinux integration, and Pod Security Standards enforcement. This guide covers how to configure and use these security features in provider-rke2 deployments.

## Key Concepts

### Security-First Architecture

RKE2 differs from K3s by prioritizing security compliance and enterprise requirements:

| Feature | K3s | RKE2 |
|---------|-----|------|
| **FIPS 140-2** | No | Yes |
| **CIS Benchmark** | Manual | Built-in profiles |
| **SELinux** | Optional | Built-in policies |
| **Pod Security** | Manual | PSS enforced |
| **Audit Logging** | Manual | Pre-configured |
| **Secrets Encryption** | Optional | Recommended default |

### Security Layers

```
┌──────────────────────────────────────────────┐
│  Cluster Security (CIS Profile)             │
│  • API server hardening                     │
│  • Controller manager security              │
│  • Scheduler security                       │
│  • etcd encryption                          │
└──────────────────────────────────────────────┘
                    │
                    ▼
┌──────────────────────────────────────────────┐
│  Node Security (SELinux)                    │
│  • Process isolation                        │
│  • File system protection                   │
│  • Network restrictions                     │
└──────────────────────────────────────────────┘
                    │
                    ▼
┌──────────────────────────────────────────────┐
│  Workload Security (Pod Security Standards) │
│  • Privileged pod restrictions              │
│  • Capability limitations                   │
│  • Host namespace restrictions              │
└──────────────────────────────────────────────┘
                    │
                    ▼
┌──────────────────────────────────────────────┐
│  Cryptography (FIPS 140-2)                  │
│  • FIPS-validated crypto libraries          │
│  • TLS 1.3 enforcement                      │
│  • Secure cipher suites                     │
└──────────────────────────────────────────────┘
```

## FIPS 140-2 Compliance

### What is FIPS 140-2?

FIPS 140-2 (Federal Information Processing Standard) is a U.S. government security standard for cryptographic modules:

- **Required for**: U.S. government agencies, defense contractors, regulated industries
- **Validates**: Cryptographic algorithms, key generation, random number generation
- **Impact**: Restricts crypto to FIPS-validated implementations

### Enabling FIPS Mode

```yaml
#cloud-config
cluster:
  cluster_token: K10fips-compliant-cluster-token
  control_plane_host: 10.0.1.10
  role: init
  config: |
    fips: true                        # Enable FIPS 140-2 mode
    secrets-encryption: true          # Encrypt secrets at rest
    write-kubeconfig-mode: "0600"     # Stricter permissions

    # Enforce TLS 1.3+
    tls-min-version: "1.3"

    # FIPS-approved cipher suites only
    tls-cipher-suites:
      - TLS_AES_256_GCM_SHA384
      - TLS_AES_128_GCM_SHA256

    kube-apiserver-arg:
      - "encryption-provider-config=/etc/rancher/rke2/encryption-config.yaml"
```

### FIPS Requirements

**Operating System**:
- RHEL 8+ (FIPS mode enabled)
- Rocky Linux 8+ (FIPS mode enabled)
- Other FIPS-validated Linux distributions

**Enable FIPS on OS**:
```bash
# RHEL/Rocky Linux
fips-mode-setup --enable
reboot

# Verify FIPS mode
fips-mode-setup --check
# Should output: "FIPS mode is enabled."
```

**Container Images**:
- Must use FIPS-validated crypto libraries
- No OpenSSL unless FIPS-validated
- Use FIPS-compliant base images

### FIPS Limitations

**Performance Impact**:
- 10-15% typical overhead
- Cryptographic operations slower
- TLS handshakes take longer

**Compatibility Issues**:
```yaml
# Some workloads may fail with FIPS
# Example: Node.js applications using non-FIPS crypto

# Test workloads before production:
kubectl run fips-test --image=myapp:fips --dry-run=client
```

**Troubleshooting FIPS**:
```bash
# Check if RKE2 is running in FIPS mode
journalctl -u rke2-server | grep -i fips
# Should see: "Running in FIPS mode"

# Verify crypto module
cat /proc/sys/crypto/fips_enabled
# Should output: 1
```

## CIS Kubernetes Benchmark

### What is CIS Benchmark?

CIS (Center for Internet Security) Kubernetes Benchmark provides security configuration best practices:

- **Levels**: Level 1 (essential), Level 2 (defense-in-depth)
- **Scope**: API server, controller, scheduler, etcd, kubelet, network policies
- **Validation**: Automated scanning with kube-bench

### Built-in CIS Profiles

RKE2 includes pre-configured CIS profiles:

```yaml
#cloud-config
cluster:
  cluster_token: K10cis-hardened-cluster-token
  control_plane_host: 10.0.1.10
  role: init
  config: |
    profile: cis-1.23                 # Apply CIS 1.23 profile
    selinux: true                     # Required for CIS
    secrets-encryption: true          # CIS recommendation

    # Protect kernel defaults (CIS requirement)
    protect-kernel-defaults: true

    # Audit logging (CIS requirement)
    audit-policy-file: /etc/rancher/rke2/audit-policy.yaml

    kube-apiserver-arg:
      - "audit-log-path=/var/log/kube-audit.log"
      - "audit-log-maxage=30"
      - "audit-log-maxbackup=10"
      - "audit-log-maxsize=100"
      - "request-timeout=300s"
```

### Available CIS Profiles

| Profile | Kubernetes Version | CIS Benchmark Level |
|---------|-------------------|---------------------|
| `cis-1.23` | 1.23-1.24 | Level 1 |
| `cis-1.6` | 1.25+ | Level 1 |

### CIS Profile Effects

When CIS profile is enabled, RKE2 automatically configures:

**API Server**:
- Anonymous auth disabled
- Profiling disabled
- AlwaysPullImages admission controller enabled
- NodeRestriction admission controller enabled
- EventRateLimit admission controller enabled

**Controller Manager**:
- Service account token rotation enabled
- TLS cert rotation enabled
- Use service account credentials enabled

**Scheduler**:
- Profiling disabled

**Kubelet**:
- Anonymous auth disabled
- Authorization mode set to Webhook
- Read-only port disabled (10255)
- Streaming connection idle timeout set
- Protect kernel defaults enabled

**etcd**:
- Peer auto TLS enabled
- Client cert auth required

### Audit Policy Configuration

```yaml
#cloud-config
write_files:
  - path: /etc/rancher/rke2/audit-policy.yaml
    permissions: "0600"
    content: |
      apiVersion: audit.k8s.io/v1
      kind: Policy
      omitStages:
        - RequestReceived
      rules:
      # Log secret access
      - level: RequestResponse
        resources:
        - group: ""
          resources: ["secrets"]

      # Log configmap access
      - level: Metadata
        resources:
        - group: ""
          resources: ["configmaps"]

      # Log pod creation/deletion
      - level: RequestResponse
        verbs: ["create", "delete"]
        resources:
        - group: ""
          resources: ["pods"]

      # Don't log read-only requests
      - level: None
        verbs: ["get", "list", "watch"]
```

### Verifying CIS Compliance

Use kube-bench to validate:

```bash
# Install kube-bench
kubectl apply -f https://raw.githubusercontent.com/aquasecurity/kube-bench/main/job.yaml

# View results
kubectl logs -n default job/kube-bench

# Expected output:
# [PASS] 1.2.1 Ensure that the --anonymous-auth argument is set to false
# [PASS] 1.2.2 Ensure that the --basic-auth-file argument is not set
# ...
```

## SELinux Integration

### What is SELinux?

SELinux (Security-Enhanced Linux) provides mandatory access control (MAC):

- **Purpose**: Confine processes and limit damage from compromised applications
- **Modes**: Enforcing (blocks violations), Permissive (logs only), Disabled
- **Policies**: Type enforcement, role-based access control, multi-level security

### Enabling SELinux in RKE2

```yaml
#cloud-config
cluster:
  cluster_token: K10selinux-enabled-cluster-token
  control_plane_host: 10.0.1.10
  role: init
  config: |
    selinux: true                     # Enable SELinux enforcement
    profile: cis-1.23                 # CIS requires SELinux
```

**What happens**:
1. RKE2 installs SELinux policy module
2. Containers run with SELinux labels
3. File systems labeled appropriately
4. Processes confined to SELinux contexts

### SELinux Requirements

**Operating System**:
```bash
# RHEL/CentOS/Rocky Linux
# SELinux should be enabled by default

# Check SELinux status
getenforce
# Should output: Enforcing

# If not enforcing, enable it
setenforce 1

# Make persistent
vi /etc/selinux/config
# Set: SELINUX=enforcing
```

**Container Runtime**:
```yaml
# RKE2 automatically configures containerd for SELinux
# No manual configuration needed
```

### SELinux Contexts

RKE2 uses these SELinux contexts:

| Component | SELinux Context |
|-----------|----------------|
| RKE2 Server | `system_u:system_r:container_runtime_t` |
| RKE2 Agent | `system_u:system_r:container_runtime_t` |
| Containers | `system_u:system_r:container_t` |
| Container Files | `system_u:object_r:container_file_t` |

### Troubleshooting SELinux

**Check SELinux denials**:
```bash
# View recent denials
ausearch -m avc -ts recent

# Common denial: container trying to access host file
# Solution: Relabel the file
chcon -t container_file_t /path/to/file

# Or add volume with correct label in pod spec:
volumeMounts:
  - name: myvolume
    mountPath: /data
    seLinuxOptions:
      level: "s0:c123,c456"
```

**Temporarily disable SELinux** (troubleshooting only):
```bash
# WARNING: Only for debugging
setenforce 0
# Test if issue resolved

# Re-enable
setenforce 1
```

## Pod Security Standards (PSS)

### What are Pod Security Standards?

PSS replaces deprecated PodSecurityPolicy with admission controller:

- **Privileged**: Unrestricted (system workloads)
- **Baseline**: Minimally restrictive (prevents known privilege escalations)
- **Restricted**: Heavily restricted (defense-in-depth)

### Default PSS in RKE2

RKE2 enforces PSS by default:

```yaml
# System namespaces: Privileged
kube-system, kube-public, kube-node-lease, rke2-system: privileged

# User namespaces: Baseline (enforced), Restricted (warned)
default, user-namespaces: baseline (enforce), restricted (warn)
```

### Configuring PSS

```yaml
#cloud-config
cluster:
  cluster_token: K10pss-configured-cluster-token
  control_plane_host: 10.0.1.10
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
            audit: "restricted"         # Audit violations
            warn: "restricted"          # Warn on violations
          exemptions:
            namespaces:
            - kube-system               # Allow privileged
            - rke2-system
            - cattle-system
            usernames: []
            runtimeClasses: []
```

### PSS Levels Explained

**Privileged** (most permissive):
```yaml
# Allows:
# - Privileged containers
# - Host namespaces (network, PID, IPC)
# - Host path volumes
# - All capabilities

# Example: system daemons, CNI pods
```

**Baseline** (minimally restrictive):
```yaml
# Disallows:
# - Privileged containers
# - Host namespaces
# - Host path volumes (except allowed list)
# - Adding capabilities beyond defaults

# Allows:
# - Non-root users
# - Read-only root filesystem
# - Default capabilities (NET_BIND_SERVICE, etc.)

# Example: most application workloads
```

**Restricted** (most secure):
```yaml
# Everything in Baseline, plus:
# - Must run as non-root
# - Drop ALL capabilities
# - seccompProfile: RuntimeDefault
# - allowPrivilegeEscalation: false

# Example: security-critical applications
```

### Per-Namespace PSS

Label namespaces to override defaults:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: privileged-apps
  labels:
    pod-security.kubernetes.io/enforce: privileged
    pod-security.kubernetes.io/audit: privileged
    pod-security.kubernetes.io/warn: privileged
---
apiVersion: v1
kind: Namespace
metadata:
  name: restricted-apps
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/warn: restricted
```

### Common PSS Violations

**Issue**: Pod rejected - "must not set runAsUser=0"
```yaml
# Solution: Run as non-root user
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
```

**Issue**: Pod rejected - "must drop ALL capabilities"
```yaml
# Solution: Drop all caps, add only needed ones
securityContext:
  capabilities:
    drop:
      - ALL
    add:
      - NET_BIND_SERVICE  # If needed
```

**Issue**: Pod rejected - "must set seccompProfile"
```yaml
# Solution: Set seccomp profile
securityContext:
  seccompProfile:
    type: RuntimeDefault
```

## Secrets Encryption at Rest

### Enabling Secrets Encryption

```yaml
#cloud-config
cluster:
  cluster_token: K10secrets-encrypted-cluster-token
  control_plane_host: 10.0.1.10
  role: init
  config: |
    secrets-encryption: true          # Enable automatic encryption
    kube-apiserver-arg:
      - "encryption-provider-config=/etc/rancher/rke2/encryption-config.yaml"

write_files:
  - path: /etc/rancher/rke2/encryption-config.yaml
    permissions: "0600"
    content: |
      apiVersion: apiserver.config.k8s.io/v1
      kind: EncryptionConfiguration
      resources:
      - resources:
        - secrets
        providers:
        - aescbc:
            keys:
            - name: key1
              secret: $(head -c 32 /dev/urandom | base64)
        - identity: {}  # Fallback to plaintext for migration
```

**What happens**:
- New secrets encrypted with AES-CBC
- Existing secrets remain unencrypted until updated
- etcd stores encrypted data

### Rotating Encryption Keys

```bash
# 1. Add new key to config (key2)
# 2. Restart API server
systemctl restart rke2-server

# 3. Re-encrypt all secrets
kubectl get secrets --all-namespaces -o json | kubectl replace -f -

# 4. Remove old key from config (key1)
# 5. Restart API server again
```

## Compliance Deployment Examples

### FIPS + CIS + SELinux (Maximum Security)

```yaml
#cloud-config
cluster:
  cluster_token: K10maximum-security-cluster-token
  control_plane_host: 10.0.1.10
  role: init
  config: |
    fips: true
    selinux: true
    profile: cis-1.23
    secrets-encryption: true
    protect-kernel-defaults: true

    tls-min-version: "1.3"
    tls-cipher-suites:
      - TLS_AES_256_GCM_SHA384

    audit-policy-file: /etc/rancher/rke2/audit-policy.yaml
    pod-security-admission-config-file: /etc/rancher/rke2/pss-config.yaml

    kube-apiserver-arg:
      - "audit-log-path=/var/log/kube-audit.log"
      - "audit-log-maxage=30"
      - "request-timeout=300s"
```

### Government/Defense Configuration

```yaml
#cloud-config
cluster:
  cluster_token: K10government-cluster-token
  control_plane_host: 10.0.1.10
  role: init
  config: |
    fips: true
    selinux: true
    profile: cis-1.23
    secrets-encryption: true

    # Strict network policies
    kube-apiserver-arg:
      - "admission-control-config-file=/etc/rancher/rke2/admission-config.yaml"

    # Enhanced audit logging
    audit-policy-file: /etc/rancher/rke2/audit-policy.yaml
    kube-apiserver-arg:
      - "audit-log-path=/var/log/kube-audit.log"
      - "audit-log-maxage=90"          # 90 days retention
      - "audit-log-maxbackup=30"
      - "audit-log-maxsize=500"
```

## Related Skills

- **01-architecture.md**: RKE2 architecture overview
- **02-configuration-patterns.md**: Configuration examples
- **03-cluster-roles.md**: Role-based deployment
- **05-networking.md**: Canal CNI and NetworkPolicy

## Documentation References

- **RKE2 Security**: https://docs.rke2.io/security/hardening_guide
- **FIPS 140-2**: https://docs.rke2.io/security/fips_support
- **CIS Benchmark**: https://docs.rke2.io/security/cis_self_assessment
- **SELinux**: https://docs.rke2.io/security/selinux
- **Pod Security**: https://kubernetes.io/docs/concepts/security/pod-security-standards/
- **Audit Logging**: https://kubernetes.io/docs/tasks/debug/debug-cluster/audit/
