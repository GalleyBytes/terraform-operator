#!/bin/bash
## File is intended to run from the root of this repo
## Use with make as
##          make docker-build-job
##
echo "Fetching available hashicorp/terraform versions"
i=0
AVAILABLE_HASHICORP_TERRAFORM_IMAGES=($(
    while [[ $? -eq 0 ]]; do
        i=$((i+1))
        results=$(curl -s "https://registry.hub.docker.com/v2/repositories/hashicorp/terraform/tags/?page=$i")

        if [[ $(jq '.count' <<< "$results") -eq 0 ]];then
            break
        fi

        jq -r '."results"[]["name"]' <<< "$results"
    done
))

for TF_IMAGE in ${AVAILABLE_HASHICORP_TERRAFORM_IMAGES[@]};do
    major=$(cut -d'.' -f1 <<< $TF_IMAGE|sed -E "/^[a-zA-Z+]/d")
    minor=$(cut -d'.' -f2 <<< $TF_IMAGE|sed -E "/^[a-zA-Z+]/d")
    if [[ "$minor" -lt 11 ]] && [[ "$major" -eq 0 ]]  2>/dev/null;then
        continue
    fi

    echo "building $DOCKER_REPO/tfops:$TF_IMAGE"
    export TF_IMAGE
    docker build -t "$DOCKER_REPO/tfops:$TF_IMAGE" --build-arg TF_IMAGE=${TF_IMAGE} docker/terraform/

done
