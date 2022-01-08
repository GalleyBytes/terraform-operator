#!/bin/bash -e

function join_by {
  local d="$1" f=${2:-$(</dev/stdin)};
  if [[ -z "$f" ]]; then return 1; fi
  if shift 2; then
    printf %s "$f" "${@/#/$d}"
  else
    join_by "$d" $f
  fi
}

function version_gt_or_eq {
  if [ "$(printf '%s\n' "$1" "$2" | sort -V | head -n1)" = "$1" ]; then
    return 0
  else
    return 1
  fi
}
terraform_version=$(terraform version | head -n1 |sed "s/^.*v//")
module=""
if ! version_gt_or_eq "0.15.0" "$terraform_version"; then
  module="."
fi

cd "$TFO_MAIN_MODULE"
out="$TFO_ROOT_PATH"/generations/$TFO_GENERATION
mkdir -p "$out"
vardir="$out/tfvars"
vars=
if [[ $(ls $vardir | wc -l) -gt 0 ]]; then
  vars="-var-file $(find $vardir -type f | sort -n | join_by ' -var-file ')"
fi

case "$TFO_RUNNER" in
    init | init-delete)
        terraform init $module 2>&1 | tee "$out"/"$TFO_RUNNER".out
        ;;
    plan)
        terraform plan $vars -out tfplan $module 2>&1 | tee "$out"/"$TFO_RUNNER".out
        ;;
    plan-delete)
        terraform plan $vars -destroy -out tfplan $module 2>&1 | tee "$out"/"$TFO_RUNNER".out
        ;;
    apply | apply-delete)
        terraform apply tfplan 2>&1 | tee "$out"/"$TFO_RUNNER".out
        ;;
esac
status=${PIPESTATUS[0]}
if [[ $status -gt 0 ]];then exit $status;fi

if [[ "$TFO_RUNNER" == "apply" ]] && [[ "$TFO_SAVE_OUTPUTS" == "true" ]]; then
  # On sccessful apply, save outputs as k8s-secret
  data=$(mktemp)
  printf '[
    {"op":"replace","path":"/data","value":{}}
  ]' > "$data"
  t=$(mktemp)
  include=( $(echo "$TFO_OUTPUTS_TO_INCLUDE" | tr "," " ") )
  omit=( $(echo "$TFO_OUTPUTS_TO_OMIT" | tr "," " ") )
  jsonoutput=$(terraform output -json)
  keys=( $(jq -r '.|keys[]' <<< $jsonoutput) )
  for key in ${keys[@]}; do
    if [[ "${#include[@]}" -gt 0 ]] && [[ ! " ${include[*]} " =~ " $key " ]]; then
      echo "Skipping $key"
      continue
    fi
    if [[ "${#omit[@]}" -gt 0 ]] && [[ " ${omit[*]} " =~ " $key " ]]; then
      echo "Omitting $key"
      continue
    fi
    b64value=$(jq -r --arg key $key '.[$key]' <<< $jsonoutput|base64|tr -d '[:space:]')
    jq -Mc --arg key $key --arg value $b64value '. += [
      {"op":"add","path":"/data/\($key)","value":"\($value)"}
    ]' "$data" > "$t"
    cp "$t" "$data"
  done
  kubectl patch secret "$TFO_OUTPUTS_SECRET_NAME" --type json --patch-file "$data"
fi