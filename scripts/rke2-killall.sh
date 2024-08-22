#!/bin/sh

# Ensure the script is run as root
if [ ! $(id -u) -eq 0 ]; then
    echo "$(basename "${0}"): must be run as root" >&2
    exit 1
fi

# Function to find child processes of a given parent process
pschildren() {
    ps -e -o ppid= -o pid= | \
    sed -e 's/^\s*//g; s/\s\s*/\t/g;' | \
    grep -w "^$1" | \
    cut -f2
}

# Function to recursively build a process tree starting from a given process
pstree() {
    for pid in "$@"; do
        echo ${pid}
        for child in $(pschildren ${pid}); do
            pstree ${child}
        done
    done
}

# Function to kill all processes in a tree starting from a given parent process
killtree() {
    kill -9 $(
        { set +x; } 2>/dev/null;
        pstree "$@";
        set -x;
    ) 2>/dev/null
}

# Function to find containerd-shim processes related to RKE2
getshims() {
    COLUMNS=2147483647 ps -e -o pid= -o args= | sed -e 's/^ *//; s/\s\s*/\t/;' | grep -w "${RKE2_DATA_DIR}"'/data/[^/]*/bin/containerd-shim' | cut -f1
}

# Function to unmount and remove directories
do_unmount_and_remove() {
    { set +x; } 2>/dev/null
    MOUNTS=
    while read ignore mount ignore; do
        MOUNTS="${mount}\n${MOUNTS}"
    done </proc/self/mounts
    MOUNTS=$(printf ${MOUNTS} | grep "^$1" | sort -r)
    if [ -n "${MOUNTS}" ]; then
        set -x
        umount ${MOUNTS}
        rm -rf --one-file-system ${MOUNTS}
    else
        set -x
    fi
}

# Load custom environment variables from /etc/spectro/environment if it exists
if [ -f /etc/spectro/environment ]; then
    . /etc/spectro/environment
fi

# Ensure STYLUS_ROOT does not have a trailing slash
STYLUS_ROOT="${STYLUS_ROOT%/}"

# Determine the base paths, use default if STYLUS_ROOT is not set
RKE2_DATA_DIR=${STYLUS_ROOT}/var/lib/rancher/rke2
RUN_DIR=/run/k3s
KUBELET_PODS_DIR=${STYLUS_ROOT}/var/lib/kubelet/pods
NETNS_CNI_DIR=/run/netns/cni-
CNI_DIR=${STYLUS_ROOT}/var/lib/cni/

export PATH=$PATH:${RKE2_DATA_DIR}/bin

set -x

# Stop RKE2 services
systemctl stop rke2-server.service || true
systemctl stop rke2-agent.service || true

# Kill all relevant processes
killtree $({ set +x; } 2>/dev/null; getshims; set -x)

# Unmount and remove directories
do_unmount_and_remove "${RUN_DIR}"
do_unmount_and_remove "${KUBELET_PODS_DIR}"
do_unmount_and_remove "${NETNS_CNI_DIR}"

# Delete network interface(s) that match 'master cni0'
ip link show 2>/dev/null | grep 'master cni0' | while read ignore iface ignore; do
    iface=${iface%%@*}
    [ -z "$iface" ] || ip link delete $iface
done

# Delete additional network interfaces
ip link delete cni0
ip link delete flannel.1
ip link delete flannel.4096
ip link delete flannel-v6.1
ip link delete flannel-v6.4096
ip link delete flannel-wg
ip link delete flannel-wg-v6
ip link delete vxlan.calico
ip link delete vxlan-v6.calico
ip link delete cilium_vxlan
ip link delete cilium_net
ip link delete cilium_wg0
ip link delete kube-ipvs0

# Delete nodeLocalDNS objects
if [ -d /sys/class/net/nodelocaldns ]; then
  for i in $(ip address show nodelocaldns | grep inet | awk '{print $2}'); do
    iptables-save | grep -v $i | iptables-restore
  done
  ip link delete nodelocaldns || true
fi

# Remove directories related to CNI and pod logs
rm -rf ${CNI_DIR} ${STYLUS_ROOT}/var/log/pods/ ${STYLUS_ROOT}/var/log/containers

# Remove pod manifest files for RKE2 components
POD_MANIFESTS_DIR=${RKE2_DATA_DIR}/agent/pod-manifests

rm -f "${POD_MANIFESTS_DIR}/etcd.yaml" \
      "${POD_MANIFESTS_DIR}/kube-apiserver.yaml" \
      "${POD_MANIFESTS_DIR}/kube-controller-manager.yaml" \
      "${POD_MANIFESTS_DIR}/cloud-controller-manager.yaml" \
      "${POD_MANIFESTS_DIR}/kube-scheduler.yaml" \
      "${POD_MANIFESTS_DIR}/kube-proxy.yaml"

# Cleanup iptables created by CNI plugins or Kubernetes (kube-proxy)
iptables-save | grep -v KUBE- | grep -v CNI- | grep -v cali- | grep -v cali: | grep -v CILIUM_ | grep -v flannel | iptables-restore
ip6tables-save | grep -v KUBE- | grep -v CNI- | grep -v cali- | grep -v cali: | grep -v CILIUM_ | grep -v flannel | ip6tables-restore

set +x

# Provide a message for additional iptables cleanup if needed
echo 'If this cluster was upgraded from an older release of the Canal CNI, you may need to manually remove some flannel iptables rules:'
echo -e '\texport cluster_cidr=YOUR-CLUSTER-CIDR'
echo -e '\tiptables -D POSTROUTING -s $cluster_cidr -j MASQUERADE --random-fully'
echo -e '\tiptables -D POSTROUTING ! -s $cluster_cidr -d  -j MASQUERADE --random-fully'
