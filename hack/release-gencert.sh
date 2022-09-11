#!/bin/bash
set -o nounset
set -o errexit
set -o pipefail

export version="$1"
export DOCKER_REPO=${DOCKER_REPO:-"isaaguilar"}
export IMAGE_NAME=${IMAGE_NAME:-"terraform-operator-gencert"}
repo="$DOCKER_REPO/$IMAGE_NAME"


cd $(git rev-parse --show-toplevel)
existing_builds=($(
    page="https://registry.hub.docker.com/v2/repositories/$repo/tags/?page=1"
    while [[ $? -eq 0 ]]; do
        results=$(curl -s "$page")

        if [[ $(jq '.count' <<< "$results") -eq 0 ]]; then
            break
        fi

        jq -r '."results"[]["name"]' <<< "$results"
        page=`jq -r '.next//empty' <<< "$results"`
        if [[ -z "$page" ]]; then
            break
        fi
    done
))
if [[ " ${existing_builds[*]} " =~ " $version " ]]; then
    printf "\n\n----------------\n$IMG already exists\n"
    exit 0
fi

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -v -o projects/gencert/bin/gencert projects/gencert/main.go
docker build -t ${repo}:${version} -f projects/gencert/Dockerfile projects/gencert/
docker push ${repo}:${version}
