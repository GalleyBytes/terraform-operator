package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient
// Terraform is the Schema for the terraforms API
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=terraforms,shortName=tf
// +kubebuilder:singular=terraform
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

	// KeepCompletedPods when true will keep completed pods. Default is false
	// and completed pods are removed.
	KeepCompletedPods bool `json:"keepCompletedPods,omitempty"`

	// RunnerRules are RBAC rules that will be added to all runner pods.
	RunnerRules []rbacv1.PolicyRule `json:"runnerRules,omitempty"`

	// RunnerAnnotations are annotations that will be added to all runner pods.
	RunnerAnnotations map[string]string `json:"runnerAnnotations,omitempty"`

	// TerraformVersion helps the operator decide which image tag to pull for
	// the terraform runner. Defaults to "0.11.14"
	TerraformVersion    string `json:"terraformVersion,omitempty"`
	ScriptRunnerVersion string `json:"scriptRunnerVersion,omitempty"`
	SetupRunnerVersion  string `json:"setupRunnerVersion,omitempty"`

	// TerraformRunner gives the user the ability to inject their own container
	// image to execute terraform. This is very helpful for users who need to
	// have a certain toolset installed on their images, or who can't pull
	// public images, such as the default image "isaaguilar/tfops".
	TerraformRunner string `json:"terraformRunner,omitempty"`
	ScriptRunner    string `json:"scriptRunner,omitempty"`
	SetupRunner     string `json:"setupRunner,omitempty"`

	// TerraformRunnerExecutionScriptConfigMap allows the user to define a
	// custom terraform runner script that gets executed instead of the default
	// script built into the runner image. The configmap "name" and "key" are
	// required.
	TerraformRunnerExecutionScriptConfigMap *corev1.ConfigMapKeySelector `json:"terraformRunnerExecutionScriptConfigMap,omitempty"`

	// ScriptRunnerExecutionScriptConfigMap allows the user to define a
	// custom terraform runner script that gets executed instead of the default
	// script built into the runner image. The configmap "name" and "key" are
	// required.
	ScriptRunnerExecutionScriptConfigMap *corev1.ConfigMapKeySelector `json:"scriptRunnerExecutionScriptConfigMap,omitempty"`

	// SetupRunnerExecutionScriptConfigMap allows the user to define a
	// custom terraform runner script that gets executed instead of the default
	// script built into the runner image. The configmap "name" and "key" are
	// required.
	SetupRunnerExecutionScriptConfigMap *corev1.ConfigMapKeySelector `json:"setupRunnerExecutionScriptConfigMap,omitempty"`

	// TerraformRunnerPullPolicy describes a policy for if/when to pull the
	// TerraformRunner image. Acceptable values are "Always", "Never", or
	// "IfNotPresent".
	TerraformRunnerPullPolicy corev1.PullPolicy `json:"terraformRunnerPullPolicy,omitempty"`
	ScriptRunnerPullPolicy    corev1.PullPolicy `json:"scriptRunnerPullPolicy,omitempty"`
	SetupRunnerPullPolicy     corev1.PullPolicy `json:"setupRunnerPullPolicy,omitempty"`

	// TerraformModule is the terraform module scm address. Currently supports
	// git protocol over SSH or HTTPS.
	//
	// Precedence of "terraformModule*" to use as the main module is
	// determined by the setup runner. See the runners/setup.sh for the
	// module configuration.
	TerraformModule string `json:"terraformModule,omitempty"`

	// TerraformModuleConfigMap is the configMap that contains terraform module
	// resources. The module will be fetched by the setup runner. In order
	// for terraform to understand it's a module reosurce, the configmap keys
	// must end in `.tf` or `.tf.json`.
	TerraformModuleConfigMap *ConfigMapSelector `json:"terraformModuleConfigMap,omitempty"`

	// TerraformModuleInline is an incline terraform module definition. The
	// contents of the inline definition will be used to create
	// `inline-module.tf`
	TerraformModuleInline string `json:"terraformModuleInline,omitempty"`

	// OutputsSecret will create a secret with the outputs from the module. All
	// outputs from the module will be written to the secret unless the user
	// defines "outputsToInclude" or "outputsToOmit".
	OutputsSecret string `json:"outputsSecret,omitempty"`

	// OutputsToInclude is a whitelist of outputs to write when writing the
	// outputs to kubernetes.
	OutputsToInclude []string `json:"outputsToInclude,omitempty"`

	// OutputsToOmit is a blacklist of outputs to omit when writing the
	// outputs to kubernetes.
	OutputsToOmit []string `json:"outputsToOmit,omitempty"`

	// WriteOutputsToStatus will add the outputs from the module to the status
	// of the Terraform CustomResource.
	WriteOutputsToStatus bool `json:"writeOutputsToStatus,omitempty"`

	// ResourceDownloads defines other files to download into the module
	// directory that can be used by the terraform workflow runners.
	// The `tfvar` type will also be fetched by the `exportRepo` option (if
	// defined) to aggregate the set of tfvars to save to an scm system.
	ResourceDownloads []*ResourceDownload `json:"resourceDownloads,omitempty"`

	// Env is used to define a common set of environment variables into the
	// workflow runners. The `TF_VAR_` prefix will also be used by the
	// `exportRepo` option.
	Env []corev1.EnvVar `json:"env,omitempty"`

	// ServiceAccount use a specific kubernetes ServiceAccount for running the create + destroy pods.
	// If not specified we create a new ServiceAccount per Terraform
	ServiceAccount string `json:"serviceAccount,omitempty"`

	// Credentials is an array of credentials generally used for Terraform
	// providers
	Credentials []Credentials `json:"credentials,omitempty"`

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

	// PreInitScript lets the user define a script that will run before
	// terraform commands are executed on the terraform-execution pod. The pod
	// will have already set up cloudProfile (eg cloud credentials) so the
	// script can make use of it.
	//
	// Setting this field will create a key in the tfvars configmap called
	// "prerun.sh". This means the user can also pass in a prerun.sh file via
	// config "Sources".
	PreInitScript  string `json:"preInitScript,omitempty"`
	PostInitScript string `json:"postInitScript,omitempty"`

	PrePlanScript  string `json:"prePlanScript,omitempty"`
	PostPlanScript string `json:"postPlanScript,omitempty"`

	PreApplyScript string `json:"preApplyScript,omitempty"`

	// PostApplyScript lets the user define a script that will run after
	// terraform commands are executed on the terraform-execution pod. The pod
	// will have already set up cloudProfile (eg cloud credentials) so the
	// script can make use of it.
	//
	// Setting this field will create a key in the tfvars configmap called
	// "postrun.sh". This means the user can alternatively pass in a
	// posterun.sh file via config "Sources".
	PostApplyScript string `json:"postApplyScript,omitempty"`

	PreInitDeleteScript   string `json:"preInitDeleteScript,omitempty"`
	PostInitDeleteScript  string `json:"postInitDeleteScript,omitempty"`
	PrePlanDeleteScript   string `json:"prePlanDeleteScript,omitempty"`
	PostPlanDeleteScript  string `json:"postPlanDeleteScript,omitempty"`
	PreApplyDeleteScript  string `json:"preApplyDeleteScript,omitempty"`
	PostApplyDeleteScript string `json:"postApplyDeleteScript,omitempty"`

	// SSHTunnel can be defined for pulling from scm sources that cannot be
	// accessed by the network the operator/runner runs in. An example is
	// Enterprise Github servers running on a private network.
	SSHTunnel *ProxyOpts `json:"sshTunnel,omitempty"`

	// SCMAuthMethods define multiple SCMs that require tokens/keys
	SCMAuthMethods []SCMAuthMethod `json:"scmAuthMethods,omitempty"`
}

// A simple selector for configmaps that can select on the name of the configmap
// with the optional key. The namespace is not an option since only runners
// with a namespace'd role will utilize this map.
type ConfigMapSelector struct {
	Name string `json:"name"`
	Key  string `json:"key,omitempty"`
}

// SCMAuthMethod definition of SCMs that require tokens/keys
type SCMAuthMethod struct {
	Host string `json:"host"`

	// Git configuration options for auth methods of git
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
	TFVarsFile string `json:"tfvarsFile,omitempty"`

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

// ResourceDownload (formerly SrcOpts) defines a resource to fetch using one
// of the configured protocols: ssh|http|https (eg git::SSH or git::HTTPS)
type ResourceDownload struct {

	// Address defines the source address resources to fetch.
	Address string `json:"address"`

	// Path will download the resources into this path which is relative to
	// the main module directory.
	Path string `json:"path,omitempty"`

	// UseAsVar will add the file as a tfvar via the -var-file flag of the
	// terraform plan command. The downloaded resource must not be a directory.
	UseAsVar bool `json:"useAsVar,omitempty"`
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

	// PodNamePrefix is used to identify this installation of the resource. For
	// very long resource names, like those greater than 220 characters, the
	// prefix ensures resource uniqueness for runners and other resources used
	// by the runner.
	// Another case for the pod name prefix is when rapidly deleteing a resource
	// and recreating it, the chance of recycling existing resources is reduced
	// to virtually nil.
	PodNamePrefix           string            `json:"podNamePrefix"`
	Phase                   StatusPhase       `json:"phase"`
	LastCompletedGeneration int64             `json:"lastCompletedGeneration"`
	Outputs                 map[string]string `json:"outputs,omitempty"`
	Stages                  []Stage           `json:"stages"`
	Exported                Exported          `json:"exported,omitempty"`
}

type Exported string

const (
	ExportedTrue       Exported = "true"
	ExportedFalse      Exported = "false"
	ExportedInProgress Exported = "in-progress"
	ExportedFailed     Exported = "failed"
	ExportedPending    Exported = "pending"
	ExportCreating     Exported = "creating"
)

type Stage struct {
	Generation int64      `json:"generation"`
	State      StageState `json:"state"`
	PodType    PodType    `json:"podType"`

	// Interruptible is set to false when the pod should not be terminated
	// such as when doing a terraform apply
	Interruptible Interruptible `json:"interruptible"`
	Reason        string        `json:"reason"`
	StartTime     metav1.Time   `json:"startTime,omitempty"`
	StopTime      metav1.Time   `json:"stopTime,omitempty"`
}

type StatusPhase string

const (
	PhaseInitializing StatusPhase = "initializing"
	PhaseCompleted    StatusPhase = "completed"
	PhaseRunning      StatusPhase = "running"
	PhaseInitDelete   StatusPhase = "initializing-delete"
	PhaseDeleting     StatusPhase = "deleting"
	PhaseDeleted      StatusPhase = "deleted"
)

type PodType string

const (
	PodSetupDelete     PodType = "setup-delete"
	PodPreInitDelete   PodType = "init0-delete"
	PodInitDelete      PodType = "init-delete"
	PodPostInitDelete  PodType = "init1-delete"
	PodPrePlanDelete   PodType = "plan0-delete"
	PodPlanDelete      PodType = "plan-delete"
	PodPostPlanDelete  PodType = "plan1-delete"
	PodPreApplyDelete  PodType = "apply0-delete"
	PodApplyDelete     PodType = "apply-delete"
	PodPostApplyDelete PodType = "post-delete"

	PodSetup     PodType = "setup"
	PodPreInit   PodType = "init0"
	PodInit      PodType = "init"
	PodPostInit  PodType = "init1"
	PodPrePlan   PodType = "plan0"
	PodPlan      PodType = "plan"
	PodPostPlan  PodType = "plan1"
	PodPreApply  PodType = "apply0"
	PodApply     PodType = "apply"
	PodPostApply PodType = "post"
	PodNil       PodType = ""

	PodExport PodType = "export"
)

type StageState string

const (
	StateInitializing StageState = "initializing"
	StateComplete     StageState = "complete"
	StateFailed       StageState = "failed"
	StateInProgress   StageState = "in-progress"
	StateUnknown      StageState = "unknown"
)

type Interruptible bool

const (
	CanNotBeInterrupt Interruptible = false
	CanBeInterrupt    Interruptible = true
)

func init() {
	SchemeBuilder.Register(&Terraform{}, &TerraformList{})
}
