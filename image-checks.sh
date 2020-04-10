#!/bin/bash
# This script runs some very basic commands to ensure that the newly build
# images are working correctly. Invoke as:
# ./image-checks.sh <image-tag> <registry-name>
TAG=$1
REGISTRY=${2:-gcr.io/google-containers}
echo ${REGISTRY}
docker run --rm -it --entrypoint=iptables ${REGISTRY}/k8s-dns-node-cache:${TAG}
docker run --rm -it --entrypoint=/dnsmasq-nanny ${REGISTRY}/k8s-dns-dnsmasq-nanny:${TAG}
docker run --rm -it --entrypoint=/kube-dns ${REGISTRY}/k8s-dns-kube-dns:${TAG}
docker run --rm -it --entrypoint=/sidecar ${REGISTRY}/k8s-dns-sidecar:${TAG}
