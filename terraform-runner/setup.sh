#!/bin/bash -e

# Setup SSH
mkdir -p "$TFO_ROOT_PATH"/.ssh/
if stat "$TFO_SSH"/* >/dev/null 2>/dev/null; then
  cp -Lr "$TFO_SSH"/* "$TFO_ROOT_PATH"/.ssh/
  chmod -R 0600 "$TFO_ROOT_PATH"/.ssh/*
fi

out="$TFO_ROOT_PATH"/generations/$TFO_GENERATION
vardir="$out/tfvars"
mkdir -p "$out"
mkdir -p "$vardir"

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

function join_by {
	local d="$1" f=${2:-$(</dev/stdin)};
	if [[ -z "$f" ]]; then return 1; fi
	if shift 2; then
		printf %s "$f" "${@/#/$d}"
	else
	  join_by "$d" $f
	fi
}

function add_file_as_next_index {
    dir="$1"
    file="$2"
    idx=$(ls "$dir"|wc -l)
    cp "$file" "$dir/${idx}_$(basename $2)"
}

function fetch_git {
	temp="$(mktemp -d)"
	repo="$1"
	relpath="$2"
	files="$3"
    tfvar="$4"
	path="$TFO_MAIN_MODULE/$relpath"
	if [[ "$files" == "." ]] && ( [[ "$relpath" == "." ]] || [[ "$relpath" == "" ]] ); then
	  a=$(basename $repo)
	  path="$TFO_MAIN_MODULE/${a%.*}"
	fi
	# printf -- 'mkdir -p "'$path'"\n'
	mkdir -p "$path"
	# printf -- 'git clone "'$repo'" "'$temp'"\n'
	echo "Downloading resources from $repo"
	git clone "$repo" "$temp"
	cd "$temp" # All files are relative to the root of the repo
	# printf -- "cp -r $files $path\n"
	cp -r $files $path

    if [[ "$tfvar" == "true" ]]; then
        for file in $files; do
          if [[ -f "$file" ]]; then
            # printf 'add_file_as_next_index "'$vardir'" "'$file'"'
            add_file_as_next_index "$vardir" "$file"
          fi
        done
    fi
    echo done
}

FILE="/tmp/downloads/.__TFO__ResourceDownloads.json"
LENGTH=$(jq '.|length' $FILE)

for i in $(seq 0 $((LENGTH - 1))); do
    DATA=$(mktemp)
	jq --argjson i $i '.[$i]' $FILE > $DATA
	fetchtype=$(jq -r '.detect' $DATA)
	repo=$(jq -r '.repo' $DATA)
	files=$(jq -r '.files[]' $DATA | join_by "  ")
	path=$(jq -r '.path' $DATA)
    tfvar=$(jq -r '.useAsVar' $DATA)
	if [[ "$fetchtype" == "git" ]];then
		fetch_git "$repo" "$path" "$files" "$tfvar"
	fi
done