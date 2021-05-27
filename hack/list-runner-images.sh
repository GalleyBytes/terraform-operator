#!/bin/bash
#
dir=$(dirname $0)
doc="$dir/../docs/terraform-runners.md"

printf '# Terraform Runner Images

The default Terraform Runner is named `isaaguilar/tfops`. This can be changed by updating `spec.terraformRunner`. Note that the image name will be a composition of `spec.terraformRunner` and `spec.terraformVersion`. (eg `isaaguilar/tfops:0.13.7`)

The following is a list of `terraformRunner` versions available:

' > "$doc"

i=0
while [[ $? -eq 0 ]]; do
    i=$((i+1))
    results=$(curl -s "https://registry.hub.docker.com/v2/repositories/isaaguilar/tfops/tags/?page=$i")

    if [[ $(jq '.count' <<< "$results") -eq 0 ]];then
        break
    fi

    jq -r '."results"[]["name"]' <<< "$results"
done | sort -n |sed "s/^/- /g" >> "$doc"


