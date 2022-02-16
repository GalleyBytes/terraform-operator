#!/bin/bash
##
HUB_TOKEN=$(curl -s -H "Content-Type: application/json" -X POST -d '{"username": "'${DOCKER_HUB_USER}'", "password": "'${DOCKER_HUB_PASSWORD}'"}' https://hub.docker.com/v2/users/login/ | jq -r .token)
## Requires:
##  DOCKER_REPO
##
export DOCKER_REPO=${DOCKER_REPO:-"isaaguilar"}

function cleanup {
    curl -X DELETE -H "Accept: application/json" -H "Authorization: JWT $HUB_TOKEN" "https://hub.docker.com/v2/repositories/$DOCKER_REPO/$1/tags/$2-amd64/"
    curl -X DELETE -H "Accept: application/json" -H "Authorization: JWT $HUB_TOKEN" "https://hub.docker.com/v2/repositories/$DOCKER_REPO/$1/tags/$2-arm64v8/"
}

##
## Build setup-runner
##
SETUP_RUNNER_IMAGE_NAME="setup-runner"
SETUP_RUNNER_TAG="1.1.3"
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
build=true
if [[ " ${BUILT_SETUP_RUNNER_IMAGES[*]} " =~ " $SETUP_RUNNER_TAG " ]];then
    IMAGES_IN_MANIFEST=$(curl -s https://registry.hub.docker.com/v2/repositories/$DOCKER_REPO/$SETUP_RUNNER_IMAGE_NAME/tags/$SETUP_RUNNER_TAG|jq '.images|length')
    if [[ "$IMAGES_IN_MANIFEST" -eq 2 ]];then
        printf "\n\n----------------\n$DOCKER_IMAGE already exists\n"
        build=false
    fi
fi
if [[ "$build" == "true" ]]; then
    printf "\n\n----------------\nBuilding $DOCKER_IMAGE\n"

    envsubst < setup.Dockerfile > temp
    docker build . -f temp -t "$DOCKER_IMAGE"-amd64 --build-arg ARCH=amd64
    docker push "$DOCKER_IMAGE"-amd64

    envsubst < setup-arm64.Dockerfile > temp
    docker build . -f temp -t "$DOCKER_IMAGE"-arm64v8 --build-arg ARCH=arm64v8
    docker push "$DOCKER_IMAGE"-arm64v8

    docker manifest create "$DOCKER_IMAGE" \
    --amend "$DOCKER_IMAGE"-amd64 \
    --amend "$DOCKER_IMAGE"-arm64v8

    docker manifest push "$DOCKER_IMAGE" && cleanup $SETUP_RUNNER_IMAGE_NAME $SETUP_RUNNER_TAG

fi


##
## Build script-runner
##
SCRIPT_RUNNER_IMAGE_NAME="script-runner"
SCRIPT_RUNNER_TAG="1.0.2"
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
build=true
if [[ " ${BUILT_SCRIPT_RUNNER_IMAGES[*]} " =~ " $SCRIPT_RUNNER_TAG " ]];then
    IMAGES_IN_MANIFEST=$(curl -s https://registry.hub.docker.com/v2/repositories/$DOCKER_REPO/$SCRIPT_RUNNER_IMAGE_NAME/tags/$SCRIPT_RUNNER_TAG|jq '.images|length')
    if [[ "$IMAGES_IN_MANIFEST" -eq 2 ]];then
        printf "\n\n----------------\n$DOCKER_IMAGE already exists\n"
        build=false
    fi
fi
if [[ "$build" == "true" ]]; then
    printf "\n\n----------------\nBuilding $DOCKER_IMAGE\n"

    envsubst < script.Dockerfile > temp
    docker build . -f temp -t "$DOCKER_IMAGE"-amd64
    docker push "$DOCKER_IMAGE"-amd64

    envsubst < script-arm64.Dockerfile > temp
    docker build . -f temp -t "$DOCKER_IMAGE"-arm64v8 --build-arg ARCH=arm64v8
    docker push "$DOCKER_IMAGE"-arm64v8

    docker manifest create "$DOCKER_IMAGE" \
    --amend "$DOCKER_IMAGE"-amd64 \
    --amend "$DOCKER_IMAGE"-arm64v8

    docker manifest push "$DOCKER_IMAGE" && cleanup $SCRIPT_RUNNER_IMAGE_NAME $SCRIPT_RUNNER_TAG
fi

## To terraform runners, fetch a list of already supported terraform images.
## Use this list to also build images for other distributions of linux, eg arm64
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

KNOWN_BROKEN_TAGS=( "0.11.3" "0.11.2" "0.11.0-beta1" "0.11.0" "0.11.1" "0.11.4" "0.11.5" "0.11.6" "0.11.7" )  # Generally due to base image not being compatible with the package installs


##
## Build tf-runner(s)
##
TF_RUNNER_IMAGE_NAME="tf-runner-v5beta1"
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
        IMAGES_IN_MANIFEST=$(curl -s https://registry.hub.docker.com/v2/repositories/$DOCKER_REPO/$TF_RUNNER_IMAGE_NAME/tags/$TF_IMAGE|jq '.images|length')
        if [[ "$IMAGES_IN_MANIFEST" -eq 2 ]];then
            printf "\n\n----------------\n$DOCKER_IMAGE already exists\n"
            continue
        fi
    fi
    printf "\n\n----------------\nBuilding $DOCKER_IMAGE\n"

    envsubst < tf.Dockerfile > temp
    docker build . -f temp -t "$DOCKER_IMAGE"-amd64 --build-arg ARCH=amd64
    docker push "$DOCKER_IMAGE"-amd64

    envsubst < tf-arm64.Dockerfile > temp
    docker build . -f temp -t "$DOCKER_IMAGE"-arm64v8 --build-arg ARCH=arm64v8
    docker push "$DOCKER_IMAGE"-arm64v8

    docker manifest create "$DOCKER_IMAGE" \
    --amend "$DOCKER_IMAGE"-amd64 \
    --amend "$DOCKER_IMAGE"-arm64v8

    docker manifest push "$DOCKER_IMAGE" && cleanup $TF_RUNNER_IMAGE_NAME $TF_IMAGE

done
