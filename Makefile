PKG ?= github.com/isaaguilar/terraform-operator
DOCKER_REPO ?= isaaguilar
IMAGE_NAME ?= terraform-operator
DEPLOYMENT ?= ${IMAGE_NAME}
NAMESPACE ?= tf-system
VERSION ?= $(shell if git status |grep "nothing to commit\|nothing added to commit" >/dev/null;then git tag --points-at HEAD|cat; fi)
ifeq ($(VERSION),)
VERSION := v0.0.0
endif
OS := $(shell uname -s | tr A-Z a-z)

all: build

k8s-gen:
	operator-sdk generate k8s 
	operator-sdk generate crds

openapi-gen:
	# If you're missing the openapi bin (eg '[openapi-gen] Error 127'), download it using the following script:
	# wget -O ./bin/kube-openapi.zip https://github.com/kubernetes/kube-openapi/archive/master.zip
	# unzip ./bin/kube-openapi.zip  ./bin
	# go build -o ./bin/openapi-gen ./bin/kube-openapi-master/cmd/openapi-gen/openapi-gen.go
	./bin/openapi-gen --logtostderr=true -o "" -i ./pkg/apis/tf/v1alpha1 -O zz_generated.openapi -p ./pkg/apis/tf/v1alpha1 -h ./hack/boilerplate.go.txt -r "-"
	
	
docker-build:
	operator-sdk build ${DOCKER_REPO}/${IMAGE_NAME}:${VERSION}

docker-push:
	docker push ${DOCKER_REPO}/${IMAGE_NAME}:${VERSION}

docker-build-job:
	DOCKER_REPO=${DOCKER_REPO} /bin/bash docker/terraform/build.sh

docker-push-job:
	docker images ${DOCKER_REPO}/tfops --format '{{ .Repository }}:{{ .Tag }}'| grep -v '<none>'|xargs -n1 -t docker push

deploy:
	kubectl delete pod --selector name=${DEPLOYMENT} --namespace ${NAMESPACE} && sleep 4
	kubectl logs -f --selector name=${DEPLOYMENT} --namespace ${NAMESPACE}

build: k8s-gen openapi-gen docker-build
build-all: build docker-build-job
push: docker-push
push-all: push docker-push-job
run: docker-build deploy

.PHONY: build push run docker-build docker-push deploy openapi-gen k8s-gen
