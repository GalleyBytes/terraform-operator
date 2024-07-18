package controllers

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	tfv1beta1 "github.com/galleybytes/terraform-operator/pkg/apis/tf/v1beta1"
	"github.com/galleybytes/terraform-operator/pkg/utils"
	"github.com/go-logr/logr"
	getter "github.com/hashicorp/go-getter"
	localcache "github.com/patrickmn/go-cache"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runtimecontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

//go:embed scripts/tf.sh
var defaultInlineTerraformTaskExecutionFile string

//go:embed scripts/setup.sh
var defaultInlineSetupTaskExecutionFile string

//go:embed scripts/noop.sh
var defaultInlineNoOpExecutionFile string

// ReconcileTerraform reconciles a Terraform object
type ReconcileTerraform struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	Client                  client.Client
	Scheme                  *runtime.Scheme
	Recorder                record.EventRecorder
	Log                     logr.Logger
	MaxConcurrentReconciles int
	Cache                   *localcache.Cache

	GlobalEnvFromConfigmapData map[string]string
	GlobalEnvFromSecretData    map[string][]byte
	GlobalEnvSuffix            string

	// InheritNodeSelector to use the controller's nodeSelectors for every task created by the controller.
	// Value of this field will come from the owning deployment and cached.
	InheritNodeSelector  bool
	NodeSelectorCacheKey string

	// InheritAffinity to use the controller's affinity rules for every task created by the controller
	// Value of this field will come from the owning deployment and cached.
	InheritAffinity  bool
	AffinityCacheKey string

	// InheritTolerations to use the controller's tolerations for every task created by the controller
	// Value of this field will come from the owning deployment and cached.
	InheritTolerations  bool
	TolerationsCacheKey string

	// When requireApproval is true, the require-approval plugin is injected into the plan pod
	// when generating the pod manifest. The require-approval image is not modifiable via the Terraform
	// Resource in order to ensure the highest compatibility with the other TFO projects (like
	// terraform-operator-api and terraform-operator-dashboard).
	RequireApprovalImage string
}

// getNamespace returns the namespace to create Terraform task pods and related resources in.
func getNamespace(tf *tfv1beta1.Terraform) string {
	podNamespace := os.Getenv("POD_NAMESPACE")
	if podNamespace != "" && os.Getenv("USE_CONTROLLER_NAMESPACE") == "true" {
		return podNamespace
	}
	return tf.Namespace
}

// createEnvFromSources adds any of the global environment vars defined at the controller scope
// and generates a configmap or secret that will be loaded into the resource Task pods.
//
// TODO Each time a new generation is created of the tfo resource, this "global" env from vars should
// generate a new configap and secret. The reason for this is to prevent a generation from producing a
// different plan when is was the controller that changed options. A new generation should be forced
// if the plan needs to change.
func (r ReconcileTerraform) createEnvFromSources(ctx context.Context, tf *tfv1beta1.Terraform) error {

	resourceName := tf.Name
	resourceNamespace := getNamespace(tf)
	name := fmt.Sprintf("%s-%s", resourceName, r.GlobalEnvSuffix)
	if len(r.GlobalEnvFromConfigmapData) > 0 {
		configMap := corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: resourceNamespace,
			},
			Data: r.GlobalEnvFromConfigmapData,
		}
		controllerutil.SetControllerReference(tf, &configMap, r.Scheme)
		errOnCreate := r.Client.Create(ctx, &configMap)
		if errOnCreate != nil {
			if errors.IsAlreadyExists(errOnCreate) {
				errOnUpdate := r.Client.Update(ctx, &configMap)
				if errOnUpdate != nil {
					return errOnUpdate
				}
			} else {
				return errOnCreate
			}
		}
	}

	if len(r.GlobalEnvFromSecretData) > 0 {
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: resourceNamespace,
			},
			Data: r.GlobalEnvFromSecretData,
		}
		controllerutil.SetControllerReference(tf, &secret, r.Scheme)
		errOnCreate := r.Client.Create(ctx, &secret)
		if errOnCreate != nil {
			if errors.IsAlreadyExists(errOnCreate) {
				errOnUpdate := r.Client.Update(ctx, &secret)
				if errOnUpdate != nil {
					return errOnUpdate
				}
			} else {
				return errOnCreate
			}
		}
	}

	return nil
}

// listEnvFromSources makes an assumption that if global envs are defined in the controller, the
// configmap and secrets for the envs have been created or updated when initializing the workflow.
//
// This function will return the envFrom of the resources that should exist but does not validate that
// they do exist. If the configmap or secret is missing, force the generation of the tfo resource to update
// and the controller will recreate the missing resources.
func (r ReconcileTerraform) listEnvFromSources(tf *tfv1beta1.Terraform) []corev1.EnvFromSource {
	envFrom := []corev1.EnvFromSource{}
	resourceName := tf.Name
	name := fmt.Sprintf("%s-%s", resourceName, r.GlobalEnvSuffix)

	if len(r.GlobalEnvFromConfigmapData) > 0 {
		// ConfigMap that should exist
		envFrom = append(envFrom, corev1.EnvFromSource{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: name,
				},
			},
		})
	}

	if len(r.GlobalEnvFromSecretData) > 0 {
		// Secret that should exist
		envFrom = append(envFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: name,
				},
			},
		})
	}

	return envFrom
}

const labelPrefix = "terraforms.tf.galleybytes.com/"

// podToTerraformResource returns the Terraform resource associated with a
// Terraform task pod as a reconciliation request.
func podToTerraformResource(_ context.Context, obj client.Object) []reconcile.Request {
	annotations := obj.GetAnnotations()
	if resourceName, ok := annotations[labelPrefix+"resourceName"]; ok {
		if resourceNamespace, ok := annotations[labelPrefix+"resourceNamespace"]; ok {
			return []reconcile.Request{{
				NamespacedName: types.NamespacedName{Name: resourceName, Namespace: resourceNamespace},
			}}
		}
	}
	return []reconcile.Request{}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ReconcileTerraform) SetupWithManager(mgr ctrl.Manager) error {
	controllerOptions := runtimecontroller.Options{
		MaxConcurrentReconciles: r.MaxConcurrentReconciles,
	}
	err := ctrl.NewControllerManagedBy(mgr).
		For(&tfv1beta1.Terraform{}).
		Owns(&corev1.Pod{}).
		Watches(&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(podToTerraformResource),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		WithOptions(controllerOptions).
		Complete(r)
	if err != nil {
		return err
	}
	return nil
}

func terraformTaskList() []tfv1beta1.TaskName {
	return []tfv1beta1.TaskName{
		tfv1beta1.RunInit,
		tfv1beta1.RunInitDelete,
		tfv1beta1.RunPlan,
		tfv1beta1.RunPlanDelete,
		tfv1beta1.RunApply,
		tfv1beta1.RunApplyDelete,
	}
}

func scriptTaskList() []tfv1beta1.TaskName {
	return []tfv1beta1.TaskName{
		tfv1beta1.RunPreInit,
		tfv1beta1.RunPreInitDelete,
		tfv1beta1.RunPostInit,
		tfv1beta1.RunPostInitDelete,
		tfv1beta1.RunPrePlan,
		tfv1beta1.RunPrePlanDelete,
		tfv1beta1.RunPostPlan,
		tfv1beta1.RunPostPlanDelete,
		tfv1beta1.RunPreApply,
		tfv1beta1.RunPreApplyDelete,
		tfv1beta1.RunPostApply,
		tfv1beta1.RunPostApplyDelete,
	}
}

func setupTaskList() []tfv1beta1.TaskName {
	return []tfv1beta1.TaskName{
		tfv1beta1.RunSetup,
		tfv1beta1.RunSetupDelete,
	}
}

// ParsedAddress uses go-getter's detect mechanism to get the parsed url
// TODO ParsedAddress can be moved into it's own package
type ParsedAddress struct {
	// DetectedScheme is the name of the bin or protocol to use to fetch. For
	// example, git will be used to fetch git repos (over https or ssh
	// "protocol").
	DetectedScheme string `json:"detect"`

	// Path the target path for the downloaded file or directory
	Path string `json:"path"`

	// The files downloaded get called out in the terraform plan as -var-file
	UseAsVar bool `json:"useAsVar"`

	// Url is the raw address + query
	Url string `json:"url"`

	// Files are the files to find with a repo.
	Files []string `json:"files"`

	// Hash is also known as the `ref` query argument. For git this is the
	// commit-sha or branch-name to checkout.
	Hash string `json:"hash"`

	// UrlScheme is the protocol of the URL
	UrlScheme string `json:"protocol"`

	// Uri is the path of the URL after the proto://host.
	Uri string `json:"uri"`

	// Host is the host of the URL.
	Host string `json:"host"`

	// Port is the port to use when fetching the URL.
	Port string `json:"port"`

	// User is the user to use when fetching the URL.
	User string `json:"user"`

	// Repo when using a SCM is the URL of the repo which is the same as the
	// URL and omitting the query args.
	Repo string `json:"repo"`
}

type TaskOptions struct {
	annotations map[string]string

	// configMapSourceName (and configMapSourceKey) is used to populate an environment variable of the task pod.
	// When not empty should be understood by the task to use the configmap as the execution script
	configMapSourceName string
	// configMapSourceKey (and configMapSourceName) is used to populate an environment variable of the task pod.
	// When not empty should be understood by the task to use the configmap as the execution script
	configMapSourceKey string

	credentials           []tfv1beta1.Credentials
	env                   []corev1.EnvVar
	envFrom               []corev1.EnvFromSource
	generation            int64
	image                 string
	imagePullPolicy       corev1.PullPolicy
	inheritedAffinity     *corev1.Affinity
	inheritedNodeSelector map[string]string
	inheritedTolerations  []corev1.Toleration

	// inlineTaskExecutionFile is used to populate an environment variable of the task pod. When not empty the
	// task should use this filename which should exist from a configmap mount in the pod.
	inlineTaskExecutionFile string

	labels                              map[string]string
	mainModulePluginData                map[string]string
	namespace                           string
	outputsSecretName                   string
	outputsToInclude                    []string
	outputsToOmit                       []string
	policyRules                         []rbacv1.PolicyRule
	prefixedName                        string
	resourceLabels                      map[string]string
	resourceName                        string
	resourceUUID                        string
	task                                tfv1beta1.TaskName
	saveOutputs                         bool
	secretData                          map[string][]byte
	serviceAccount                      string
	cleanupDisk                         bool
	stripGenerationLabelOnOutputsSecret bool
	terraformModuleParsed               ParsedAddress
	terraformVersion                    string

	// urlSource is used to populate an environment variable of the task pod. When not empty is used by the task
	// as the download location for the script to execute in the task.
	urlSource string

	versionedName        string
	requireApproval      bool
	requireApprovalImage string
	restartPolicy        corev1.RestartPolicy

	volumes      []corev1.Volume
	volumeMounts []corev1.VolumeMount

	// When a plugin is defined to run as a sidecar, this field will be filled in and attached to current task
	sidecarPlugins []corev1.Pod
}

func newTaskOptions(tf *tfv1beta1.Terraform, task tfv1beta1.TaskName, generation int64, globalEnvFrom []corev1.EnvFromSource, affinity *corev1.Affinity, nodeSelector map[string]string, tolerations []corev1.Toleration, requireApprovalImage string) TaskOptions {
	// TODO Read the tfstate and decide IF_NEW_RESOURCE based on that
	// applyAction := false
	resourceName := tf.Name
	resourceUUID := string(tf.UID)
	prefixedName := tf.Status.PodNamePrefix
	versionedName := prefixedName + "-v" + fmt.Sprint(tf.Generation)
	terraformVersion := tf.Spec.TerraformVersion
	if terraformVersion == "" {
		terraformVersion = "latest"
	}

	image := ""
	imagePullPolicy := corev1.PullAlways
	policyRules := []rbacv1.PolicyRule{}
	labels := make(map[string]string)
	annotations := make(map[string]string)
	env := []corev1.EnvVar{}
	envFrom := globalEnvFrom
	cleanupDisk := false
	urlSource := ""
	configMapSourceName := ""
	configMapSourceKey := ""
	restartPolicy := corev1.RestartPolicyNever
	inlineTaskExecutionFile := ""
	useDefaultInlineTaskExecutionFile := false
	volumes := []corev1.Volume{}
	volumeMounts := []corev1.VolumeMount{}

	// TaskOptions have data for all the tasks but since we're only interested
	// in the ones for this taskType, extract and add them to RunOptions
	for _, taskOption := range tf.Spec.TaskOptions {
		if tfv1beta1.ListContainsTask(taskOption.For, task) ||
			tfv1beta1.ListContainsTask(taskOption.For, "*") {

			// This statement finds taskOptions that match this current task OR *
			policyRules = append(policyRules, taskOption.PolicyRules...)
			for key, value := range taskOption.Annotations {
				annotations[key] = value
			}
			for key, value := range taskOption.Labels {
				labels[key] = value
			}
			env = append(env, taskOption.Env...)
			envFrom = append(envFrom, taskOption.EnvFrom...)
			if taskOption.RestartPolicy != "" {
				restartPolicy = taskOption.RestartPolicy
			}

			volumes = append(volumes, taskOption.Volumes...)
			volumeMounts = append(volumeMounts, taskOption.VolumeMounts...)
		}
		if tfv1beta1.ListContainsTask(taskOption.For, task) {
			// This statement only matches taskOptions that match this current task only
			urlSource = taskOption.Script.Source
			if configMapSelector := taskOption.Script.ConfigMapSelector; configMapSelector != nil {
				configMapSourceName = configMapSelector.Name
				configMapSourceKey = configMapSelector.Key
			}
			if inlineScript := taskOption.Script.Inline; inlineScript != "" {
				inlineTaskExecutionFile = fmt.Sprintf("inline-%s.sh", task)
			}
		}
	}

	images := tf.Spec.Images
	if images == nil {
		// setup default images
		images = &tfv1beta1.Images{}
	}

	if images.Terraform == nil {
		images.Terraform = &tfv1beta1.ImageConfig{
			ImagePullPolicy: corev1.PullIfNotPresent,
		}
	}

	if images.Terraform.Image == "" {
		images.Terraform.Image = fmt.Sprintf("%s:%s", tfv1beta1.TerraformTaskImageRepoDefault, terraformVersion)
	} else {
		terraformImage := images.Terraform.Image
		splitImage := strings.Split(images.Terraform.Image, ":")
		if length := len(splitImage); length > 1 {
			terraformImage = strings.Join(splitImage[:length-1], ":")
		}
		images.Terraform.Image = fmt.Sprintf("%s:%s", terraformImage, terraformVersion)
	}

	if images.Setup == nil {
		images.Setup = &tfv1beta1.ImageConfig{
			ImagePullPolicy: corev1.PullIfNotPresent,
		}
	}

	if images.Setup.Image == "" {
		images.Setup.Image = fmt.Sprintf("%s:%s", tfv1beta1.SetupTaskImageRepoDefault, tfv1beta1.SetupTaskImageTagDefault)
	}

	if images.Script == nil {
		images.Script = &tfv1beta1.ImageConfig{
			ImagePullPolicy: corev1.PullIfNotPresent,
		}
	}

	if images.Script.Image == "" {
		images.Script.Image = fmt.Sprintf("%s:%s", tfv1beta1.ScriptTaskImageRepoDefault, tfv1beta1.ScriptTaskImageTagDefault)
	}

	if inlineTaskExecutionFile == "" && urlSource == "" && (configMapSourceKey == "" || configMapSourceName == "") {
		useDefaultInlineTaskExecutionFile = true
	}
	if tfv1beta1.ListContainsTask(terraformTaskList(), task) {
		image = images.Terraform.Image
		imagePullPolicy = images.Terraform.ImagePullPolicy
		if useDefaultInlineTaskExecutionFile {
			inlineTaskExecutionFile = "default-terraform.sh"
		}
	} else if tfv1beta1.ListContainsTask(scriptTaskList(), task) {
		image = images.Script.Image
		imagePullPolicy = images.Script.ImagePullPolicy
		if useDefaultInlineTaskExecutionFile {
			inlineTaskExecutionFile = "default-noop.sh"
		}
	} else if tfv1beta1.ListContainsTask(setupTaskList(), task) {
		image = images.Setup.Image
		imagePullPolicy = images.Setup.ImagePullPolicy
		if useDefaultInlineTaskExecutionFile {
			inlineTaskExecutionFile = "default-setup.sh"
		}
	}

	// sshConfig := utils.TruncateResourceName(tf.Name, 242) + "-ssh-config"
	serviceAccount := tf.Spec.ServiceAccount
	if serviceAccount == "" {
		// By prefixing the service account with "tf-", IRSA roles can use wildcard
		// "tf-*" service account for AWS credentials.
		serviceAccount = "tf-" + versionedName
	}

	credentials := tf.Spec.Credentials

	// Outputs will be saved as a secret that will have the same lifecycle
	// as the Terraform CustomResource by adding the ownership metadata
	outputsSecretName := versionedName + "-outputs"
	saveOutputs := false
	stripGenerationLabelOnOutputsSecret := false
	if tf.Spec.OutputsSecret != "" {
		outputsSecretName = tf.Spec.OutputsSecret
		saveOutputs = true
		stripGenerationLabelOnOutputsSecret = true
	} else if tf.Spec.WriteOutputsToStatus {
		saveOutputs = true
	}
	outputsToInclude := tf.Spec.OutputsToInclude
	outputsToOmit := tf.Spec.OutputsToOmit

	if tf.Spec.Setup != nil {
		cleanupDisk = tf.Spec.Setup.CleanupDisk
	}

	resourceLabels := map[string]string{
		labelPrefix + "generation":        fmt.Sprintf("%d", generation),
		labelPrefix + "resourceName":      utils.AutoHashLabeler(resourceName),
		labelPrefix + "resourceNamespace": utils.AutoHashLabeler(tf.Namespace),
		labelPrefix + "podPrefix":         prefixedName,
		labelPrefix + "terraformVersion":  terraformVersion,
		"app.kubernetes.io/name":          "terraform-operator",
		"app.kubernetes.io/component":     "terraform-operator-runner",
		"app.kubernetes.io/created-by":    "controller",
	}
	// Add the full resource name and namespace to the annotations
	annotations[labelPrefix+"resourceName"] = resourceName
	annotations[labelPrefix+"resourceNamespace"] = tf.Namespace

	requireApproval := tf.Spec.RequireApproval

	if task.ID() == -2 {
		// This is not one of the main tasks so it's probably an plugin
		resourceLabels[labelPrefix+"isPlugin"] = "true"
	}

	return TaskOptions{
		env:                                 env,
		generation:                          generation,
		configMapSourceName:                 configMapSourceName,
		configMapSourceKey:                  configMapSourceKey,
		envFrom:                             envFrom,
		policyRules:                         policyRules,
		annotations:                         annotations,
		labels:                              labels,
		imagePullPolicy:                     imagePullPolicy,
		inheritedAffinity:                   affinity,
		inheritedNodeSelector:               nodeSelector,
		inheritedTolerations:                tolerations,
		inlineTaskExecutionFile:             inlineTaskExecutionFile,
		namespace:                           getNamespace(tf),
		resourceName:                        resourceName,
		prefixedName:                        prefixedName,
		versionedName:                       versionedName,
		credentials:                         credentials,
		terraformVersion:                    terraformVersion,
		image:                               image,
		task:                                task,
		resourceLabels:                      resourceLabels,
		resourceUUID:                        resourceUUID,
		serviceAccount:                      serviceAccount,
		mainModulePluginData:                make(map[string]string),
		secretData:                          make(map[string][]byte),
		cleanupDisk:                         cleanupDisk,
		outputsSecretName:                   outputsSecretName,
		saveOutputs:                         saveOutputs,
		stripGenerationLabelOnOutputsSecret: stripGenerationLabelOnOutputsSecret,
		outputsToInclude:                    outputsToInclude,
		outputsToOmit:                       outputsToOmit,
		urlSource:                           urlSource,
		requireApproval:                     requireApproval,
		requireApprovalImage:                requireApprovalImage,
		restartPolicy:                       restartPolicy,
		volumes:                             volumes,
		volumeMounts:                        volumeMounts,
		sidecarPlugins:                      nil,
	}
}

const terraformFinalizer = "finalizer.tf.galleybytes.com"

// Reconcile reads that state of the cluster for a Terraform object and makes changes based on the state read
// and what is in the Terraform.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileTerraform) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reconcilerID := string(uuid.NewUUID())
	reqLogger := r.Log.WithValues("Terraform", request.NamespacedName, "id", reconcilerID)
	err := r.cacheNodeSelectors(ctx, reqLogger)
	if err != nil {
		panic(err)
	}
	lockKey := request.String() + "-reconcile-lock"
	lockOwner, lockFound := r.Cache.Get(lockKey)
	if lockFound {
		reqLogger.Info(fmt.Sprintf("Request is locked by '%s'", lockOwner.(string)))
		return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}
	r.Cache.Set(lockKey, reconcilerID, -1)
	defer r.Cache.Delete(lockKey)
	defer reqLogger.V(6).Info("Request has released reconcile lock")
	reqLogger.V(6).Info("Request has acquired reconcile lock")

	tf, err := r.getTerraformResource(ctx, request.NamespacedName, 3, reqLogger)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			// reqLogger.Info(fmt.Sprintf("Not found, instance is defined as: %+v", instance))
			reqLogger.V(1).Info("Terraform resource not found. Ignoring since object must be deleted")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reqLogger.Error(err, "Failed to get Terraform")
		return reconcile.Result{}, err
	}

	// Final delete by removing finalizers
	if tf.Status.Phase == tfv1beta1.PhaseDeleted {
		reqLogger.Info("Remove finalizers")
		if err := r.updateSecretFinalizer(ctx, tf); err != nil {
			r.Recorder.Event(tf, "Warning", "ProcessingError", err.Error())
			return reconcile.Result{}, err
		}
		_ = updateFinalizer(tf)
		err := r.update(ctx, tf)
		if err != nil {
			r.Recorder.Event(tf, "Warning", "ProcessingError", err.Error())
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// Finalizers
	if updateFinalizer(tf) {
		err := r.update(ctx, tf)
		if err != nil {
			return reconcile.Result{}, err
		}
		reqLogger.V(1).Info("Updated finalizer")
		return reconcile.Result{}, nil
	}

	// Initialize resource
	if tf.Status.PodNamePrefix == "" {
		// Generate a unique name for everything related to this tf resource
		// Must trucate at 220 chars of original name to ensure room for the
		// suffixes that will be added (and possible future suffix expansion)
		tf.Status.PodNamePrefix = fmt.Sprintf("%s-%s",
			utils.TruncateResourceName(tf.Name, 54),
			utils.StringWithCharset(8, utils.AlphaNum),
		)
		tf.Status.LastCompletedGeneration = 0
		tf.Status.Phase = tfv1beta1.PhaseInitializing

		err := r.updateStatusWithRetry(ctx, tf, &tf.Status, reqLogger)
		if err != nil {
			reqLogger.V(1).Info(err.Error())
		}
		return reconcile.Result{}, nil
	}

	// Add the first stage
	if tf.Status.Stage.Generation == 0 {
		task := tfv1beta1.RunSetup
		stageState := tfv1beta1.StateInitializing
		interruptible := tfv1beta1.CanNotBeInterrupt
		stage := newStage(tf, task, "TF_RESOURCE_CREATED", interruptible, stageState)
		if stage == nil {
			return reconcile.Result{}, fmt.Errorf("failed to create a new stage")
		}
		tf.Status.Stage = *stage
		tf.Status.PluginsStarted = []tfv1beta1.TaskName{}

		err := r.updateStatusWithRetry(ctx, tf, &tf.Status, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	deletePhases := []string{
		string(tfv1beta1.PhaseDeleting),
		string(tfv1beta1.PhaseInitDelete),
		string(tfv1beta1.PhaseDeleted),
	}

	// Check if the resource is marked to be deleted which is
	// indicated by the deletion timestamp being set.
	if tf.GetDeletionTimestamp() != nil && !utils.ListContainsStr(deletePhases, string(tf.Status.Phase)) {
		tf.Status.Phase = tfv1beta1.PhaseInitDelete
	}

	// // TODO Check the status on stages that have not completed
	// for _, stage := range tf.Status.Stages {
	// 	if stage.State == tfv1alpha1.StateInProgress {
	//
	// 	}
	// }

	retry := false
	if tf.Labels != nil {
		if label, found := tf.Labels["kubernetes.io/change-cause"]; found {

			if tf.Status.RetryEventReason == nil {
				retry = true
			} else if *tf.Status.RetryEventReason != label {
				retry = true
			}

			if retry {
				// Once a single retry is triggered via the change-cause label method,
				// the retry* status entries will persist for the lifetime of
				// the resource. This doesn't affect workflows, but it's a little annoying to see the
				// status long after the retry has occurred. In the future, see if there is a way to clean
				// up the status.
				// As of today, attempting to clean the retry* status when the change-cause label still exists
				// causes the controller to skip new generation steps like creating configmaps, secrets, etc.
				// TODO clean retry* status
				now := metav1.Now()
				tf.Status.RetryEventReason = &label // saved via updateStatusWithRetry
				tf.Status.RetryTimestamp = &now     // saved via updateStatusWithRetry
				tf.Status.Phase = tfv1beta1.PhaseInitializing
			}
		}
	}

	stage := r.checkSetNewStage(ctx, tf, retry)
	if stage != nil {
		if stage.Reason == "RESTARTED_WORKFLOW" || stage.Reason == "RESTARTED_DELETE_WORKFLOW" {
			_ = r.removeOldPlan(tf)
			// TODO what to do if the remove old plan function fails
		}
		reqLogger.V(2).Info(fmt.Sprintf("Stage moving from '%s' -> '%s'", tf.Status.Stage.TaskType, stage.TaskType))
		tf.Status.Stage = *stage
		desiredStatus := tf.Status
		err := r.updateStatusWithRetry(ctx, tf, &desiredStatus, reqLogger)
		if err != nil {
			reqLogger.V(1).Info(fmt.Sprintf("Error adding stage '%s': %s", stage.TaskType, err.Error()))
		}
		if tf.Spec.KeepLatestPodsOnly {
			go r.backgroundReapOldGenerationPods(tf, 0)
		}
		return reconcile.Result{}, nil
	}

	globalEnvFrom := r.listEnvFromSources(tf)
	if err != nil {
		return reconcile.Result{}, err
	}
	currentStage := tf.Status.Stage
	podType := currentStage.TaskType
	generation := currentStage.Generation
	affinity, nodeSelector, tolerations := r.getNodeSelectorsFromCache()
	runOpts := newTaskOptions(tf, currentStage.TaskType, generation, globalEnvFrom, affinity, nodeSelector, tolerations, r.RequireApprovalImage)

	if podType == tfv1beta1.RunNil {
		// podType is blank when the terraform workflow has completed for
		// either create or delete.

		if tf.Status.Phase == tfv1beta1.PhaseRunning {
			// Updates the status as "completed" on the resource
			tf.Status.Phase = tfv1beta1.PhaseCompleted
			if tf.Spec.WriteOutputsToStatus {
				// runOpts.outputsSecetName
				secret, err := r.loadSecret(ctx, runOpts.outputsSecretName, runOpts.namespace)
				if err != nil {
					reqLogger.Error(err, fmt.Sprintf("failed to load secret '%s'", runOpts.outputsSecretName))
				}
				// Get a list of outputs to clean up any removed outputs
				keysInOutputs := []string{}
				for key := range secret.Data {
					keysInOutputs = append(keysInOutputs, key)
				}
				for key := range tf.Status.Outputs {
					if !utils.ListContainsStr(keysInOutputs, key) {
						// remove the key if its not in the new list of outputs
						delete(tf.Status.Outputs, key)
					}
				}
				for key, value := range secret.Data {
					if tf.Status.Outputs == nil {
						tf.Status.Outputs = make(map[string]string)
					}
					tf.Status.Outputs[key] = string(value)
				}
			}
			err := r.updateStatusWithRetry(ctx, tf, &tf.Status, reqLogger)
			if err != nil {
				reqLogger.V(1).Info(err.Error())
				return reconcile.Result{}, err
			}
		} else if tf.Status.Phase == tfv1beta1.PhaseDeleting {
			// Updates the status as "deleted" which will be used to tell the
			// controller to remove any finalizers).
			tf.Status.Phase = tfv1beta1.PhaseDeleted
			err := r.updateStatusWithRetry(ctx, tf, &tf.Status, reqLogger)
			if err != nil {
				reqLogger.V(1).Info(err.Error())
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{Requeue: false}, nil
	}

	// Check for the current stage pod
	inNamespace := client.InNamespace(getNamespace(tf))
	f := fields.Set{
		"metadata.generateName": fmt.Sprintf("%s-%s-", tf.Status.PodNamePrefix+"-v"+fmt.Sprint(generation), podType),
	}
	labelSelector := map[string]string{
		labelPrefix + "generation": fmt.Sprintf("%d", generation),
	}
	matchingFields := client.MatchingFields(f)
	matchingLabels := client.MatchingLabels(labelSelector)
	pods := &corev1.PodList{}
	err = r.Client.List(ctx, pods, inNamespace, matchingFields, matchingLabels)
	if err != nil {
		reqLogger.Error(err, "")
		return reconcile.Result{}, nil
	}

	if tf.Status.RetryTimestamp != nil {
		podSlice := []corev1.Pod{}
		for _, pod := range pods.Items {
			if pod.CreationTimestamp.IsZero() || !pod.CreationTimestamp.Before(tf.Status.RetryTimestamp) {
				podSlice = append(podSlice, pod)
			}
		}
		pods.Items = podSlice
	}

	if len(pods.Items) == 0 && tf.Status.Stage.State == tfv1beta1.StateInProgress {
		// This condition is generally met when the user deletes the pod.
		// Force the state to transition away from in-progress and then
		// requeue.
		tf.Status.Stage.State = tfv1beta1.StateInitializing
		err = r.updateStatusWithRetry(ctx, tf, &tf.Status, reqLogger)
		if err != nil {
			reqLogger.V(1).Info(err.Error())
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, nil
	}

	if len(pods.Items) == 0 {
		// Trigger a new pod when no pods are found for current stage
		sidecarNames := []string{}
		for pluginTaskName, pluginConfig := range tf.Spec.Plugins {
			if tfv1beta1.ListContainsTask(tf.Status.PluginsStarted, pluginTaskName) {
				continue
			}

			when := pluginConfig.When
			whenTask := pluginConfig.Task
			switch when {
			case "After":
				if whenTask.ID() < podType.ID() {
					defer r.createPluginJob(ctx, reqLogger, tf, pluginTaskName, pluginConfig, globalEnvFrom)
				}
			case "At":
				if whenTask.ID() == podType.ID() {
					defer r.createPluginJob(ctx, reqLogger, tf, pluginTaskName, pluginConfig, globalEnvFrom)
				}
			case "Sidecar":
				if whenTask.ID() == podType.ID() {
					pluginSidecarPod, err := r.getPluginSidecarPod(ctx, reqLogger, tf, pluginTaskName, pluginConfig, globalEnvFrom)
					if err != nil {
						if pluginConfig.Must {
							reqLogger.V(1).Info(err.Error())
							return reconcile.Result{Requeue: true}, nil
						}
						reqLogger.V(1).Info("Error adding sidecar plugin: %s", err.Error())
						continue
					}

					exists := false
					for _, c := range pluginSidecarPod.Spec.Containers {
						if utils.ListContainsStr(sidecarNames, c.Name) {
							exists = true
						}
					}
					if !exists {
						sidecarNames = append(sidecarNames, getContainerNames(pluginSidecarPod)...)
						runOpts.sidecarPlugins = append(runOpts.sidecarPlugins, *pluginSidecarPod)
					}
				}
			}
		}

		if (podType == tfv1beta1.RunPlan || podType == tfv1beta1.RunPlanDelete) && runOpts.requireApproval {
			requireApprovalSidecarPlugin := tfv1beta1.Plugin{
				ImageConfig: tfv1beta1.ImageConfig{
					Image:           runOpts.requireApprovalImage,
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
				Must: true,
			}
			pluginSidecarPod, err := r.getPluginSidecarPod(ctx, reqLogger, tf, tfv1beta1.TaskName("require-approval"), requireApprovalSidecarPlugin, globalEnvFrom)
			if err != nil {
				reqLogger.V(1).Info("Error adding require-approval plugin: %s", err.Error())
				return reconcile.Result{Requeue: true}, nil
			}

			exists := false
			for _, c := range pluginSidecarPod.Spec.Containers {
				if utils.ListContainsStr(sidecarNames, c.Name) {
					exists = true
				}
			}
			if !exists {
				runOpts.sidecarPlugins = append(runOpts.sidecarPlugins, *pluginSidecarPod)
			}
		}

		reqLogger.V(1).Info(fmt.Sprintf("Setting up the '%s' pod", podType))
		err := r.setupAndRun(ctx, tf, runOpts)
		if err != nil {
			reqLogger.Error(err, err.Error())
			return reconcile.Result{}, err
		}
		if tf.Status.Phase == tfv1beta1.PhaseInitializing {
			tf.Status.Phase = tfv1beta1.PhaseRunning
		} else if tf.Status.Phase == tfv1beta1.PhaseInitDelete {
			tf.Status.Phase = tfv1beta1.PhaseDeleting
		}
		tf.Status.Stage.State = tfv1beta1.StateInProgress

		// TODO because the pod is already running, is it critical that the
		// phase and state be updated. The updateStatus function needs to retry
		// if it fails to update.
		err = r.updateStatusWithRetry(ctx, tf, &tf.Status, reqLogger)
		if err != nil {
			reqLogger.V(1).Info(err.Error())
			return reconcile.Result{Requeue: true}, nil
		}
		// When the pod is created, don't requeue. The pod's status changes
		// will trigger tfo to reconcile.
		return reconcile.Result{}, nil
	}

	// At this point, a pod is found for the current stage. We can check the
	// pod status to find out more info about the pod.
	realPod := pods.Items[0]
	podName := realPod.ObjectMeta.Name
	podPhase := realPod.Status.Phase
	msg := fmt.Sprintf("Pod '%s' %s", podName, podPhase)

	// if tf.Status.Stage.PodName != podName {
	// 	if tf.Status.Stage.PodName == "" {
	// 		// This is the first time this pod is found. Set the rerun attempt to 0
	// 		tf.Status.Stage.RerunAttempt = 0
	// 	} else {
	// 		tf.Status.Stage.RerunAttempt++
	// 	}
	// }
	tf.Status.Stage.PodUID = string(realPod.UID)
	tf.Status.Stage.PodName = podName
	if tf.Status.Stage.Message != msg {
		tf.Status.Stage.Message = msg
		reqLogger.Info(msg)
	}

	// TODO Does the user need reason and message?
	// reason := realPod.Status.Reason
	// message := realPod.Status.Message
	// if reason != "" {
	// 	msg = fmt.Sprintf("%s %s", msg, reason)
	// }
	// if message != "" {
	// 	msg = fmt.Sprintf("%s %s", msg, message)
	// }

	if realPod.Status.Phase == corev1.PodFailed {
		tf.Status.Stage.State = tfv1beta1.StateFailed
		tf.Status.Stage.StopTime = metav1.NewTime(time.Now())
		err = r.updateStatusWithRetry(ctx, tf, &tf.Status, reqLogger)
		if err != nil {
			reqLogger.V(1).Info(err.Error())
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if realPod.Status.Phase == corev1.PodSucceeded {
		tf.Status.Stage.State = tfv1beta1.StateComplete
		tf.Status.Stage.StopTime = metav1.NewTime(time.Now())
		err = r.updateStatusWithRetry(ctx, tf, &tf.Status, reqLogger)
		if err != nil {
			reqLogger.V(1).Info(err.Error())
			return reconcile.Result{}, err
		}
		if !tf.Spec.KeepCompletedPods && !tf.Spec.KeepLatestPodsOnly {
			err := r.Client.Delete(ctx, &realPod)
			if err != nil {
				reqLogger.V(1).Info(err.Error())
			}
		}
		return reconcile.Result{}, nil
	}
	tf.Status.Stage.State = tfv1beta1.StageState(realPod.Status.Phase)

	// Finally, update any statuses that have been changed if not already saved. This is probablye
	// for pending condition that does not require anything to be done.
	err = r.updateStatusWithRetry(ctx, tf, &tf.Status, reqLogger)
	if err != nil {
		reqLogger.V(1).Info(err.Error())
		return reconcile.Result{}, err
	}

	// TODO should tf operator "auto" reconciliate (eg plan+apply)?
	// TODO how should we handle manually triggering apply
	return reconcile.Result{}, nil
}

// getTerraformResource fetches the terraform resource with a retry
func (r ReconcileTerraform) getTerraformResource(ctx context.Context, namespacedName types.NamespacedName, maxRetry int, reqLogger logr.Logger) (*tfv1beta1.Terraform, error) {
	tf := &tfv1beta1.Terraform{}
	for retryCount := 1; retryCount <= maxRetry; retryCount++ {
		err := r.Client.Get(ctx, namespacedName, tf)
		if err != nil {
			if errors.IsNotFound(err) {
				return tf, err
			} else if retryCount < maxRetry {
				time.Sleep(100 * time.Millisecond)
				continue
			}
			return tf, err
		} else {
			break
		}
	}
	return tf, nil
}

func newStage(tf *tfv1beta1.Terraform, taskType tfv1beta1.TaskName, reason string, interruptible tfv1beta1.Interruptible, stageState tfv1beta1.StageState) *tfv1beta1.Stage {
	if reason == "GENERATION_CHANGE" {
		tf.Status.PluginsStarted = []tfv1beta1.TaskName{}
		tf.Status.Phase = tfv1beta1.PhaseInitializing
	}
	startTime := metav1.NewTime(time.Now())
	stopTime := metav1.NewTime(time.Unix(0, 0))
	if stageState == tfv1beta1.StateComplete {
		stopTime = startTime
	}
	return &tfv1beta1.Stage{
		Generation:    tf.Generation,
		Interruptible: interruptible,
		Reason:        reason,
		State:         stageState,
		TaskType:      taskType,
		StartTime:     startTime,
		StopTime:      stopTime,
	}
}

func getConfiguredTasks(taskOptions *[]tfv1beta1.TaskOption) []tfv1beta1.TaskName {
	tasks := []tfv1beta1.TaskName{
		tfv1beta1.RunSetup,
		tfv1beta1.RunInit,
		tfv1beta1.RunPlan,
		tfv1beta1.RunApply,
		tfv1beta1.RunSetupDelete,
		tfv1beta1.RunInitDelete,
		tfv1beta1.RunPlanDelete,
		tfv1beta1.RunApplyDelete,
	}
	if taskOptions == nil {
		return tasks
	}
	for _, taskOption := range *taskOptions {
		for _, affected := range taskOption.For {
			if affected == "*" {
				continue
			}
			if !tfv1beta1.ListContainsTask(tasks, affected) {
				tasks = append(tasks, affected)
			}
		}
	}
	return tasks
}

// checkSetNewStage uses the tf resource's `.status.stage` state to find the next stage of the terraform run.
// The following set of rules are used:
//
// 1. Generation - Check that the resource's generation matches the stage's generation. When the generation
// changes the old generation can no longer add a new stage.
//
// 2. Check that the current stage is completed. If it is not, this function returns false and the pod status
// will be determined which will update the stage for the next iteration.
//
// 3. Scripts defined in the tf resource manifest will trigger the script runner podTypes.
//
// When a stage has already triggered a pod, the only way for the pod to transition to the next stage is for
// the pod to complete successfully. Any other pod phase will keep the pod in the current stage, or in the
// case of the apply task, the workflow will be restarted.
func (r ReconcileTerraform) checkSetNewStage(ctx context.Context, tf *tfv1beta1.Terraform, isRetry bool) *tfv1beta1.Stage {
	var isNewStage bool
	var podType tfv1beta1.TaskName
	var reason string
	configuredTasks := getConfiguredTasks(&tf.Spec.TaskOptions)

	deletePhases := []string{
		string(tfv1beta1.PhaseDeleted),
		string(tfv1beta1.PhaseInitDelete),
		string(tfv1beta1.PhaseDeleting),
	}
	isToBeDeletedOrIsDeleting := utils.ListContainsStr(deletePhases, string(tf.Status.Phase))
	initDelete := tf.Status.Phase == tfv1beta1.PhaseInitDelete
	stageState := tfv1beta1.StateInitializing
	interruptible := tfv1beta1.CanBeInterrupt

	currentStage := tf.Status.Stage
	currentStagePodType := currentStage.TaskType
	currentStageCanNotBeInterrupted := currentStage.Interruptible == tfv1beta1.CanNotBeInterrupt
	currentStageIsRunning := currentStage.State == tfv1beta1.StateInProgress
	isNewGeneration := currentStage.Generation != tf.Generation

	if isRetry && !isToBeDeletedOrIsDeleting && !isNewGeneration {
		isNewStage = true
		reason = *tf.Status.RetryEventReason
		podType = tfv1beta1.RunInit
		if strings.HasSuffix(reason, ".setup") {
			podType = tfv1beta1.RunSetup
		}
		interruptible = isTaskInterruptable(podType)
	} else if isRetry && isToBeDeletedOrIsDeleting && !isNewGeneration {
		isNewStage = true
		reason = *tf.Status.RetryEventReason
		podType = tfv1beta1.RunInitDelete
		if strings.HasSuffix(reason, ".setup") {
			podType = tfv1beta1.RunSetupDelete
		}
		interruptible = isTaskInterruptable(podType)
	} else if currentStageCanNotBeInterrupted && currentStageIsRunning {
		// Cannot change to the next stage because the current stage cannot be
		// interrupted and is currently running
		isNewStage = false
	} else if isNewGeneration && !isToBeDeletedOrIsDeleting {
		// The current generation has changed and this is the first pod in the
		// normal terraform workflow
		isNewStage = true
		reason = "GENERATION_CHANGE"
		podType = tfv1beta1.RunSetup

		// } else if initDelete && !utils.ListContainsStr(deletePodTypes, string(currentStagePodType)) {
	} else if isNewGeneration && initDelete {
		// The tf resource is marked for deletion and this is the first pod
		// in the terraform destroy workflow.
		isNewStage = true
		reason = "TF_RESOURCE_DELETED"
		podType = tfv1beta1.RunSetupDelete
		interruptible = tfv1beta1.CanNotBeInterrupt
	} else if isNewGeneration && isToBeDeletedOrIsDeleting {
		// The tf resource is marked for deletion but got updated. It is still going to be deleted but starts
		// a new terraform destroy workflow.
		isNewStage = true
		reason = "TF_RESOURCE_DELETED"
		podType = tfv1beta1.RunSetupDelete
	} else if currentStage.State == tfv1beta1.StateComplete {
		isNewStage = true
		reason = fmt.Sprintf("COMPLETED_%s", strings.ToUpper(currentStage.TaskType.String()))

		switch currentStagePodType {

		case tfv1beta1.RunNil:
			isNewStage = false

		default:
			podType = nextTask(currentStagePodType, configuredTasks)
			interruptible = isTaskInterruptable(podType)
			if podType == tfv1beta1.RunNil {
				stageState = tfv1beta1.StateComplete
			}
		}
	} else if currentStage.State == tfv1beta1.StateFailed {
		if currentStage.TaskType == tfv1beta1.RunApply {

			err := r.Client.Get(ctx, types.NamespacedName{Namespace: getNamespace(tf), Name: tf.Status.Stage.PodName}, &corev1.Pod{})
			if err != nil && errors.IsNotFound(err) {
				// If the task failed, is of type "apply", and the pod does not exist, restart the workflow.
				isNewStage = true
				reason = "RESTARTED_WORKFLOW"
				podType = nextTask(tfv1beta1.RunPostInit, configuredTasks)
				interruptible = isTaskInterruptable(podType)
			}
		} else if currentStage.TaskType == tfv1beta1.RunApplyDelete {
			pod := corev1.Pod{}
			err := r.Client.Get(ctx, types.NamespacedName{Namespace: getNamespace(tf), Name: tf.Status.Stage.PodName}, &pod)
			if err != nil && errors.IsNotFound(err) {
				// If the task failed, is of type "apply", and the pod does not exist, restart the workflow.
				isNewStage = true
				reason = "RESTARTED_DELETE_WORKFLOW"
				podType = nextTask(tfv1beta1.RunPostInitDelete, configuredTasks)
				interruptible = isTaskInterruptable(podType)
			}
		}

	}
	if !isNewStage {
		return nil
	}
	return newStage(tf, podType, reason, interruptible, stageState)

}

func (r ReconcileTerraform) removeOldPlan(tf *tfv1beta1.Terraform) error {
	reason := tf.Status.Stage.Reason
	labelSelectors := getCurrentGenerationLabelSelectors(tf)
	labelSelectors = append(labelSelectors, "app.kubernetes.io/instance")
	if reason == "RESTARTED_WORKFLOW" {
		labelSelectors = append(labelSelectors, []string{
			fmt.Sprintf("app.kubernetes.io/instance!=%s", tfv1beta1.RunSetup),
			fmt.Sprintf("app.kubernetes.io/instance!=%s", tfv1beta1.RunPreInit),
			fmt.Sprintf("app.kubernetes.io/instance!=%s", tfv1beta1.RunInit),
			fmt.Sprintf("app.kubernetes.io/instance!=%s", tfv1beta1.RunPostInit),
		}...)
	} else if reason == "RESTARTED_DELETE_WORKFLOW" {
		labelSelectors = append(labelSelectors, []string{
			fmt.Sprintf("app.kubernetes.io/instance!=%s", tfv1beta1.RunSetupDelete),
			fmt.Sprintf("app.kubernetes.io/instance!=%s", tfv1beta1.RunPreInitDelete),
			fmt.Sprintf("app.kubernetes.io/instance!=%s", tfv1beta1.RunInitDelete),
			fmt.Sprintf("app.kubernetes.io/instance!=%s", tfv1beta1.RunPostInitDelete),
		}...)
	}
	labelSelector, err := labels.Parse(strings.Join(labelSelectors, ","))
	if err != nil {
		return err
	}
	fieldSelector, err := fields.ParseSelector("status.phase!=Running")
	if err != nil {
		return err
	}
	err = r.Client.DeleteAllOf(context.TODO(), &corev1.Pod{}, &client.DeleteAllOfOptions{
		ListOptions: client.ListOptions{
			LabelSelector: labelSelector,
			Namespace:     getNamespace(tf),
			FieldSelector: fieldSelector,
		},
	})
	if err != nil {
		return err
	}
	return nil
}

// These are pods that are known to cause issues with terraform state when
// not run to completion.
func isTaskInterruptable(task tfv1beta1.TaskName) tfv1beta1.Interruptible {
	uninterruptibleTasks := []tfv1beta1.TaskName{
		tfv1beta1.RunInit,
		tfv1beta1.RunPlan,
		tfv1beta1.RunApply,
		tfv1beta1.RunInitDelete,
		tfv1beta1.RunPlanDelete,
		tfv1beta1.RunApplyDelete,
	}
	if tfv1beta1.ListContainsTask(uninterruptibleTasks, task) {
		return tfv1beta1.CanNotBeInterrupt
	}
	return tfv1beta1.CanBeInterrupt
}

func nextTask(currentTask tfv1beta1.TaskName, configuredTasks []tfv1beta1.TaskName) tfv1beta1.TaskName {
	tasksInOrder := []tfv1beta1.TaskName{
		tfv1beta1.RunSetup,
		tfv1beta1.RunPreInit,
		tfv1beta1.RunInit,
		tfv1beta1.RunPostInit,
		tfv1beta1.RunPrePlan,
		tfv1beta1.RunPlan,
		tfv1beta1.RunPostPlan,
		tfv1beta1.RunPreApply,
		tfv1beta1.RunApply,
		tfv1beta1.RunPostApply,
	}
	deleteTasksInOrder := []tfv1beta1.TaskName{
		tfv1beta1.RunSetupDelete,
		tfv1beta1.RunPreInitDelete,
		tfv1beta1.RunInitDelete,
		tfv1beta1.RunPostInitDelete,
		tfv1beta1.RunPrePlanDelete,
		tfv1beta1.RunPlanDelete,
		tfv1beta1.RunPostPlanDelete,
		tfv1beta1.RunPreApplyDelete,
		tfv1beta1.RunApplyDelete,
		tfv1beta1.RunPostApplyDelete,
	}

	next := tfv1beta1.RunNil
	isUpNext := false
	if tfv1beta1.ListContainsTask(tasksInOrder, currentTask) {
		for _, task := range tasksInOrder {
			if task == currentTask {
				isUpNext = true
				continue
			}
			if isUpNext && tfv1beta1.ListContainsTask(configuredTasks, task) {
				next = task
				break
			}
		}
	} else if tfv1beta1.ListContainsTask(deleteTasksInOrder, currentTask) {
		for _, task := range deleteTasksInOrder {
			if task == currentTask {
				isUpNext = true
				continue
			}
			if isUpNext && tfv1beta1.ListContainsTask(configuredTasks, task) {
				next = task
				break
			}
		}
	}
	return next
}

func getOldGenerationLabelSelectorString(tf *tfv1beta1.Terraform) string {
	return strings.Join(getOldGenerationLabelSelectors(tf), ",")
}

// getOldGenerationLabelSelectors returns the label requirements used to filter
// pods and other resources for generation other than the current one.
func getOldGenerationLabelSelectors(tf *tfv1beta1.Terraform) []string {
	// 1. The terraforms.tf.galleybytes.com/generation key MUST exist
	// 2. The terraforms.tf.galleybytes.com/generation value MUST NOT match the current resource generation
	// 3. The terraforms.tf.galleybytes.com/resourceName value MUST match the resource name
	// 4. The terraforms.tf.galleybytes.com/resourceNamespace value MUST match the resource namespace
	return []string{
		labelPrefix + "generation",
		labelPrefix + "generation!=" + fmt.Sprintf("%d", tf.Generation),
		labelPrefix + "resourceName=" + utils.AutoHashLabeler(tf.Name),
		labelPrefix + "resourceNamespace=" + utils.AutoHashLabeler(tf.Namespace),
	}
}

// getCurrentGenerationLabelSelectors returns the label requirements used to
// filter pods and other resources for the current generation.
func getCurrentGenerationLabelSelectors(tf *tfv1beta1.Terraform) []string {
	// 1. The terraforms.tf.galleybytes.com/generation value MUST match the current resource generation
	// 2. The terraforms.tf.galleybytes.com/resourceName value MUST match the resource name
	// 3. The terraforms.tf.galleybytes.com/resourceNamespace value MUST match the resource namespace
	return []string{
		labelPrefix + "generation=" + fmt.Sprintf("%d", tf.Generation),
		labelPrefix + "resourceName=" + utils.AutoHashLabeler(tf.Name),
		labelPrefix + "resourceNamespace=" + utils.AutoHashLabeler(tf.Namespace),
	}
}

func (r ReconcileTerraform) backgroundReapOldGenerationPods(tf *tfv1beta1.Terraform, attempt int) {
	logger := r.Log.WithName("Reaper").WithValues("Terraform", fmt.Sprintf("%s/%s", tf.Namespace, tf.Name))
	if attempt > 20 {
		// TODO explain what and way resources cannot be reaped
		logger.Info("Could not reap resources: Max attempts to reap old-generation resources")
		return
	}

	// Before running a deletion, make sure we've got the most up-to-date resource in case a background
	// process takes longer than normal to complete.
	ctx := context.TODO()
	namespacedName := types.NamespacedName{Namespace: tf.Namespace, Name: tf.Name}
	tf, err := r.getTerraformResource(ctx, namespacedName, 3, logger)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.V(1).Info("Terraform resource not found. Ignoring since object must be deleted")
			return
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get Terraform")
		return
	}

	labelSelector, err := labels.Parse(getOldGenerationLabelSelectorString(tf))
	if err != nil {
		logger.Error(err, "Could not parse labels")
		return
	}
	fieldSelector, err := fields.ParseSelector("status.phase!=Running")
	if err != nil {
		logger.Error(err, "Could not parse fields")
		return
	}

	err = r.Client.DeleteAllOf(context.TODO(), &corev1.Pod{}, &client.DeleteAllOfOptions{
		ListOptions: client.ListOptions{
			LabelSelector: labelSelector,
			Namespace:     getNamespace(tf),
			FieldSelector: fieldSelector,
		},
	})
	if err != nil {
		logger.Error(err, "Could not reap old generation pods")
		return
	}

	// Wait for all the pods of the previous generations to be gone. Only after
	// the pods are cleaned up, clean up other associated resources like roles
	// and rolebindings.
	podList := corev1.PodList{}
	err = r.Client.List(context.TODO(), &podList, &client.ListOptions{
		LabelSelector: labelSelector,
		Namespace:     getNamespace(tf),
	})
	if err != nil {
		logger.Error(err, "Could not list pods to reap")
		return
	}
	if len(podList.Items) > 0 {
		// There are still some pods from a previous generation hanging around
		// for some reason. Wait some time and try to reap again later.
		time.Sleep(30 * time.Second)
		attempt++
		go r.backgroundReapOldGenerationPods(tf, attempt)
	} else {
		// All old pods are gone and the other resouces will now be removed
		err = r.Client.DeleteAllOf(context.TODO(), &corev1.ConfigMap{}, &client.DeleteAllOfOptions{
			ListOptions: client.ListOptions{
				LabelSelector: labelSelector,
				Namespace:     getNamespace(tf),
			},
		})
		if err != nil {
			logger.Error(err, "Could not reap old generation configmaps")
			return
		}

		err = r.Client.DeleteAllOf(context.TODO(), &corev1.Secret{}, &client.DeleteAllOfOptions{
			ListOptions: client.ListOptions{
				LabelSelector: labelSelector,
				Namespace:     getNamespace(tf),
			},
		})
		if err != nil {
			logger.Error(err, "Could not reap old generation secrets")
			return
		}

		err = r.Client.DeleteAllOf(context.TODO(), &rbacv1.Role{}, &client.DeleteAllOfOptions{
			ListOptions: client.ListOptions{
				LabelSelector: labelSelector,
				Namespace:     getNamespace(tf),
			},
		})
		if err != nil {
			logger.Error(err, "Could not reap old generation roles")
			return
		}

		err = r.Client.DeleteAllOf(context.TODO(), &rbacv1.RoleBinding{}, &client.DeleteAllOfOptions{
			ListOptions: client.ListOptions{
				LabelSelector: labelSelector,
				Namespace:     getNamespace(tf),
			},
		})
		if err != nil {
			logger.Error(err, "Could not reap old generation roleBindings")
			return
		}

		err = r.Client.DeleteAllOf(context.TODO(), &corev1.ServiceAccount{}, &client.DeleteAllOfOptions{
			ListOptions: client.ListOptions{
				LabelSelector: labelSelector,
				Namespace:     getNamespace(tf),
			},
		})
		if err != nil {
			logger.Error(err, "Could not reap old generation serviceAccounts")
			return
		}
	}
}

func (r ReconcileTerraform) reapPlugins(tf *tfv1beta1.Terraform, attempt int) {
	logger := r.Log.WithName("ReaperPlugins").WithValues("Terraform", fmt.Sprintf("%s/%s", tf.Namespace, tf.Name))
	if attempt > 20 {
		// TODO explain what and way resources cannot be reaped
		logger.Info("Could not reap resources: Max attempts to reap old-generation resources")
		return
	}
	// Before running a deletion, make sure we've got the most up-to-date resource in case a background
	// process takes longer than normal to complete.
	ctx := context.TODO()
	namespacedName := types.NamespacedName{Namespace: tf.Namespace, Name: tf.Name}
	tf, err := r.getTerraformResource(ctx, namespacedName, 3, logger)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.V(1).Info("Terraform resource not found. Ignoring since object must be deleted")
			return
		}
		// Error reading the object - requeue the request.
		logger.Error(err, "Failed to get Terraform")
		return
	}

	// Delete old plugins regardless of pod phase
	labelSelectorForPlugins, err := labels.Parse(labelPrefix + "isPlugin=true," + getOldGenerationLabelSelectorString(tf))
	if err != nil {
		logger.Error(err, "Could not parse labels")
		return
	}

	deleteProppagationBackground := metav1.DeletePropagationBackground
	err = r.Client.DeleteAllOf(context.TODO(), &batchv1.Job{}, &client.DeleteAllOfOptions{
		ListOptions: client.ListOptions{
			LabelSelector: labelSelectorForPlugins,
			Namespace:     getNamespace(tf),
		},
		DeleteOptions: client.DeleteOptions{
			PropagationPolicy: &deleteProppagationBackground,
		},
	})
	if err != nil {
		logger.Error(err, "Could not reap old generation jobs")
	}

	err = r.Client.DeleteAllOf(context.TODO(), &corev1.Pod{}, &client.DeleteAllOfOptions{
		ListOptions: client.ListOptions{
			LabelSelector: labelSelectorForPlugins,
			Namespace:     getNamespace(tf),
		},
	})
	if err != nil {
		logger.Error(err, "Could not reap old generation pods")
	}

	// Wait for all the pods of the previous generations to be gone. Only after
	// the pods are cleaned up, clean up other associated resources like roles
	// and rolebindings.
	podList := corev1.PodList{}
	err = r.Client.List(context.TODO(), &podList, &client.ListOptions{
		LabelSelector: labelSelectorForPlugins,
		Namespace:     getNamespace(tf),
	})
	if err != nil {
		logger.Error(err, "Could not list pods to reap")
	}
	if len(podList.Items) > 0 {
		// There are still some pods from a previous generation hanging around
		// for some reason. Wait some time and try to reap again later.
		time.Sleep(30 * time.Second)
		attempt++
		go r.reapPlugins(tf, attempt)
	}
}

func (r ReconcileTerraform) getNodeSelectorsFromCache() (*corev1.Affinity, map[string]string, []corev1.Toleration) {
	var affinity *corev1.Affinity
	var nodeSelector map[string]string
	var tolerations []corev1.Toleration
	if r.InheritAffinity {
		if obj, found := r.Cache.Get(r.AffinityCacheKey); found {
			affinity = obj.(*corev1.Affinity)
		}
	}
	if r.InheritNodeSelector {
		if obj, found := r.Cache.Get(r.NodeSelectorCacheKey); found {
			nodeSelector = obj.(map[string]string)
		}
	}
	if r.InheritTolerations {
		if obj, found := r.Cache.Get(r.TolerationsCacheKey); found {
			tolerations = obj.([]corev1.Toleration)
		}
	}

	return affinity, nodeSelector, tolerations
}

// Define a set of TaskOptions specific for the plugin task
func (r ReconcileTerraform) getPluginRunOpts(tf *tfv1beta1.Terraform, pluginTaskName tfv1beta1.TaskName, pluginConfig tfv1beta1.Plugin, globalEnvFrom []corev1.EnvFromSource) TaskOptions {
	affinity, nodeSelector, tolerations := r.getNodeSelectorsFromCache()
	pluginRunOpts := newTaskOptions(tf, pluginTaskName, tf.Generation, globalEnvFrom, affinity, nodeSelector, tolerations, r.RequireApprovalImage)
	pluginRunOpts.image = pluginConfig.Image
	pluginRunOpts.imagePullPolicy = pluginConfig.ImagePullPolicy
	return pluginRunOpts
}

func (r ReconcileTerraform) getPluginSidecarPod(ctx context.Context, logger logr.Logger, tf *tfv1beta1.Terraform, pluginTaskName tfv1beta1.TaskName, pluginConfig tfv1beta1.Plugin, globalEnvFrom []corev1.EnvFromSource) (*corev1.Pod, error) {
	return r.getPluginRunOpts(tf, pluginTaskName, pluginConfig, globalEnvFrom).generatePod()
}

// createPluginJob will attempt to create the plugin pod and mark it as added in the resource's status.
// No logic is used to determine if the plugin was successful. If the createPod function errors, a log event
// is recorded in the controller.
func (r ReconcileTerraform) createPluginJob(ctx context.Context, logger logr.Logger, tf *tfv1beta1.Terraform, pluginTaskName tfv1beta1.TaskName, pluginConfig tfv1beta1.Plugin, globalEnvFrom []corev1.EnvFromSource) (reconcile.Result, error) {
	pluginRunOpts := r.getPluginRunOpts(tf, pluginTaskName, pluginConfig, globalEnvFrom)

	go func() {
		err := r.createJob(ctx, tf, pluginRunOpts)
		if err != nil {
			logger.Error(err, fmt.Sprintf("Failed creating plugin job %s", pluginTaskName))
		} else {
			logger.Info(fmt.Sprintf("Starting the plugin job '%s'", pluginTaskName.String()))
		}
	}()
	tf.Status.PluginsStarted = append(tf.Status.PluginsStarted, pluginTaskName)
	err := r.updateStatusWithRetry(ctx, tf, &tf.Status, logger)
	if err != nil {
		logger.V(1).Info(err.Error())
	}
	return reconcile.Result{}, err
}

// updateFinalizer sets and unsets the finalizer on the tf resource. When
// IgnoreDelete is true, the finalizer is removed. When IgnoreDelete is false,
// the finalizer is added.
//
// The finalizer will be responsible for starting the destroy-workflow.
func updateFinalizer(tf *tfv1beta1.Terraform) bool {
	finalizers := tf.GetFinalizers()

	if tf.Status.Phase == tfv1beta1.PhaseDeleted {
		if utils.ListContainsStr(finalizers, terraformFinalizer) {
			tf.SetFinalizers(utils.ListRemoveStr(finalizers, terraformFinalizer))
			return true
		}
	}

	if tf.Spec.IgnoreDelete && len(finalizers) > 0 {
		if utils.ListContainsStr(finalizers, terraformFinalizer) {
			tf.SetFinalizers(utils.ListRemoveStr(finalizers, terraformFinalizer))
			return true
		}
	}

	if !tf.Spec.IgnoreDelete {
		if !utils.ListContainsStr(finalizers, terraformFinalizer) {
			tf.SetFinalizers(append(finalizers, terraformFinalizer))
			return true
		}
	}
	return false
}

// Here we determine if secret in SCMAuthMethods array should be locked via finalizer or not

type gitSecret struct {
	name          string
	namespace     string
	shoudBeLocked bool
}

func (r ReconcileTerraform) getGitSecrets(tf *tfv1beta1.Terraform) []gitSecret {
	secrets := []gitSecret{}
	for _, m := range tf.Spec.SCMAuthMethods {
		if m.Git.HTTPS != nil {
			ref := m.Git.HTTPS.TokenSecretRef
			namespace := ref.Namespace
			if ref.Namespace == "" {
				namespace = getNamespace(tf)
			}
			secrets = append(secrets, gitSecret{
				name:          ref.Name,
				namespace:     namespace,
				shoudBeLocked: ref.LockSecretDeletion && !tf.Spec.IgnoreDelete,
			})
		}
		if m.Git.SSH != nil {
			ref := m.Git.SSH.SSHKeySecretRef
			namespace := ref.Namespace
			if ref.Namespace == "" {
				namespace = getNamespace(tf)
			}
			secrets = append(secrets, gitSecret{
				name:          ref.Name,
				namespace:     namespace,
				shoudBeLocked: ref.LockSecretDeletion && !tf.Spec.IgnoreDelete,
			})
		}
	}
	return secrets
}

// updateSecretFinalizer sets and unsets finalizers on all secrets mentioned in spec.scmAuthMethods
// to ensure terraform workflow will work properly.
func (r ReconcileTerraform) updateSecretFinalizer(ctx context.Context, tf *tfv1beta1.Terraform) error {
	finalizerKey := utils.TruncateResourceName(fmt.Sprintf("finalizer.tf.galleybytes.com/%s", tf.Name), 53)

	secrets := r.getGitSecrets(tf)
	for _, m := range secrets {
		if m.shoudBeLocked && tf.Status.Phase != tfv1beta1.PhaseDeleted {
			if err := r.lockGitSecretDeletion(ctx, m.name, m.namespace, finalizerKey); err != nil {
				return err
			}
		} else {
			if err := r.unlockGitSecretDeletion(ctx, m.name, m.namespace, finalizerKey); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r ReconcileTerraform) lockGitSecretDeletion(ctx context.Context, name, namespace, finalizerKey string) error {
	secret, err := r.loadSecret(ctx, name, namespace)
	if err != nil {
		return err
	}
	if !controllerutil.ContainsFinalizer(secret, finalizerKey) {
		controllerutil.AddFinalizer(secret, finalizerKey)
		if err := r.Client.Update(ctx, secret); err != nil {
			return err
		}
	}
	return nil
}

func (r ReconcileTerraform) unlockGitSecretDeletion(ctx context.Context, name, namespace, finalizerKey string) error {
	secret, err := r.loadSecret(ctx, name, namespace)
	if err != nil {
		return err
	}
	if controllerutil.ContainsFinalizer(secret, finalizerKey) {
		controllerutil.RemoveFinalizer(secret, finalizerKey)
		if err := r.Client.Update(ctx, secret); err != nil {
			return err
		}
	}
	return nil
}

func (r ReconcileTerraform) update(ctx context.Context, tf *tfv1beta1.Terraform) error {
	err := r.Client.Update(ctx, tf)
	if err != nil {
		return fmt.Errorf("failed to update tf resource: %s", err)
	}
	return nil
}

func (r ReconcileTerraform) updateStatus(ctx context.Context, tf *tfv1beta1.Terraform) error {
	err := r.Client.Status().Update(ctx, tf)
	if err != nil {
		return fmt.Errorf("failed to update tf status: %s", err)
	}
	return nil
}

func (r ReconcileTerraform) updateStatusWithRetry(ctx context.Context, tf *tfv1beta1.Terraform, desiredStatus *tfv1beta1.TerraformStatus, logger logr.Logger) error {
	resourceNamespacedName := types.NamespacedName{Namespace: tf.Namespace, Name: tf.Name}
	var getResourceErr error
	var updateErr error
	for i := 0; i < 10; i++ {
		if i > 0 {
			n := math.Pow(2, float64(i+3))
			backoffTime := math.Ceil(.5 * (n - 1))
			time.Sleep(time.Duration(backoffTime) * time.Millisecond)
			tf, getResourceErr = r.getTerraformResource(ctx, resourceNamespacedName, 10, logger)
			if getResourceErr != nil {
				return fmt.Errorf("failed to get latest terraform while updating status: %s", getResourceErr)
			}
			if desiredStatus != nil {
				tf.Status = *desiredStatus
			}
		}
		updateErr = r.Client.Status().Update(ctx, tf)
		if updateErr != nil {
			logger.V(7).Info(fmt.Sprintf("Retrying to update status because an error has occurred while updating: %s", updateErr))
			continue
		}

		// Confirm the status is up to date
		isUpdateConfirmed := false
		for j := 0; j < 10; j++ {
			tf, updatedResourceErr := r.getTerraformResource(ctx, resourceNamespacedName, 10, logger)
			if updatedResourceErr != nil {
				return fmt.Errorf("failed to get latest terraform while validating status: %s", updatedResourceErr)
			}

			if !tfv1beta1.TaskListsAreEqual(tf.Status.PluginsStarted, desiredStatus.PluginsStarted) {
				logger.V(7).Info(fmt.Sprintf("Failed to confirm the status update because plugins did not equal. Have %s and Want %s", tf.Status.PluginsStarted, desiredStatus.PluginsStarted))

			} else if stageItem := tf.Status.Stage.IsEqual(desiredStatus.Stage); stageItem != "" {
				logger.V(7).Info(fmt.Sprintf("Failed to confirm the status update because stage item %s did not equal", stageItem))

			} else if tf.Status.Phase != desiredStatus.Phase {
				logger.V(7).Info("Failed to confirm the status update because phase did not equal")

			} else if tf.Status.PodNamePrefix != desiredStatus.PodNamePrefix {
				logger.V(7).Info("Failed to confirm the status update because podNamePrefix did not equal")

			} else {
				isUpdateConfirmed = true
			}

			if isUpdateConfirmed {
				break
			}

			logger.V(7).Info("Retrying to confirm the status update")
			n := math.Pow(2, float64(j+3))
			backoffTime := math.Ceil(.5 * (n - 1))
			time.Sleep(time.Duration(backoffTime) * time.Millisecond)
		}

		if isUpdateConfirmed {
			break
		}
		logger.V(7).Info("Retrying to update status because the update was not confirmed")

	}
	if updateErr != nil {
		return fmt.Errorf("failed to update tf status: %s", updateErr)
	}
	return nil
}

// IsJobFinished returns true if the job has completed
func IsJobFinished(job *batchv1.Job) bool {
	BackoffLimit := job.Spec.BackoffLimit
	return job.Status.CompletionTime != nil || (job.Status.Active == 0 && BackoffLimit != nil && job.Status.Failed >= *BackoffLimit)
}

func formatJobSSHConfig(ctx context.Context, reqLogger logr.Logger, tf *tfv1beta1.Terraform, k8sclient client.Client) (map[string][]byte, error) {
	data := make(map[string]string)
	dataAsByte := make(map[string][]byte)
	if tf.Spec.SSHTunnel != nil {
		data["config"] = fmt.Sprintf("Host proxy\n"+
			"\tStrictHostKeyChecking no\n"+
			"\tUserKnownHostsFile=/dev/null\n"+
			"\tUser %s\n"+
			"\tHostname %s\n"+
			"\tIdentityFile ~/.ssh/proxy_key\n",
			tf.Spec.SSHTunnel.User,
			tf.Spec.SSHTunnel.Host)
		k := tf.Spec.SSHTunnel.SSHKeySecretRef.Key
		if k == "" {
			k = "id_rsa"
		}
		ns := tf.Spec.SSHTunnel.SSHKeySecretRef.Namespace
		if ns == "" {
			ns = getNamespace(tf)
		}

		key, err := loadPassword(ctx, k8sclient, k, tf.Spec.SSHTunnel.SSHKeySecretRef.Name, ns)
		if err != nil {
			return dataAsByte, err
		}
		data["proxy_key"] = key

	}

	for _, m := range tf.Spec.SCMAuthMethods {

		// TODO validate SSH in resource manifest
		if m.Git.SSH != nil {
			if m.Git.SSH.RequireProxy {
				data["config"] += fmt.Sprintf("\nHost %s\n"+
					"\tStrictHostKeyChecking no\n"+
					"\tUserKnownHostsFile=/dev/null\n"+
					"\tHostname %s\n"+
					"\tIdentityFile ~/.ssh/%s\n"+
					"\tProxyJump proxy",
					m.Host,
					m.Host,
					m.Host)
			} else {
				data["config"] += fmt.Sprintf("\nHost %s\n"+
					"\tStrictHostKeyChecking no\n"+
					"\tUserKnownHostsFile=/dev/null\n"+
					"\tHostname %s\n"+
					"\tIdentityFile ~/.ssh/%s\n",
					m.Host,
					m.Host,
					m.Host)
			}
			k := m.Git.SSH.SSHKeySecretRef.Key
			if k == "" {
				k = "id_rsa"
			}
			ns := m.Git.SSH.SSHKeySecretRef.Namespace
			if ns == "" {
				ns = getNamespace(tf)
			}
			key, err := loadPassword(ctx, k8sclient, k, m.Git.SSH.SSHKeySecretRef.Name, ns)
			if err != nil {
				return dataAsByte, err
			}
			data[m.Host] = key
		}
	}

	for k, v := range data {
		dataAsByte[k] = []byte(v)
	}

	return dataAsByte, nil
}

func (r *ReconcileTerraform) setupAndRun(ctx context.Context, tf *tfv1beta1.Terraform, runOpts TaskOptions) error {
	reqLogger := r.Log.WithValues("Terraform", types.NamespacedName{Name: tf.Name, Namespace: tf.Namespace}.String())
	var err error

	reason := tf.Status.Stage.Reason
	isNewGeneration := reason == "GENERATION_CHANGE" || reason == "TF_RESOURCE_DELETED"
	isFirstInstall := reason == "TF_RESOURCE_CREATED"
	isChanged := isNewGeneration || isFirstInstall
	// r.Recorder.Event(tf, "Normal", "InitializeJobCreate", fmt.Sprintf("Setting up a Job"))
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.
	scmMap := make(map[string]scmType)
	for _, v := range tf.Spec.SCMAuthMethods {
		if v.Git != nil {
			scmMap[v.Host] = gitScmType
		}
	}

	if tf.Spec.TerraformModule.Inline != "" {
		// Add add inline to configmap and instruct the pod to fetch the
		// configmap as the main module
		runOpts.mainModulePluginData["inline-module.tf"] = tf.Spec.TerraformModule.Inline
	} else if tf.Spec.TerraformModule.ConfigMapSeclector_x != nil {
		b, err := json.Marshal(tf.Spec.TerraformModule.ConfigMapSeclector_x)
		if err != nil {
			return err
		}
		runOpts.mainModulePluginData[".__TFO__ConfigMapModule.json"] = string(b)
	} else if tf.Spec.TerraformModule.ConfigMapSelector != nil {
		// Instruct the setup pod to fetch the configmap as the main module
		b, err := json.Marshal(tf.Spec.TerraformModule.ConfigMapSelector)
		if err != nil {
			return err
		}
		runOpts.mainModulePluginData[".__TFO__ConfigMapModule.json"] = string(b)
	} else if tf.Spec.TerraformModule.Source != "" {
		runOpts.terraformModuleParsed, err = getParsedAddress(tf.Spec.TerraformModule.Source, "", false, scmMap)
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("no terraform module detected")
	}

	if isChanged {
		// Secret finalizers
		if err := r.updateSecretFinalizer(ctx, tf); err != nil {
			reqLogger.V(3).Info("Could not update secret finalizer", "ERR", err.Error())
		}

		go r.reapPlugins(tf, 0)

		// Add all default inine files
		runOpts.mainModulePluginData["default-terraform.sh"] = defaultInlineTerraformTaskExecutionFile
		runOpts.mainModulePluginData["default-setup.sh"] = defaultInlineSetupTaskExecutionFile
		runOpts.mainModulePluginData["default-noop.sh"] = defaultInlineNoOpExecutionFile

		for _, taskOption := range tf.Spec.TaskOptions {
			if inlineScript := taskOption.Script.Inline; inlineScript != "" {
				for _, affected := range taskOption.For {
					if affected.String() == "*" {
						continue
					}
					// This adds all the inline scripts found in taskOptions into a configmap. The configmap is not changed
					// for the generation of the workflow.
					runOpts.mainModulePluginData[fmt.Sprintf("inline-%s.sh", affected)] = inlineScript

				}
			}
		}

		// Set up the HTTPS token to use if defined
		for _, m := range tf.Spec.SCMAuthMethods {
			// This loop is used to find the first HTTPS token-based
			// authentication which gets added to all runners' "GIT_ASKPASS"
			// script/env var.
			// TODO
			//		Is there a way to allow multiple tokens for HTTPS access
			//		to git scm?
			if m.Git.HTTPS != nil {
				if _, found := runOpts.secretData["gitAskpass"]; found {
					continue
				}
				tokenSecret := *m.Git.HTTPS.TokenSecretRef
				if tokenSecret.Key == "" {
					tokenSecret.Key = "token"
				}
				gitAskpass, err := r.createGitAskpass(ctx, tokenSecret)
				if err != nil {
					return err
				}
				runOpts.secretData["gitAskpass"] = gitAskpass

			}
		}

		// Set up the SSH keys to use if defined
		sshConfigData, err := formatJobSSHConfig(ctx, reqLogger, tf, r.Client)
		if err != nil {
			r.Recorder.Event(tf, "Warning", "SSHConfigError", fmt.Errorf("%v", err).Error())
			return fmt.Errorf("error setting up sshconfig: %v", err)
		}
		for k, v := range sshConfigData {
			runOpts.secretData[k] = v
		}

		resourceDownloadItems := []ParsedAddress{}
		// Configure the resourceDownloads in JSON that the setupRunner will
		// use to download the resources into the main module directory

		// ConfigMap Data only needs to be updated when generation changes
		if tf.Spec.Setup != nil {
			for _, s := range tf.Spec.Setup.ResourceDownloads {
				address := strings.TrimSpace(s.Address)
				parsedAddress, err := getParsedAddress(address, s.Path, s.UseAsVar, scmMap)
				if err != nil {
					return err
				}
				// b, err := json.Marshal(parsedAddress)
				// if err != nil {
				// 	return err
				// }
				resourceDownloadItems = append(resourceDownloadItems, parsedAddress)
			}
		}
		b, err := json.Marshal(resourceDownloadItems)
		if err != nil {
			return err
		}
		resourceDownloads := string(b)

		runOpts.mainModulePluginData[".__TFO__ResourceDownloads.json"] = resourceDownloads

		// Override the backend.tf by inserting a custom backend
		runOpts.mainModulePluginData["backend_override.tf"] = tf.Spec.Backend
	}

	// RUN
	err = r.run(ctx, reqLogger, tf, runOpts, isNewGeneration, isFirstInstall)
	if err != nil {
		return err
	}

	return nil
}

func (r ReconcileTerraform) checkPersistentVolumeClaimExists(ctx context.Context, lookupKey types.NamespacedName) (*corev1.PersistentVolumeClaim, bool, error) {
	resource := &corev1.PersistentVolumeClaim{}

	err := r.Client.Get(ctx, lookupKey, resource)
	if err != nil && errors.IsNotFound(err) {
		return resource, false, nil
	} else if err != nil {
		return resource, false, err
	}
	return resource, true, nil
}

func (r ReconcileTerraform) createPVC(ctx context.Context, tf *tfv1beta1.Terraform, runOpts TaskOptions) error {
	kind := "PersistentVolumeClaim"
	_, found, err := r.checkPersistentVolumeClaimExists(ctx, types.NamespacedName{
		Name:      runOpts.prefixedName,
		Namespace: runOpts.namespace,
	})
	if err != nil {
		return nil
	} else if found {
		return nil
	}
	persistentVolumeSize := resource.MustParse("2Gi")
	if tf.Spec.PersistentVolumeSize != nil {
		persistentVolumeSize = *tf.Spec.PersistentVolumeSize
	}
	resource := runOpts.generatePVC(persistentVolumeSize, tf.Spec.StorageClassName)
	controllerutil.SetControllerReference(tf, resource, r.Scheme)

	err = r.Client.Create(ctx, resource)
	if err != nil {
		r.Recorder.Event(tf, "Warning", fmt.Sprintf("%sCreateError", kind), fmt.Sprintf("Could not create %s %v", kind, err))
		return err
	}
	r.Recorder.Event(tf, "Normal", "SuccessfulCreate", fmt.Sprintf("Created %s: '%s'", kind, resource.Name))
	return nil
}

func (r ReconcileTerraform) checkConfigMapExists(ctx context.Context, lookupKey types.NamespacedName) (*corev1.ConfigMap, bool, error) {
	resource := &corev1.ConfigMap{}

	err := r.Client.Get(ctx, lookupKey, resource)
	if err != nil && errors.IsNotFound(err) {
		return resource, false, nil
	} else if err != nil {
		return resource, false, err
	}
	return resource, true, nil
}

func (r ReconcileTerraform) deleteConfigMapIfExists(ctx context.Context, name, namespace string) error {
	lookupKey := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	resource, found, err := r.checkConfigMapExists(ctx, lookupKey)
	if err != nil {
		return err
	}
	if found {
		err = r.Client.Delete(ctx, resource)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r ReconcileTerraform) createConfigMap(ctx context.Context, tf *tfv1beta1.Terraform, runOpts TaskOptions) error {
	kind := "ConfigMap"

	resource := runOpts.generateConfigMap()
	controllerutil.SetControllerReference(tf, resource, r.Scheme)

	err := r.deleteConfigMapIfExists(ctx, resource.Name, resource.Namespace)
	if err != nil {
		return err
	}
	err = r.Client.Create(ctx, resource)
	if err != nil {
		r.Recorder.Event(tf, "Warning", fmt.Sprintf("%sCreateError", kind), fmt.Sprintf("Could not create %s %v", kind, err))
		return err
	}
	r.Recorder.Event(tf, "Normal", "SuccessfulCreate", fmt.Sprintf("Created %s: '%s'", kind, resource.Name))
	return nil
}

func (r ReconcileTerraform) checkSecretExists(ctx context.Context, lookupKey types.NamespacedName) (*corev1.Secret, bool, error) {
	resource := &corev1.Secret{}

	err := r.Client.Get(ctx, lookupKey, resource)
	if err != nil && errors.IsNotFound(err) {
		return resource, false, nil
	} else if err != nil {
		return resource, false, err
	}
	return resource, true, nil
}

func (r ReconcileTerraform) deleteSecretIfExists(ctx context.Context, name, namespace string) error {
	lookupKey := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	resource, found, err := r.checkSecretExists(ctx, lookupKey)
	if err != nil {
		return err
	}
	if found {
		err = r.Client.Delete(ctx, resource)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r ReconcileTerraform) createSecret(ctx context.Context, tf *tfv1beta1.Terraform, name, namespace string, data map[string][]byte, recreate bool, labelsToOmit []string, runOpts TaskOptions) error {
	kind := "Secret"

	// Must make a clean map of labels since the memory address is shared
	// for the entire RunOptions struct
	labels := make(map[string]string)
	for key, value := range runOpts.resourceLabels {
		labels[key] = value
	}
	for _, labelKey := range labelsToOmit {
		delete(labels, labelKey)
	}

	resource := runOpts.generateSecret(name, namespace, data, labels)
	controllerutil.SetControllerReference(tf, resource, r.Scheme)

	if recreate {
		err := r.deleteSecretIfExists(ctx, resource.Name, resource.Namespace)
		if err != nil {
			return err
		}
	}

	err := r.Client.Create(ctx, resource)
	if err != nil {
		if !recreate && errors.IsAlreadyExists(err) {
			// This is acceptable since the resource exists and was not
			// expected to be a new resource.
		} else {
			r.Recorder.Event(tf, "Warning", fmt.Sprintf("%sCreateError", kind), fmt.Sprintf("Could not create %s %v", kind, err))
			return err
		}
	} else {
		r.Recorder.Event(tf, "Normal", "SuccessfulCreate", fmt.Sprintf("Created %s: '%s'", kind, resource.Name))
	}
	return nil
}

func (r ReconcileTerraform) checkServiceAccountExists(ctx context.Context, lookupKey types.NamespacedName) (*corev1.ServiceAccount, bool, error) {
	resource := &corev1.ServiceAccount{}

	err := r.Client.Get(ctx, lookupKey, resource)
	if err != nil && errors.IsNotFound(err) {
		return resource, false, nil
	} else if err != nil {
		return resource, false, err
	}
	return resource, true, nil
}

func (r ReconcileTerraform) deleteServiceAccountIfExists(ctx context.Context, name, namespace string) error {
	lookupKey := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	resource, found, err := r.checkServiceAccountExists(ctx, lookupKey)
	if err != nil {
		return err
	}
	if found {
		err = r.Client.Delete(ctx, resource)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r ReconcileTerraform) createServiceAccount(ctx context.Context, tf *tfv1beta1.Terraform, runOpts TaskOptions) error {
	kind := "ServiceAccount"

	resource := runOpts.generateServiceAccount()
	controllerutil.SetControllerReference(tf, resource, r.Scheme)

	err := r.deleteServiceAccountIfExists(ctx, resource.Name, resource.Namespace)
	if err != nil {
		return err
	}
	err = r.Client.Create(ctx, resource)
	if err != nil {
		r.Recorder.Event(tf, "Warning", fmt.Sprintf("%sCreateError", kind), fmt.Sprintf("Could not create %s %v", kind, err))
		return err
	}
	r.Recorder.Event(tf, "Normal", "SuccessfulCreate", fmt.Sprintf("Created %s: '%s'", kind, resource.Name))
	return nil
}

func (r ReconcileTerraform) checkRoleExists(ctx context.Context, lookupKey types.NamespacedName) (*rbacv1.Role, bool, error) {
	resource := &rbacv1.Role{}
	err := r.Client.Get(ctx, lookupKey, resource)
	if err != nil && errors.IsNotFound(err) {
		return resource, false, nil
	} else if err != nil {
		return resource, false, err
	}
	return resource, true, nil
}

func (r ReconcileTerraform) deleteRoleIfExists(ctx context.Context, name, namespace string) error {
	lookupKey := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	resource, found, err := r.checkRoleExists(ctx, lookupKey)
	if err != nil {
		return err
	}
	if found {
		err = r.Client.Delete(ctx, resource)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r ReconcileTerraform) createRole(ctx context.Context, tf *tfv1beta1.Terraform, runOpts TaskOptions) error {
	kind := "Role"

	resource := runOpts.generateRole()
	controllerutil.SetControllerReference(tf, resource, r.Scheme)

	err := r.deleteRoleIfExists(ctx, resource.Name, resource.Namespace)
	if err != nil {
		return err
	}
	err = r.Client.Create(ctx, resource)
	if err != nil {
		r.Recorder.Event(tf, "Warning", fmt.Sprintf("%sCreateError", kind), fmt.Sprintf("Could not create %s %v", kind, err))
		return err
	}
	r.Recorder.Event(tf, "Normal", "SuccessfulCreate", fmt.Sprintf("Created %s: '%s'", kind, resource.Name))
	return nil
}

func (r ReconcileTerraform) checkRoleBindingExists(ctx context.Context, lookupKey types.NamespacedName) (*rbacv1.RoleBinding, bool, error) {
	resource := &rbacv1.RoleBinding{}
	err := r.Client.Get(ctx, lookupKey, resource)
	if err != nil && errors.IsNotFound(err) {
		return resource, false, nil
	} else if err != nil {
		return resource, false, err
	}
	return resource, true, nil
}

func (r ReconcileTerraform) deleteRoleBindingIfExists(ctx context.Context, name, namespace string) error {
	lookupKey := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}
	resource, found, err := r.checkRoleBindingExists(ctx, lookupKey)
	if err != nil {
		return err
	}
	if found {
		err = r.Client.Delete(ctx, resource)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r ReconcileTerraform) createRoleBinding(ctx context.Context, tf *tfv1beta1.Terraform, runOpts TaskOptions) error {
	kind := "RoleBinding"

	resource := runOpts.generateRoleBinding()
	controllerutil.SetControllerReference(tf, resource, r.Scheme)

	err := r.deleteRoleBindingIfExists(ctx, resource.Name, resource.Namespace)
	if err != nil {
		return err
	}
	err = r.Client.Create(ctx, resource)
	if err != nil {
		r.Recorder.Event(tf, "Warning", fmt.Sprintf("%sCreateError", kind), fmt.Sprintf("Could not create %s %v", kind, err))
		return err
	}
	r.Recorder.Event(tf, "Normal", "SuccessfulCreate", fmt.Sprintf("Created %s: '%s'", kind, resource.Name))
	return nil
}

func (r ReconcileTerraform) createPod(ctx context.Context, tf *tfv1beta1.Terraform, runOpts TaskOptions) error {
	kind := "Pod"

	resource, err := runOpts.generatePod()
	if err != nil {
		r.Recorder.Event(tf, "Warning", fmt.Sprintf("%sCreateError", kind), fmt.Sprintf("%s", err))
		return err
	}

	controllerutil.SetControllerReference(tf, resource, r.Scheme)

	err = r.Client.Create(ctx, resource)
	if err != nil {
		r.Recorder.Event(tf, "Warning", fmt.Sprintf("%sCreateError", kind), fmt.Sprintf("Could not create %s %v", kind, err))
		return err
	}
	r.Recorder.Event(tf, "Normal", "SuccessfulCreate", fmt.Sprintf("Created %s: '%s'", kind, resource.Name))
	return nil
}

func int32p(i int32) *int32 {
	return &i
}

func (r ReconcileTerraform) createJob(ctx context.Context, tf *tfv1beta1.Terraform, runOpts TaskOptions) error {
	kind := "Job"

	resource := runOpts.generateJob()
	controllerutil.SetControllerReference(tf, resource, r.Scheme)

	err := r.Client.Create(ctx, resource)
	if err != nil {
		r.Recorder.Event(tf, "Warning", fmt.Sprintf("%sCreateError", kind), fmt.Sprintf("Could not create %s %v", kind, err))
		return err
	}
	r.Recorder.Event(tf, "Normal", "SuccessfulCreate", fmt.Sprintf("Created %s: '%s'", kind, resource.Name))
	return nil
}

func (r TaskOptions) generateJob() *batchv1.Job {
	pod, _ := r.generatePod()

	// In a job, pod's can only have OnFailure or Never restart policies
	if pod.Spec.RestartPolicy == corev1.RestartPolicyAlways || pod.Spec.RestartPolicy == corev1.RestartPolicyOnFailure {
		pod.Spec.RestartPolicy = corev1.RestartPolicyOnFailure
	} else {
		pod.Spec.RestartPolicy = corev1.RestartPolicyNever
	}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:         pod.Name,
			GenerateName: pod.GenerateName,
			Labels:       pod.Labels,
			Annotations:  pod.Annotations,
			Namespace:    pod.Namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: int32p(1000000),
			Template: corev1.PodTemplateSpec{
				Spec: pod.Spec,
			},
		},
	}
}

func (r TaskOptions) generateConfigMap() *corev1.ConfigMap {

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.versionedName,
			Namespace: r.namespace,
			Labels:    r.resourceLabels,
		},
		Data: r.mainModulePluginData,
	}
	return cm
}

func (r TaskOptions) generateServiceAccount() *corev1.ServiceAccount {
	annotations := make(map[string]string)

	for _, c := range r.credentials {
		for k, v := range c.ServiceAccountAnnotations {
			annotations[k] = v
		}
		if c.AWSCredentials.IRSA != "" {
			annotations["eks.amazonaws.com/role-arn"] = c.AWSCredentials.IRSA
		}
	}

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        r.serviceAccount, // "tf-" + r.versionedName
			Namespace:   r.namespace,
			Annotations: annotations,
			Labels:      r.resourceLabels,
		},
	}
	return sa
}

func (r TaskOptions) generateRole() *rbacv1.Role {
	// TODO tighten up default rbac security since all the cm and secret names
	// can be predicted.

	rules := []rbacv1.PolicyRule{
		{
			Verbs:     []string{"*"},
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
		},
		{
			Verbs:         []string{"get"},
			APIGroups:     []string{"tf.galleybytes.com"},
			Resources:     []string{"terraforms"},
			ResourceNames: []string{r.resourceName},
		},
	}

	// When using the Kubernetes backend, allow the operator to create secrets and leases
	secretsRule := rbacv1.PolicyRule{
		Verbs:     []string{"*"},
		APIGroups: []string{""},
		Resources: []string{"secrets"},
	}
	leasesRule := rbacv1.PolicyRule{
		Verbs:     []string{"*"},
		APIGroups: []string{"coordination.k8s.io"},
		Resources: []string{"leases"},
	}
	if r.mainModulePluginData["backend_override.tf"] != "" {
		// parse the backennd string the way most people write it
		// example:
		// terraform {
		//   backend "kubernetes" {
		//     ...
		//   }
		// }
		s := strings.Split(r.mainModulePluginData["backend_override.tf"], "\n")
		for _, line := range s {
			// Assuming that config lines contain an equal sign
			// All other lines are discarded
			if strings.Contains(line, "backend ") && strings.Contains(line, "kubernetes") {
				// the extra space in "backend " is intentional since thats generally
				// how it's written
				rules = append(rules, secretsRule, leasesRule)
				break
			}
		}
	}

	rules = append(rules, r.policyRules...)

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.versionedName,
			Namespace: r.namespace,
			Labels:    r.resourceLabels,
		},
		Rules: rules,
	}
	return role
}

func (r TaskOptions) generateRoleBinding() *rbacv1.RoleBinding {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.versionedName,
			Namespace: r.namespace,
			Labels:    r.resourceLabels,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      r.serviceAccount,
				Namespace: r.namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			Name:     r.versionedName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
	return rb
}

func (r TaskOptions) generatePVC(size resource.Quantity, storageClassName *string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.prefixedName,
			Namespace: r.namespace,
			Labels:    r.resourceLabels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			StorageClassName: storageClassName,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
		},
	}
}

func (r TaskOptions) validateVolume() error {
	prohibitedNames := map[string]string{
		"tfohome":            "",
		"config-map-source":  "",
		"main-module-addons": "",
		"gitaskpass":         "",
		"ssh":                "",
	}
	mounts := map[string]string{}
	volumes := map[string]string{}

	for _, v := range r.volumeMounts {
		mounts[v.Name] = ""
	}

	for _, v := range r.volumes {
		// check if any system volume name is defined in task volumes
		_, ok := prohibitedNames[v.Name]
		if ok {
			return fmt.Errorf("task '%s' is misconfigured: volume name '%s' is reserved by tf-operator", r.task, v.Name)
		}
		// check if volume name has his own volumeMount
		_, ok = mounts[v.Name]
		if !ok {
			return fmt.Errorf("task '%s' is misconfigured: volume: '%s' doesn't have corresponding volumeMount", r.task, v.Name)
		}
		volumes[v.Name] = ""
	}

	for _, v := range r.volumeMounts {
		// check if volumeMount refers to existing volume
		_, ok := volumes[v.Name]
		if !ok {
			return fmt.Errorf("task '%s' is misconfigured: volumeMount: '%s' doesn't have corresponding volume", r.task, v.Name)
		}
	}

	return nil
}

// generatePod puts together all the contents required to execute the taskType.
// Although most of the tasks use similar.... (TODO EDIT ME)
func (r TaskOptions) generatePod() (*corev1.Pod, error) {

	home := "/home/tfo-runner"
	generateName := r.versionedName + "-" + r.task.String() + "-"
	generationPath := fmt.Sprintf("%s/generations/%d", home, r.generation)

	runnerLabels := r.labels
	annotations := r.annotations
	envFrom := r.envFrom
	envs := r.env
	envs = append(envs, []corev1.EnvVar{
		{
			Name: "POD_UID",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.uid",
				},
			},
		},
		{
			/*

				What is the significance of having an env about the TFO_RUNNER?

				Only used to idenify the taskType for the log.out file. This
				should simply be the taskType name.

			*/
			Name:  "TFO_TASK",
			Value: r.task.String(),
		},
		{
			Name:  "TFO_TASK_EXEC_URL_SOURCE",
			Value: r.urlSource,
		},
		{
			Name:  "TFO_TASK_EXEC_CONFIGMAP_SOURCE_NAME",
			Value: r.configMapSourceName,
		},
		{
			Name:  "TFO_TASK_EXEC_CONFIGMAP_SOURCE_KEY",
			Value: r.configMapSourceKey,
		},
		{
			Name:  "TFO_TASK_EXEC_INLINE_SOURCE_FILE",
			Value: r.inlineTaskExecutionFile,
		},
		{
			Name:  "TFO_RESOURCE",
			Value: r.resourceName,
		},
		{
			Name:  "TFO_RESOURCE_UUID",
			Value: r.resourceUUID,
		},
		{
			Name:  "TFO_NAMESPACE",
			Value: r.namespace,
		},
		{
			Name:  "TFO_GENERATION",
			Value: fmt.Sprintf("%d", r.generation),
		},
		{
			Name:  "TFO_GENERATION_PATH",
			Value: generationPath,
		},
		{
			Name:  "TFO_MAIN_MODULE",
			Value: generationPath + "/main",
		},
		{
			Name:  "TFO_TERRAFORM_VERSION",
			Value: r.terraformVersion,
		},
		{
			Name:  "TFO_SAVE_OUTPUTS",
			Value: strconv.FormatBool(r.saveOutputs),
		},
		{
			Name:  "TFO_OUTPUTS_SECRET_NAME",
			Value: r.outputsSecretName,
		},
		{
			Name:  "TFO_OUTPUTS_TO_INCLUDE",
			Value: strings.Join(r.outputsToInclude, ","),
		},
		{
			Name:  "TFO_OUTPUTS_TO_OMIT",
			Value: strings.Join(r.outputsToOmit, ","),
		},
	}...)

	if r.cleanupDisk {
		envs = append(envs, corev1.EnvVar{
			Name:  "TFO_CLEANUP_DISK",
			Value: "true",
		})
	}

	volumes := []corev1.Volume{
		{
			Name: "tfohome",
			VolumeSource: corev1.VolumeSource{
				//
				// TODO add an option to the tf to use host or pvc
				// 		for the plan.
				//
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: r.prefixedName,
					ReadOnly:  false,
				},
				//
				// TODO if host is used, develop a cleanup plan so
				//		so the volume does not fill up with old data
				//
				// TODO if host is used, affinity rules must be placed
				// 		that will ensure all the pods use the same host
				//
				// HostPath: &corev1.HostPathVolumeSource{
				// 	Path: "/mnt",
				// },
			},
		},
	}

	if err := r.validateVolume(); err != nil {
		return nil, err
	}
	volumes = append(volumes, r.volumes...)
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "tfohome",
			MountPath: home,
			ReadOnly:  false,
		},
	}
	volumeMounts = append(volumeMounts, r.volumeMounts...)
	envs = append(envs, corev1.EnvVar{
		Name:  "TFO_ROOT_PATH",
		Value: home,
	})

	if r.terraformModuleParsed.Repo != "" {
		envs = append(envs, []corev1.EnvVar{
			{
				Name:  "TFO_MAIN_MODULE_REPO",
				Value: r.terraformModuleParsed.Repo,
			},
			{
				Name:  "TFO_MAIN_MODULE_REPO_REF",
				Value: r.terraformModuleParsed.Hash,
			},
		}...)

		if len(r.terraformModuleParsed.Files) > 0 {
			// The terraform module may be in a sub-directory of the repo
			// Add this subdir value to envs so the pod can properly fetch it
			value := r.terraformModuleParsed.Files[0]
			if value == "" {
				value = "."
			}
			envs = append(envs, []corev1.EnvVar{
				{
					Name:  "TFO_MAIN_MODULE_REPO_SUBDIR",
					Value: value,
				},
			}...)
		} else {
			// TODO maybe set a default in r.stack.subdirs[0] so we can get rid
			//		of this if statement
			envs = append(envs, []corev1.EnvVar{
				{
					Name:  "TFO_MAIN_MODULE_REPO_SUBDIR",
					Value: ".",
				},
			}...)
		}
	}

	configMapSourceVolumeName := "config-map-source"
	configMapSourcePath := "/tmp/config-map-source"
	if r.configMapSourceName != "" && r.configMapSourceKey != "" {
		volumes = append(volumes, corev1.Volume{
			Name: configMapSourceVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.configMapSourceName,
					},
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      configMapSourceVolumeName,
			MountPath: configMapSourcePath,
		})
	}
	envs = append(envs, []corev1.EnvVar{
		{
			Name:  "TFO_TASK_EXEC_CONFIGMAP_SOURCE_PATH",
			Value: configMapSourcePath,
		},
	}...)

	mainModulePluginsConfigMapName := "main-module-addons"
	mainModulePluginsConfigMapPath := "/tmp/main-module-addons"
	volumes = append(volumes, []corev1.Volume{
		{
			Name: mainModulePluginsConfigMapName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: r.versionedName,
					},
				},
			},
		},
	}...)
	volumeMounts = append(volumeMounts, []corev1.VolumeMount{
		{
			Name:      mainModulePluginsConfigMapName,
			MountPath: mainModulePluginsConfigMapPath,
		},
	}...)
	envs = append(envs, []corev1.EnvVar{
		{
			Name:  "TFO_MAIN_MODULE_ADDONS",
			Value: mainModulePluginsConfigMapPath,
		},
	}...)

	optional := true
	xmode := int32(0775)
	volumes = append(volumes, corev1.Volume{
		Name: "gitaskpass",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: r.versionedName,
				Optional:   &optional,
				Items: []corev1.KeyToPath{
					{
						Key:  "gitAskpass",
						Path: "GIT_ASKPASS",
						Mode: &xmode,
					},
				},
			},
		},
	})
	volumeMounts = append(volumeMounts, []corev1.VolumeMount{
		{
			Name:      "gitaskpass",
			MountPath: "/git/askpass",
		},
	}...)
	envs = append(envs, []corev1.EnvVar{
		{
			Name:  "GIT_ASKPASS",
			Value: "/git/askpass/GIT_ASKPASS",
		},
	}...)

	sshMountName := "ssh"
	sshMountPath := "/tmp/ssh"
	mode := int32(0775)
	sshConfigItems := []corev1.KeyToPath{}
	keysToIgnore := []string{"gitAskpass"}
	for key := range r.secretData {
		if utils.ListContainsStr(keysToIgnore, key) {
			continue
		}
		sshConfigItems = append(sshConfigItems, corev1.KeyToPath{
			Key:  key,
			Path: key,
			Mode: &mode,
		})
	}
	volumes = append(volumes, []corev1.Volume{
		{
			Name: sshMountName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  r.versionedName,
					DefaultMode: &mode,
					Optional:    &optional,
					Items:       sshConfigItems,
				},
			},
		},
	}...)
	volumeMounts = append(volumeMounts, []corev1.VolumeMount{
		{
			Name:      sshMountName,
			MountPath: sshMountPath,
		},
	}...)
	envs = append(envs, []corev1.EnvVar{
		{
			Name:  "TFO_SSH",
			Value: sshMountPath,
		},
	}...)

	for _, c := range r.credentials {
		if c.AWSCredentials.KIAM != "" {
			annotations["iam.amazonaws.com/role"] = c.AWSCredentials.KIAM
		}
	}

	for _, c := range r.credentials {
		if (tfv1beta1.SecretNameRef{}) != c.SecretNameRef {
			envFrom = append(envFrom, []corev1.EnvFromSource{
				{
					SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: c.SecretNameRef.Name,
						},
					},
				},
			}...)
		}
	}

	// labels for all resources for use in queries
	for key, value := range r.resourceLabels {
		runnerLabels[key] = value
	}
	runnerLabels["app.kubernetes.io/instance"] = r.task.String()

	// Make sure to use the same uid for containers so the dir in the
	// PersistentVolume have the correct permissions for the user
	user := int64(2000)
	group := int64(2000)
	runAsNonRoot := true
	securityContext := &corev1.SecurityContext{
		RunAsUser:    &user,
		RunAsGroup:   &group,
		RunAsNonRoot: &runAsNonRoot,
	}
	restartPolicy := r.restartPolicy

	containerName := "task"
	if r.task.ID() == -2 {
		containerName = string(r.task)
	}

	containers := []corev1.Container{}
	containers = append(containers, corev1.Container{
		Name:            containerName,
		SecurityContext: securityContext,
		Image:           r.image,
		ImagePullPolicy: r.imagePullPolicy,
		EnvFrom:         envFrom,
		Env:             envs,
		VolumeMounts:    volumeMounts,
	})

	if r.sidecarPlugins != nil {
		for _, sidecarPlugin := range r.sidecarPlugins {
			spec := sidecarPlugin.Spec
			// Updates with sidecar container info when found
			containers = append(containers, spec.Containers...)

			volumeList := []string{}
			for _, volume := range volumes {
				volumeList = append(volumeList, volume.Name)
			}

			for _, volume := range spec.Volumes {
				if !utils.ListContainsStr(volumeList, volume.Name) {
					volumes = append(volumes, volume)
				}
			}
		}
	}

	podSecurityContext := corev1.PodSecurityContext{
		FSGroup: &user,
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: generateName,
			Namespace:    r.namespace,
			Labels:       runnerLabels,
			Annotations:  annotations,
		},
		Spec: corev1.PodSpec{
			Affinity:           r.inheritedAffinity,
			NodeSelector:       r.inheritedNodeSelector,
			Tolerations:        r.inheritedTolerations,
			SecurityContext:    &podSecurityContext,
			ServiceAccountName: r.serviceAccount,
			RestartPolicy:      restartPolicy,
			Containers:         containers,
			Volumes:            volumes,
		},
	}

	return pod, nil
}

func (r ReconcileTerraform) run(ctx context.Context, reqLogger logr.Logger, tf *tfv1beta1.Terraform, runOpts TaskOptions, isNewGeneration, isFirstInstall bool) (err error) {

	if isFirstInstall || isNewGeneration {
		if err := r.createEnvFromSources(ctx, tf); err != nil {
			return err
		}

		if err := r.createPVC(ctx, tf, runOpts); err != nil {
			return err
		}

		if err := r.createSecret(ctx, tf, runOpts.versionedName, runOpts.namespace, runOpts.secretData, true, []string{}, runOpts); err != nil {
			return err
		}

		if err := r.createConfigMap(ctx, tf, runOpts); err != nil {
			return err
		}

		if err := r.createRoleBinding(ctx, tf, runOpts); err != nil {
			return err
		}

		if err := r.createRole(ctx, tf, runOpts); err != nil {
			return err
		}

		if tf.Spec.ServiceAccount == "" {
			// since sa is not defined in the resource spec, it must be created
			if err := r.createServiceAccount(ctx, tf, runOpts); err != nil {
				return err
			}
		}

		labelsToOmit := []string{}
		if runOpts.stripGenerationLabelOnOutputsSecret {
			labelsToOmit = append(labelsToOmit, labelPrefix+"generation")
		}
		if err := r.createSecret(ctx, tf, runOpts.outputsSecretName, runOpts.namespace, map[string][]byte{}, false, labelsToOmit, runOpts); err != nil {
			return err
		}

	} else {
		// check resources exists
		lookupKey := types.NamespacedName{
			Name:      runOpts.prefixedName,
			Namespace: runOpts.namespace,
		}

		if _, found, err := r.checkPersistentVolumeClaimExists(ctx, lookupKey); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("could not find PersistentVolumeClaim '%s'", lookupKey)
		}

		lookupVersionedKey := types.NamespacedName{
			Name:      runOpts.versionedName,
			Namespace: runOpts.namespace,
		}

		if _, found, err := r.checkConfigMapExists(ctx, lookupVersionedKey); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("could not find ConfigMap '%s'", lookupVersionedKey)
		}

		if _, found, err := r.checkSecretExists(ctx, lookupVersionedKey); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("could not find Secret '%s'", lookupVersionedKey)
		}

		if _, found, err := r.checkRoleBindingExists(ctx, lookupVersionedKey); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("could not find RoleBinding '%s'", lookupVersionedKey)
		}

		if _, found, err := r.checkRoleExists(ctx, lookupVersionedKey); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("could not find Role '%s'", lookupVersionedKey)
		}

		serviceAccountLookupKey := types.NamespacedName{
			Name:      runOpts.serviceAccount,
			Namespace: runOpts.namespace,
		}
		if _, found, err := r.checkServiceAccountExists(ctx, serviceAccountLookupKey); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("could not find ServiceAccount '%s'", serviceAccountLookupKey)
		}

	}

	if err := r.createPod(ctx, tf, runOpts); err != nil {
		return err
	}

	return nil
}

func (r ReconcileTerraform) createGitAskpass(ctx context.Context, tokenSecret tfv1beta1.TokenSecretRef) ([]byte, error) {
	secret, err := r.loadSecret(ctx, tokenSecret.Name, tokenSecret.Namespace)
	if err != nil {
		return []byte{}, err
	}
	if key, ok := secret.Data[tokenSecret.Key]; !ok {
		return []byte{}, fmt.Errorf("secret '%s' did not contain '%s'", secret.Name, key)
	}
	s := heredoc.Docf(`
		#!/bin/sh
		exec echo "%s"
	`, secret.Data[tokenSecret.Key])
	gitAskpass := []byte(s)
	return gitAskpass, nil

}

func (r ReconcileTerraform) loadSecret(ctx context.Context, name, namespace string) (*corev1.Secret, error) {
	if namespace == "" {
		namespace = "default"
	}
	lookupKey := types.NamespacedName{Name: name, Namespace: namespace}
	secret := &corev1.Secret{}
	err := r.Client.Get(ctx, lookupKey, secret)
	if err != nil {
		return secret, err
	}
	return secret, nil
}

func (r ReconcileTerraform) cacheNodeSelectors(ctx context.Context, logger logr.Logger) error {
	var affinity *corev1.Affinity
	var tolerations []corev1.Toleration
	var nodeSelector map[string]string
	if !r.InheritAffinity && !r.InheritNodeSelector && !r.InheritTolerations {
		return nil
	}
	foundAll := true
	_, found := r.Cache.Get(r.AffinityCacheKey)
	if r.InheritAffinity && !found {
		foundAll = false
	}
	_, found = r.Cache.Get(r.NodeSelectorCacheKey)
	if r.InheritNodeSelector && !found {
		foundAll = false
	}
	_, found = r.Cache.Get(r.TolerationsCacheKey)
	if r.InheritTolerations && !found {
		foundAll = false
	}
	if foundAll {
		return nil
	}
	podNamespace := os.Getenv("POD_NAMESPACE")
	if podNamespace == "" {
		logger.Info("POD_NAMESPACE not found but required to get node selectors configs")
		return nil
	}
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		logger.Info("POD_NAME not found but required to get node selectors configs")
		return nil
	}
	podNamespacedName := types.NamespacedName{Namespace: podNamespace, Name: podName}
	pod := corev1.Pod{}
	err := r.Client.Get(ctx, podNamespacedName, &pod)
	if err != nil {
		logger.Info(fmt.Sprintf("Could not get pod '%s'", podNamespacedName.String()))
		return nil
	}
	if len(pod.ObjectMeta.OwnerReferences) != 1 {
		logger.Info(fmt.Sprintf("unexpected ownership for pod '%s'", podNamespacedName.String()))
		return nil
	}
	if pod.ObjectMeta.OwnerReferences[0].Kind != "ReplicaSet" {
		logger.Info(fmt.Sprintf("unexpected ownership kind for pod '%s'", podNamespacedName.String()))
		return nil
	}

	replicaSetName := pod.ObjectMeta.OwnerReferences[0].Name
	replicaSetNamespacedName := types.NamespacedName{Namespace: podNamespace, Name: replicaSetName}
	replicaSet := appsv1.ReplicaSet{}
	err = r.Client.Get(ctx, replicaSetNamespacedName, &replicaSet)
	if err != nil {
		logger.Info(fmt.Sprintf("Could not get replicaset '%s'", replicaSetNamespacedName.String()))
		return nil
	}
	if len(replicaSet.ObjectMeta.OwnerReferences) != 1 {
		logger.Info(fmt.Sprintf("unexpected ownership for replicaSet '%s'", replicaSetNamespacedName.String()))
		return nil
	}
	if replicaSet.ObjectMeta.OwnerReferences[0].Kind != "Deployment" {
		logger.Info(fmt.Sprintf("unexpected ownership kind for replicaSet '%s'", replicaSetNamespacedName.String()))
		return nil
	}

	deploymentName := replicaSet.ObjectMeta.OwnerReferences[0].Name
	deploymentNamespacedName := types.NamespacedName{Namespace: podNamespace, Name: deploymentName}
	deployment := appsv1.Deployment{}
	err = r.Client.Get(ctx, deploymentNamespacedName, &deployment)
	if err != nil {
		logger.Info(fmt.Sprintf("Could not get deployment '%s'", deploymentNamespacedName.String()))
		return nil
	}

	affinity = deployment.Spec.Template.Spec.Affinity
	tolerations = deployment.Spec.Template.Spec.Tolerations
	nodeSelector = deployment.Spec.Template.Spec.NodeSelector

	if r.InheritAffinity {
		r.Cache.Set(r.AffinityCacheKey, affinity, localcache.NoExpiration)
	}

	if r.InheritNodeSelector {
		r.Cache.Set(r.NodeSelectorCacheKey, nodeSelector, localcache.NoExpiration)
	}

	if r.InheritTolerations {
		r.Cache.Set(r.TolerationsCacheKey, tolerations, localcache.NoExpiration)
	}

	return nil
}

func (r TaskOptions) generateSecret(name, namespace string, data map[string][]byte, labels map[string]string) *corev1.Secret {
	secretObject := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Data: data,
		Type: corev1.SecretTypeOpaque,
	}
	return secretObject
}

func loadPassword(ctx context.Context, k8sclient client.Client, key, name, namespace string) (string, error) {

	secret := &corev1.Secret{}
	namespacedName := types.NamespacedName{Namespace: namespace, Name: name}
	err := k8sclient.Get(ctx, namespacedName, secret)
	// secret, err := c.clientset.CoreV1().Secrets(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("could not get secret: %v", err)
	}

	var password []byte
	for k, value := range secret.Data {
		if k == key {
			password = value
		}
	}

	if len(password) == 0 {
		return "", fmt.Errorf("unable to locate '%s' in secret: %v", key, err)
	}

	return string(password), nil

}

// forcedRegexp is the regular expression that finds forced getters. This
// syntax is schema::url, example: git::https://foo.com
var forcedRegexp = regexp.MustCompile(`^([A-Za-z0-9]+)::(.+)$`)

// getForcedGetter takes a source and returns the tuple of the forced
// getter and the raw URL (without the force syntax).
func getForcedGetter(src string) (string, string) {
	var forced string
	if ms := forcedRegexp.FindStringSubmatch(src); ms != nil {
		forced = ms[1]
		src = ms[2]
	}

	return forced, src
}

var sshPattern = regexp.MustCompile("^(?:([^@]+)@)?([^:]+):/?(.+)$")

type sshDetector struct{}

func (s *sshDetector) Detect(src, _ string) (string, bool, error) {
	matched := sshPattern.FindStringSubmatch(src)
	if matched == nil {
		return "", false, nil
	}

	user := matched[1]
	host := matched[2]
	path := matched[3]
	qidx := strings.Index(path, "?")
	if qidx == -1 {
		qidx = len(path)
	}

	var u url.URL
	u.Scheme = "ssh"
	u.User = url.User(user)
	u.Host = host
	u.Path = path[0:qidx]
	if qidx < len(path) {
		q, err := url.ParseQuery(path[qidx+1:])
		if err != nil {
			return "", false, fmt.Errorf("error parsing GitHub SSH URL: %s", err)
		}
		u.RawQuery = q.Encode()
	}

	return u.String(), true, nil
}

type scmType string

var gitScmType scmType = "git"

func getParsedAddress(address, path string, useAsVar bool, scmMap map[string]scmType) (ParsedAddress, error) {
	detectors := []getter.Detector{
		new(sshDetector),
	}

	detectors = append(detectors, getter.Detectors...)

	output, err := getter.Detect(address, "moduleDir", detectors)
	if err != nil {
		return ParsedAddress{}, err
	}

	forcedDetect, result := getForcedGetter(output)
	urlSource, filesSource := getter.SourceDirSubdir(result)

	parsedURL, err := url.Parse(urlSource)
	if err != nil {
		return ParsedAddress{}, err
	}

	scheme := parsedURL.Scheme

	// TODO URL parse rules: github.com should check the url is 'host/user/repo'
	// Currently the below is just a host check which isn't 100% correct
	if utils.ListContainsStr([]string{"github.com"}, parsedURL.Host) {
		scheme = "git"
	}

	// Check scm configuration for hosts and what scheme to map them as
	// Use the scheme of the scm configuration.
	// If git && another scm is defined in the scm configuration, select git.
	// If the user needs another scheme, the user must use forceDetect
	// (ie scheme::url://host...)
	hosts := []string{}
	for host := range scmMap {
		hosts = append(hosts, host)
	}
	if utils.ListContainsStr(hosts, parsedURL.Host) {
		scheme = string(scmMap[parsedURL.Host])
	}

	// forceDetect shall override all other schemes
	if forcedDetect != "" {
		scheme = forcedDetect
	}

	y, err := url.ParseQuery(parsedURL.RawQuery)
	if err != nil {
		return ParsedAddress{}, err
	}
	hash := y.Get("ref")
	if hash == "" {
		hash = "master"
	}

	// subdir can contain a list seperated by double slashes
	files := strings.Split(filesSource, "//")
	if len(files) == 1 && files[0] == "" {
		files = []string{"."}
	}

	// Assign default ports for common protos
	port := parsedURL.Port()
	if port == "" {
		if parsedURL.Scheme == "ssh" {
			port = "22"
		} else if parsedURL.Scheme == "https" {
			port = "443"
		}
	}

	p := ParsedAddress{
		DetectedScheme: scheme,
		Path:           path,
		UseAsVar:       useAsVar,
		Url:            parsedURL.String(),
		Files:          files,
		Hash:           hash,
		UrlScheme:      parsedURL.Scheme,
		Host:           parsedURL.Host,
		Uri:            strings.Split(parsedURL.RequestURI(), "?")[0],
		Port:           port,
		User:           parsedURL.User.Username(),
		Repo:           strings.Split(parsedURL.String(), "?")[0],
	}
	return p, nil
}

func getContainerNames(pod *corev1.Pod) []string {
	s := []string{}
	for _, container := range pod.Spec.Containers {
		s = append(s, container.Name)
	}
	return s
}
