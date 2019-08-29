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

	SSHProxy *ProxyOpts `json:"sshProxy,omitempty"`
	Auth     *AuthOpts  `json:"auth,omitempty"`
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

// SrcOpts defines a source for tf resources
type SrcOpts struct {

	// Address defines the source address of the tf resources. This this var
	// will try to accept any format defined in
	// https://www.terraform.io/docs/modules/sources.html
	// When downloading `tfvars`, the double slash `//` syntax is used to
	// define dir or tfvar files. This can be used multiple times for
	// multiple items.
	Address string    `json:"address"`
	Auth    *AuthOpts `json:"auth,omitempty"`
	Extras  []string  `json:"extras,omitempty"`

	// SSHProxy can be defined for each source, or omit this to use global
	SSHProxy *ProxyOpts `json:"sshProxy,omitempty"`
}

// SSHProxy configures ssh tunnel/socks5 for downloading ssh/https resources
type ProxyOpts struct {
	// Auth   string  `json:"auth,omitempty"`
	Host string    `json:"host,omitempty"`
	User string    `json:"user,omitempty"`
	Auth *AuthOpts `json:"auth,omitempty"`
}

// Auth defines the name and key to extract
type AuthOpts struct {
	// Type is either 'key' or 'password' or omit to skip creds
	Type string `json:"type"`
	Key  string `json:"key"`
	Name string `json:"name"`
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

}

func init() {
	SchemeBuilder.Register(&Terraform{}, &TerraformList{})
}
