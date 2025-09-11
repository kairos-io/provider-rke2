#!/bin/sh -x

CONTENT_PATH=$1
mkdir -p /var/lib/rancher/rke2/agent/images
find -L "$CONTENT_PATH" -name "*.tar" -type f | while read -r tarfile; do
  cp $tarfile /var/lib/rancher/rke2/agent/images
done
