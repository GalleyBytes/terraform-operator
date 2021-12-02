#!/bin/bash
## File is intended to run from the root of this repo
## Use with make as
##          make docker-build-job
##
## Requires:
##  DOCKER_REPO
##
DOCKER_REPO=${DOCKER_REPO:-"isaaguilar"}

##
## Build setup-runner
##
SETUP_RUNNER_IMAGE_NAME="setup-runner"
SETUP_RUNNER_TAG="1.1.2"
export USER_UID=2000
export DOCKER_IMAGE="$DOCKER_REPO/$SETUP_RUNNER_IMAGE_NAME:$SETUP_RUNNER_TAG"
i=0
BUILT_SETUP_RUNNER_IMAGES=($(
    while [[ $? -eq 0 ]]; do
        i=$((i+1))
        results=$(curl -s "https://registry.hub.docker.com/v2/repositories/$DOCKER_REPO/$SETUP_RUNNER_IMAGE_NAME/tags/?page=$i")

        if [[ $(jq '.count' <<< "$results") -eq 0 ]];then
            break
        fi

        jq -r '."results"[]["name"]' <<< "$results"
    done
))
if [[ " ${BUILT_SETUP_RUNNER_IMAGES[*]} " =~ " $SETUP_RUNNER_TAG " ]];then
    printf "\n\n----------------\n$DOCKER_IMAGE already exists\n"
else
    printf "\n\n----------------\nBuilding $DOCKER_IMAGE\n"

    envsubst < setup.Dockerfile > temp
    docker build . -f temp -t "$DOCKER_IMAGE"
    docker push "$DOCKER_IMAGE"
fi

##
## Build script-runner
##
SCRIPT_RUNNER_IMAGE_NAME="setup-runner"
SCRIPT_RUNNER_TAG="1.0.0"
export USER_UID=2000
export DOCKER_IMAGE="$DOCKER_REPO/$SCRIPT_RUNNER_IMAGE_NAME:$SCRIPT_RUNNER_TAG"
i=0
BUILT_SCRIPT_RUNNER_IMAGES=($(
    while [[ $? -eq 0 ]]; do
        i=$((i+1))
        results=$(curl -s "https://registry.hub.docker.com/v2/repositories/$DOCKER_REPO/$SCRIPT_RUNNER_IMAGE_NAME/tags/?page=$i")

        if [[ $(jq '.count' <<< "$results") -eq 0 ]];then
            break
        fi

        jq -r '."results"[]["name"]' <<< "$results"
    done
))
if [[ " ${BUILT_SCRIPT_RUNNER_IMAGES[*]} " =~ " $SCRIPT_RUNNER_TAG " ]];then
    printf "\n\n----------------\n$DOCKER_IMAGE already exists\n"
else
    printf "\n\n----------------\nBuilding $DOCKER_IMAGE\n"

    envsubst < script.Dockerfile > temp
    docker build . -f temp -t "$DOCKER_IMAGE"
    docker push "$DOCKER_IMAGE"
fi

##
## Build tf-runner(s)
##
TF_RUNNER_IMAGE_NAME="tf-runner-v5alpha3"
printf "\n\n----------------\nFetching available hashicorp/terraform versions"
i=0
BUILT_TF_RUNNER_IMAGES=($(
    while [[ $? -eq 0 ]]; do
        i=$((i+1))
        results=$(curl -s "https://registry.hub.docker.com/v2/repositories/$DOCKER_REPO/$TF_RUNNER_IMAGE_NAME/tags/?page=$i")

        if [[ $(jq '.count' <<< "$results") -eq 0 ]];then
            break
        fi

        jq -r '."results"[]["name"]' <<< "$results"
    done
))

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

KNOWN_BROKEN_TAGS=( "0.11.3" "0.11.2" )  # Generally due to base image not being compatible with the package installs

for TF_IMAGE in ${AVAILABLE_HASHICORP_TERRAFORM_IMAGES[@]};do
    major=$(cut -d'.' -f1 <<< $TF_IMAGE|sed -E "/^[a-zA-Z+]/d")
    minor=$(cut -d'.' -f2 <<< $TF_IMAGE|sed -E "/^[a-zA-Z+]/d")
    if [[ "$minor" -lt 11 ]] && [[ "$major" -eq 0 ]]  2>/dev/null;then
        continue
    fi
    if [[ " ${KNOWN_BROKEN_TAGS[*]} " =~ " $TF_IMAGE " ]];then
        continue
    fi
    export TF_IMAGE
    export USER_UID=2000
    export DOCKER_IMAGE="$DOCKER_REPO/$TF_RUNNER_IMAGE_NAME:$TF_IMAGE"
    if [[ " ${BUILT_TF_RUNNER_IMAGES[*]} " =~ " $TF_IMAGE " ]];then
        printf "\n\n----------------\n$DOCKER_IMAGE already exists\n"
        continue
    fi
    printf "\n\n----------------\nBuilding $DOCKER_IMAGE\n"

    envsubst < tf.Dockerfile > temp
    docker build . -f temp -t "$DOCKER_IMAGE"
    docker push "$DOCKER_IMAGE"
done
