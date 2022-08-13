#!/usr/bin/env zsh
set -o nounset
set -o errexit
set -o pipefail

UPGRADE_TYPE=${1:-""} # patch or minor or major
PROJECT="terraform-operator"
cd $(git rev-parse --show-toplevel)
now="$(date +%Y%m%d%H%M%S)"
gitshort=$(git rev-list HEAD --abbrev-commit --max-count 1)
repo="isaaguilar/$PROJECT"

LATEST_TAG="v0.0.0" # default for new projects or a fallback in case logic breaks
if [[ -z "$DESIRED_VERSION" ]]; then
if git tag --sort=version:refname --sort=-creatordate | head -n1; then
LATEST_TAG=$(git tag --sort=version:refname --sort=-creatordate | head -n1)
fi
fi

# Dev builds are usually for a new version that has not been tagged yet.
# In order to make this easy, allow an environment var to define the desired
# version.
CURRENT_VERSION=${DESIRED_VERSION:-LATEST_TAG}
if [[ "$CURRENT_VERSION" != "v"* ]]; then
exit 1
fi

CURRENT_VERSION=${CURRENT_VERSION#v}
CURRENT_VERSION=${CURRENT_VERSION%-*}
major=$(echo $CURRENT_VERSION|cut -d'.' -f1)
minor=$(echo $CURRENT_VERSION|cut -d'.' -f2)
patch=$(echo $CURRENT_VERSION|cut -d'.' -f3)
if [[ "$UPGRADE_TYPE" == "patch" ]]; then
patch=$(( $patch + 1 ))
elif [[ "$UPGRADE_TYPE" == "minor" ]]; then
patch="0"
minor=$(( $minor + 1 ))
elif [[ "$UPGRADE_TYPE" == "major" ]]; then
patch="0"
minor="0"
major=$(( $major + 1 ))
fi
build_version="v${major}.${minor}.${patch}-${now}-${gitshort}"
export IMG=$repo:$build_version
printf "Will build $repo:$build_version\n"
make build push

