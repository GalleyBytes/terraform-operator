#!/bin/bash
set -o nounset
set -o errexit
set -o pipefail

export tag="${1#v}"
export host=${2:-"ghcr.io"}
export org=${3:-"galleybytes"}
export image_name=${4:-"terraform-operator-gencert"}

# Execute at root of terraform-operator repo
cd "$(git rev-parse --show-toplevel)"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -v -o projects/gencert/bin/gencert-amd64 projects/gencert/main.go
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 GO111MODULE=on go build -v -o projects/gencert/bin/gencert-arm64 projects/gencert/main.go

tmpdir="$(mktemp -d)/terraform-operator-tasks"
git clone https://github.com/GalleyBytes/terraform-operator-tasks.git "$tmpdir"

# Copy the dockerfiles to the build dir
cp -r projects/gencert/* "$tmpdir/images/"

cd "$tmpdir/images"
poetry install --no-root
poetry run python builder.py --org "$org" --image "$image_name" --tag "$tag" --host "$host" --release
