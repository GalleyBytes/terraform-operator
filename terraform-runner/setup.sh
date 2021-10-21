#!/bin/sh -e

# Setup SSH
mkdir -p "$TFO_ROOT_PATH"/.ssh/
if stat "$TFO_SSH"/* >/dev/null 2>/dev/null; then
  cp -Lr "$TFO_SSH"/* "$TFO_ROOT_PATH"/.ssh/
  chmod -R 0600 "$TFO_ROOT_PATH"/.ssh/*
fi

out="$TFO_ROOT_PATH"/generations/$TFO_GENERATION
mkdir -p "$out"

if [[ -d "$TFO_MAIN_MODULE" ]]; then
    rm -rf "$TFO_MAIN_MODULE"
fi
MAIN_MODULE_TMP=`mktemp -d`
git clone "$TFO_MAIN_MODULE_REPO" "$MAIN_MODULE_TMP/stack" || exit $?
cd "$MAIN_MODULE_TMP/stack"
git checkout "$TFO_MAIN_MODULE_REPO_REF"
cp -r "$TFO_MAIN_MODULE_REPO_SUBDIR" "$TFO_MAIN_MODULE"

# Get configmap and secret files and drop them in the main module's root path
# Do not overwrite configmap
false |  cp -iLr "$TFO_DOWNLOADS"/* "$TFO_MAIN_MODULE" 2>/dev/null

cd "$TFO_MAIN_MODULE"

# Load a custom backend
if stat backend_override.tf >/dev/null 2>/dev/null; then
    echo "Using custom backend"
else
    echo "Loading hashicorp backend"
    set -x
    envsubst < /backend.tf > "$TFO_ROOT_PATH/backend_override.tf"
    mv "$TFO_ROOT_PATH/backend_override.tf" .
fi
