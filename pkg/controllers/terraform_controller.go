package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/go-logr/logr"
	getter "github.com/hashicorp/go-getter"
	tfv1alpha1 "github.com/isaaguilar/terraform-operator/pkg/apis/tf/v1alpha1"
	"github.com/isaaguilar/terraform-operator/pkg/gitclient"
	"github.com/isaaguilar/terraform-operator/pkg/utils"
	localcache "github.com/patrickmn/go-cache"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runtimecontroller "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// SetupWithManager sets up the controller with the Manager.
func (r *ReconcileTerraform) SetupWithManager(mgr ctrl.Manager) error {
	controllerOptions := runtimecontroller.Options{
		MaxConcurrentReconciles: r.MaxConcurrentReconciles,
	}

	err := ctrl.NewControllerManagedBy(mgr).
		For(&tfv1alpha1.Terraform{}).
		Watches(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &tfv1alpha1.Terraform{},
		}).
		WithOptions(controllerOptions).
		Complete(r)
	if err != nil {
		return err
	}
	return nil
}

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

type GitRepoAccessOptions struct {
	ClientGitRepo  gitclient.GitRepo
	Timeout        int64
	Address        string
	Directory      string
	Extras         []string
	SCMAuthMethods []tfv1alpha1.SCMAuthMethod
	SSHProxy       tfv1alpha1.ProxyOpts
	ParsedAddress
}

type ExportOptions struct {
	stack       ParsedAddress
	confPath    string
	tfvarsPath  string
	gitUsername string
	gitEmail    string
}

type RunOptions struct {
	namespace                               string
	tfName                                  string
	name                                    string
	versionedName                           string
	envVars                                 []corev1.EnvVar
	credentials                             []tfv1alpha1.Credentials
	terraformModuleParsed                   ParsedAddress
	serviceAccount                          string
	mainModuleAddonData                     map[string]string
	secretData                              map[string][]byte
	terraformRunner                         string
	terraformRunnerExecutionScriptConfigMap *corev1.ConfigMapKeySelector
	terraformRunnerPullPolicy               corev1.PullPolicy
	terraformVersion                        string
	scriptRunner                            string
	scriptRunnerExecutionScriptConfigMap    *corev1.ConfigMapKeySelector
	scriptRunnerPullPolicy                  corev1.PullPolicy
	scriptRunnerVersion                     string
	setupRunner                             string
	setupRunnerExecutionScriptConfigMap     *corev1.ConfigMapKeySelector
	setupRunnerPullPolicy                   corev1.PullPolicy
	setupRunnerVersion                      string
	runnerAnnotations                       map[string]string
	runnerRules                             []rbacv1.PolicyRule
	exportOptions                           ExportOptions
	outputsSecretName                       string
	saveOutputs                             bool
	outputsToInclude                        []string
	outputsToOmit                           []string
}

func newRunOptions(tf *tfv1alpha1.Terraform) RunOptions {
	// TODO Read the tfstate and decide IF_NEW_RESOURCE based on that
	// applyAction := false
	tfName := tf.Name
	name := tf.Status.PodNamePrefix
	versionedName := name + "-v" + fmt.Sprint(tf.Generation)
	terraformRunner := "isaaguilar/tf-runner-v5beta1"
	terraformRunnerPullPolicy := corev1.PullIfNotPresent
	terraformVersion := "1.1.4"

	scriptRunner := "isaaguilar/script-runner"
	scriptRunnerPullPolicy := corev1.PullIfNotPresent
	scriptRunnerVersion := "1.0.2"

	setupRunner := "isaaguilar/setup-runner"
	setupRunnerPullPolicy := corev1.PullIfNotPresent
	setupRunnerVersion := "1.1.3"

	runnerAnnotations := tf.Spec.RunnerAnnotations
	runnerRules := tf.Spec.RunnerRules

	// sshConfig := utils.TruncateResourceName(tf.Name, 242) + "-ssh-config"
	serviceAccount := tf.Spec.ServiceAccount
	if serviceAccount == "" {
		// By prefixing the service account with "tf-", IRSA roles can use wildcard
		// "tf-*" service account for AWS credentials.
		serviceAccount = "tf-" + versionedName
	}

	if tf.Spec.TerraformRunner != "" {
		terraformRunner = tf.Spec.TerraformRunner
	}
	if tf.Spec.TerraformRunnerPullPolicy != "" {
		terraformRunnerPullPolicy = tf.Spec.TerraformRunnerPullPolicy
	}
	if tf.Spec.TerraformVersion != "" {
		terraformVersion = tf.Spec.TerraformVersion
	}

	if tf.Spec.ScriptRunner != "" {
		scriptRunner = tf.Spec.ScriptRunner
	}
	if tf.Spec.ScriptRunnerPullPolicy != "" {
		scriptRunnerPullPolicy = tf.Spec.ScriptRunnerPullPolicy
	}
	if tf.Spec.ScriptRunnerVersion != "" {
		scriptRunnerVersion = tf.Spec.ScriptRunnerVersion
	}

	if tf.Spec.SetupRunner != "" {
		setupRunner = tf.Spec.SetupRunner
	}
	if tf.Spec.SetupRunnerPullPolicy != "" {
		setupRunnerPullPolicy = tf.Spec.SetupRunnerPullPolicy
	}
	if tf.Spec.SetupRunnerVersion != "" {
		setupRunnerVersion = tf.Spec.SetupRunnerVersion
	}
	credentials := tf.Spec.Credentials

	// Outputs will be saved as a secret that will have the same lifecycle
	// as the Terraform CustomResource by adding the ownership metadata
	outputsSecretName := name + "-outputs"
	saveOutputs := false
	if tf.Spec.OutputsSecret != "" {
		outputsSecretName = tf.Spec.OutputsSecret
		saveOutputs = true
	} else if tf.Spec.WriteOutputsToStatus {
		saveOutputs = true
	}
	outputsToInclude := tf.Spec.OutputsToInclude
	outputsToOmit := tf.Spec.OutputsToOmit

	return RunOptions{
		namespace:                               tf.Namespace,
		tfName:                                  tfName,
		name:                                    name,
		versionedName:                           versionedName,
		envVars:                                 tf.Spec.Env,
		credentials:                             credentials,
		terraformVersion:                        terraformVersion,
		terraformRunner:                         terraformRunner,
		terraformRunnerExecutionScriptConfigMap: tf.Spec.TerraformRunnerExecutionScriptConfigMap,
		terraformRunnerPullPolicy:               terraformRunnerPullPolicy,
		runnerAnnotations:                       runnerAnnotations,
		serviceAccount:                          serviceAccount,
		mainModuleAddonData:                     make(map[string]string),
		secretData:                              make(map[string][]byte),
		scriptRunner:                            scriptRunner,
		scriptRunnerPullPolicy:                  scriptRunnerPullPolicy,
		scriptRunnerExecutionScriptConfigMap:    tf.Spec.ScriptRunnerExecutionScriptConfigMap,
		scriptRunnerVersion:                     scriptRunnerVersion,
		setupRunner:                             setupRunner,
		setupRunnerPullPolicy:                   setupRunnerPullPolicy,
		setupRunnerExecutionScriptConfigMap:     tf.Spec.SetupRunnerExecutionScriptConfigMap,
		setupRunnerVersion:                      setupRunnerVersion,
		runnerRules:                             runnerRules,
		outputsSecretName:                       outputsSecretName,
		saveOutputs:                             saveOutputs,
		outputsToInclude:                        outputsToInclude,
		outputsToOmit:                           outputsToOmit,
	}
}

const terraformFinalizer = "finalizer.tf.isaaguilar.com"

// Reconcile reads that state of the cluster for a Terraform object and makes changes based on the state read
// and what is in the Terraform.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileTerraform) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reconcilerID := string(uuid.NewUUID())
	reqLogger := r.Log.WithValues("Terraform", request.NamespacedName, "id", reconcilerID)
	lockKey := request.String() + "-reconcile-lock"
	lockOwner, lockFound := r.Cache.Get(lockKey)
	if lockFound {
		reqLogger.Info(fmt.Sprintf("Request is locked by '%s'", lockOwner.(string)))
		return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
	}
	r.Cache.Set(lockKey, reconcilerID, -1)
	defer r.Cache.Delete(lockKey)
	defer reqLogger.V(2).Info("Request has released reconcile lock")
	reqLogger.V(2).Info("Request has acquired reconcile lock")

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
	if tf.Status.Phase == tfv1alpha1.PhaseDeleted {
		reqLogger.Info("Remove finalizers")
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
			utils.TruncateResourceName(tf.Name, 220),
			utils.StringWithCharset(8, utils.AlphaNum),
		)
		tf.Status.Stages = []tfv1alpha1.Stage{}
		tf.Status.LastCompletedGeneration = 0
		tf.Status.Phase = tfv1alpha1.PhaseInitializing
		err := r.updateStatus(ctx, tf)
		if err != nil {
			reqLogger.V(1).Info(err.Error())
		}
		return reconcile.Result{}, nil
	}

	// Add the first stage
	if len(tf.Status.Stages) == 0 {
		podType := tfv1alpha1.PodSetup
		stageState := tfv1alpha1.StateInitializing
		interruptible := tfv1alpha1.CanNotBeInterrupt
		addNewStage(tf, podType, "TF_RESOURCE_CREATED", interruptible, stageState)
		tf.Status.Exported = tfv1alpha1.ExportedFalse
		err := r.updateStatus(ctx, tf)
		if err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	deletePhases := []string{
		string(tfv1alpha1.PhaseDeleting),
		string(tfv1alpha1.PhaseInitDelete),
		string(tfv1alpha1.PhaseDeleted),
	}

	// Check if the resource is marked to be deleted which is
	// indicated by the deletion timestamp being set.
	if tf.GetDeletionTimestamp() != nil && !utils.ListContainsStr(deletePhases, string(tf.Status.Phase)) {
		tf.Status.Phase = tfv1alpha1.PhaseInitDelete
	}

	// // TODO Check the status on stages that have not completed
	// for _, stage := range tf.Status.Stages {
	// 	if stage.State == tfv1alpha1.StateInProgress {
	//
	// 	}
	// }

	if checkSetNewStage(tf) {
		err := r.updateStatus(ctx, tf)
		if err != nil {
			reqLogger.V(1).Info(err.Error())
		}
		return reconcile.Result{}, nil
	}

	var podType tfv1alpha1.PodType
	var generation int64
	n := len(tf.Status.Stages)
	currentStage := tf.Status.Stages[n-1]
	podType = currentStage.PodType
	generation = currentStage.Generation
	runOpts := newRunOptions(tf)

	if tf.Spec.ExportRepo != nil && podType != tfv1alpha1.PodSetup {
		// Run the export-runner
		exportedStatus, err := r.exportRepo(ctx, tf, runOpts, generation, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
		if tf.Status.Exported != exportedStatus {
			tf.Status.Exported = exportedStatus
			err := r.updateStatus(ctx, tf)
			if err != nil {
				reqLogger.V(1).Info(err.Error())
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		}
	}

	if podType == "" {
		// podType is blank when the terraform workflow has completed for
		// either create or delete.

		if tf.Status.Phase == tfv1alpha1.PhaseRunning {
			// Updates the status as "completed" on the resource
			tf.Status.Phase = tfv1alpha1.PhaseCompleted
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
			err := r.updateStatus(ctx, tf)
			if err != nil {
				reqLogger.V(1).Info(err.Error())
				return reconcile.Result{}, err
			}
		} else if tf.Status.Phase == tfv1alpha1.PhaseDeleting {
			// Updates the status as "deleted" which will be used to tell the
			// controller to remove any finalizers).
			tf.Status.Phase = tfv1alpha1.PhaseDeleted
			err := r.updateStatus(ctx, tf)
			if err != nil {
				reqLogger.V(1).Info(err.Error())
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// Check for the current stage pod
	inNamespace := client.InNamespace(tf.Namespace)
	f := fields.Set{
		"metadata.generateName": fmt.Sprintf("%s-%s-", tf.Status.PodNamePrefix+"-v"+fmt.Sprint(generation), podType),
	}
	labels := map[string]string{
		"terraforms.tf.isaaguilar.com/generation": fmt.Sprintf("%d", generation),
	}
	matchingFields := client.MatchingFields(f)
	matchingLabels := client.MatchingLabels(labels)
	pods := &corev1.PodList{}
	err = r.Client.List(ctx, pods, inNamespace, matchingFields, matchingLabels)
	if err != nil {
		reqLogger.Error(err, "")
		return reconcile.Result{}, nil
	}

	if len(pods.Items) == 0 && tf.Status.Stages[n-1].State == tfv1alpha1.StateInProgress {
		// This condition is generally met when the user deletes the pod.
		// Force the state to transition away from in-progress and then
		// requeue.
		tf.Status.Stages[n-1].State = tfv1alpha1.StateInitializing
		err = r.updateStatus(ctx, tf)
		if err != nil {
			reqLogger.V(1).Info(err.Error())
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, nil
	}

	if len(pods.Items) == 0 {
		// Trigger a new pod when no pods are found for current stage
		reqLogger.V(1).Info(fmt.Sprintf("Setting up the '%s' pod", podType))
		err := r.setupAndRun(ctx, tf, runOpts)
		if err != nil {
			reqLogger.Error(err, "")
			return reconcile.Result{}, err
		}
		if tf.Status.Phase == tfv1alpha1.PhaseInitializing {
			tf.Status.Phase = tfv1alpha1.PhaseRunning
		} else if tf.Status.Phase == tfv1alpha1.PhaseInitDelete {
			tf.Status.Phase = tfv1alpha1.PhaseDeleting
		}
		tf.Status.Stages[n-1].State = tfv1alpha1.StateInProgress

		// TODO Becuase the pod is already running, is it critical that the
		// phase and state be updated. The updateStatus function needs to retry
		// if it fails to update.
		err = r.updateStatus(ctx, tf)
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
	podName := pods.Items[0].ObjectMeta.Name
	podPhase := pods.Items[0].Status.Phase
	msg := fmt.Sprintf("Pod '%s' %s", podName, podPhase)

	// TODO Does the user need reason and message?
	// reason := pods.Items[0].Status.Reason
	// message := pods.Items[0].Status.Message
	// if reason != "" {
	// 	msg = fmt.Sprintf("%s %s", msg, reason)
	// }
	// if message != "" {
	// 	msg = fmt.Sprintf("%s %s", msg, message)
	// }
	reqLogger.V(1).Info(msg)

	if pods.Items[0].Status.Phase == corev1.PodFailed {
		tf.Status.Stages[n-1].State = tfv1alpha1.StateFailed
		tf.Status.Stages[n-1].StopTime = metav1.NewTime(time.Now())
		err = r.updateStatus(ctx, tf)
		if err != nil {
			reqLogger.V(1).Info(err.Error())
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if pods.Items[0].Status.Phase == corev1.PodSucceeded {
		tf.Status.Stages[n-1].State = tfv1alpha1.StateComplete
		tf.Status.Stages[n-1].StopTime = metav1.NewTime(time.Now())
		err = r.updateStatus(ctx, tf)
		if err != nil {
			reqLogger.V(1).Info(err.Error())
			return reconcile.Result{}, err
		}
		if !tf.Spec.KeepCompletedPods {
			err := r.Client.Delete(ctx, &pods.Items[0])
			if err != nil {
				reqLogger.V(1).Info(err.Error())
			}
		}
		return reconcile.Result{}, nil
	}

	// TODO should tf operator "auto" reconciliate (eg plan+apply)?
	// TODO how should we handle manually triggering apply
	return reconcile.Result{}, nil
}

// getTerraformResource fetches the terraform resource with a retry
func (r ReconcileTerraform) getTerraformResource(ctx context.Context, namespacedName types.NamespacedName, maxRetry int, reqLogger logr.Logger) (*tfv1alpha1.Terraform, error) {
	tf := &tfv1alpha1.Terraform{}
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

func addNewStage(tf *tfv1alpha1.Terraform, podType tfv1alpha1.PodType, reason string, interruptible tfv1alpha1.Interruptible, stageState tfv1alpha1.StageState) {
	if reason == "GENERATION_CHANGE" {
		tf.Status.Exported = tfv1alpha1.ExportedFalse
		tf.Status.Phase = tfv1alpha1.PhaseInitializing
	}
	startTime := metav1.NewTime(time.Now())
	stopTime := metav1.NewTime(time.Unix(0, 0))
	if stageState == tfv1alpha1.StateComplete {
		stopTime = startTime
	}
	tf.Status.Stages = append(tf.Status.Stages, tfv1alpha1.Stage{
		Generation:    tf.Generation,
		Interruptible: interruptible,
		Reason:        reason,
		State:         stageState,
		PodType:       podType,
		StartTime:     startTime,
		StopTime:      stopTime,
	})
}

// checkSetNewStage uses the tf resource's `.status.stage` state to find the next stage of the terraform run. The following set of rules are used:
//
// 1. Generation - Check that the resource's generation matches the stage's generation. When the generation changes the old generation can no longer add a new stage.
//
// 2. Check that the current stage is completed. If it is not, this function returns false and the pod status will be determined which will update the stage for the next iteration.
//
// 3. Scripts defined in the tf resource manifest will trigger the script runner podTypes.
//
// When a stage has already triggered a pod, the only way for the pod to
// transition to the next stage is for the pod to complete successfully. Any
// other pod phase will keep the pod in the current stage.
//
// TODO Should stages be cleaned? If not, the `.status.stages` list can become very long.
//
func checkSetNewStage(tf *tfv1alpha1.Terraform) bool {
	var isNewStage bool
	var podType tfv1alpha1.PodType
	var reason string

	deletePhases := []string{
		string(tfv1alpha1.PhaseDeleted),
		string(tfv1alpha1.PhaseInitDelete),
		string(tfv1alpha1.PhaseDeleted),
	}
	tfIsFinalizing := utils.ListContainsStr(deletePhases, string(tf.Status.Phase))
	tfIsNotFinalizing := !tfIsFinalizing
	initDelete := tf.Status.Phase == tfv1alpha1.PhaseInitDelete
	stageState := tfv1alpha1.StateInitializing
	interruptible := tfv1alpha1.CanBeInterrupt
	// n is the last stage which should be the current stage
	n := len(tf.Status.Stages)
	// if n == 0 {
	// 	// There should always be at least 1 stage, this shouldn't happen
	// }
	currentStage := tf.Status.Stages[n-1]
	currentStagePodType := currentStage.PodType
	currentStageCanNotBeInterrupted := currentStage.Interruptible == tfv1alpha1.CanNotBeInterrupt
	currentStageIsRunning := currentStage.State == tfv1alpha1.StateInProgress
	isNewGeneration := currentStage.Generation != tf.Generation

	// resource status
	if currentStageCanNotBeInterrupted && currentStageIsRunning {
		// Cannot change to the next stage becuase the current stage cannot be
		// interrupted and is currently running
		isNewStage = false
	} else if isNewGeneration && tfIsNotFinalizing {
		// The current generation has changed and this is the first pod in the
		// normal terraform workflow
		isNewStage = true
		reason = "GENERATION_CHANGE"
		podType = tfv1alpha1.PodSetup

		// } else if initDelete && !utils.ListContainsStr(deletePodTypes, string(currentStagePodType)) {
	} else if initDelete && isNewGeneration {
		// The tf resource is marked for deletion and this is the first pod
		// in the terraform destroy workflow.
		isNewStage = true
		reason = "TF_RESOURCE_DELETED"
		podType = tfv1alpha1.PodSetupDelete
		interruptible = tfv1alpha1.CanNotBeInterrupt

	} else if currentStage.State == tfv1alpha1.StateComplete {
		isNewStage = true
		reason = ""

		switch currentStagePodType {
		//
		// setup
		//
		case tfv1alpha1.PodSetup:
			podType = tfv1alpha1.PodInit

		//
		// init types
		//
		case tfv1alpha1.PodInit:
			if tf.Spec.PostInitScript != "" {
				podType = tfv1alpha1.PodPostInit
			} else {
				podType = tfv1alpha1.PodPlan
				interruptible = tfv1alpha1.CanNotBeInterrupt
			}

		case tfv1alpha1.PodPostInit:
			podType = tfv1alpha1.PodPlan
			interruptible = tfv1alpha1.CanNotBeInterrupt

		//
		// plan types
		//
		case tfv1alpha1.PodPlan:
			if tf.Spec.PostPlanScript != "" {
				podType = tfv1alpha1.PodPostPlan
			} else {
				podType = tfv1alpha1.PodApply
				interruptible = tfv1alpha1.CanNotBeInterrupt
			}

		case tfv1alpha1.PodPostPlan:
			podType = tfv1alpha1.PodApply
			interruptible = tfv1alpha1.CanNotBeInterrupt

		//
		// apply types
		//
		case tfv1alpha1.PodApply:
			if tf.Spec.PostApplyScript != "" {
				podType = tfv1alpha1.PodPostApply
			} else {
				reason = "COMPLETED_TERRAFORM"
				podType = tfv1alpha1.PodNil
				stageState = tfv1alpha1.StateComplete
			}

		case tfv1alpha1.PodPostApply:
			reason = "COMPLETED_TERRAFORM"
			podType = tfv1alpha1.PodNil
			stageState = tfv1alpha1.StateComplete

		//
		// setup delete
		//
		case tfv1alpha1.PodSetupDelete:
			podType = tfv1alpha1.PodInitDelete

		//
		// init (delete) types
		case tfv1alpha1.PodInitDelete:
			if tf.Spec.PostInitDeleteScript != "" {
				podType = tfv1alpha1.PodPostInitDelete
			} else {
				podType = tfv1alpha1.PodPlanDelete
				interruptible = tfv1alpha1.CanNotBeInterrupt
			}

		case tfv1alpha1.PodPostInitDelete:
			podType = tfv1alpha1.PodPlanDelete
			interruptible = tfv1alpha1.CanNotBeInterrupt

		//
		// plan (delete) types
		//
		case tfv1alpha1.PodPlanDelete:
			if tf.Spec.PostPlanDeleteScript != "" {
				podType = tfv1alpha1.PodPostPlanDelete
			} else {
				podType = tfv1alpha1.PodApplyDelete
				interruptible = tfv1alpha1.CanNotBeInterrupt
			}

		case tfv1alpha1.PodPostPlanDelete:
			podType = tfv1alpha1.PodApplyDelete
			interruptible = tfv1alpha1.CanNotBeInterrupt

		//
		// apply (delete) types
		//
		case tfv1alpha1.PodApplyDelete:
			if tf.Spec.PostApplyDeleteScript != "" {
				podType = tfv1alpha1.PodPostApplyDelete
			} else {
				reason = "COMPLETED_TERRAFORM"
				podType = tfv1alpha1.PodNil
				stageState = tfv1alpha1.StateComplete
			}

		case tfv1alpha1.PodPostApplyDelete:
			reason = "COMPLETED_TERRAFORM"
			podType = tfv1alpha1.PodNil
			stageState = tfv1alpha1.StateComplete

		case tfv1alpha1.PodNil:
			isNewStage = false
		}

	}
	if isNewStage {
		addNewStage(tf, podType, reason, interruptible, stageState)
	}
	return isNewStage
}

// updateFinalizer sets and unsets the finalizer on the tf resource. When
// IgnoreDelete is true, the finalizer is removed. When IgnoreDelete is false,
// the finalizer is added.
//
// The finalizer will be responsible for starting the destroy-workflow.
func updateFinalizer(tf *tfv1alpha1.Terraform) bool {
	finalizers := tf.GetFinalizers()

	if tf.Status.Phase == tfv1alpha1.PhaseDeleted {
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

func (r ReconcileTerraform) update(ctx context.Context, tf *tfv1alpha1.Terraform) error {
	err := r.Client.Update(ctx, tf)
	if err != nil {
		return fmt.Errorf("failed to update tf resource: %s", err)
	}
	return nil
}

func (r ReconcileTerraform) updateStatus(ctx context.Context, tf *tfv1alpha1.Terraform) error {
	err := r.Client.Status().Update(ctx, tf)
	if err != nil {
		return fmt.Errorf("failed to update tf status: %s", err)
	}
	return nil
}

func (r ReconcileTerraform) PodStatus(ctx context.Context, tf tfv1alpha1.Terraform, stage tfv1alpha1.Stage) (tfv1alpha1.StageState, error) {

	return tfv1alpha1.StateInProgress, nil

}

// IsJobFinished returns true if the job has completed
func IsJobFinished(job *batchv1.Job) bool {
	BackoffLimit := job.Spec.BackoffLimit
	return job.Status.CompletionTime != nil || (job.Status.Active == 0 && BackoffLimit != nil && job.Status.Failed >= *BackoffLimit)
}

func formatJobSSHConfig(ctx context.Context, reqLogger logr.Logger, instance *tfv1alpha1.Terraform, k8sclient client.Client) (map[string][]byte, error) {
	data := make(map[string]string)
	dataAsByte := make(map[string][]byte)
	if instance.Spec.SSHTunnel != nil {
		data["config"] = fmt.Sprintf("Host proxy\n"+
			"\tStrictHostKeyChecking no\n"+
			"\tUserKnownHostsFile=/dev/null\n"+
			"\tUser %s\n"+
			"\tHostname %s\n"+
			"\tIdentityFile ~/.ssh/proxy_key\n",
			instance.Spec.SSHTunnel.User,
			instance.Spec.SSHTunnel.Host)
		k := instance.Spec.SSHTunnel.SSHKeySecretRef.Key
		if k == "" {
			k = "id_rsa"
		}
		ns := instance.Spec.SSHTunnel.SSHKeySecretRef.Namespace
		if ns == "" {
			ns = instance.Namespace
		}

		key, err := loadPassword(ctx, k8sclient, k, instance.Spec.SSHTunnel.SSHKeySecretRef.Name, ns)
		if err != nil {
			return dataAsByte, err
		}
		data["proxy_key"] = key

	}

	for _, m := range instance.Spec.SCMAuthMethods {

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
				ns = instance.Namespace
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

func (r *ReconcileTerraform) setupAndRun(ctx context.Context, tf *tfv1alpha1.Terraform, runOpts RunOptions) error {
	reqLogger := r.Log.WithValues("Terraform", types.NamespacedName{Name: tf.Name, Namespace: tf.Namespace}.String())
	var err error
	n := len(tf.Status.Stages)
	podType := tf.Status.Stages[n-1].PodType
	generation := tf.Status.Stages[n-1].Generation
	reason := tf.Status.Stages[n-1].Reason
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

	if tf.Spec.TerraformModuleInline != "" {
		// Add add inline to configmap and instruct the pod to fetch the
		// configmap as the main module
		runOpts.mainModuleAddonData["inline-module.tf"] = tf.Spec.TerraformModuleInline
	}

	if tf.Spec.TerraformModuleConfigMap != nil {
		// Instruct the setup pod to fetch the configmap as the main module
		b, err := json.Marshal(tf.Spec.TerraformModuleConfigMap)
		if err != nil {
			return err
		}
		runOpts.mainModuleAddonData[".__TFO__ConfigMapModule.json"] = string(b)
	}

	if tf.Spec.TerraformModule != "" {
		runOpts.terraformModuleParsed, err = getParsedAddress(tf.Spec.TerraformModule, "", false, scmMap)
		if err != nil {
			return err
		}
	}

	if isChanged {
		for _, m := range tf.Spec.SCMAuthMethods {
			// I think Terraform only allows for one git token. Add the first one
			// to the job's env vars as GIT_PASSWORD.
			if m.Git.HTTPS != nil {
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
			if m.Git.SSH != nil {
				sshConfigData, err := formatJobSSHConfig(ctx, reqLogger, tf, r.Client)
				if err != nil {
					r.Recorder.Event(tf, "Warning", "SSHConfigError", fmt.Errorf("%v", err).Error())
					return fmt.Errorf("error setting up sshconfig: %v", err)
				}
				for k, v := range sshConfigData {
					runOpts.secretData[k] = v
				}

			}
			if m.Git.HTTPS == nil && m.Git.SSH == nil {
				continue
			} else {
				break
			}
		}

		resourceDownloadItems := []ParsedAddress{}
		// Configure the resourceDownloads in JSON that the setupRunner will
		// use to download the resources into the main module directory

		// ConfigMap Data only needs to be updated when generation changes

		for _, s := range tf.Spec.ResourceDownloads {
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
		b, err := json.Marshal(resourceDownloadItems)
		if err != nil {
			return err
		}
		resourceDownloads := string(b)

		runOpts.mainModuleAddonData[".__TFO__ResourceDownloads.json"] = resourceDownloads

		// Override the backend.tf by inserting a custom backend
		if tf.Spec.CustomBackend != "" {
			runOpts.mainModuleAddonData["backend_override.tf"] = tf.Spec.CustomBackend
		}

		if tf.Spec.PreInitScript != "" {
			runOpts.mainModuleAddonData[string(tfv1alpha1.PodPreInit)] = tf.Spec.PreInitScript
		}

		if tf.Spec.PostInitScript != "" {
			runOpts.mainModuleAddonData[string(tfv1alpha1.PodPostInit)] = tf.Spec.PostInitScript
		}

		if tf.Spec.PrePlanScript != "" {
			runOpts.mainModuleAddonData[string(tfv1alpha1.PodPrePlan)] = tf.Spec.PrePlanScript
		}

		if tf.Spec.PostPlanScript != "" {
			runOpts.mainModuleAddonData[string(tfv1alpha1.PodPostPlan)] = tf.Spec.PostPlanScript
		}

		if tf.Spec.PreApplyScript != "" {
			runOpts.mainModuleAddonData[string(tfv1alpha1.PodPreApply)] = tf.Spec.PreApplyScript
		}

		if tf.Spec.PostApplyScript != "" {
			runOpts.mainModuleAddonData[string(tfv1alpha1.PodPostApply)] = tf.Spec.PostApplyScript
		}

		if tf.Spec.PreInitDeleteScript != "" {
			runOpts.mainModuleAddonData[string(tfv1alpha1.PodPreInitDelete)] = tf.Spec.PreInitDeleteScript
		}

		if tf.Spec.PostInitDeleteScript != "" {
			runOpts.mainModuleAddonData[string(tfv1alpha1.PodPostInitDelete)] = tf.Spec.PostInitDeleteScript
		}

		if tf.Spec.PrePlanDeleteScript != "" {
			runOpts.mainModuleAddonData[string(tfv1alpha1.PodPrePlanDelete)] = tf.Spec.PrePlanDeleteScript
		}

		if tf.Spec.PostPlanDeleteScript != "" {
			runOpts.mainModuleAddonData[string(tfv1alpha1.PodPostPlanDelete)] = tf.Spec.PostPlanDeleteScript
		}

		if tf.Spec.PreApplyDeleteScript != "" {
			runOpts.mainModuleAddonData[string(tfv1alpha1.PodPreApplyDelete)] = tf.Spec.PostApplyScript
		}

		if tf.Spec.PostApplyDeleteScript != "" {
			runOpts.mainModuleAddonData[string(tfv1alpha1.PodPostApplyDelete)] = tf.Spec.PostApplyDeleteScript
		}
	}

	// RUN
	err = r.run(ctx, reqLogger, tf, runOpts, isNewGeneration, isFirstInstall, podType, generation)
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

func (r ReconcileTerraform) createPVC(ctx context.Context, tf *tfv1alpha1.Terraform, runOpts RunOptions) error {
	kind := "PersistentVolumeClaim"
	_, found, err := r.checkPersistentVolumeClaimExists(ctx, types.NamespacedName{
		Name:      runOpts.name,
		Namespace: runOpts.namespace,
	})
	if err != nil {
		return nil
	} else if found {
		return nil
	}
	resource := runOpts.generatePVC("2Gi")
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

func (r ReconcileTerraform) createConfigMap(ctx context.Context, tf *tfv1alpha1.Terraform, runOpts RunOptions) error {
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

func (r ReconcileTerraform) createSecret(ctx context.Context, tf *tfv1alpha1.Terraform, name, namespace string, data map[string][]byte, recreate bool) error {
	kind := "Secret"

	resource := generateSecret(name, namespace, data)
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

func (r ReconcileTerraform) createServiceAccount(ctx context.Context, tf *tfv1alpha1.Terraform, runOpts RunOptions) error {
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

func (r ReconcileTerraform) createRole(ctx context.Context, tf *tfv1alpha1.Terraform, runOpts RunOptions) error {
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

func (r ReconcileTerraform) createRoleBinding(ctx context.Context, tf *tfv1alpha1.Terraform, runOpts RunOptions) error {
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

func (r ReconcileTerraform) createPod(ctx context.Context, tf *tfv1alpha1.Terraform, runOpts RunOptions, podType tfv1alpha1.PodType, generation int64) error {
	kind := "Pod"

	tfRunnerPodTypes := []string{
		string(tfv1alpha1.PodInit),
		string(tfv1alpha1.PodInitDelete),
		string(tfv1alpha1.PodPlan),
		string(tfv1alpha1.PodPlanDelete),
		string(tfv1alpha1.PodApply),
		string(tfv1alpha1.PodApplyDelete),
	}

	isTFRunner := utils.ListContainsStr(tfRunnerPodTypes, string(podType))

	var preScriptPodType tfv1alpha1.PodType
	if podType == tfv1alpha1.PodInit && tf.Spec.PreInitScript != "" {
		preScriptPodType = tfv1alpha1.PodPreInit
	} else if podType == tfv1alpha1.PodInitDelete && tf.Spec.PreInitDeleteScript != "" {
		preScriptPodType = tfv1alpha1.PodPreInitDelete
	} else if podType == tfv1alpha1.PodPlan && tf.Spec.PrePlanScript != "" {
		preScriptPodType = tfv1alpha1.PodPrePlan
	} else if podType == tfv1alpha1.PodPlanDelete && tf.Spec.PrePlanDeleteScript != "" {
		preScriptPodType = tfv1alpha1.PodPrePlanDelete
	} else if podType == tfv1alpha1.PodApply && tf.Spec.PreApplyScript != "" {
		preScriptPodType = tfv1alpha1.PodPreApply
	} else if podType == tfv1alpha1.PodApplyDelete && tf.Spec.PreApplyDeleteScript != "" {
		preScriptPodType = tfv1alpha1.PodPreApplyDelete
	}

	resource := runOpts.generatePod(podType, preScriptPodType, isTFRunner, generation)
	controllerutil.SetControllerReference(tf, resource, r.Scheme)

	err := r.Client.Create(ctx, resource)
	if err != nil {
		r.Recorder.Event(tf, "Warning", fmt.Sprintf("%sCreateError", kind), fmt.Sprintf("Could not create %s %v", kind, err))
		return err
	}
	r.Recorder.Event(tf, "Normal", "SuccessfulCreate", fmt.Sprintf("Created %s: '%s'", kind, resource.Name))
	return nil
}

func (r RunOptions) generateConfigMap() *corev1.ConfigMap {

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.versionedName,
			Namespace: r.namespace,
		},
		Data: r.mainModuleAddonData,
	}
	return cm
}

func (r RunOptions) generateServiceAccount() *corev1.ServiceAccount {
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
		},
	}
	return sa
}

func (r RunOptions) generateRole() *rbacv1.Role {
	// TODO tighten up default rbac security since all the cm and secret names
	// can be predicted.

	rules := []rbacv1.PolicyRule{
		{
			Verbs:     []string{"*"},
			APIGroups: []string{""},
			Resources: []string{"configmaps"},
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
	if r.mainModuleAddonData["backend_override.tf"] != "" {
		// parse the backennd string the way most people write it
		// example:
		// terraform {
		//   backend "kubernetes" {
		//     ...
		//   }
		// }
		s := strings.Split(r.mainModuleAddonData["backend_override.tf"], "\n")
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

	rules = append(rules, r.runnerRules...)

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.versionedName,
			Namespace: r.namespace,
		},
		Rules: rules,
	}
	return role
}

func (r RunOptions) generateRoleBinding() *rbacv1.RoleBinding {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.versionedName,
			Namespace: r.namespace,
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

func (r RunOptions) generatePVC(size string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.name,
			Namespace: r.namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(size),
				},
			},
		},
	}
}

func (r RunOptions) generatePod(podType, preScriptPodType tfv1alpha1.PodType, isTFRunner bool, generation int64) *corev1.Pod {

	generateName := r.versionedName + "-" + string(podType) + "-"

	generationPath := "/home/tfo-runner/generations/" + fmt.Sprint(generation)

	envs := r.envVars
	envs = append(envs, []corev1.EnvVar{
		{
			Name:  "TFO_RUNNER",
			Value: r.versionedName,
		},
		{
			Name:  "TFO_RESOURCE",
			Value: r.tfName,
		},
		{
			Name:  "TFO_NAMESPACE",
			Value: r.namespace,
		},
		{
			Name:  "TFO_GENERATION",
			Value: fmt.Sprintf("%d", generation),
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

	volumes := []corev1.Volume{
		{
			Name: "tfohome",
			VolumeSource: corev1.VolumeSource{
				//
				// TODO add an option to the tf to use host or pvc
				// 		for the plan.
				//
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: r.name,
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
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "tfohome",
			MountPath: "/home/tfo-runner",
			ReadOnly:  false,
		},
	}
	envs = append(envs, corev1.EnvVar{
		Name:  "TFO_ROOT_PATH",
		Value: "/home/tfo-runner",
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

	mainModuleAddonsConfigMapName := "main-module-addons"
	mainModuleAddonsConfigMapPath := "/tmp/main-module-addons"
	volumes = append(volumes, []corev1.Volume{
		{
			Name: mainModuleAddonsConfigMapName,
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
			Name:      mainModuleAddonsConfigMapName,
			MountPath: mainModuleAddonsConfigMapPath,
		},
	}...)
	envs = append(envs, []corev1.EnvVar{
		{
			Name:  "TFO_MAIN_MODULE_ADDONS",
			Value: mainModuleAddonsConfigMapPath,
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

	// Envs from this point may differ between runners
	terraformRunnerEnvs := make([]corev1.EnvVar, len(envs))
	scriptRunnerEnvs := make([]corev1.EnvVar, len(envs))
	setupRunnerEnvs := make([]corev1.EnvVar, len(envs))
	exportRunnerEnvs := make([]corev1.EnvVar, len(envs))
	copy(terraformRunnerEnvs, envs)
	copy(scriptRunnerEnvs, envs)
	copy(setupRunnerEnvs, envs)
	copy(exportRunnerEnvs, envs)

	executionScript := "tfo_runner.sh"
	if r.terraformRunnerExecutionScriptConfigMap != nil {
		executionScriptVolumeName := "terraform-runner-execution-script"
		tfo_runner_script := "/terraform-runner/" + executionScript
		volumes = append(volumes, []corev1.Volume{
			{
				Name: executionScriptVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: r.terraformRunnerExecutionScriptConfigMap.LocalObjectReference,
						DefaultMode:          &xmode,
						Optional:             r.terraformRunnerExecutionScriptConfigMap.Optional,
						Items: []corev1.KeyToPath{
							{
								Key:  r.terraformRunnerExecutionScriptConfigMap.Key,
								Path: executionScript,
							},
						},
					},
				},
			},
		}...)
		volumeMounts = append(volumeMounts, []corev1.VolumeMount{
			{
				Name:      executionScriptVolumeName,
				MountPath: tfo_runner_script,
				SubPath:   executionScript,
			},
		}...)
		terraformRunnerEnvs = append(terraformRunnerEnvs, corev1.EnvVar{
			Name:  "TFO_RUNNER_SCRIPT",
			Value: tfo_runner_script,
		})
	}

	if r.scriptRunnerExecutionScriptConfigMap != nil {
		executionScriptVolumeName := "script-runner-execution-script"
		tfo_runner_script := "/script-runner/" + executionScript
		volumes = append(volumes, []corev1.Volume{
			{
				Name: executionScriptVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: r.scriptRunnerExecutionScriptConfigMap.LocalObjectReference,
						DefaultMode:          &xmode,
						Optional:             r.scriptRunnerExecutionScriptConfigMap.Optional,
						Items: []corev1.KeyToPath{
							{
								Key:  r.scriptRunnerExecutionScriptConfigMap.Key,
								Path: executionScript,
							},
						},
					},
				},
			},
		}...)
		volumeMounts = append(volumeMounts, []corev1.VolumeMount{
			{
				Name:      executionScriptVolumeName,
				MountPath: tfo_runner_script,
				SubPath:   executionScript,
			},
		}...)
		scriptRunnerEnvs = append(scriptRunnerEnvs, corev1.EnvVar{
			Name:  "TFO_RUNNER_SCRIPT",
			Value: tfo_runner_script,
		})
	}

	if r.setupRunnerExecutionScriptConfigMap != nil {
		executionScriptVolumeName := "setup-runner-execution-script"
		tfo_runner_script := "/setup-runner/" + executionScript
		volumes = append(volumes, []corev1.Volume{
			{
				Name: executionScriptVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: r.setupRunnerExecutionScriptConfigMap.LocalObjectReference,
						DefaultMode:          &xmode,
						Optional:             r.setupRunnerExecutionScriptConfigMap.Optional,
						Items: []corev1.KeyToPath{
							{
								Key:  r.setupRunnerExecutionScriptConfigMap.Key,
								Path: executionScript,
							},
						},
					},
				},
			},
		}...)
		volumeMounts = append(volumeMounts, []corev1.VolumeMount{
			{
				Name:      executionScriptVolumeName,
				MountPath: tfo_runner_script,
				SubPath:   executionScript,
			},
		}...)
		setupRunnerEnvs = append(setupRunnerEnvs, corev1.EnvVar{
			Name:  "TFO_RUNNER_SCRIPT",
			Value: tfo_runner_script,
		})
	}

	annotations := r.runnerAnnotations
	envFrom := []corev1.EnvFromSource{}

	for _, c := range r.credentials {
		if c.AWSCredentials.KIAM != "" {
			annotations["iam.amazonaws.com/role"] = c.AWSCredentials.KIAM
		}
	}

	for _, c := range r.credentials {
		if (tfv1alpha1.SecretNameRef{}) != c.SecretNameRef {
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

	labels := make(map[string]string)
	labels["terraforms.tf.isaaguilar.com/generation"] = fmt.Sprintf("%d", generation)
	labels["terraforms.tf.isaaguilar.com/resourceName"] = r.tfName
	labels["terraforms.tf.isaaguilar.com/podPrefix"] = r.name
	labels["terraforms.tf.isaaguilar.com/terraformVersion"] = r.terraformVersion
	labels["app.kubernetes.io/name"] = "terraform-operator"
	labels["app.kubernetes.io/component"] = "terraform-operator-runner"
	labels["app.kubernetes.io/instance"] = string(podType)
	labels["app.kubernetes.io/created-by"] = "controller"

	initContainers := []corev1.Container{}
	containers := []corev1.Container{}

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

	if podType == tfv1alpha1.PodSetup || podType == tfv1alpha1.PodSetupDelete {
		// setup once per generation
		setupRunnerEnvs = append(setupRunnerEnvs, corev1.EnvVar{
			Name:  "TFO_RUNNER",
			Value: string(podType),
		})
		containers = append(containers, corev1.Container{
			Name:            "tfo-setup",
			SecurityContext: securityContext,
			Image:           r.setupRunner + ":" + r.setupRunnerVersion,
			ImagePullPolicy: r.setupRunnerPullPolicy,
			EnvFrom:         envFrom,
			Env:             setupRunnerEnvs,
			VolumeMounts:    volumeMounts,
		})
	} else if isTFRunner {
		terraformRunnerEnvs = append(terraformRunnerEnvs, corev1.EnvVar{
			Name:  "TFO_RUNNER",
			Value: string(podType),
		})
		scriptRunnerEnvs = append(scriptRunnerEnvs, corev1.EnvVar{
			Name:  "TFO_RUNNER",
			Value: string(podType),
		})
		containers = append(containers, corev1.Container{
			SecurityContext: securityContext,
			Name:            "tf",
			Image:           r.terraformRunner + ":" + r.terraformVersion,
			ImagePullPolicy: r.terraformRunnerPullPolicy,
			EnvFrom:         envFrom,
			Env:             terraformRunnerEnvs,
			VolumeMounts:    volumeMounts,
		})

		if preScriptPodType != "" {
			scriptRunnerEnvs = append(scriptRunnerEnvs, corev1.EnvVar{
				Name:  "TFO_SCRIPT",
				Value: string(preScriptPodType),
			})
			initContainers = append(initContainers, corev1.Container{
				SecurityContext: securityContext,
				Name:            "pre-script",
				Image:           r.scriptRunner + ":" + r.scriptRunnerVersion,
				ImagePullPolicy: r.scriptRunnerPullPolicy,
				EnvFrom:         envFrom,
				Env:             scriptRunnerEnvs,
				VolumeMounts:    volumeMounts,
			})
		}
	} else if podType == tfv1alpha1.PodExport {
		exportRef := r.exportOptions.stack.Hash
		if exportRef == "" {
			exportRef = "master"
		}
		exportRunnerEnvs = append(exportRunnerEnvs, []corev1.EnvVar{
			{
				Name:  "TFO_EXPORT_REPO_PATH",
				Value: generationPath + "/export",
			},
			{
				Name:  "TFO_EXPORT_REPO",
				Value: r.exportOptions.stack.Repo,
			},
			{
				Name:  "TFO_EXPORT_REPO_REF",
				Value: exportRef,
			},
			{
				Name:  "TFO_EXPORT_REPO_SUBDIR",
				Value: exportRef,
			},
			{
				Name:  "TFO_EXPORT_CONF_PATH",
				Value: r.exportOptions.confPath,
			},
			{
				Name:  "TFO_EXPORT_TFVARS_PATH",
				Value: r.exportOptions.tfvarsPath,
			},
			{
				Name:  "TFO_EXPORT_REPO_AUTOMATED_USER_NAME",
				Value: r.exportOptions.gitUsername,
			},
			{
				Name:  "TFO_EXPORT_REPO_AUTOMATED_USER_EMAIL",
				Value: r.exportOptions.gitEmail,
			},
		}...)
		containers = append(containers, corev1.Container{
			SecurityContext: securityContext,
			Name:            "export",
			Image:           "docker.io/isaaguilar/export-runner" + ":" + "0.0.1", // CHANGE THIS TO EXPORT RUNNER, FOR NOW USE SOMETHING
			ImagePullPolicy: corev1.PullAlways,
			EnvFrom:         envFrom,
			Env:             exportRunnerEnvs,
			VolumeMounts:    volumeMounts,
			// Command:         []string{"/bin/sleep"},
			// Args:            []string{"3600"},
		})
	} else {
		scriptRunnerEnvs = append(scriptRunnerEnvs, corev1.EnvVar{
			Name:  "TFO_SCRIPT",
			Value: string(podType),
		})
		containers = append(containers, corev1.Container{
			SecurityContext: securityContext,
			Name:            "script",
			Image:           r.scriptRunner + ":" + r.scriptRunnerVersion,
			ImagePullPolicy: r.scriptRunnerPullPolicy,
			EnvFrom:         envFrom,
			Env:             scriptRunnerEnvs,
			VolumeMounts:    volumeMounts,
		})
	}
	podSecurityContext := corev1.PodSecurityContext{
		FSGroup: &user,
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: generateName,
			Namespace:    r.namespace,
			Labels:       labels,
			Annotations:  annotations,
		},
		Spec: corev1.PodSpec{
			SecurityContext:    &podSecurityContext,
			ServiceAccountName: r.serviceAccount,
			RestartPolicy:      corev1.RestartPolicyNever,
			InitContainers:     initContainers,
			Containers:         containers,
			Volumes:            volumes,
		},
	}

	return pod
}

func (r ReconcileTerraform) run(ctx context.Context, reqLogger logr.Logger, tf *tfv1alpha1.Terraform, runOpts RunOptions, isNewGeneration, isFirstInstall bool, podType tfv1alpha1.PodType, generation int64) (err error) {

	if isFirstInstall || isNewGeneration {
		if err := r.createPVC(ctx, tf, runOpts); err != nil {
			return err
		}

		if err := r.createSecret(ctx, tf, runOpts.versionedName, runOpts.namespace, runOpts.secretData, true); err != nil {
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

		if err := r.createSecret(ctx, tf, runOpts.outputsSecretName, runOpts.namespace, map[string][]byte{}, false); err != nil {
			return err
		}

	} else {
		// check resources exists
		lookupKey := types.NamespacedName{
			Name:      runOpts.name,
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

	if err := r.createPod(ctx, tf, runOpts, podType, generation); err != nil {
		return err
	}

	return nil
}

func (r ReconcileTerraform) createGitAskpass(ctx context.Context, tokenSecret tfv1alpha1.TokenSecretRef) ([]byte, error) {
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

func generateSecret(name, namespace string, data map[string][]byte) *corev1.Secret {
	secretObject := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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

func (r *ReconcileTerraform) exportRepo(ctx context.Context, tf *tfv1alpha1.Terraform, runOpts RunOptions, generation int64, reqLogger logr.Logger) (tfv1alpha1.Exported, error) {
	var exportedStatus tfv1alpha1.Exported = tf.Status.Exported
	if tf.Status.Exported == tfv1alpha1.ExportedTrue {
		return exportedStatus, nil
	}

	var err error
	var exportPodType tfv1alpha1.PodType = tfv1alpha1.PodExport

	// Updates from exported False to InProgress
	if tf.Status.Exported == tfv1alpha1.ExportedFalse {
		exportedStatus = tfv1alpha1.ExportedPending
		return exportedStatus, nil
	}

	inNamespace := client.InNamespace(tf.Namespace)
	f := fields.Set{
		"metadata.generateName": fmt.Sprintf("%s-%s-", tf.Status.PodNamePrefix+"-v"+fmt.Sprint(generation), exportPodType),
	}
	labels := map[string]string{
		"terraforms.tf.isaaguilar.com/generation": fmt.Sprintf("%d", generation),
	}
	matchingFields := client.MatchingFields(f)
	matchingLabels := client.MatchingLabels(labels)
	pods := &corev1.PodList{}
	err = r.Client.List(ctx, pods, inNamespace, matchingFields, matchingLabels)
	if err != nil {
		reqLogger.Error(err, "")
		return exportedStatus, err
	}

	if len(pods.Items) == 0 && tf.Status.Exported == tfv1alpha1.ExportedInProgress {
		reqLogger.V(1).Info("No pods found. Removing the in-progress status")
		// This case can happen if the pod is running and the user deletes it.
		// Reset the phase to pending and then update the status which will
		// force the resource to requeue.
		exportedStatus = tfv1alpha1.ExportedPending
		return exportedStatus, nil
	}

	if len(pods.Items) == 0 && tf.Status.Exported == tfv1alpha1.ExportedPending {
		var err error
		n := len(tf.Status.Stages)
		generation := tf.Status.Stages[n-1].Generation
		scmMap := make(map[string]scmType)
		for _, v := range tf.Spec.SCMAuthMethods {
			if v.Git != nil {
				scmMap[v.Host] = gitScmType
			}
		}

		runOpts.terraformModuleParsed, err = getParsedAddress(tf.Spec.TerraformModule, "", false, scmMap)
		if err != nil {
			r.Recorder.Event(tf, "Warning", "ConfigError", err.Error())
			return exportedStatus, err
		}
		runOpts.exportOptions.stack, err = getParsedAddress(tf.Spec.ExportRepo.Address, "", false, scmMap)
		if err != nil {
			r.Recorder.Event(tf, "Warning", "ConfigError", err.Error())
			return exportedStatus, err
		}
		runOpts.exportOptions.confPath = tf.Spec.ExportRepo.ConfFile
		runOpts.exportOptions.tfvarsPath = tf.Spec.ExportRepo.TFVarsFile
		runOpts.exportOptions.gitEmail = "terraform-operator@example.com"
		runOpts.exportOptions.gitUsername = "Terraform Operator"

		err = r.run(ctx, reqLogger, tf, runOpts, false, false, tfv1alpha1.PodExport, generation)
		if err != nil {
			return exportedStatus, err
		}
		exportedStatus = tfv1alpha1.ExportCreating
	}

	if len(pods.Items) > 0 {
		if pods.Items[0].Status.Phase == corev1.PodFailed {
			exportedStatus = tfv1alpha1.ExportedPending
			return exportedStatus, nil
		}

		if pods.Items[0].Status.Phase == corev1.PodSucceeded {
			exportedStatus = tfv1alpha1.ExportedTrue
			if !tf.Spec.KeepCompletedPods {
				err := r.Client.Delete(ctx, &pods.Items[0])
				if err != nil {
					reqLogger.V(1).Info(err.Error())
				}
			}
			return exportedStatus, nil
		}

		if tf.Status.Exported != tfv1alpha1.ExportedInProgress {
			exportedStatus = tfv1alpha1.ExportedInProgress
			return exportedStatus, nil
		}

	}
	return exportedStatus, nil
}
