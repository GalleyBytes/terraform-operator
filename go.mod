module github.com/isaaguilar/terraform-operator

require (
	github.com/Azure/azure-sdk-for-go v33.4.0+incompatible // indirect
	github.com/NYTimes/gziphandler v1.0.1 // indirect
	github.com/aliyun/alibaba-cloud-sdk-go v0.0.0-20190329064014-6e358769c32a // indirect
	github.com/aliyun/aliyun-oss-go-sdk v0.0.0-20190103054945-8205d1f41e70 // indirect
	github.com/aliyun/aliyun-tablestore-go-sdk v4.1.2+incompatible // indirect
	github.com/apparentlymart/go-cidr v1.0.1 // indirect
	github.com/apparentlymart/go-dump v0.0.0-20190214190832-042adf3cf4a0 // indirect
	github.com/armon/circbuf v0.0.0-20190214190532-5111143e8da2 // indirect
	github.com/armon/go-radix v1.0.0 // indirect
	github.com/aws/aws-sdk-go v1.22.0 // indirect
	github.com/baiyubin/aliyun-sts-go-sdk v0.0.0-20180326062324-cfa1a18b161f // indirect
	github.com/blang/semver v3.5.1+incompatible // indirect
	github.com/bmatcuk/doublestar v1.1.5 // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/chzyer/readline v0.0.0-20180603132655-2972be24d48e // indirect
	github.com/chzyer/test v0.0.0-20180213035817-a1ea475d72b1 // indirect
	github.com/cloudfoundry/go-socks5 v0.0.0-20180221174514-54f73bdb8a8e // indirect
	github.com/dnaeon/go-vcr v0.0.0-20180920040454-5637cf3d8a31 // indirect
	github.com/dylanmei/winrmtest v0.0.0-20190225150635-99b7fe2fddf1 // indirect
	github.com/elliotchance/sshtunnel v1.0.0
	github.com/go-openapi/spec v0.19.0
	github.com/googleapis/gax-go v2.0.0+incompatible // indirect
	github.com/hashicorp/go-azure-helpers v0.0.0-20190129193224-166dfd221bb2 // indirect
	github.com/hashicorp/go-checkpoint v0.5.0 // indirect
	github.com/hashicorp/go-getter v1.4.0
	github.com/hashicorp/go-hclog v0.0.0-20181001195459-61d530d6c27f // indirect
	github.com/hashicorp/go-msgpack v0.5.4 // indirect
	github.com/hashicorp/go-plugin v1.0.1-0.20190610192547-a1bc61569a26 // indirect
	github.com/hashicorp/go-rootcerts v1.0.0 // indirect
	github.com/hashicorp/go-tfe v0.3.16 // indirect
	github.com/hashicorp/hcl v1.0.0
	github.com/hashicorp/hil v0.0.0-20190212112733-ab17b08d6590 // indirect
	github.com/hashicorp/logutils v1.0.0 // indirect
	github.com/hashicorp/memberlist v0.1.0 // indirect
	github.com/hashicorp/terraform v0.11.14
	github.com/hashicorp/terraform-config-inspect v0.0.0-20190821133035-82a99dc22ef4 // indirect
	github.com/hashicorp/vault v0.10.4 // indirect
	github.com/isaaguilar/socks5-proxy v0.3.0
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0 // indirect
	github.com/lib/pq v1.0.0 // indirect
	github.com/marstr/guid v1.1.0 // indirect
	github.com/masterzen/winrm v0.0.0-20190223112901-5e5c9a7fe54b // indirect
	github.com/mattn/go-colorable v0.1.1 // indirect
	github.com/mattn/go-shellwords v1.0.4 // indirect
	github.com/mitchellh/cli v1.0.0 // indirect
	github.com/mitchellh/colorstring v0.0.0-20190213212951-d06e56a500db // indirect
	github.com/mitchellh/copystructure v1.0.0 // indirect
	github.com/mitchellh/go-linereader v0.0.0-20190213213312-1b945b3263eb // indirect
	github.com/mitchellh/hashstructure v1.0.0 // indirect
	github.com/mitchellh/panicwrap v0.0.0-20190213213626-17011010aaa4 // indirect
	github.com/mitchellh/prefixedio v0.0.0-20190213213902-5733675afd51 // indirect
	github.com/operator-framework/operator-sdk v0.10.1-0.20190829173600-c0947baa1fed
	github.com/pkg/browser v0.0.0-20180916011732-0a3d74bf9ce4 // indirect
	github.com/posener/complete v1.2.1 // indirect
	github.com/shopspring/decimal v0.0.0-20190905144223-a36b5d85f337 // indirect
	github.com/spf13/pflag v1.0.3
	github.com/ugorji/go v1.1.1 // indirect
	github.com/vmihailenco/msgpack v4.0.1+incompatible // indirect
	github.com/whilp/git-urls v0.0.0-20160530060445-31bac0d230fa
	github.com/zclconf/go-cty v1.0.1-0.20190708163926-19588f92a98f // indirect
	github.com/zclconf/go-cty-yaml v1.0.1 // indirect
	golang.org/x/crypto v0.0.0-20190701094942-4def268fd1a4
	golang.org/x/net v0.0.0-20190724013045-ca1201d0de80
	golang.org/x/sys v0.0.0-20190804053845-51ab0e2deafa // indirect
	gopkg.in/ini.v1 v1.42.0 // indirect
	gopkg.in/src-d/go-git.v4 v4.13.1
	k8s.io/api v0.0.0-20190612125737-db0771252981
	k8s.io/apimachinery v0.0.0-20190612125636-6a5db36e93ad
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/kube-openapi v0.0.0-20190603182131-db7b694dc208
	sigs.k8s.io/controller-runtime v0.1.12
	sigs.k8s.io/controller-tools v0.1.10
)

// Pinned to kubernetes-1.13.4
replace (
	k8s.io/api => k8s.io/api v0.0.0-20190222213804-5cb15d344471
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.0.0-20190228180357-d002e88f6236
	k8s.io/apimachinery => k8s.io/apimachinery v0.0.0-20190221213512-86fb29eff628
	k8s.io/client-go => k8s.io/client-go v0.0.0-20190228174230-b40b2a5939e4
)

replace (
	github.com/coreos/prometheus-operator => github.com/coreos/prometheus-operator v0.29.0
	// Pinned to v2.9.2 (kubernetes-1.13.1) so https://proxy.golang.org can
	// resolve it correctly.
	github.com/prometheus/prometheus => github.com/prometheus/prometheus v0.0.0-20190424153033-d3245f150225
	k8s.io/kube-state-metrics => k8s.io/kube-state-metrics v1.6.0
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.1.12
	sigs.k8s.io/controller-tools => sigs.k8s.io/controller-tools v0.1.11-0.20190411181648-9d55346c2bde
)

replace github.com/operator-framework/operator-sdk => github.com/operator-framework/operator-sdk v0.10.0
