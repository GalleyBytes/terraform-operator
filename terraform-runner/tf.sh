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

exit ${PIPESTATUS[0]}