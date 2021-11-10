module github.com/isaaguilar/terraform-operator

replace (
	google.golang.org/grpc => google.golang.org/grpc v1.29.1
	k8s.io/kube-state-metrics => k8s.io/kube-state-metrics v1.6.0
	sigs.k8s.io/controller-tools => sigs.k8s.io/controller-tools v0.1.11-0.20190411181648-9d55346c2bde
)

replace github.com/docker/docker => github.com/moby/moby v0.7.3-0.20190826074503-38ab9da00309 // Required by Helm

replace github.com/openshift/api => github.com/openshift/api v0.0.0-20190924102528-32369d4db2ad // Required until https://github.com/operator-framework/operator-lifecycle-manager/pull/1241 is resolved

go 1.15

require (
	github.com/MakeNowJust/heredoc v0.0.0-20170808103936-bb23615498cd
	github.com/aws/aws-sdk-go v1.27.0 // indirect
	github.com/elliotchance/sshtunnel v1.1.1
	github.com/go-logr/logr v0.3.0
	github.com/go-openapi/spec v0.19.6
	github.com/gobuffalo/envy v1.7.1 // indirect
	github.com/hashicorp/go-getter v1.5.2
	github.com/hashicorp/go-version v1.2.0 // indirect
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	github.com/patrickmn/go-cache v2.1.0+incompatible
	github.com/rogpeppe/go-internal v1.4.0 // indirect
	go.uber.org/zap v1.15.0
	golang.org/x/crypto v0.0.0-20201002170205-7f63de1d35b0
	golang.org/x/tools v0.0.0-20201014231627-1610a49f37af // indirect
	google.golang.org/grpc v1.30.0 // indirect
	gopkg.in/src-d/go-git.v4 v4.13.1
	k8s.io/api v0.20.1
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.1
	k8s.io/kube-openapi v0.0.0-20201113171705-d219536bb9fd
	sigs.k8s.io/controller-runtime v0.8.0
	sigs.k8s.io/controller-tools v0.4.1
)
