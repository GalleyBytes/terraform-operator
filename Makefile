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

PLUGIN_SET_AWS_SESSION = aws-session
PLUGIN_SET_AWS_SESSION_VER = 0.1.0
PLUGIN_SET_AWS_SESSION_NAME = kubectl-${PLUGIN_SET_AWS_SESSION}
PLUGIN_SET_AWS_SESSION_NAME_VER = ${PLUGIN_SET_AWS_SESSION_NAME}-v${PLUGIN_SET_AWS_SESSION_VER}

all: build

k8s-gen:
	operator-sdk generate k8s 
	operator-sdk generate crds

openapi-gen:
	./bin/openapi-gen --logtostderr=true -o "" -i ./pkg/apis/tf/v1alpha1 -O zz_generated.openapi -p ./pkg/apis/tf/v1alpha1 -h ./hack/boilerplate.go.txt -r "-"
	# If you're missing the openapi bin, download it using the following script:
	# wget -O ./bin/kube-openapi.zip https://github.com/kubernetes/kube-openapi/archive/master.zip
	# unzip ./bin/kubeopen-api.zip  ./bin
	# go build -o ./bin/openapi-gen ./bin/kube-openapi-master/cmd/openapi-gen/openapi-gen.go
	
docker-build:
	operator-sdk build ${DOCKER_REPO}/${IMAGE_NAME}:${VERSION}

docker-push:
	docker push ${DOCKER_REPO}/${IMAGE_NAME}:${VERSION}

docker-build-job:
	docker build -t ${DOCKER_REPO}/tfops:0.11.14 -f docker/terraform/terraform-0.11.14.Dockerfile docker/terraform/
	docker build -t ${DOCKER_REPO}/tfops:0.12.21 -f docker/terraform/terraform-0.12.21.Dockerfile docker/terraform/
	docker build -t ${DOCKER_REPO}/tfops:0.12.23 -f docker/terraform/terraform-0.12.23.Dockerfile docker/terraform/

docker-push-job:
	docker push ${DOCKER_REPO}/tfops:0.11.14
	docker push ${DOCKER_REPO}/tfops:0.12.21
	docker push ${DOCKER_REPO}/tfops:0.12.23

plugins-build:
	env GOOS=linux  GOARCH=amd64 go build -i -v -o bin/${PLUGIN_SET_AWS_SESSION_NAME}-linux  -ldflags="-X main.version=v${PLUGIN_SET_AWS_SESSION_VER}" plugins/${PLUGIN_SET_AWS_SESSION}/main.go
	env GOOS=darwin GOARCH=amd64 go build -i -v -o bin/${PLUGIN_SET_AWS_SESSION_NAME}-darwin -ldflags="-X main.version=v${PLUGIN_SET_AWS_SESSION_VER}" plugins/${PLUGIN_SET_AWS_SESSION}/main.go
	cd bin && mv ${PLUGIN_SET_AWS_SESSION_NAME}-linux  ${PLUGIN_SET_AWS_SESSION_NAME} && tar czf ${PLUGIN_SET_AWS_SESSION_NAME_VER}-linux.tgz  ${PLUGIN_SET_AWS_SESSION_NAME}
	cd bin && mv ${PLUGIN_SET_AWS_SESSION_NAME}-darwin ${PLUGIN_SET_AWS_SESSION_NAME} && tar czf ${PLUGIN_SET_AWS_SESSION_NAME_VER}-darwin.tgz ${PLUGIN_SET_AWS_SESSION_NAME}

plugins-push:
	curl -H "X-JFrog-Art-Api:${ARTIFACTORY_TOKEN}" -T bin/${PLUGIN_SET_AWS_SESSION_NAME_VER}-linux.tgz  "https://artifactory.isaaguilar.com/artifactory/bins/${PLUGIN_SET_AWS_SESSION_NAME}/${PLUGIN_SET_AWS_SESSION_NAME_VER}-linux.tgz"
	curl -H "X-JFrog-Art-Api:${ARTIFACTORY_TOKEN}" -T bin/${PLUGIN_SET_AWS_SESSION_NAME_VER}-darwin.tgz "https://artifactory.isaaguilar.com/artifactory/bins/${PLUGIN_SET_AWS_SESSION_NAME}/${PLUGIN_SET_AWS_SESSION_NAME_VER}-darwin.tgz"

deploy:
	kubectl delete pod --selector name=${DEPLOYMENT} --namespace ${NAMESPACE} && sleep 4
	kubectl logs -f --selector name=${DEPLOYMENT} --namespace ${NAMESPACE}

build: k8s-gen openapi-gen docker-build
build-all: build docker-build-job
push: docker-push
push-all: push docker-push-job
run: docker-build deploy

.PHONY: build push run docker-build docker-push deploy openapi-gen k8s-gen
