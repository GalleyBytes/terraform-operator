#!/bin/bash -e

cd "$TFO_MAIN_MODULE"
out="$TFO_ROOT_PATH"/generations/$TFO_GENERATION
mkdir -p "$out"

case "$TFO_RUNNER" in
    init | init-delete)
        terraform init . 2>&1 | tee "$out"/"$TFO_RUNNER".out
        ;;
    plan)
        terraform plan -var-file tfvars -out tfplan . 2>&1 | tee "$out"/"$TFO_RUNNER".out
        ;;
    plan-delete)
        terraform plan -var-file tfvars -destroy -out tfplan . 2>&1 | tee "$out"/"$TFO_RUNNER".out
        ;;
    apply | apply-delete)
        terraform apply tfplan 2>&1 | tee "$out"/"$TFO_RUNNER".out
        ;;
esac

exit ${PIPESTATUS[0]}