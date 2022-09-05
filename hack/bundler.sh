#!/bin/bash
# Usage:
#   /bin/bash hack/bundler.sh v0.9.0
#
# To be executed from root of repo
set -o errexit
set -o nounset
set -o pipefail

ver=$1
name="${2:-}"

bundle_dir="deploy/bundles/$ver"
mkdir -p $bundle_dir

bundle="$bundle_dir/${name:-$ver}.yaml"
cat /dev/null > $bundle
printf -- '---\n# namespace\n' >>  $bundle
cat deploy/namespace.yaml >> $bundle
printf -- '\n\n---\n# serviceaccount\n' >>  $bundle
cat deploy/serviceaccount.yaml >> $bundle
printf -- '\n\n---\n# service\n' >>  $bundle
cat deploy/service.yaml >> $bundle
printf -- '\n\n---\n# clusterrole\n' >>  $bundle
cat deploy/clusterrole.yaml >> $bundle
printf -- '\n\n---\n# clusterrolebinding\n' >>  $bundle
cat deploy/clusterrolebinding.yaml >> $bundle
printf -- '\n\n---\n# deployment\n' >>  $bundle
cat deploy/deployment.yaml >> $bundle
printf -- '\n\n---\n# crd\n' >>  $bundle
cat deploy/crds/tf.isaaguilar.com_terraforms_crd.yaml >> $bundle

>&2 printf "Saved "
printf "$bundle\n"