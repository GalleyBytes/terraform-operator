package v1alpha1

import (
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
	Stack  *TerraformStack  `json:"stack"`
	Config *TerraformConfig `json:"config"`

	// TODO refactor to SSHTunnel
	SSHProxy *ProxyOpts `json:"sshProxy,omitempty"`

	SSHKeySecretRefs []SSHKeySecretRef `json:"sshKeySecretRefs,omitempty"`
	TokenSecretRefs  []TokenSecretRef  `json:"tokenSecretRefs,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TerraformList contains a list of Terraform
type TerraformList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Terraform `json:"items"`
}

// TerraformStack points to the stack root
type TerraformStack struct {
	ConfigMap string   `json:"configMap,omitempty"`
	Source    *SrcOpts `json:"source,omitempty"`
}

// TerraformConfig points to the tfvars to deploy against the stack
type TerraformConfig struct {
	Sources []*SrcOpts `json:"sources,omitempty"`
	Env     []EnvVar   `json:"env,omitempty"`

	// CloudProfile defines the selector to cloud access keys or roles to
	// attach to the terraform job executor. If it begines with "kiam-", the
	// controller will assume this is a role and add the kiam annotation.
	// Other clouds may have different role based mechanisms so other prefixes
	// would need to be hard coded in the controller's if statements later.
	// The default behaviour is to load the CloudProfile from a secret
	// as environment vars.
	CloudProfile string `json:"cloudProfile,omitempty"`

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

	// Reconcile are the settings used for auto-reconciliation
	Reconcile *ReconcileTerraformDeployment `json:"reconcile,omitempty"`

	// TerraformVersion helps the operator decide which terraform image to
	// run the terraform in. Defaults to v0.11.14
	TerraformVersion string `json:"terraformVersion,omitempty"`
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
}

// SSHProxy configures ssh tunnel/socks5 for downloading ssh/https resources
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

// TokenSecetRef defines the token or password that can be used to log into a system (eg git)
type TokenSecretRef struct {
	// Name the secret name that has the token or password
	Name string `json:"name"`
	// Namespace of the secret; Default is the namespace of the terraform resource
	Namespace string `json:"namespace,omitempty"`
	// Key in the secret ref. Default to `token`
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
