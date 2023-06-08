#!/usr/bin/env zsh
set -o nounset
set -o errexit
set -o pipefail

cd $(git rev-parse --show-toplevel)
build_version="$(git describe --tags --dirty)-$(date +%Y%m%d%H%M%S)"
export IMG="ghcr.io/galleybytes/terraform-operator:$build_version"
printf "Will build $IMG\n"
make docker-build-local docker-push

