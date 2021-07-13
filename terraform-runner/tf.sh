#!/bin/bash -e

version_gt_or_eq () {
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

case "$TFO_RUNNER" in
    init | init-delete)
        terraform init $module 2>&1 | tee "$out"/"$TFO_RUNNER".out
        ;;
    plan)
        terraform plan -var-file tfvars -out tfplan $module 2>&1 | tee "$out"/"$TFO_RUNNER".out
        ;;
    plan-delete)
        terraform plan -var-file tfvars -destroy -out tfplan $module 2>&1 | tee "$out"/"$TFO_RUNNER".out
        ;;
    apply | apply-delete)
        terraform apply tfplan 2>&1 | tee "$out"/"$TFO_RUNNER".out
        ;;
esac

exit ${PIPESTATUS[0]}