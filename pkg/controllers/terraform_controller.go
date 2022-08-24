package controllers

import (
	"context"
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
	"github.com/go-logr/logr"
	getter "github.com/hashicorp/go-getter"
	tfv1alpha2 "github.com/isaaguilar/terraform-operator/pkg/apis/tf/v1alpha2"
	"github.com/isaaguilar/terraform-operator/pkg/utils"
	localcache "github.com/patrickmn/go-cache"
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

	if os.Getenv("DO_NOT_RECONCILE") != "" {
		/*

			Webhooks only! useful for testing

		*/
		return nil
	}
	// only listedn to v1alpha2
	err := ctrl.NewControllerManagedBy(mgr).
		For(&tfv1alpha2.Terraform{}).
		Watches(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &tfv1alpha2.Terraform{},
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

type RunOptions struct {
	annotations                         map[string]string
	configMapSourceName                 string
	configMapSourceKey                  string
	credentials                         []tfv1alpha2.Credentials
	env                                 []corev1.EnvVar
	envFrom                             []corev1.EnvFromSource
	generation                          int64
	image                               string
	imagePullPolicy                     corev1.PullPolicy
	labels                              map[string]string
	mainModuleAddonData                 map[string]string
	namespace                           string
	outputsSecretName                   string
	outputsToInclude                    []string
	outputsToOmit                       []string
	policyRules                         []rbacv1.PolicyRule
	prefixedName                        string
	resourceLabels                      map[string]string
	resourceName                        string
	runType                             tfv1alpha2.TaskType
	saveOutputs                         bool
	secretData                          map[string][]byte
	serviceAccount                      string
	cleanupDisk                         bool
	stripGenerationLabelOnOutputsSecret bool
	terraformModuleParsed               ParsedAddress
	terraformVersion                    string
	urlSource                           string
	versionedName                       string
}

func newRunOptions(tf *tfv1alpha2.Terraform, runType tfv1alpha2.TaskType, generation int64) RunOptions {
	// TODO Read the tfstate and decide IF_NEW_RESOURCE based on that
	// applyAction := false
	resourceName := tf.Name
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
	envFrom := []corev1.EnvFromSource{}
	cleanupDisk := false
	urlSource := ""
	configMapSourceName := ""
	configMapSourceKey := ""

	// TaskOptions have data for all the tasks but since we're only interested
	// in the ones for this taskType, extract and add them to RunOptions
	for _, taskOption := range tf.Spec.TaskOptions {
		if tfv1alpha2.ListContainsRunType(taskOption.TaskTypes, runType) ||
			tfv1alpha2.ListContainsRunType(taskOption.TaskTypes, "*") {
			policyRules = append(policyRules, taskOption.PolicyRules...)
			for key, value := range taskOption.Annotations {
				annotations[key] = value
			}
			for key, value := range taskOption.Labels {
				labels[key] = value
			}
			env = append(env, taskOption.Env...)
			envFrom = append(envFrom, taskOption.EnvFrom...)
		}
		if tfv1alpha2.ListContainsRunType(taskOption.TaskTypes, runType) {
			urlSource = taskOption.Script.Source
			if configMapSelector := taskOption.Script.ConfigMapSelector; configMapSelector != nil {
				configMapSourceName = configMapSelector.Name
				configMapSourceKey = configMapSelector.Key
			}
		}
	}

	images := tf.Spec.Images
	if images == nil {
		// setup default images
		images = &tfv1alpha2.Images{}
	}

	if images.Terraform == nil {
		images.Terraform = &tfv1alpha2.ImageConfig{
			ImagePullPolicy: corev1.PullIfNotPresent,
		}
	}

	if images.Terraform.Image == "" {
		images.Terraform.Image = fmt.Sprintf("ghcr.io/galleybytes/terraform-operator-tftaskv1:%s", terraformVersion)
	} else {
		terraformImage := images.Terraform.Image
		splitImage := strings.Split(images.Terraform.Image, ":")
		if length := len(splitImage); length > 1 {
			terraformImage = strings.Join(splitImage[:length-1], ":")
		}
		images.Terraform.Image = fmt.Sprintf("%s:%s", terraformImage, terraformVersion)
	}

	if images.Setup == nil {
		images.Setup = &tfv1alpha2.ImageConfig{
			ImagePullPolicy: corev1.PullIfNotPresent,
		}
	}

	if images.Setup.Image == "" {
		images.Setup.Image = "ghcr.io/galleybytes/terraform-operator-setup:1.0.0"
	}

	if images.Script == nil {
		images.Script = &tfv1alpha2.ImageConfig{
			ImagePullPolicy: corev1.PullIfNotPresent,
		}
	}

	if images.Script.Image == "" {
		images.Script.Image = "ghcr.io/galleybytes/terraform-operator-script:1.0.0"
	}

	terraformRunTypes := []tfv1alpha2.TaskType{
		tfv1alpha2.RunInit,
		tfv1alpha2.RunInitDelete,
		tfv1alpha2.RunPlan,
		tfv1alpha2.RunPlanDelete,
		tfv1alpha2.RunApply,
		tfv1alpha2.RunApplyDelete,
	}

	scriptRunTypes := []tfv1alpha2.TaskType{
		tfv1alpha2.RunPreInit,
		tfv1alpha2.RunPreInitDelete,
		tfv1alpha2.RunPostInit,
		tfv1alpha2.RunPostInitDelete,
		tfv1alpha2.RunPrePlan,
		tfv1alpha2.RunPrePlanDelete,
		tfv1alpha2.RunPostPlan,
		tfv1alpha2.RunPostPlanDelete,
		tfv1alpha2.RunPreApply,
		tfv1alpha2.RunPreApplyDelete,
		tfv1alpha2.RunPostApply,
		tfv1alpha2.RunPostApplyDelete,
	}

	setupRunTypes := []tfv1alpha2.TaskType{
		tfv1alpha2.RunSetup,
		tfv1alpha2.RunSetupDelete,
	}

	if tfv1alpha2.ListContainsRunType(terraformRunTypes, runType) {
		image = images.Terraform.Image
		imagePullPolicy = images.Terraform.ImagePullPolicy
		if urlSource == "" {
			urlSource = "https://raw.githubusercontent.com/GalleyBytes/terraform-operator-tasks/master/tf.sh"
		}
	} else if tfv1alpha2.ListContainsRunType(scriptRunTypes, runType) {
		image = images.Script.Image
		imagePullPolicy = images.Script.ImagePullPolicy
		if urlSource == "" {
			urlSource = "https://raw.githubusercontent.com/GalleyBytes/terraform-operator-tasks/master/noop.sh"
		}
	} else if tfv1alpha2.ListContainsRunType(setupRunTypes, runType) {
		image = images.Setup.Image
		imagePullPolicy = images.Setup.ImagePullPolicy
		if urlSource == "" {
			urlSource = "https://raw.githubusercontent.com/GalleyBytes/terraform-operator-tasks/master/setup.sh"
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
	outputsSecretName := prefixedName + "-outputs"
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

	resourceLabels := make(map[string]string)

	if tf.Spec.Setup != nil {
		cleanupDisk = tf.Spec.Setup.CleanupDisk
	}

	return RunOptions{
		env:                                 env,
		generation:                          generation,
		configMapSourceName:                 configMapSourceName,
		configMapSourceKey:                  configMapSourceKey,
		envFrom:                             envFrom,
		policyRules:                         policyRules,
		annotations:                         annotations,
		labels:                              labels,
		imagePullPolicy:                     imagePullPolicy,
		namespace:                           tf.Namespace,
		resourceName:                        resourceName,
		prefixedName:                        prefixedName,
		versionedName:                       versionedName,
		credentials:                         credentials,
		terraformVersion:                    terraformVersion,
		image:                               image,
		runType:                             runType,
		resourceLabels:                      resourceLabels,
		serviceAccount:                      serviceAccount,
		mainModuleAddonData:                 make(map[string]string),
		secretData:                          make(map[string][]byte),
		cleanupDisk:                         cleanupDisk,
		outputsSecretName:                   outputsSecretName,
		saveOutputs:                         saveOutputs,
		stripGenerationLabelOnOutputsSecret: stripGenerationLabelOnOutputsSecret,
		outputsToInclude:                    outputsToInclude,
		outputsToOmit:                       outputsToOmit,
		urlSource:                           urlSource,
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
	if tf.Status.Phase == tfv1alpha2.PhaseDeleted {
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
		tf.Status.Stages = []tfv1alpha2.Stage{}
		tf.Status.LastCompletedGeneration = 0
		tf.Status.Phase = tfv1alpha2.PhaseInitializing

		err := r.updateStatusWithRetry(ctx, tf, &tf.Status, reqLogger)
		if err != nil {
			reqLogger.V(1).Info(err.Error())
		}
		return reconcile.Result{}, nil
	}

	// Add the first stage
	if tf.Status.Stage.Generation == 0 {
		runType := tfv1alpha2.RunSetup
		stageState := tfv1alpha2.StateInitializing
		interruptible := tfv1alpha2.CanNotBeInterrupt
		stage := newStage(tf, runType, "TF_RESOURCE_CREATED", interruptible, stageState)
		if stage == nil {
			return reconcile.Result{}, fmt.Errorf("failed to create a new stage")
		}
		tf.Status.Stage = *stage
		tf.Status.Exported = tfv1alpha2.ExportedFalse

		err := r.updateStatusWithRetry(ctx, tf, &tf.Status, reqLogger)
		if err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	deletePhases := []string{
		string(tfv1alpha2.PhaseDeleting),
		string(tfv1alpha2.PhaseInitDelete),
		string(tfv1alpha2.PhaseDeleted),
	}

	// Check if the resource is marked to be deleted which is
	// indicated by the deletion timestamp being set.
	if tf.GetDeletionTimestamp() != nil && !utils.ListContainsStr(deletePhases, string(tf.Status.Phase)) {
		tf.Status.Phase = tfv1alpha2.PhaseInitDelete
	}

	// // TODO Check the status on stages that have not completed
	// for _, stage := range tf.Status.Stages {
	// 	if stage.State == tfv1alpha1.StateInProgress {
	//
	// 	}
	// }
	stage := checkSetNewStage(tf)
	if stage != nil {
		reqLogger.V(3).Info(fmt.Sprintf("Stage moving from '%s' -> '%s'", tf.Status.Stage.PodType, stage.PodType))
		tf.Status.Stage = *stage
		desiredStatus := tf.Status
		err := r.updateStatusWithRetry(ctx, tf, &desiredStatus, reqLogger)
		if err != nil {
			reqLogger.V(1).Info(fmt.Sprintf("Error adding stage '%s': %s", stage.PodType, err.Error()))
		}
		if tf.Spec.KeepLatestPodsOnly {
			go r.backgroundReapOldGenerationPods(tf, 0)
		}
		return reconcile.Result{}, nil
	}

	currentStage := tf.Status.Stage
	podType := currentStage.PodType
	generation := currentStage.Generation
	runOpts := newRunOptions(tf, currentStage.PodType, generation)

	/*
		The export repo will be it's own project and
		will use the tf resource as a ref.  The
		ref will look for the status to get the
		stage's generation and have logic to decide
		if it should do an export. The export fields
			will be removed from the v1alpha2 api and
			added to the export controller.
	*/

	// if OLDTF.Spec.ExportRepo != nil && podType != tfv1alpha1.PodSetup {
	// 	// Run the export-runner
	// 	exportedStatus, err := r.exportRepo(ctx, OLDTF, runOpts, generation, reqLogger)
	// 	if err != nil {
	// 		return reconcile.Result{}, err
	// 	}
	// 	if OLDTF.Status.Exported != exportedStatus {
	// 		OLDTF.Status.Exported = exportedStatus
	// 		err := r.OLD_updateStatus(ctx, OLDTF)
	// 		if err != nil {
	// 			reqLogger.V(1).Info(err.Error())
	// 			return reconcile.Result{}, err
	// 		}
	// 		return reconcile.Result{}, nil
	// 	}
	// }

	if podType == tfv1alpha2.RunNil {
		// podType is blank when the terraform workflow has completed for
		// either create or delete.

		if tf.Status.Phase == tfv1alpha2.PhaseRunning {
			// Updates the status as "completed" on the resource
			tf.Status.Phase = tfv1alpha2.PhaseCompleted
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
		} else if tf.Status.Phase == tfv1alpha2.PhaseDeleting {
			// Updates the status as "deleted" which will be used to tell the
			// controller to remove any finalizers).
			tf.Status.Phase = tfv1alpha2.PhaseDeleted
			err := r.updateStatusWithRetry(ctx, tf, &tf.Status, reqLogger)
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
	labelSelector := map[string]string{
		"terraforms.tf.isaaguilar.com/generation": fmt.Sprintf("%d", generation),
	}
	matchingFields := client.MatchingFields(f)
	matchingLabels := client.MatchingLabels(labelSelector)
	pods := &corev1.PodList{}
	err = r.Client.List(ctx, pods, inNamespace, matchingFields, matchingLabels)
	if err != nil {
		reqLogger.Error(err, "")
		return reconcile.Result{}, nil
	}

	if len(pods.Items) == 0 && tf.Status.Stage.State == tfv1alpha2.StateInProgress {
		// This condition is generally met when the user deletes the pod.
		// Force the state to transition away from in-progress and then
		// requeue.
		tf.Status.Stage.State = tfv1alpha2.StateInitializing
		err = r.updateStatusWithRetry(ctx, tf, &tf.Status, reqLogger)
		if err != nil {
			reqLogger.V(1).Info(err.Error())
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, nil
	}

	// if 1 > 0 {
	// 	// fmt.Println(utils.PrettyStruct(runOpts))
	// 	fmt.Printf("%+v\n", runOpts)
	// 	reqLogger.Info("Not doing anything until further notice.")
	// 	return reconcile.Result{}, nil
	// }
	// OLDTF := &tfv1alpha1.Terraform{}

	if len(pods.Items) == 0 {
		// Trigger a new pod when no pods are found for current stage
		reqLogger.V(1).Info(fmt.Sprintf("Setting up the '%s' pod", podType))
		err := r.setupAndRun(ctx, tf, runOpts)
		if err != nil {
			reqLogger.Error(err, "")
			return reconcile.Result{}, err
		}
		if tf.Status.Phase == tfv1alpha2.PhaseInitializing {
			tf.Status.Phase = tfv1alpha2.PhaseRunning
		} else if tf.Status.Phase == tfv1alpha2.PhaseInitDelete {
			tf.Status.Phase = tfv1alpha2.PhaseDeleting
		}
		tf.Status.Stage.State = tfv1alpha2.StateInProgress

		// TODO Becuase the pod is already running, is it critical that the
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
		tf.Status.Stage.State = tfv1alpha2.StateFailed
		tf.Status.Stage.StopTime = metav1.NewTime(time.Now())
		err = r.updateStatusWithRetry(ctx, tf, &tf.Status, reqLogger)
		if err != nil {
			reqLogger.V(1).Info(err.Error())
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	if pods.Items[0].Status.Phase == corev1.PodSucceeded {
		tf.Status.Stage.State = tfv1alpha2.StateComplete
		tf.Status.Stage.StopTime = metav1.NewTime(time.Now())
		err = r.updateStatusWithRetry(ctx, tf, &tf.Status, reqLogger)
		if err != nil {
			reqLogger.V(1).Info(err.Error())
			return reconcile.Result{}, err
		}
		if !tf.Spec.KeepCompletedPods && !tf.Spec.KeepLatestPodsOnly {
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
func (r ReconcileTerraform) getTerraformResource(ctx context.Context, namespacedName types.NamespacedName, maxRetry int, reqLogger logr.Logger) (*tfv1alpha2.Terraform, error) {
	tf := &tfv1alpha2.Terraform{}
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

func newStage(tf *tfv1alpha2.Terraform, runType tfv1alpha2.TaskType, reason string, interruptible tfv1alpha2.Interruptible, stageState tfv1alpha2.StageState) *tfv1alpha2.Stage {
	if reason == "GENERATION_CHANGE" {
		tf.Status.Exported = tfv1alpha2.ExportedFalse
		tf.Status.Phase = tfv1alpha2.PhaseInitializing
	}
	startTime := metav1.NewTime(time.Now())
	stopTime := metav1.NewTime(time.Unix(0, 0))
	if stageState == tfv1alpha2.StateComplete {
		stopTime = startTime
	}
	return &tfv1alpha2.Stage{
		Generation:    tf.Generation,
		Interruptible: interruptible,
		Reason:        reason,
		State:         stageState,
		PodType:       runType,
		StartTime:     startTime,
		StopTime:      stopTime,
	}
}

func getConfiguredTasks(taskOptions *[]tfv1alpha2.TaskOption) []tfv1alpha2.TaskType {
	tasks := []tfv1alpha2.TaskType{
		tfv1alpha2.RunSetup,
		tfv1alpha2.RunInit,
		tfv1alpha2.RunPlan,
		tfv1alpha2.RunApply,
		tfv1alpha2.RunSetupDelete,
		tfv1alpha2.RunInitDelete,
		tfv1alpha2.RunPlanDelete,
		tfv1alpha2.RunApplyDelete,
	}
	if taskOptions == nil {
		return tasks
	}
	for _, taskOption := range *taskOptions {
		for _, runType := range taskOption.TaskTypes {
			if runType == "*" {
				continue
			}
			if !tfv1alpha2.ListContainsRunType(tasks, runType) {
				tasks = append(tasks, runType)
			}
		}
	}
	return tasks
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
func checkSetNewStage(tf *tfv1alpha2.Terraform) *tfv1alpha2.Stage {
	var isNewStage bool
	var podType tfv1alpha2.TaskType
	var reason string
	configuredTasks := getConfiguredTasks(&tf.Spec.TaskOptions)

	deletePhases := []string{
		string(tfv1alpha2.PhaseDeleted),
		string(tfv1alpha2.PhaseInitDelete),
		string(tfv1alpha2.PhaseDeleted),
	}
	tfIsFinalizing := utils.ListContainsStr(deletePhases, string(tf.Status.Phase))
	tfIsNotFinalizing := !tfIsFinalizing
	initDelete := tf.Status.Phase == tfv1alpha2.PhaseInitDelete
	stageState := tfv1alpha2.StateInitializing
	interruptible := tfv1alpha2.CanBeInterrupt

	currentStage := tf.Status.Stage
	currentStagePodType := currentStage.PodType
	currentStageCanNotBeInterrupted := currentStage.Interruptible == tfv1alpha2.CanNotBeInterrupt
	currentStageIsRunning := currentStage.State == tfv1alpha2.StateInProgress
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
		podType = tfv1alpha2.RunSetup

		// } else if initDelete && !utils.ListContainsStr(deletePodTypes, string(currentStagePodType)) {
	} else if initDelete && isNewGeneration {
		// The tf resource is marked for deletion and this is the first pod
		// in the terraform destroy workflow.
		isNewStage = true
		reason = "TF_RESOURCE_DELETED"
		podType = tfv1alpha2.RunSetupDelete
		interruptible = tfv1alpha2.CanNotBeInterrupt

	} else if currentStage.State == tfv1alpha2.StateComplete {
		isNewStage = true
		reason = fmt.Sprintf("COMPLETED_%s", strings.ToUpper(currentStage.PodType.String()))

		switch currentStagePodType {

		case tfv1alpha2.RunNil:
			isNewStage = false

		default:
			podType = nextTask(currentStagePodType, configuredTasks)
			interruptible = isTaskInterruptable(podType)
			if podType == tfv1alpha2.RunNil {
				stageState = tfv1alpha2.StateComplete
			}
		}

	}
	if !isNewStage {
		return nil
	}
	return newStage(tf, podType, reason, interruptible, stageState)

}

// These are pods that are known to cause issues with terraform state when
// not run to completion.
func isTaskInterruptable(task tfv1alpha2.TaskType) tfv1alpha2.Interruptible {
	uninterruptibleTasks := []tfv1alpha2.TaskType{
		tfv1alpha2.RunInit,
		tfv1alpha2.RunPlan,
		tfv1alpha2.RunApply,
		tfv1alpha2.RunInitDelete,
		tfv1alpha2.RunPlanDelete,
		tfv1alpha2.RunApplyDelete,
	}
	if tfv1alpha2.ListContainsRunType(uninterruptibleTasks, task) {
		return tfv1alpha2.CanNotBeInterrupt
	}
	return tfv1alpha2.CanBeInterrupt
}

func nextTask(currentTask tfv1alpha2.TaskType, configuredTasks []tfv1alpha2.TaskType) tfv1alpha2.TaskType {
	tasksInOrder := []tfv1alpha2.TaskType{
		tfv1alpha2.RunSetup,
		tfv1alpha2.RunPreInit,
		tfv1alpha2.RunInit,
		tfv1alpha2.RunPostInit,
		tfv1alpha2.RunPrePlan,
		tfv1alpha2.RunPlan,
		tfv1alpha2.RunPostPlan,
		tfv1alpha2.RunPreApply,
		tfv1alpha2.RunApply,
		tfv1alpha2.RunPostApply,
	}
	deleteTasksInOrder := []tfv1alpha2.TaskType{
		tfv1alpha2.RunSetupDelete,
		tfv1alpha2.RunPreInitDelete,
		tfv1alpha2.RunInitDelete,
		tfv1alpha2.RunPostInitDelete,
		tfv1alpha2.RunPrePlanDelete,
		tfv1alpha2.RunPlanDelete,
		tfv1alpha2.RunPostPlanDelete,
		tfv1alpha2.RunPreApplyDelete,
		tfv1alpha2.RunApplyDelete,
		tfv1alpha2.RunPostApplyDelete,
	}

	next := tfv1alpha2.RunNil
	isUpNext := false
	if tfv1alpha2.ListContainsRunType(tasksInOrder, currentTask) {
		for _, task := range tasksInOrder {
			if task == currentTask {
				isUpNext = true
				continue
			}
			if isUpNext && tfv1alpha2.ListContainsRunType(configuredTasks, task) {
				next = task
				break
			}
		}
	} else if tfv1alpha2.ListContainsRunType(deleteTasksInOrder, currentTask) {
		for _, task := range deleteTasksInOrder {
			if task == currentTask {
				isUpNext = true
				continue
			}
			if isUpNext && tfv1alpha2.ListContainsRunType(configuredTasks, task) {
				next = task
				break
			}
		}
	}
	return next
}

func (r ReconcileTerraform) backgroundReapOldGenerationPods(tf *tfv1alpha2.Terraform, attempt int) {
	logger := r.Log.WithName("Reaper").WithValues("Terraform", fmt.Sprintf("%s/%s", tf.Namespace, tf.Name))
	if attempt > 20 {
		// TODO explain what and way resources cannot be reaped
		logger.Info("Could not reap resources: Max attempts to reap old-generation resources")
		return
	}

	// The labels required are read as:
	// 1. The terraforms.tf.isaaguilar.com/generation key MUST exist
	// 2. The terraforms.tf.isaaguilar.com/generation value MUST match the current resource generation
	// 3. The terraforms.tf.isaaguilar.com/resourceName key MUST exist
	// 4. The terraforms.tf.isaaguilar.com/resourceName value MUST match the resource name
	labelSelector, err := labels.Parse(fmt.Sprintf("terraforms.tf.isaaguilar.com/generation,terraforms.tf.isaaguilar.com/generation!=%d,terraforms.tf.isaaguilar.com/resourceName,terraforms.tf.isaaguilar.com/resourceName=%s", tf.Generation, tf.Name))
	if err != nil {
		logger.Error(err, "Could not parse labels")
	}
	fieldSelector, err := fields.ParseSelector("status.phase!=Running")
	if err != nil {
		logger.Error(err, "Could not parse fields")
	}

	err = r.Client.DeleteAllOf(context.TODO(), &corev1.Pod{}, &client.DeleteAllOfOptions{
		ListOptions: client.ListOptions{
			LabelSelector: labelSelector,
			Namespace:     tf.Namespace,
			FieldSelector: fieldSelector,
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
		LabelSelector: labelSelector,
		Namespace:     tf.Namespace,
	})
	if err != nil {
		logger.Error(err, "Could not list pods to reap")
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
				Namespace:     tf.Namespace,
			},
		})
		if err != nil {
			logger.Error(err, "Could not reap old generation configmaps")
		}

		err = r.Client.DeleteAllOf(context.TODO(), &corev1.Secret{}, &client.DeleteAllOfOptions{
			ListOptions: client.ListOptions{
				LabelSelector: labelSelector,
				Namespace:     tf.Namespace,
			},
		})
		if err != nil {
			logger.Error(err, "Could not reap old generation secrets")
		}

		err = r.Client.DeleteAllOf(context.TODO(), &rbacv1.Role{}, &client.DeleteAllOfOptions{
			ListOptions: client.ListOptions{
				LabelSelector: labelSelector,
				Namespace:     tf.Namespace,
			},
		})
		if err != nil {
			logger.Error(err, "Could not reap old generation roles")
		}

		err = r.Client.DeleteAllOf(context.TODO(), &rbacv1.RoleBinding{}, &client.DeleteAllOfOptions{
			ListOptions: client.ListOptions{
				LabelSelector: labelSelector,
				Namespace:     tf.Namespace,
			},
		})
		if err != nil {
			logger.Error(err, "Could not reap old generation roleBindings")
		}

		err = r.Client.DeleteAllOf(context.TODO(), &corev1.ServiceAccount{}, &client.DeleteAllOfOptions{
			ListOptions: client.ListOptions{
				LabelSelector: labelSelector,
				Namespace:     tf.Namespace,
			},
		})
		if err != nil {
			logger.Error(err, "Could not reap old generation serviceAccounts")
		}
	}
}

// updateFinalizer sets and unsets the finalizer on the tf resource. When
// IgnoreDelete is true, the finalizer is removed. When IgnoreDelete is false,
// the finalizer is added.
//
// The finalizer will be responsible for starting the destroy-workflow.
func updateFinalizer(tf *tfv1alpha2.Terraform) bool {
	finalizers := tf.GetFinalizers()

	if tf.Status.Phase == tfv1alpha2.PhaseDeleted {
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

func (r ReconcileTerraform) update(ctx context.Context, tf *tfv1alpha2.Terraform) error {
	err := r.Client.Update(ctx, tf)
	if err != nil {
		return fmt.Errorf("failed to update tf resource: %s", err)
	}
	return nil
}

func (r ReconcileTerraform) updateStatus(ctx context.Context, tf *tfv1alpha2.Terraform) error {
	err := r.Client.Status().Update(ctx, tf)
	if err != nil {
		return fmt.Errorf("failed to update tf status: %s", err)
	}
	return nil
}

func (r ReconcileTerraform) updateStatusWithRetry(ctx context.Context, tf *tfv1alpha2.Terraform, desiredStatus *tfv1alpha2.TerraformStatus, logger logr.Logger) error {
	var getResourceErr error
	var updateErr error
	for i := 0; i < 10; i++ {
		if i > 0 {
			n := math.Pow(2, float64(i+3))
			backoffTime := math.Ceil(.5 * (n - 1))
			time.Sleep(time.Duration(backoffTime) * time.Millisecond)
			tf, getResourceErr = r.getTerraformResource(ctx, types.NamespacedName{Namespace: tf.Namespace, Name: tf.Name}, 10, logger)
			if getResourceErr != nil {
				return fmt.Errorf("failed to get latest terraform: %s", getResourceErr)
			}
			if desiredStatus != nil {
				tf.Status = *desiredStatus
			}
		}
		updateErr = r.Client.Status().Update(ctx, tf)
		if updateErr != nil {
			continue
		}
		break
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

func formatJobSSHConfig(ctx context.Context, reqLogger logr.Logger, tf *tfv1alpha2.Terraform, k8sclient client.Client) (map[string][]byte, error) {
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
			ns = tf.Namespace
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
				ns = tf.Namespace
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

func (r *ReconcileTerraform) setupAndRun(ctx context.Context, tf *tfv1alpha2.Terraform, runOpts RunOptions) error {
	reqLogger := r.Log.WithValues("Terraform", types.NamespacedName{Name: tf.Name, Namespace: tf.Namespace}.String())
	var err error

	generation := runOpts.generation
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

	/*


		Load the terraform module from one of source, inline, or configmap


		To test new version, just load the source version.

	*/

	if tf.Spec.TerraformModule.Inline != "" {
		// Add add inline to configmap and instruct the pod to fetch the
		// configmap as the main module
		runOpts.mainModuleAddonData["inline-module.tf"] = tf.Spec.TerraformModule.Inline
	}

	if tf.Spec.TerraformModule.ConfigMapSelector != nil {
		// Instruct the setup pod to fetch the configmap as the main module
		b, err := json.Marshal(tf.Spec.TerraformModule.ConfigMapSelector)
		if err != nil {
			return err
		}
		runOpts.mainModuleAddonData[".__TFO__ConfigMapModule.json"] = string(b)
	}

	if tf.Spec.TerraformModule.Source != "" {
		runOpts.terraformModuleParsed, err = getParsedAddress(tf.Spec.TerraformModule.Source, "", false, scmMap)
		if err != nil {
			return err
		}
	} else {

		// TODO REMOVE ME
		return fmt.Errorf("for testing, only allow the terraform module from source")

	}

	if isChanged {

		for _, taskOption := range tf.Spec.TaskOptions {
			if inlineScript := taskOption.Script.Inline; inlineScript != "" {
				for _, taskType := range taskOption.TaskTypes {
					if taskType.String() == "*" {
						continue
					}
					runOpts.mainModuleAddonData[fmt.Sprintf("inline-%s.sh", taskType)] = inlineScript
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

		runOpts.mainModuleAddonData[".__TFO__ResourceDownloads.json"] = resourceDownloads

		// Override the backend.tf by inserting a custom backend
		runOpts.mainModuleAddonData["backend_override.tf"] = tf.Spec.Backend

		/*


			All the tasks will perform external fetching of the scripts to
			execute. The downloader has yet to be determined... working on it

			:)


		*/
		// if tf.Spec.PreInitScript != "" {
		// 	runOpts.mainModuleAddonData[string(tfv1alpha1.PodPreInit)] = tf.Spec.PreInitScript
		// }

		// if tf.Spec.PostInitScript != "" {
		// 	runOpts.mainModuleAddonData[string(tfv1alpha1.PodPostInit)] = tf.Spec.PostInitScript
		// }

		// if tf.Spec.PrePlanScript != "" {
		// 	runOpts.mainModuleAddonData[string(tfv1alpha1.PodPrePlan)] = tf.Spec.PrePlanScript
		// }

		// if tf.Spec.PostPlanScript != "" {
		// 	runOpts.mainModuleAddonData[string(tfv1alpha1.PodPostPlan)] = tf.Spec.PostPlanScript
		// }

		// if tf.Spec.PreApplyScript != "" {
		// 	runOpts.mainModuleAddonData[string(tfv1alpha1.PodPreApply)] = tf.Spec.PreApplyScript
		// }

		// if tf.Spec.PostApplyScript != "" {
		// 	runOpts.mainModuleAddonData[string(tfv1alpha1.PodPostApply)] = tf.Spec.PostApplyScript
		// }

		// if tf.Spec.PreInitDeleteScript != "" {
		// 	runOpts.mainModuleAddonData[string(tfv1alpha1.PodPreInitDelete)] = tf.Spec.PreInitDeleteScript
		// }

		// if tf.Spec.PostInitDeleteScript != "" {
		// 	runOpts.mainModuleAddonData[string(tfv1alpha1.PodPostInitDelete)] = tf.Spec.PostInitDeleteScript
		// }

		// if tf.Spec.PrePlanDeleteScript != "" {
		// 	runOpts.mainModuleAddonData[string(tfv1alpha1.PodPrePlanDelete)] = tf.Spec.PrePlanDeleteScript
		// }

		// if tf.Spec.PostPlanDeleteScript != "" {
		// 	runOpts.mainModuleAddonData[string(tfv1alpha1.PodPostPlanDelete)] = tf.Spec.PostPlanDeleteScript
		// }

		// if tf.Spec.PreApplyDeleteScript != "" {
		// 	runOpts.mainModuleAddonData[string(tfv1alpha1.PodPreApplyDelete)] = tf.Spec.PostApplyScript
		// }

		// if tf.Spec.PostApplyDeleteScript != "" {
		// 	runOpts.mainModuleAddonData[string(tfv1alpha1.PodPostApplyDelete)] = tf.Spec.PostApplyDeleteScript
		// }
	}

	runOpts.resourceLabels["terraforms.tf.isaaguilar.com/generation"] = fmt.Sprintf("%d", generation)
	runOpts.resourceLabels["terraforms.tf.isaaguilar.com/resourceName"] = runOpts.resourceName
	runOpts.resourceLabels["terraforms.tf.isaaguilar.com/podPrefix"] = runOpts.prefixedName
	runOpts.resourceLabels["terraforms.tf.isaaguilar.com/terraformVersion"] = runOpts.terraformVersion
	runOpts.resourceLabels["app.kubernetes.io/name"] = "terraform-operator"
	runOpts.resourceLabels["app.kubernetes.io/component"] = "terraform-operator-runner"
	runOpts.resourceLabels["app.kubernetes.io/created-by"] = "controller"

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

func (r ReconcileTerraform) createPVC(ctx context.Context, tf *tfv1alpha2.Terraform, runOpts RunOptions) error {
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
	resource := runOpts.generatePVC(persistentVolumeSize)
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

func (r ReconcileTerraform) createConfigMap(ctx context.Context, tf *tfv1alpha2.Terraform, runOpts RunOptions) error {
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

func (r ReconcileTerraform) createSecret(ctx context.Context, tf *tfv1alpha2.Terraform, name, namespace string, data map[string][]byte, recreate bool, labelsToOmit []string, runOpts RunOptions) error {
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

func (r ReconcileTerraform) createServiceAccount(ctx context.Context, tf *tfv1alpha2.Terraform, runOpts RunOptions) error {
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

func (r ReconcileTerraform) createRole(ctx context.Context, tf *tfv1alpha2.Terraform, runOpts RunOptions) error {
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

func (r ReconcileTerraform) createRoleBinding(ctx context.Context, tf *tfv1alpha2.Terraform, runOpts RunOptions) error {
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

func (r ReconcileTerraform) createPod(ctx context.Context, tf *tfv1alpha2.Terraform, runOpts RunOptions) error {
	kind := "Pod"

	resource := runOpts.generatePod()
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
			Labels:    r.resourceLabels,
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
			Labels:      r.resourceLabels,
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

func (r RunOptions) generateRoleBinding() *rbacv1.RoleBinding {
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

func (r RunOptions) generatePVC(size resource.Quantity) *corev1.PersistentVolumeClaim {
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
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
		},
	}
}

// generatePod puts together all the contents required to execute the taskType.
// Although most of the tasks use similar.... (TODO EDIT ME)
func (r RunOptions) generatePod() *corev1.Pod {

	home := "/home/tfo-runner"
	generateName := r.versionedName + "-" + r.runType.String() + "-"
	generationPath := fmt.Sprintf("%s/generations/%d", home, r.generation)

	runnerLabels := r.labels
	annotations := r.annotations
	envFrom := r.envFrom
	envs := r.env
	envs = append(envs, []corev1.EnvVar{
		{
			/*

				What is the significance of having an env about the TFO_RUNNER?

				Only used to idenify the taskType for the log.out file. This
				should simply be the taskType name.

			*/
			Name:  "TFO_TASK",
			Value: r.runType.String(),
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
			Name:  "TFO_RESOURCE",
			Value: r.resourceName,
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
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "tfohome",
			MountPath: home,
			ReadOnly:  false,
		},
	}
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

	for _, c := range r.credentials {
		if c.AWSCredentials.KIAM != "" {
			annotations["iam.amazonaws.com/role"] = c.AWSCredentials.KIAM
		}
	}

	for _, c := range r.credentials {
		if (tfv1alpha2.SecretNameRef{}) != c.SecretNameRef {
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
	runnerLabels["app.kubernetes.io/instance"] = r.runType.String()

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
	restartPolicy := corev1.RestartPolicyNever

	containers := []corev1.Container{}
	containers = append(containers, corev1.Container{
		Name:            "task",
		SecurityContext: securityContext,
		Image:           r.image,
		ImagePullPolicy: r.imagePullPolicy,
		EnvFrom:         envFrom,
		Env:             envs,
		VolumeMounts:    volumeMounts,
	})

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
			SecurityContext:    &podSecurityContext,
			ServiceAccountName: r.serviceAccount,
			RestartPolicy:      restartPolicy,
			Containers:         containers,
			Volumes:            volumes,
		},
	}

	return pod
}

func (r ReconcileTerraform) run(ctx context.Context, reqLogger logr.Logger, tf *tfv1alpha2.Terraform, runOpts RunOptions, isNewGeneration, isFirstInstall bool) (err error) {

	if isFirstInstall || isNewGeneration {
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
			labelsToOmit = append(labelsToOmit, "terraforms.tf.isaaguilar.com/generation")
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

func (r ReconcileTerraform) createGitAskpass(ctx context.Context, tokenSecret tfv1alpha2.TokenSecretRef) ([]byte, error) {
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

func (r RunOptions) generateSecret(name, namespace string, data map[string][]byte, labels map[string]string) *corev1.Secret {
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
