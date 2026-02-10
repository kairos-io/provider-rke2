---
skill: RKE2 Provider Deployment Modes
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

# RKE2 Provider Deployment Modes

## Overview

Provider-rke2 supports two deployment modes: **Appliance Mode** (immutable OS with Kairos) and **Agent Mode** (mutable Linux with Stylus agent). Both modes use the same RKE2 provider code but differ in filesystem layout, update mechanisms, and integration patterns. The `STYLUS_ROOT` environment variable enables unified path handling across both modes.

## Key Concepts

### Deployment Mode Comparison

| Aspect | Appliance Mode | Agent Mode |
|--------|---------------|------------|
| **OS Type** | Immutable (Kairos) | Mutable (standard Linux) |
| **Base Path** | `/` (root) | `/persistent/spectro` (custom) |
| **STYLUS_ROOT** | `/` or not set | `/persistent/spectro` |
| **Updates** | A/B partition, reboot | In-place, selective restart |
| **Services** | `cos-setup-*`, RKE2 | `spectro-palette-agent-*`, RKE2 |
| **Boot Detection** | `cos-img` in `/proc/cmdline` | Stylus agent running |
| **Image Format** | ISO, disk images | Container images + agent |
| **Use Case** | Edge appliances, IoT | General-purpose servers |
| **Rollback** | Automatic (previous partition) | Manual (backup/restore) |

### STYLUS_ROOT Variable

**Purpose**: Unified path prefix for both deployment modes.

**Values**:
- Appliance mode: `STYLUS_ROOT=/` (or unset, defaults to `/`)
- Agent mode: `STYLUS_ROOT=/persistent/spectro`

**Usage in scripts**:
```bash
# Source environment if present
if [ -f /etc/spectro/environment ]; then
    . /etc/spectro/environment
fi

# Remove trailing slash
STYLUS_ROOT="${STYLUS_ROOT%/}"

# Use in all paths
RKE2_DATA_DIR=${STYLUS_ROOT}/var/lib/rancher/rke2
RKE2_CONFIG=${STYLUS_ROOT}/etc/rancher/rke2/config.yaml
```

**Result**:
- Appliance mode: `/var/lib/rancher/rke2`, `/etc/rancher/rke2/config.yaml`
- Agent mode: `/persistent/spectro/var/lib/rancher/rke2`, `/persistent/spectro/etc/rancher/rke2/config.yaml`

## Appliance Mode

### What is Appliance Mode?

Appliance mode uses Kairos immutable Linux distribution:

**Characteristics**:
- **Immutable OS**: A/B partitions, read-only root filesystem
- **Atomic Updates**: Full OS image updates, reboot to apply
- **Boot Stages**: YIP-based configuration during boot
- **Auto-Recovery**: Rolls back to previous partition on failure
- **Edge-Optimized**: Small footprint, predictable behavior

### Detecting Appliance Mode

```bash
# Check for Kairos/C3OS boot parameter
cat /proc/cmdline | grep -q cos-img

# Check for Kairos services
systemctl list-units | grep cos-setup
```

### Appliance Mode Architecture

```
┌─────────────────────────────────────────────┐
│  Immutable OS (Kairos)                     │
│  ┌────────────────────────────────────┐    │
│  │ Partition A (Active)               │    │
│  │ • RKE2 binaries                    │    │
│  │ • Provider-rke2 plugin             │    │
│  │ • OS packages                      │    │
│  │ • Read-only root (/)               │    │
│  └────────────────────────────────────┘    │
│  ┌────────────────────────────────────┐    │
│  │ Partition B (Standby)              │    │
│  │ • Previous OS version              │    │
│  │ • Rollback target                  │    │
│  └────────────────────────────────────┘    │
│  ┌────────────────────────────────────┐    │
│  │ Persistent Data (/var, /etc)       │    │
│  │ • RKE2 data (/var/lib/rancher)     │    │
│  │ • RKE2 config (/etc/rancher)       │    │
│  │ • Cluster state                    │    │
│  └────────────────────────────────────┘    │
└─────────────────────────────────────────────┘
```

### Appliance Mode Configuration

```yaml
#cloud-config
# Kairos-specific configuration
hostname: rke2-appliance-01

# Standard cluster configuration (no STYLUS_ROOT needed)
cluster:
  cluster_token: K10appliance-mode-token
  control_plane_host: 10.0.1.10
  role: worker
  config: |
    node-name: appliance-worker-01
    node-label:
      - "deployment=appliance"
      - "os=immutable"
    selinux: true
```

**Provider behavior**:
- Uses root filesystem (`/`)
- Writes to `/etc/rancher/rke2/config.d/`
- RKE2 data in `/var/lib/rancher/rke2`
- No STYLUS_ROOT environment variable needed

### Appliance Mode Updates

**Update Process**:
```
1. Download new OS image (includes RKE2 updates)
2. Write to standby partition (B)
3. Set active flag to partition B
4. Reboot
5. Boot from partition B (new image)
6. Verify cluster health
7. If fail: Automatic rollback to partition A
```

**Provider role**: Provider-rke2 plugin included in OS image, updated atomically.

### Appliance Mode Advantages

✅ **Predictable State**: OS and RKE2 version locked together
✅ **Easy Rollback**: Automatic revert to previous partition
✅ **Secure**: Immutable root prevents tampering
✅ **Simplified Management**: Single image for OS + Kubernetes
✅ **Fast Recovery**: Boot from known-good partition

### Appliance Mode Limitations

❌ **Update Size**: Full OS image (hundreds of MB)
❌ **Update Time**: Requires reboot, cluster downtime
❌ **Flexibility**: Can't install arbitrary packages
❌ **Debugging**: Limited tools in immutable OS

## Agent Mode

### What is Agent Mode?

Agent mode uses standard mutable Linux with Stylus palette agent:

**Characteristics**:
- **Mutable OS**: Traditional Linux, read-write filesystem
- **Selective Updates**: Update RKE2 independently of OS
- **Custom Paths**: All Spectro components under `/persistent/spectro`
- **No Reboot**: In-place RKE2 updates, restart services only
- **Flexible**: Install packages, debug tools as needed

### Detecting Agent Mode

```bash
# Check for Stylus agent services
systemctl list-units | grep spectro-palette-agent

# Check for STYLUS_ROOT environment
cat /etc/spectro/environment
```

### Agent Mode Architecture

```
┌─────────────────────────────────────────────┐
│  Mutable OS (Ubuntu, RHEL, etc.)           │
│  ┌────────────────────────────────────┐    │
│  │ Root Filesystem (/)                │    │
│  │ • Standard OS packages             │    │
│  │ • Stylus palette agent             │    │
│  │ • Debugging tools                  │    │
│  └────────────────────────────────────┘    │
│  ┌────────────────────────────────────┐    │
│  │ Spectro Directory                  │    │
│  │ /persistent/spectro/               │    │
│  │ ├── etc/rancher/rke2/              │    │
│  │ ├── var/lib/rancher/rke2/          │    │
│  │ ├── usr/local/bin/ (RKE2 bins)     │    │
│  │ └── opt/rke2/ (provider)           │    │
│  └────────────────────────────────────┘    │
└─────────────────────────────────────────────┘
```

### Agent Mode Configuration

```yaml
#cloud-config
hostname: rke2-agent-01

# Standard cluster configuration
# STYLUS_ROOT is set by /etc/spectro/environment (created by Stylus agent)
cluster:
  cluster_token: K10agent-mode-token
  control_plane_host: 10.0.1.10
  role: worker
  config: |
    node-name: agent-worker-01
    node-label:
      - "deployment=agent"
      - "os=mutable"
    selinux: true
```

**STYLUS_ROOT Setup** (by Stylus agent):
```bash
# /etc/spectro/environment (created by Stylus agent)
export STYLUS_ROOT=/persistent/spectro
```

**Provider behavior**:
- Sources `/etc/spectro/environment`
- Uses `${STYLUS_ROOT}` prefix for all paths
- Writes to `/persistent/spectro/etc/rancher/rke2/config.d/`
- RKE2 data in `/persistent/spectro/var/lib/rancher/rke2`

### Agent Mode Updates

**Update Process**:
```
1. Stylus agent receives update command
2. Download new RKE2 binary
3. Stop rke2-server or rke2-agent service
4. Replace binary in /persistent/spectro/usr/local/bin/
5. Update provider-rke2 plugin
6. Start rke2-server or rke2-agent service
7. Verify cluster health
8. If fail: Rollback to previous binary (manual)
```

**Provider role**: Provider-rke2 plugin updated independently via Stylus.

### Agent Mode Advantages

✅ **Selective Updates**: Update RKE2 without OS reboot
✅ **No Reboot**: Faster updates, less downtime
✅ **Flexible**: Install any OS packages, debugging tools
✅ **Debugging**: Full Linux toolchain available
✅ **Efficient**: Update only changed components

### Agent Mode Limitations

❌ **Manual Rollback**: No automatic OS-level rollback
❌ **State Drift**: OS and RKE2 versions can diverge
❌ **Complex Path**: All paths prefixed with STYLUS_ROOT
❌ **Security**: Mutable OS more vulnerable to tampering

## Implementation Details

### STYLUS_ROOT in Scripts

From `/Users/rishi/work/src/provider-rke2/scripts/rke2-uninstall.sh`:

```bash
#!/bin/sh

# Load custom environment variables from /etc/spectro/environment if it exists
if [ -f /etc/spectro/environment ]; then
    . /etc/spectro/environment
fi

# Ensure STYLUS_ROOT does not have a trailing slash
STYLUS_ROOT="${STYLUS_ROOT%/}"

# Use STYLUS_ROOT in all paths
RKE2_DATA_DIR=${STYLUS_ROOT}/var/lib/rancher/rke2
RKE2_CONFIG_DIR=${STYLUS_ROOT}/etc/rancher/rke2

# Cleanup
rm -rf ${STYLUS_ROOT}/etc/rancher || true
rm -rf ${STYLUS_ROOT}/etc/cni
rm -rf ${STYLUS_ROOT}/opt/cni/bin
rm -rf ${STYLUS_ROOT}/var/lib/kubelet || true
rm -rf "${RKE2_DATA_DIR}"
```

### Provider File Paths

With STYLUS_ROOT, provider-rke2 uses these paths:

| Purpose | Appliance Mode Path | Agent Mode Path |
|---------|-------------------|----------------|
| Config directory | `/etc/rancher/rke2/config.d/` | `/persistent/spectro/etc/rancher/rke2/config.d/` |
| Final config | `/etc/rancher/rke2/config.yaml` | `/persistent/spectro/etc/rancher/rke2/config.yaml` |
| Data directory | `/var/lib/rancher/rke2/` | `/persistent/spectro/var/lib/rancher/rke2/` |
| Proxy config | `/etc/default/rke2-server` | `/persistent/spectro/etc/default/rke2-server` |
| Binary path | `/usr/local/bin/rke2` | `/persistent/spectro/usr/local/bin/rke2` |
| Logs | `/var/log/rke2.log` | `/persistent/spectro/var/log/rke2.log` |

### Dual Mode Support

Provider-rke2 automatically detects mode:

```go
// Provider code (conceptual)
func getSTYLUSRoot() string {
    // Check for /etc/spectro/environment
    if fileExists("/etc/spectro/environment") {
        // Agent mode: Source environment, return STYLUS_ROOT
        return sourceEnvironment()
    }
    // Appliance mode: Return empty or "/"
    return ""
}

func getConfigPath() string {
    stylusRoot := getSTYLUSRoot()
    if stylusRoot != "" {
        return stylusRoot + "/etc/rancher/rke2/config.d/"
    }
    return "/etc/rancher/rke2/config.d/"
}
```

## Choosing Deployment Mode

### Use Appliance Mode When:

✅ **Edge Appliances**: Purpose-built devices
✅ **IoT Deployments**: Resource-constrained devices
✅ **Security Critical**: Immutable OS requirements
✅ **Simplified Management**: Single image, atomic updates
✅ **Predictable Behavior**: Locked OS + RKE2 versions
✅ **Auto-Recovery**: Need automatic rollback capability

**Example**: Factory floor gateway devices, retail kiosks, remote sensors

### Use Agent Mode When:

✅ **General-Purpose Servers**: Standard Linux installations
✅ **Flexible Requirements**: Need to install additional packages
✅ **Debugging Needs**: Require full Linux toolchain
✅ **Selective Updates**: Update RKE2 independently
✅ **No Reboot Tolerance**: Minimize downtime
✅ **Existing Infrastructure**: Deploy to existing Linux hosts

**Example**: Data center servers, VM instances, existing Linux hosts

## Troubleshooting

### Verify Deployment Mode

```bash
# Check for appliance mode (Kairos)
cat /proc/cmdline | grep cos-img && echo "Appliance Mode" || echo "Not Appliance"

# Check for agent mode (Stylus agent)
systemctl is-active spectro-palette-agent && echo "Agent Mode" || echo "Not Agent"

# Check STYLUS_ROOT
if [ -f /etc/spectro/environment ]; then
    cat /etc/spectro/environment
    echo "Agent Mode with STYLUS_ROOT"
else
    echo "Appliance Mode (no STYLUS_ROOT)"
fi
```

### Check RKE2 Paths

```bash
# Appliance mode
ls -la /etc/rancher/rke2/
ls -la /var/lib/rancher/rke2/

# Agent mode
ls -la /persistent/spectro/etc/rancher/rke2/
ls -la /persistent/spectro/var/lib/rancher/rke2/
```

### Verify RKE2 Binary Location

```bash
# Appliance mode
which rke2
# Should show: /usr/local/bin/rke2

# Agent mode
ls -la /persistent/spectro/usr/local/bin/rke2
```

### Check Systemd Service

```bash
# Appliance mode
systemctl cat rke2-server
# Should show: ExecStart=/usr/local/bin/rke2 server

# Agent mode
systemctl cat rke2-server
# Should show: ExecStart=/persistent/spectro/usr/local/bin/rke2 server
# Should source: EnvironmentFile=/etc/spectro/environment
```

## Migration Between Modes

### Appliance to Agent (Not Supported)

**Recommendation**: Deploy new agent mode nodes, migrate workloads, decommission appliance nodes.

### Agent to Appliance (Not Supported)

**Recommendation**: Build new appliance image, deploy fresh, migrate workloads.

### Why No Direct Migration?

- Filesystem layout differs significantly
- Systemd services configured differently
- STYLUS_ROOT affects all paths
- Update mechanisms incompatible
- Risk of cluster instability

## Integration Points

### Stylus Integration

Stylus orchestrates both modes:

**Appliance Mode**:
- Generates Kairos ISO with embedded RKE2
- Manages OS image updates
- Provides cluster configuration via cloud-init
- Monitors cluster health

**Agent Mode**:
- Deploys Stylus palette agent to Linux host
- Manages RKE2 binary updates via agent
- Provides cluster configuration via agent API
- Monitors cluster and agent health

### Provider Events

Provider-rke2 emits events for Stylus (agent mode):

```go
// Provider events (conceptual)
type ProviderEvent struct {
    Type    string // "config_updated", "cluster_ready", "error"
    Message string
    Time    time.Time
}
```

**Stylus agent** listens for events and reports to Palette management plane.

## Related Skills

- **01-architecture.md**: RKE2 architecture overview
- **02-configuration-patterns.md**: Configuration examples
- **03-cluster-roles.md**: Role-based deployment
- **08-troubleshooting.md**: Troubleshooting deployment issues

## Documentation References

- **RKE2 Scripts**: `/Users/rishi/work/src/provider-rke2/scripts/rke2-uninstall.sh` (STYLUS_ROOT usage)
- **Kairos Docs**: https://kairos.io/docs/
- **Stylus Docs**: See Stylus ai-knowledge-base for agent mode details
- **Immutable OS**: https://en.wikipedia.org/wiki/Immutable_operating_system
