#!/bin/bash
function join_by {
	local d="$1" f=${2:-$(</dev/stdin)};
	if [[ -z "$f" ]]; then return 1; fi
	if shift 2; then
		printf %s "$f" "${@/#/$d}"
	else
	  join_by "$d" $f
	fi
}

set -o nounset
set -o errexit
set -o pipefail

if [[ -d "$TFO_EXPORT_REPO_PATH" ]]; then
    rm -rf "$TFO_EXPORT_REPO_PATH"
fi
git clone "$TFO_EXPORT_REPO" "$TFO_EXPORT_REPO_PATH" || exit $?
cd "$TFO_EXPORT_REPO_PATH"
git checkout "$TFO_EXPORT_REPO_REF"
git config --global user.email "$TFO_EXPORT_REPO_AUTOMATED_USER_EMAIL"
git config --global user.name "$TFO_EXPORT_REPO_AUTOMATED_USER_NAME"

if [[ -n "$TFO_EXPORT_TFVARS_PATH" ]]; then
    mkdir -p $(dirname "$TFO_EXPORT_TFVARS_PATH")
    cd "$TFO_GENERATION_PATH/tfvars"
    if [[ -n "$(ls -A .)" ]]; then
        tfvars=$(tfvar-consolidate --var-file $(ls|join_by ',') --use-envs 2>/dev/null)
    else
        tfvars=$(tfvar-consolidate --use-envs 2>/dev/null)
    fi
    cd "$TFO_EXPORT_REPO_PATH"
    mv "$tfvars" "$TFO_EXPORT_TFVARS_PATH"
    git add "$TFO_EXPORT_TFVARS_PATH"
fi
if [[ -n "$TFO_EXPORT_CONF_PATH" ]]; then
    mkdir -p $(dirname "$TFO_EXPORT_CONF_PATH")
    conf=$(tfvar-consolidate --backend "$TFO_MAIN_MODULE/backend_override.tf" 2>/dev/null)
    mv "$conf" "$TFO_EXPORT_CONF_PATH"
    git add "$TFO_EXPORT_CONF_PATH"
fi

RET=0
git commit \
    -m "auto update of tfvars for $TFO_NAMESPACE/$TFO_RESOURCE" \
    -m "Updated by $TFO_NAMESPACE/$HOSTNAME" \
    >/tmp/msg 2>/tmp/err || RET=$?

cat /tmp/msg
if [[ $RET -gt 0 ]]; then
    cat /tmp/err
    grep "nothing to commit, working tree clean" /tmp/msg >/dev/null 2>/dev/null && exit 0 || exit $RET
    exit 2
fi

git push origin "$TFO_EXPORT_REPO_REF"
