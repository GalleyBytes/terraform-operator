PKG ?= github.com/isaaguilar/terraform-operator
DOCKER_REPO ?= isaaguilar
IMAGE_NAME ?= terraform-operator
DEPLOYMENT ?= ${IMAGE_NAME}
NAMESPACE ?= tf-system
VERSION ?= $(shell git ls-remote .|grep $$(git rev-parse HEAD).*tags|head -n1|sed "s/^.*\///")
ifeq ($(VERSION),)
VERSION := v0.0.0
endif
OS := $(shell uname -s | tr A-Z a-z)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: build

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.3.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

openapi-gen-bin:
ifeq (, $(shell which openapi-gen))
	@{ \
	set -e ;\
	OPENAPI_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$OPENAPI_GEN_TMP_DIR ;\
	wget -qO kube-openapi.zip https://github.com/kubernetes/kube-openapi/archive/master.zip  ;\
	unzip ./kube-openapi.zip  ;\
	cd kube-openapi-master ;\
	go build -o $(GOBIN)/openapi-gen cmd/openapi-gen/openapi-gen.go ;\
	rm -rf $$OPENAPI_GEN_TMP_DIR ;\
	}
OPENAPI_GEN=$(GOBIN)/openapi-gen
else
OPENAPI_GEN=$(shell which openapi-gen)
endif


client-gen-bin:
ifeq (, $(shell which client-gen))
	@{ \
	set -e ;\
	CLIENT_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CLIENT_GEN_TMP_DIR ;\
	git clone https://github.com/kubernetes/code-generator.git ;\
	cd code-generator ;\
	go install ./cmd/client-gen ;\
	rm -rf $$CLIENT_GEN_TMP_DIR ;\
	}
CLIENT_GEN=$(GOBIN)/client-gen
else
CLIENT_GEN=$(shell which client-gen)
endif


# rbac:roleName=manager-role
# Generate manifests e.g. CRD, RBAC etc.
crds: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) paths="./..." output:crd:stdout > deploy/crds/tf.isaaguilar.com_terraforms_crd.yaml

generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

openapi-gen: openapi-gen-bin
	$(OPENAPI_GEN) --logtostderr=true -o "" -i ./pkg/apis/tf/v1alpha1 -O zz_generated.openapi -p ./pkg/apis/tf/v1alpha1 -h ./hack/boilerplate.go.txt -r "-"

client-gen: client-gen-bin
	$(CLIENT_GEN) -n versioned --input-base ""  --input ${PKG}/pkg/apis/tf/v1alpha1 -p ${PKG}/pkg/client/clientset -h ./hack/boilerplate.go.txt

k8s-gen: crds generate openapi-gen client-gen

docker-build:
	docker build -t ${DOCKER_REPO}/${IMAGE_NAME}:${VERSION} -f build/Dockerfile .

docker-build-local:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -v -o build/_output/manager cmd/manager/main.go
	docker build -t ${DOCKER_REPO}/${IMAGE_NAME}:${VERSION} -f build/Dockerfile.local build/

docker-push:
	docker push ${DOCKER_REPO}/${IMAGE_NAME}:${VERSION}

docker-build-job:
	DOCKER_REPO=${DOCKER_REPO} /bin/bash docker/terraform/build.sh

docker-push-job:
	docker images ${DOCKER_REPO}/tfops --format '{{ .Repository }}:{{ .Tag }}'| grep -v '<none>'|xargs -n1 -t docker push

deploy:
	kubectl delete pod --selector name=${DEPLOYMENT} --namespace ${NAMESPACE} && sleep 4
	kubectl logs -f --selector name=${DEPLOYMENT} --namespace ${NAMESPACE}

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

install: crds
	kubectl apply -f deploy/crds/tf.isaaguilar.com_terraforms_crd.yaml


# Run against the configured Kubernetes cluster in ~/.kube/config
run: fmt vet
	go run cmd/manager/main.go --max-concurrent-reconciles 10

# Run tests
ENVTEST_ASSETS_DIR=$(shell pwd)/testbin
test: openapi-gen fmt vet crds
	mkdir -p ${ENVTEST_ASSETS_DIR}
	test -f ${ENVTEST_ASSETS_DIR}/setup-envtest.sh || curl -sSLo ${ENVTEST_ASSETS_DIR}/setup-envtest.sh https://raw.githubusercontent.com/kubernetes-sigs/controller-runtime/v0.7.0/hack/setup-envtest.sh
	source ${ENVTEST_ASSETS_DIR}/setup-envtest.sh; fetch_envtest_tools $(ENVTEST_ASSETS_DIR); setup_envtest_env $(ENVTEST_ASSETS_DIR); go test ./... -coverprofile cover.out

build: k8s-gen openapi-gen docker-build-local
build-all: build docker-build-job
push: docker-push
push-all: push docker-push-job

.PHONY: build push run install fmt vet docker-build docker-build-local docker-push deploy openapi-gen k8s-gen crds contoller-gen client-gen
