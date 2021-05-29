#!/bin/bash -e

cd "$TFO_MAIN_MODULE"
out="$TFO_ROOT_PATH"/generations/$TFO_GENERATION
mkdir -p "$out"

# The $TFO_SCRIPT executes from the $TFO_MAIN_MODULE dir
chmod +x "$TFO_SCRIPT"
./"$TFO_SCRIPT" | tee "$out"/"$TFO_SCRIPT".out
exit ${PIPESTATUS[0]}