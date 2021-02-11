package v1alpha1

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Terraform is the Schema for the terraforms API
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
type Terraform struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TerraformSpec   `json:"spec,omitempty"`
	Status TerraformStatus `json:"status,omitempty"`
}

// TerraformSpec defines the desired state of Terraform
// +k8s:openapi-gen=true
type TerraformSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html

	// TerraformVersion helps the operator decide which image tag to pull for
	// the terraform runner. Defaults to "0.11.14"
	TerraformVersion string `json:"terraformVersion,omitempty"`

	// TerraformRunner gives the user the ability to inject their own container
	// image to execute terraform. This is very helpful for users who need to
	// have a certain toolset installed on their images, or who can't pull
	// public images, such as the default image "isaaguilar/tfops".
	TerraformRunner string `json:"terraformRunner,omitempty"`

	// TerraformModule is the terraform module scm address. Currently supports
	// git protocol over SSH or HTTPS
	TerraformModule *SrcOpts `json:"terraformModule"`

	Sources []*SrcOpts `json:"sources,omitempty"`
	Env     []EnvVar   `json:"env,omitempty"`

	// ServiceAccount use a specific kubernetes ServiceAccount for running the create + destroy pods.
	// If not specified we create a new ServiceAccount per Terraform
	ServiceAccount string `json:"serviceAccount,omitempty"`

	// Credentials is an array of credentials generally used for Terraform
	// providers
	Credentails []Credentials `json:"credentials,omitempty"`

	// ApplyOnCreate is used to apply any planned changes when the resource is
	// first created. Omitting this or setting it to false will resort to
	// on demand apply. Defaults to false.
	ApplyOnCreate bool `json:"applyOnCreate,omitempty"`

	// ApplyOnUpdate is used to apply any planned changes when the resource is
	// updated. Omitting this or setting it to false will resort to
	// on demand apply. Defaults to false.
	ApplyOnUpdate bool `json:"applyOnUpdate,omitempty"`

	// ApplyOnDelete is used to apply the destroy plan when the terraform
	// resource is being deleted. Omitting this or setting it to false will
	// require "on-demand" apply. Defaults to false.
	ApplyOnDelete bool `json:"applyOnDelete,omitempty"`

	// IgnoreDelete will bypass the finalization process and remove the tf
	// resource without running any delete jobs.
	IgnoreDelete bool `json:"ignoreDelete,omitempty"`

	// Reconcile are the settings used for auto-reconciliation
	Reconcile *ReconcileTerraformDeployment `json:"reconcile,omitempty"`

	// CustomBackend will allow the user to configure the backend of their
	// choice. If this is omitted, the default consul template will be used.
	CustomBackend string `json:"customBackend,omitempty"`

	// ExportRepo allows the user to define
	ExportRepo *ExportRepo `json:"exportRepo,omitempty"`

	// PrerunScript lets the user define a script that will run before
	// terraform commands are executed on the terraform-execution pod. The pod
	// will have already set up cloudProfile (eg cloud credentials) so the
	// script can make use of it.
	//
	// Setting this field will create a key in the tfvars configmap called
	// "prerun.sh". This means the user can also pass in a prerun.sh file via
	// config "Sources".
	PrerunScript string `json:"prerunScript,omitempty"`

	// PostrunScript lets the user define a script that will run after
	// terraform commands are executed on the terraform-execution pod. The pod
	// will have already set up cloudProfile (eg cloud credentials) so the
	// script can make use of it.
	//
	// Setting this field will create a key in the tfvars configmap called
	// "postrun.sh". This means the user can alternatively pass in a
	// posterun.sh file via config "Sources".
	PostrunScript string `json:"postrunScript,omitempty"`

	// SSHTunnel can be defined for pulling from scm sources that cannot be
	// accessed by the network the operator/runner runs in. An example is
	// Enterprise Github servers running on a private network.
	SSHTunnel *ProxyOpts `json:"sshTunnel,omitempty"`

	// SCMAuthMethods define multiple SCMs that require tokens/keys
	SCMAuthMethods []SCMAuthMethod `json:"scmAuthMethods,omitempty"`
}

// SCMAuthMethod definition of SCMs that require tokens/keys
type SCMAuthMethod struct {
	Host string `json:"host"`
	// SCM define the SCM for a host which is defined at a higher-level
	Git *GitSCM `json:"git,omitempty"`
}

// GitSCM define the auth methods of git
type GitSCM struct {
	SSH   *GitSSH   `json:"ssh,omitempty"`
	HTTPS *GitHTTPS `json:"https,omitempty"`
}

// GitSSH configurs the setup for git over ssh with optional proxy
type GitSSH struct {
	RequireProxy    bool             `json:"requireProxy,omitempty"`
	SSHKeySecretRef *SSHKeySecretRef `json:"sshKeySecretRef"`
}

// GitHTTPS configures the setup for git over https using tokens. Proxy is not
// supported in the terraform job pod at this moment
// TODO HTTPS Proxy support
type GitHTTPS struct {
	RequireProxy   bool            `json:"requireProxy,omitempty"`
	TokenSecretRef *TokenSecretRef `json:"tokenSecretRef"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TerraformList contains a list of Terraform
type TerraformList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Terraform `json:"items"`
}

// ExportRepo is used to allow the tfvars passed into the job to also be
// exported to a different git repo. The main use-case for this would be to
// allow terraform execution outside of the terraform-operator for any reason
type ExportRepo struct {
	// Address is the git repo to save to. At this time, only SSH is allowed
	Address string `json:"address"`

	// TFVarsFile is the full path relative to the root of the repo
	TFVarsFile string `json:"tfvarsFile"`

	// ConfFile is the full path relative to the root of the repo
	ConfFile string `json:"confFile,omitempty"`
}

// ReconcileTerraformDeployment is used to configure auto watching the resources
// created by terraform and re-applying them automatically if they are not
// in-sync with the terraform state.
type ReconcileTerraformDeployment struct {
	// Enable used to turn on the auto reconciliation of tfstate to actual
	// provisions. Default to false
	Enable bool `json:"enable"`
	// SyncPeriod can be used to set a custom time to check actual provisions
	// to tfstate. Defaults to 60 minutes
	SyncPeriod int64 `json:"syncPeriod,omitempty"`
}

// EnvVar defines key/value pairs of env vars that get picked up by terraform
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	// Source for the environment variable's value. Cannot be used if value is not empty.
	// +optional
	ValueFrom *EnvVarSource `json:"valueFrom,omitempty" protobuf:"bytes,3,opt,name=valueFrom"`
}

func (e *EnvVar) ToValueFrom() *v1.EnvVarSource {
	if e.ValueFrom == nil || e.ValueFrom.SecretKeyRef == nil {
		return nil
	}
	ref := e.ValueFrom.SecretKeyRef
	return &v1.EnvVarSource{
		SecretKeyRef: &v1.SecretKeySelector{
			LocalObjectReference: v1.LocalObjectReference{
				Name: ref.LocalObjectReference.Name,
			},
			Key:      ref.Key,
			Optional: ref.Optional,
		},
	}
}

// EnvVarSource represents a source for the value of an EnvVar.
type EnvVarSource struct {
	// Selects a key of a secret in the pod's namespace
	// +optional
	SecretKeyRef *SecretKeySelector `json:"secretKeyRef,omitempty" protobuf:"bytes,4,opt,name=secretKeyRef"`
}

// SecretKeySelector selects a key of a Secret.
type SecretKeySelector struct {
	// The name of the secret in the pod's namespace to select from.
	LocalObjectReference `json:",inline" protobuf:"bytes,1,opt,name=localObjectReference"`
	// The key of the secret to select from.  Must be a valid secret key.
	Key string `json:"key" protobuf:"bytes,2,opt,name=key"`
	// Specify whether the Secret or its key must be defined
	// +optional
	Optional *bool `json:"optional,omitempty" protobuf:"varint,3,opt,name=optional"`
}

// LocalObjectReference contains enough information to let you locate the
// referenced object inside the same namespace.
type LocalObjectReference struct {
	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// TODO: Add other useful fields. apiVersion, kind, uid?
	// +optional
	Name string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
}

// Source is used to describe details of where to find configs
type Source struct {
	Source    *SrcOpts       `json:"source,omitempty"`
	ConfigMap *ConfigMapOpts `json:"configMap,omitempty"`
}

// ConfigMapOpts used to define the configmap and relevant data keys
type ConfigMapOpts struct {
	Name string   `json:"name"`
	Keys []string `json:"keys,omitempty"`
}

// SrcOpts defines a terraform source location (eg git::SSH or git::HTTPS)
type SrcOpts struct {

	// Address defines the source address of the tf resources. This this var
	// will try to accept any format defined in
	// https://www.terraform.io/docs/modules/sources.html
	// When downloading `tfvars`, the double slash `//` syntax is used to
	// define dir or tfvar files. This can be used multiple times for
	// multiple items.
	Address string `json:"address"`

	// Extras will allow for giving the controller specific instructions for
	// fetching files from the address.
	Extras []string `json:"extras,omitempty"`
}

// ProxyOpts configures ssh tunnel/socks5 for downloading ssh/https resources
type ProxyOpts struct {
	Host            string          `json:"host,omitempty"`
	User            string          `json:"user,omitempty"`
	SSHKeySecretRef SSHKeySecretRef `json:"sshKeySecretRef"`
}

// SSHKeySecretRef defines the secret where the SSH key (for the proxy, git, etc) is stored
type SSHKeySecretRef struct {
	// Name the secret name that has the SSH key
	Name string `json:"name"`
	// Namespace of the secret; Default is the namespace of the terraform resource
	Namespace string `json:"namespace,omitempty"`
	// Key in the secret ref. Default to `id_rsa`
	Key string `json:"key,omitempty"`
}

// TokenSecretRef defines the token or password that can be used to log into a system (eg git)
type TokenSecretRef struct {
	// Name the secret name that has the token or password
	Name string `json:"name"`
	// Namespace of the secret; Default is the namespace of the terraform resource
	Namespace string `json:"namespace,omitempty"`
	// Key in the secret ref. Default to `token`
	Key string `json:"key,omitempty"`
}

// Credentials are used for adding credentials for terraform providers.
// For example, in AWS, the AWS Terraform Provider uses the default credential chain
// of the AWS SDK, one of which are environment variables (eg AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY)
type Credentials struct {
	// SecretNameRef will load environment variables into the terraform runner
	// from a kubernetes secret
	SecretNameRef SecretNameRef `json:"secretNameRef,omitempty"`
	// AWSCredentials contains the different methods to load AWS credentials
	// for the Terraform AWS Provider. If using AWS_ACCESS_KEY_ID and/or environment
	// variables for credentials, use fromEnvs.
	AWSCredentials AWSCredentials `json:"aws,omitempty"`

	// ServiceAccountAnnotations allows the service account to be annotated with
	// cloud IAM roles such as Workload Identity on GCP
	ServiceAccountAnnotations map[string]string `json:"serviceAccountAnnotations,omitempty"`

	// TODO Add other commonly used cloud providers to this list
}

// AWSCredentials provides a few different k8s-specific methods of adding
// crednetials to pods. This includes KIAM and IRSA.
//
// To use environment variables, use a secretNameRef instead.
type AWSCredentials struct {
	// IRSA requires the irsa role-arn as the string input. This will create a
	// serice account named tf-<resource-name>. In order for the pod to be able to
	// use this role, the "Trusted Entity" of the IAM role must allow this
	// serice account name and namespace.
	//
	// Using a TrustEntity policy that includes "StringEquals" setting it as the serivce account name
	// is the most secure way to use IRSA.
	//
	// However, for a reusable policy consider "StringLike" with a few wildcards to make
	// the irsa role usable by pods created by terraform-operator. The example below is
	// pretty liberal, but will work for any pod created by the terraform-operator.
	//
	// {
	//   "Version": "2012-10-17",
	//   "Statement": [
	//     {
	//       "Effect": "Allow",
	//       "Principal": {
	//         "Federated": "${OIDC_ARN}"
	//       },
	//       "Action": "sts:AssumeRoleWithWebIdentity",
	//       "Condition": {
	//         "StringLike": {
	//           "${OIDC_URL}:sub": "system:serviceaccount:*:tf-*"
	//         }
	//       }
	//     }
	//   ]
	// }
	IRSA string `json:"irsa,omitempty"`

	// KIAM requires the kiam role-name as the string input. This will add the
	// correct annotation to the terraform execution pod
	KIAM string `json:"kiam,omitempty"`
}

// SecretNameRef is the name of the kubernetes secret to use
type SecretNameRef struct {
	// Name of the secret
	Name string `json:"name"`
	// Namespace of the secret; Defaults to namespace of the tf resource
	Namespace string `json:"namespace,omitempty"`
	// Key of the secret
	Key string `json:"key,omitempty"`
}

// Inline definitions of configmaps
type Inline struct {
	ConfigMapFiles map[string]string `json:"scripts"`
}

// TerraformStatus defines the observed state of Terraform
// +k8s:openapi-gen=true
type TerraformStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "operator-sdk generate k8s" to regenerate code after modifying this file
	// Add custom validation using kubebuilder tags: https://book-v1.book.kubebuilder.io/beyond_basics/generating_crd.html
	Phase          string `json:"phase"`
	LastGeneration int64  `json:"lastGeneration"`
}

func init() {
	SchemeBuilder.Register(&Terraform{}, &TerraformList{})
}
