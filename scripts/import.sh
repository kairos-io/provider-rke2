#!/bin/bash -x

CONTENT_PATH=$1
mkdir -p /var/lib/rancher/rke2/agent/images
for tarfile in $(find $CONTENT_PATH -name "*.tar" -type f)
do
  cp $tarfile /var/lib/rancher/rke2/agent/images
done