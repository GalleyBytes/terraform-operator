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
export USER_UID=2000
export DOCKER_IMAGE="$DOCKER_REPO/setup-runner-alphav5:1.0.0"
printf "\n\n----------------\nBuilding $DOCKER_IMAGE\n"

envsubst < setup.Dockerfile > temp
docker build . -f temp -t "$DOCKER_IMAGE"
docker push "$DOCKER_IMAGE"

##
## Build script-runner
##
export USER_UID=2000
export DOCKER_IMAGE="$DOCKER_REPO/script-runner-alphav4:1.0.0"
printf "\n\n----------------\nBuilding $DOCKER_IMAGE\n"

envsubst < script.Dockerfile > temp
docker build . -f temp -t "$DOCKER_IMAGE"
docker push "$DOCKER_IMAGE"

##
## Build tf-runner(s)
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

    # (($(docker images hashicorp/terraform:$TF_IMAGE --format='{{ .Repository }}{{ .Tag }}'|wc -l) > 0)) && continue
    export TF_IMAGE
    export USER_UID=2000
    export DOCKER_IMAGE="$DOCKER_REPO/tf-runner-alphav4:$TF_IMAGE"
    printf "\n\n----------------\nBuilding $DOCKER_IMAGE\n"

    envsubst < tf.Dockerfile > temp
    docker build . -f temp -t "$DOCKER_IMAGE"
    docker push "$DOCKER_IMAGE"
done


# AVAILABLE_TFOPS_IMAGES=($(
#     while [[ $? -eq 0 ]]; do
#         i=$((i+1))
#         results=$(curl -s "https://registry.hub.docker.com/v2/repositories/isaaguilar/tfops/tags/?page=$i")

#         if [[ $(jq '.count' <<< "$results") -eq 0 ]];then
#             break
#         fi

#         jq -r '."results"[]["name"]' <<< "$results"
#     done
# ))
