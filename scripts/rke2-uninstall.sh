#!/bin/sh
set -ex

# Ensure the script is run as root
if [ ! $(id -u) -eq 0 ]; then
    echo "$(basename "${0}"): must be run as root" >&2
    exit 1
fi

# Load custom environment variables from /etc/spectro/environment if it exists
if [ -f /etc/spectro/environment ]; then
    . /etc/spectro/environment
fi

# Ensure STYLUS_ROOT does not have a trailing slash
STYLUS_ROOT="${STYLUS_ROOT%/}"

# Set RKE2_DATA_DIR, defaulting to /var/lib/rancher/rke2 if STYLUS_ROOT is not set
RKE2_DATA_DIR=${STYLUS_ROOT}/var/lib/rancher/rke2

# Function to check if the target directory is a mountpoint
check_target_mountpoint() {
    mountpoint -q "$1"
}

# Function to check if the target directory is read-only
check_target_ro() {
    touch "$1"/.rke2-ro-test && rm -rf "$1"/.rke2-ro-test
    test $? -ne 0
}

# OS check and INSTALL_RKE2_ROOT setup
. /etc/os-release
if [ -r /etc/redhat-release ] || [ -r /etc/centos-release ] || [ -r /etc/oracle-release ] || [ -r /etc/amazon-linux-release ]; then
    # If redhat/oracle family OS is detected, check whether RKE2 was installed via yum or tar.
    if rpm -q rke2-common >/dev/null 2>&1; then
        INSTALL_RKE2_ROOT=${STYLUS_ROOT}/usr
    else
        INSTALL_RKE2_ROOT=${STYLUS_ROOT}/usr/local
    fi 
# Check if the OS is SUSE
elif [ "${ID_LIKE%%[ ]*}" = "suse" ]; then 
    if rpm -q rke2-common >/dev/null 2>&1; then
        INSTALL_RKE2_ROOT=${STYLUS_ROOT}/usr
        if [ -x /usr/sbin/transactional-update ]; then
            transactional_update="transactional-update -c --no-selfupdate -d run"
        fi
    elif check_target_mountpoint "${STYLUS_ROOT}/usr/local" || check_target_ro "${STYLUS_ROOT}/usr/local"; then
        INSTALL_RKE2_ROOT=${STYLUS_ROOT}/opt/rke2
    else
        INSTALL_RKE2_ROOT=${STYLUS_ROOT}/usr/local
    fi
# Default to /usr for other OSes
else
    INSTALL_RKE2_ROOT=${STYLUS_ROOT}/usr 
fi

# Uninstall killall script
uninstall_killall() {
    _killall="$(dirname "$0")/rke2-killall.sh"
    if [ -e "${_killall}" ]; then
        eval "${_killall}"
    fi
}

# Disable services
uninstall_disable_services() {
    if command -v systemctl >/dev/null 2>&1; then
        systemctl disable rke2-server || true
        systemctl disable rke2-agent || true
        systemctl reset-failed rke2-server || true
        systemctl reset-failed rke2-agent || true
        systemctl daemon-reload
    fi
}

# Remove files
uninstall_remove_files() {
    if [ -r /etc/redhat-release ] || [ -r /etc/centos-release ] || [ -r /etc/oracle-release ] || [ -r /etc/amazon-linux-release ]; then
        yum remove -y "rke2-*"
        rm -f ${STYLUS_ROOT}/etc/yum.repos.d/rancher-rke2*.repo
    fi

    if [ "${ID_LIKE%%[ ]*}" = "suse" ]; then
        if rpm -q rke2-common >/dev/null 2>&1; then
            uninstall_cmd="zypper remove -y rke2-server rke2-agent rke2-common rke2-selinux"
            if [ "${TRANSACTIONAL_UPDATE=false}" != "true" ] && [ -x /usr/sbin/transactional-update ]; then
                uninstall_cmd="transactional-update -c --no-selfupdate -d run $uninstall_cmd"
            fi
            $uninstall_cmd
            rm -f ${STYLUS_ROOT}/etc/zypp/repos.d/rancher-rke2*.repo
        fi
    fi

    $transactional_update find "${INSTALL_RKE2_ROOT}/lib/systemd/system" -name rke2-*.service -type f -delete
    $transactional_update find "${INSTALL_RKE2_ROOT}/lib/systemd/system" -name rke2-*.env -type f -delete
    find ${STYLUS_ROOT}/etc/systemd/system -name rke2-*.service -type f -delete
    $transactional_update rm -f "${INSTALL_RKE2_ROOT}/bin/rke2"
    $transactional_update rm -f "${INSTALL_RKE2_ROOT}/bin/rke2-killall.sh"
    $transactional_update rm -rf "${INSTALL_RKE2_ROOT}/share/rke2"
    
    # Removing directories with STYLUS_ROOT support
    rm -rf ${STYLUS_ROOT}/etc/rancher || true
    rm -rf ${STYLUS_ROOT}/etc/cni
    rm -rf ${STYLUS_ROOT}/opt/cni/bin
    rm -rf ${STYLUS_ROOT}/var/lib/kubelet || true
    rm -rf "${RKE2_DATA_DIR}"
    rm -d ${STYLUS_ROOT}/var/lib/rancher || true

    if type fapolicyd >/dev/null 2>&1; then
        if [ -f ${STYLUS_ROOT}/etc/fapolicyd/rules.d/80-rke2.rules ]; then
            rm -f ${STYLUS_ROOT}/etc/fapolicyd/rules.d/80-rke2.rules
        fi
        fagenrules --load
        systemctl try-restart fapolicyd
    fi
}

# Remove uninstall script
uninstall_remove_self() {
    $transactional_update rm -f "${INSTALL_RKE2_ROOT}/bin/rke2-uninstall.sh"
}

# Remove SELinux policies
uninstall_remove_policy() {
    semodule -r rke2 || true
}

uninstall_killall
trap uninstall_remove_self EXIT
uninstall_disable_services
uninstall_remove_files
uninstall_remove_policy
