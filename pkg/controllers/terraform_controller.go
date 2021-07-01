package controllers

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/elliotchance/sshtunnel"
	"github.com/go-logr/logr"
	getter "github.com/hashicorp/go-getter"
	goSocks5 "github.com/isaaguilar/socks5-proxy"
	tfv1alpha1 "github.com/isaaguilar/terraform-operator/pkg/apis/tf/v1alpha1"
	"github.com/isaaguilar/terraform-operator/pkg/gitclient"
	"github.com/isaaguilar/terraform-operator/pkg/utils"
	giturl "github.com/whilp/git-urls"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
	gitTransportClient "gopkg.in/src-d/go-git.v4/plumbing/transport/client"
	githttp "gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"

	// "k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// SetupWithManager sets up the controller with the Manager.
func (r *ReconcileTerraform) SetupWithManager(mgr ctrl.Manager) error {
	// err := ctrl.NewControllerManagedBy(mgr).
	// 	For(&tfv1alpha1.Terraform{}).
	// 	Complete(r)
	// if err != nil {
	// 	return err
	// }

	// err = ctrl.NewControllerManagedBy(mgr).
	// 	For(&batchv1.Job{}).
	// 	Watches(&source.Kind{Type: &batchv1.Job{}}, &handler.EnqueueRequestForOwner{
	// 		IsController: true,
	// 		OwnerType:    &tfv1alpha1.Terraform{},
	// 	}).
	// 	Complete(r)
	// if err != nil {
	// 	return err
	// }
	var err error
	err = ctrl.NewControllerManagedBy(mgr).
		For(&tfv1alpha1.Terraform{}).
		Watches(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &tfv1alpha1.Terraform{},
		}).
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
	Client   client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Log      logr.Logger
}

type ParsedAddress struct {
	sourcedir string
	subdirs   []string
	hash      string
	protocol  string
	uri       string
	host      string
	port      string
	user      string
	repo      string
}

type GitRepoAccessOptions struct {
	Client         gitclient.GitRepo
	Address        string
	Directory      string
	Extras         []string
	SCMAuthMethods []tfv1alpha1.SCMAuthMethod
	SSHProxy       tfv1alpha1.ProxyOpts
	tunnel         *sshtunnel.SSHTunnel
	ParsedAddress
}

type RunOptions struct {
	moduleConfigMaps          []string
	namespace                 string
	name                      string
	envVars                   []corev1.EnvVar
	credentials               []tfv1alpha1.Credentials
	stack                     ParsedAddress
	serviceAccount            string
	configMapData             map[string]string
	secretData                map[string][]byte
	terraformRunner           string
	terraformRunnerPullPolicy corev1.PullPolicy
	terraformVersion          string
	scriptRunner              string
	scriptRunnerPullPolicy    corev1.PullPolicy
	scriptRunnerVersion       string
	setupRunner               string
	setupRunnerPullPolicy     corev1.PullPolicy
	setupRunnerVersion        string
}

func newRunOptions(tf *tfv1alpha1.Terraform) RunOptions {
	// TODO Read the tfstate and decide IF_NEW_RESOURCE based on that
	// applyAction := false
	name := tf.Status.PodNamePrefix
	terraformRunner := "isaaguilar/tf-runner-alphav1"
	terraformRunnerPullPolicy := corev1.PullIfNotPresent
	terraformVersion := "0.13.7"

	scriptRunner := "isaaguilar/script-runner-alphav1"
	scriptRunnerPullPolicy := corev1.PullIfNotPresent
	scriptRunnerVersion := "1.0.0"

	setupRunner := "isaaguilar/setup-runner-alphav1"
	setupRunnerPullPolicy := corev1.PullIfNotPresent
	setupRunnerVersion := "1.0.0"

	// sshConfig := utils.TruncateResourceName(tf.Name, 242) + "-ssh-config"
	serviceAccount := tf.Spec.ServiceAccount
	if serviceAccount == "" {
		// By prefixing the service account with "tf-", IRSA roles can use wildcard
		// "tf-*" service account for AWS credentials.
		serviceAccount = "tf-" + name
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

	return RunOptions{
		namespace:                 tf.Namespace,
		name:                      name,
		envVars:                   tf.Spec.Env,
		credentials:               credentials,
		terraformVersion:          terraformVersion,
		terraformRunner:           terraformRunner,
		terraformRunnerPullPolicy: terraformRunnerPullPolicy,
		serviceAccount:            serviceAccount,
		configMapData:             make(map[string]string),
		secretData:                make(map[string][]byte),
		scriptRunner:              scriptRunner,
		scriptRunnerPullPolicy:    scriptRunnerPullPolicy,
		scriptRunnerVersion:       scriptRunnerVersion,
		setupRunner:               setupRunner,
		setupRunnerPullPolicy:     setupRunnerPullPolicy,
		setupRunnerVersion:        setupRunnerVersion,
	}
}

func (r *RunOptions) updateDownloadedModules(module string) {
	r.moduleConfigMaps = append(r.moduleConfigMaps, module)
}

func (r *RunOptions) updateEnvVars(v corev1.EnvVar) {
	r.envVars = append(r.envVars, v)
}

const terraformFinalizer = "finalizer.tf.isaaguilar.com"

var logf = ctrl.Log.WithName("terraform_controller")

// Reconcile reads that state of the cluster for a Terraform object and makes changes based on the state read
// and what is in the Terraform.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileTerraform) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.Log.WithValues("Terraform", request.NamespacedName)
	reqLogger.V(2).Info("Reconciling Terraform")

	// Check for the resource's existance
	tf := &tfv1alpha1.Terraform{}
	err := r.Client.Get(ctx, request.NamespacedName, tf)
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
		podType := tfv1alpha1.PodInit
		stageState := tfv1alpha1.StateInitializing
		interruptible := tfv1alpha1.CanNotBeInterrupt
		addNewStage(tf, podType, "TF_RESOURCE_CREATED", interruptible, stageState)
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

	// Check the status on stages that have not completed
	for _, stage := range tf.Status.Stages {
		if stage.State == tfv1alpha1.StateInProgress {
			// TODO
		}
	}

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

	if podType == "" {
		if tf.Status.Phase == tfv1alpha1.PhaseRunning {
			tf.Status.Phase = tfv1alpha1.PhaseCompleted
			err := r.updateStatus(ctx, tf)
			if err != nil {
				reqLogger.V(1).Info(err.Error())
				return reconcile.Result{}, err
			}
		} else if tf.Status.Phase == tfv1alpha1.PhaseDeleting {
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
		"metadata.generateName": fmt.Sprintf("%s-%s-", tf.Status.PodNamePrefix, podType),
	}
	labels := map[string]string{
		"tfGeneration": fmt.Sprintf("%d", generation),
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
		err := r.setupAndRun(ctx, tf)
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
		err := r.Client.Delete(ctx, &pods.Items[0])
		if err != nil {
			reqLogger.V(1).Info(err.Error())
		}
		return reconcile.Result{}, nil
	}

	// TODO should tf operator "auto" reconciliate (eg plan+apply)?
	// TODO how should we handle manually triggering apply
	return reconcile.Result{}, nil
}

// func stageCheck(tf *tfv1alpha1.Terraform) {
// 	n := len(tf.Status.Stages)
// 	if tf.Status.Stages[n-1].State == tfv1alpha1.StateComplete {
// 		// Get the next stage and set to initialize and return after appending
// 		// the new stage
// 	}
// }

func addNewStage(tf *tfv1alpha1.Terraform, podType tfv1alpha1.PodType, reason string, interruptible tfv1alpha1.Interruptible, stageState tfv1alpha1.StageState) {
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

	deletePhases := []string{
		string(tfv1alpha1.PhaseDeleted),
		string(tfv1alpha1.PhaseInitDelete),
		string(tfv1alpha1.PhaseDeleted),
	}
	tfIsFinalizing := utils.ListContainsStr(deletePhases, string(tf.Status.Phase))
	tfIsNotFinalizing := !tfIsFinalizing

	deletePodTypes := []string{
		string(tfv1alpha1.PodPreInitDelete),
		string(tfv1alpha1.PodInitDelete),
		string(tfv1alpha1.PodPostInitDelete),
	}
	initDelete := tf.Status.Phase == tfv1alpha1.PhaseInitDelete

	var podType tfv1alpha1.PodType
	var reason string
	stageState := tfv1alpha1.StateInitializing
	interruptible := tfv1alpha1.CanBeInterrupt
	// current stage
	n := len(tf.Status.Stages)
	if n == 0 {
		// There should always be at least 1 stage, this shouldn't happen
	}
	currentStage := tf.Status.Stages[n-1]
	currentStagePodType := currentStage.PodType
	currentStageCanNotBeInterrupted := currentStage.Interruptible == tfv1alpha1.CanNotBeInterrupt
	currentStageIsRunning := currentStage.State == tfv1alpha1.StateInProgress

	// resource status
	if currentStageCanNotBeInterrupted && currentStageIsRunning {
		// Cannot change to the next stage becuase the current stage cannot be
		// interrupted and is currently running
		isNewStage = false
	} else if currentStage.Generation != tf.Generation && tfIsNotFinalizing {
		// The current generation has changed and this is the first pod in the
		// normal terraform workflow
		isNewStage = true
		reason = "GENERATION_CHANGE"
		podType = tfv1alpha1.PodInit

	} else if initDelete && !utils.ListContainsStr(deletePodTypes, string(currentStagePodType)) {
		// The tf resource is marked for deletion and this is the first pod
		// in the terraform destroy workflow.
		isNewStage = true
		reason = "TF_RESOURCE_DELETED"
		podType = tfv1alpha1.PodInitDelete
		interruptible = tfv1alpha1.CanNotBeInterrupt

	} else if currentStage.State == tfv1alpha1.StateComplete {
		isNewStage = true
		reason = ""

		switch currentStagePodType {
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
// The finalizer will be responsible for kicking off the destroy workflow.
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

func (d GitRepoAccessOptions) TunnelClose(reqLogger logr.Logger) {
	reqLogger.V(1).Info("Closing tunnel")
	if d.tunnel != nil {
		d.tunnel.Close()
		reqLogger.V(1).Info("Closed tunnel")
		return
	}
	reqLogger.V(1).Info("TunnelClose called but could not find tunnel to close")
	return
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

func (r *ReconcileTerraform) setupAndRun(ctx context.Context, tf *tfv1alpha1.Terraform) error {
	reqLogger := r.Log.WithValues("Terraform", types.NamespacedName{Name: tf.Name, Namespace: tf.Namespace}.String())
	n := len(tf.Status.Stages)
	isNewGeneration := tf.Status.Stages[n-1].Reason == "GENERATION_CHANGE"
	isFirstInstall := tf.Status.Stages[n-1].Reason == "TF_RESOURCE_CREATED"
	isChanged := isNewGeneration || isFirstInstall
	// r.Recorder.Event(tf, "Normal", "InitializeJobCreate", fmt.Sprintf("Setting up a Job"))
	// TODO(user): Add the cleanup steps that the operator
	// needs to do before the CR can be deleted. Examples
	// of finalizers include performing backups and deleting
	// resources that are not owned by this CR, like a PVC.

	runOpts := newRunOptions(tf)
	// runOpts.updateEnvVars(corev1.EnvVar{
	// 	Name:  "DEPLOYMENT",
	// 	Value: instance.Name,
	// })
	// runOpts.namespace = instance.Namespace

	// Stack Download
	address := tf.Spec.TerraformModule.Address
	stackRepoAccessOptions, err := newGitRepoAccessOptionsFromSpec(tf, address, []string{})
	if err != nil {
		r.Recorder.Event(tf, "Warning", "ProcessingError", fmt.Errorf("Error in newGitRepoAccessOptionsFromSpec: %v", err).Error())
		return fmt.Errorf("Error in newGitRepoAccessOptionsFromSpec: %v", err)
	}

	err = stackRepoAccessOptions.getParsedAddress()
	if err != nil {
		r.Recorder.Event(tf, "Warning", "ProcessingError", fmt.Errorf("Error in parsing address: %v", err).Error())
		return fmt.Errorf("Error in parsing address: %v", err)
	}

	// Since we're not going to download this to a configmap, we need to
	// pass the information to the pod to do it. We should be able to
	// use stackRepoAccessOptions.parsedAddress and just send that to
	// the pod's environment vars.

	runOpts.updateDownloadedModules(stackRepoAccessOptions.hash)
	runOpts.stack = stackRepoAccessOptions.ParsedAddress
	if runOpts.stack.repo == "" {
		return fmt.Errorf("Error is parsing terraformModule")
	}

	// TODO Update secrets only when the generation changes
	if isChanged {
		for _, m := range stackRepoAccessOptions.SCMAuthMethods {
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
					return fmt.Errorf("Error setting up sshconfig: %v", err)
				}
				for k, v := range sshConfigData {
					runOpts.secretData[k] = v
				}

			}
			break
		}
	}

	//
	// TODO
	//		The following section can have downloads from other sources. This
	//		can take a few seconds to a few mintues to complete and will block
	// 		the entire queue until it is completed.
	//
	//		Implement a way to run this in the background and signal that the
	//		configmap is ready for usage which will then trigger the controller
	//		to go ahead and configure the pods.
	//
	tfvars := ""
	if isChanged {
		// ConfigMap Data only needs to be updated when generation changes

		otherConfigFiles := make(map[string]string)
		for _, s := range tf.Spec.Sources {
			address := strings.TrimSpace(s.Address)
			extras := s.Extras
			// Loop thru all the sources in spec.config
			configRepoAccessOptions, err := newGitRepoAccessOptionsFromSpec(tf, address, extras)
			if err != nil {
				r.Recorder.Event(tf, "Warning", "ConfigError", fmt.Errorf("Error in Spec: %v", err).Error())
				return fmt.Errorf("Error in newGitRepoAccessOptionsFromSpec: %v", err)
			}

			err = configRepoAccessOptions.getParsedAddress()
			if err != nil {
				return err
			}
			reqLogger.V(1).Info("Setting up download options for config repo access")

			if (tfv1alpha1.ProxyOpts{}) != configRepoAccessOptions.SSHProxy {
				if strings.Contains(configRepoAccessOptions.protocol, "http") {
					err := configRepoAccessOptions.startHTTPSProxy(ctx, r.Client, tf.Namespace, reqLogger)
					if err != nil {
						reqLogger.Error(err, "failed to start ssh proxy")
						return err
					}
				} else if configRepoAccessOptions.protocol == "ssh" {
					err := configRepoAccessOptions.startSSHProxy(ctx, r.Client, tf.Namespace, reqLogger)
					if err != nil {
						reqLogger.Error(err, "failed to start ssh proxy")
						return err
					}
					defer configRepoAccessOptions.TunnelClose(reqLogger.WithValues("Spec", "source"))
				}
			}

			err = configRepoAccessOptions.download(ctx, r.Client, tf.Namespace)
			if err != nil {
				r.Recorder.Event(tf, "Warning", "DownloadError", fmt.Errorf("Error in download: %v", err).Error())
				return fmt.Errorf("Error in download: %v", err)
			}

			reqLogger.V(1).Info(fmt.Sprintf("Config was downloaded and updated GitRepoAccessOptions: %+v", configRepoAccessOptions))

			tfvarSource, err := configRepoAccessOptions.tfvarFiles()
			if err != nil {
				r.Recorder.Event(tf, "Warning", "ReadFileError", fmt.Errorf("Error reading tfvar files: %v", err).Error())
				return fmt.Errorf("Error in reading tfvarFiles: %v", err)
			}
			tfvars += tfvarSource

			otherConfigFiles, err = configRepoAccessOptions.otherConfigFiles()
			if err != nil {
				r.Recorder.Event(tf, "Warning", "ReadFileError", fmt.Errorf("Error reading files: %v", err).Error())
				return fmt.Errorf("Error in reading otherConfigFiles: %v", err)
			}
		}

		runOpts.configMapData["tfvars"] = tfvars
		for k, v := range otherConfigFiles {
			runOpts.configMapData[k] = v
		}

		// Override the backend.tf by inserting a custom backend
		if tf.Spec.CustomBackend != "" {
			runOpts.configMapData["backend_override.tf"] = tf.Spec.CustomBackend
		}

		if tf.Spec.PreInitScript != "" {
			runOpts.configMapData[string(tfv1alpha1.PodPreInit)] = tf.Spec.PreInitScript
		}

		if tf.Spec.PostInitScript != "" {
			runOpts.configMapData[string(tfv1alpha1.PodPostInit)] = tf.Spec.PostInitScript
		}

		if tf.Spec.PrePlanScript != "" {
			runOpts.configMapData[string(tfv1alpha1.PodPrePlan)] = tf.Spec.PrePlanScript
		}

		if tf.Spec.PostPlanScript != "" {
			runOpts.configMapData[string(tfv1alpha1.PodPostPlan)] = tf.Spec.PostPlanScript
		}

		if tf.Spec.PreApplyScript != "" {
			runOpts.configMapData[string(tfv1alpha1.PodPreApply)] = tf.Spec.PreApplyScript
		}

		if tf.Spec.PostApplyScript != "" {
			runOpts.configMapData[string(tfv1alpha1.PodPostApply)] = tf.Spec.PostApplyScript
		}

		if tf.Spec.PreInitDeleteScript != "" {
			runOpts.configMapData[string(tfv1alpha1.PodPreInitDelete)] = tf.Spec.PreInitDeleteScript
		}

		if tf.Spec.PostInitDeleteScript != "" {
			runOpts.configMapData[string(tfv1alpha1.PodPostInitDelete)] = tf.Spec.PostInitDeleteScript
		}

		if tf.Spec.PrePlanDeleteScript != "" {
			runOpts.configMapData[string(tfv1alpha1.PodPrePlanDelete)] = tf.Spec.PrePlanDeleteScript
		}

		if tf.Spec.PostPlanDeleteScript != "" {
			runOpts.configMapData[string(tfv1alpha1.PodPostPlanDelete)] = tf.Spec.PostPlanDeleteScript
		}

		if tf.Spec.PreApplyDeleteScript != "" {
			runOpts.configMapData[string(tfv1alpha1.PodPreApplyDelete)] = tf.Spec.PostApplyScript
		}

		if tf.Spec.PostApplyDeleteScript != "" {
			runOpts.configMapData[string(tfv1alpha1.PodPostApplyDelete)] = tf.Spec.PostApplyDeleteScript
		}
	}

	// Flatten all the .tfvars and TF_VAR envs into a single file and push
	if tf.Spec.ExportRepo != nil && isChanged {
		e := tf.Spec.ExportRepo

		address := e.Address
		exportRepoAccessOptions, err := newGitRepoAccessOptionsFromSpec(tf, address, []string{})
		if err != nil {
			r.Recorder.Event(tf, "Warning", "ConfigError", fmt.Errorf("Error getting git repo access options: %v", err).Error())
			return fmt.Errorf("Error in newGitRepoAccessOptionsFromSpec: %v", err)
		}
		err = exportRepoAccessOptions.getParsedAddress()
		if err != nil {
			return fmt.Errorf("Error parsing export repo address %s", err)
		}

		// TODO decide what to do on errors
		// Closing the tunnel from within this function
		go exportRepoAccessOptions.commitTfvars(ctx, r.Client, tfvars, e.TFVarsFile, e.ConfFile, tf.Namespace, tf.Spec.CustomBackend, runOpts, reqLogger)
	}

	// RUN
	err = r.run(ctx, reqLogger, tf, runOpts)
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
	err := r.deleteConfigMapIfExists(ctx, runOpts.name, runOpts.namespace)
	if err != nil {
		return err
	}

	resource := runOpts.generateConfigMap()
	controllerutil.SetControllerReference(tf, resource, r.Scheme)

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

func (r ReconcileTerraform) createSecret(ctx context.Context, tf *tfv1alpha1.Terraform, runOpts RunOptions) error {
	kind := "Secret"
	err := r.deleteSecretIfExists(ctx, runOpts.name, runOpts.namespace)
	if err != nil {
		return err
	}

	resource := runOpts.generateSecret()
	controllerutil.SetControllerReference(tf, resource, r.Scheme)

	err = r.Client.Create(ctx, resource)
	if err != nil {
		r.Recorder.Event(tf, "Warning", fmt.Sprintf("%sCreateError", kind), fmt.Sprintf("Could not create %s %v", kind, err))
		return err
	}
	r.Recorder.Event(tf, "Normal", "SuccessfulCreate", fmt.Sprintf("Created %s: '%s'", kind, resource.Name))
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
	err := r.deleteServiceAccountIfExists(ctx, runOpts.serviceAccount, runOpts.namespace)
	if err != nil {
		return err
	}

	resource := runOpts.generateServiceAccount()
	controllerutil.SetControllerReference(tf, resource, r.Scheme)

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
	err := r.deleteRoleIfExists(ctx, runOpts.name, runOpts.namespace)
	if err != nil {
		return err
	}

	resource := runOpts.generateRole()
	controllerutil.SetControllerReference(tf, resource, r.Scheme)

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
	err := r.deleteRoleBindingIfExists(ctx, runOpts.name, runOpts.namespace)
	if err != nil {
		return err
	}

	resource := runOpts.generateRoleBinding()
	controllerutil.SetControllerReference(tf, resource, r.Scheme)

	err = r.Client.Create(ctx, resource)
	if err != nil {
		r.Recorder.Event(tf, "Warning", fmt.Sprintf("%sCreateError", kind), fmt.Sprintf("Could not create %s %v", kind, err))
		return err
	}
	r.Recorder.Event(tf, "Normal", "SuccessfulCreate", fmt.Sprintf("Created %s: '%s'", kind, resource.Name))
	return nil
}

func (r ReconcileTerraform) createPod(ctx context.Context, tf *tfv1alpha1.Terraform, runOpts RunOptions) error {
	kind := "Pod"

	n := len(tf.Status.Stages)
	podType := tf.Status.Stages[n-1].PodType
	generation := tf.Status.Stages[n-1].Generation

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
			Name:      r.name,
			Namespace: r.namespace,
		},
		Data: r.configMapData,
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
			Name:        r.serviceAccount,
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
	if r.configMapData["backend_override.tf"] != "" {
		// parse the backennd string the way most people write it
		// example:
		// terraform {
		//   backend "kubernetes" {
		//     ...
		//   }
		// }
		s := strings.Split(r.configMapData["backend_override.tf"], "\n")
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

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.name,
			Namespace: r.namespace,
		},
		Rules: rules,
	}
	return role
}

func (r RunOptions) generateRoleBinding() *rbacv1.RoleBinding {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.name,
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
			Name:     r.name,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
	return rb
}

// func (r RunOptions) generateJob(tfvarsConfigMap *corev1.ConfigMap) *batchv1.Job {
// 	// reqLogger := log.WithValues("function", "run")
// 	// reqLogger.Info(fmt.Sprintf("Running job with this setup: %+v", r))

// 	// TF Module
// 	if r.mainModule == "" {
// 		r.mainModule = "main_module"
// 	}
// 	envs := r.envVars
// 	envs = append(envs, []corev1.EnvVar{
// 		{
// 			Name:  "TFOPS_MAIN_MODULE",
// 			Value: r.mainModule,
// 		},
// 		{
// 			Name:  "NAMESPACE",
// 			Value: r.namespace,
// 		},
// 	}...)
// 	tfModules := []corev1.Volume{}
// 	// Check if stack is in a subdir

// 	if r.stack.repo == "" {
// 		// TODO This is an error and should not be allowed
// 		//		Find out where this is not checked and error out
// 		//		and let the user know the repo is missing (maybe check repo
// 		// 		validitiy?)
// 	}
// 	if r.stack.repo != "" {
// 		envs = append(envs, []corev1.EnvVar{
// 			{
// 				Name:  "STACK_REPO",
// 				Value: r.stack.repo,
// 			},
// 			{
// 				Name:  "STACK_REPO_HASH",
// 				Value: r.stack.hash,
// 			},
// 		}...)
// 		if r.tokenSecret != nil {
// 			if r.tokenSecret.Name != "" {
// 				envs = append(envs, []corev1.EnvVar{
// 					{
// 						Name: "GIT_PASSWORD",
// 						ValueFrom: &corev1.EnvVarSource{
// 							SecretKeyRef: &corev1.SecretKeySelector{
// 								LocalObjectReference: corev1.LocalObjectReference{
// 									Name: r.tokenSecret.Name,
// 								},
// 								Key: r.tokenSecret.Key,
// 							},
// 						},
// 					},
// 				}...)
// 			}
// 		}

// 		// r.tokenSecret.Name
// 		// if r.token != "" {

// 		// }
// 		if len(r.stack.subdirs) > 0 {
// 			envs = append(envs, []corev1.EnvVar{
// 				{
// 					Name:  "STACK_REPO_SUBDIR",
// 					Value: r.stack.subdirs[0],
// 				},
// 			}...)
// 		}
// 	} else {
// 		for i, v := range r.moduleConfigMaps {
// 			tfModules = append(tfModules, []corev1.Volume{
// 				{
// 					Name: v,
// 					VolumeSource: corev1.VolumeSource{
// 						ConfigMap: &corev1.ConfigMapVolumeSource{
// 							LocalObjectReference: corev1.LocalObjectReference{
// 								Name: v,
// 							},
// 						},
// 					},
// 				},
// 			}...)

// 			envs = append(envs, []corev1.EnvVar{
// 				{
// 					Name:  "TFOPS_MODULE" + strconv.Itoa(i),
// 					Value: v,
// 				},
// 			}...)
// 		}
// 	}

// 	// Check if is new resource
// 	if r.isNewResource {
// 		envs = append(envs, []corev1.EnvVar{
// 			{
// 				Name:  "IS_NEW_RESOURCE",
// 				Value: "true",
// 			},
// 		}...)
// 	}

// 	// This resource is used to create a volumeMount which have 63 char limits.
// 	// Truncate the instance.Name enough to fit "-tfvars" wich will be the
// 	// configmapName and volumeMount name.
// 	tfvarsConfigMapVolumeName := utils.TruncateResourceName(r.name, 56) + "-tfvars"
// 	tfVars := []corev1.Volume{}
// 	tfVars = append(tfVars, []corev1.Volume{
// 		{
// 			Name: tfvarsConfigMapVolumeName,
// 			VolumeSource: corev1.VolumeSource{
// 				ConfigMap: &corev1.ConfigMapVolumeSource{
// 					LocalObjectReference: corev1.LocalObjectReference{
// 						Name: tfvarsConfigMap.Name,
// 					},
// 				},
// 			},
// 		},
// 	}...)

// 	envs = append(envs, []corev1.EnvVar{
// 		{
// 			Name:  "TFOPS_VARFILE_FLAG",
// 			Value: "-var-file /tfops/" + tfvarsConfigMapVolumeName + "/tfvars",
// 		},
// 		{
// 			Name:  "TFOPS_CONFIGMAP_PATH",
// 			Value: "/tfops/" + tfvarsConfigMapVolumeName,
// 		},
// 	}...)

// 	volumes := append(tfModules, tfVars...)

// 	volumeMounts := []corev1.VolumeMount{}
// 	for _, v := range volumes {
// 		// setting up volumeMounts
// 		volumeMounts = append(volumeMounts, []corev1.VolumeMount{
// 			{
// 				Name:      v.Name,
// 				MountPath: "/tfops/" + v.Name,
// 			},
// 		}...)
// 	}

// 	if r.sshConfig != "" {
// 		mode := int32(0600)
// 		volumes = append(volumes, []corev1.Volume{
// 			{
// 				Name: "ssh-key",
// 				VolumeSource: corev1.VolumeSource{
// 					Secret: &corev1.SecretVolumeSource{
// 						SecretName:  r.sshConfig,
// 						DefaultMode: &mode,
// 					},
// 				},
// 			},
// 		}...)
// 		volumeMounts = append(volumeMounts, []corev1.VolumeMount{
// 			{
// 				Name:      "ssh-key",
// 				MountPath: "/root/.ssh/",
// 			},
// 		}...)
// 	}

// 	annotations := make(map[string]string)
// 	envFrom := []corev1.EnvFromSource{}

// 	for _, c := range r.credentials {
// 		if c.AWSCredentials.KIAM != "" {
// 			annotations["iam.amazonaws.com/role"] = c.AWSCredentials.KIAM
// 		}
// 	}

// 	for _, c := range r.credentials {
// 		if (tfv1alpha1.SecretNameRef{}) != c.SecretNameRef {
// 			envFrom = append(envFrom, []corev1.EnvFromSource{
// 				{
// 					SecretRef: &corev1.SecretEnvSource{
// 						LocalObjectReference: corev1.LocalObjectReference{
// 							Name: c.SecretNameRef.Name,
// 						},
// 					},
// 				},
// 			}...)
// 		}
// 	}

// 	// Create a manual selector for jobs (lets job-names be more than 63 chars)
// 	manualSelector := true

// 	// Custom UUID for label purposes
// 	uid := uuid.NewUUID()

// 	// The job-name label must be <64 chars
// 	labels := make(map[string]string)
// 	labels["tf-job"] = string(uid)
// 	labels["job-name"] = r.jobNameLabel

// 	// Schedule a job that will execute the terraform plan
// 	job := &batchv1.Job{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      r.name,
// 			Namespace: r.namespace,
// 		},
// 		Spec: batchv1.JobSpec{
// 			ManualSelector: &manualSelector,
// 			Selector: &metav1.LabelSelector{
// 				MatchLabels: labels,
// 			},
// 			Template: corev1.PodTemplateSpec{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Annotations: annotations,
// 					Labels:      labels,
// 				},
// 				Spec: corev1.PodSpec{
// 					ServiceAccountName: r.serviceAccount,
// 					RestartPolicy:      "OnFailure",
// 					Containers: []corev1.Container{
// 						{
// 							Name: "tf",
// 							// TODO Version docker images more specifically than static versions
// 							Image:           r.terraformRunner + ":" + r.terraformVersion,
// 							ImagePullPolicy: r.terraformRunnerPullPolicy,
// 							EnvFrom:         envFrom,
// 							Env: append(envs, []corev1.EnvVar{
// 								{
// 									Name:  "INSTANCE_NAME",
// 									Value: r.name,
// 								},
// 							}...),
// 							VolumeMounts: volumeMounts,
// 						},
// 					},
// 					Volumes: volumes,
// 				},
// 			},
// 		},
// 	}

// 	return job
// }

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
	pod := &corev1.Pod{}
	id := r.name
	generateName := id + "-" + string(podType) + "-"

	envs := r.envVars
	envs = append(envs, []corev1.EnvVar{
		{
			Name:  "TFO_RUNNER",
			Value: id,
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
			Name:  "TFO_MAIN_MODULE",
			Value: "/home/tfo-runner/main",
		},
	}...)

	volumes := []corev1.Volume{
		{
			Name: "tfrun",
			VolumeSource: corev1.VolumeSource{
				//
				// TODO add an option to the tf to use host or pvc
				// 		for the plan.
				//
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: id,
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
			Name:      "tfrun",
			MountPath: "/home/tfo-runner",
		},
	}
	envs = append(envs, corev1.EnvVar{
		Name:  "TFO_ROOT_PATH",
		Value: "/home/tfo-runner",
	})

	// Check if stack is in a subdir

	// if r.stack.repo == "" {
	// 	// TODO This is an error and should not be allowed
	// 	//		Find out where this is not checked and error out
	// 	//		and let the user know the repo is missing (maybe check repo
	// 	// 		validitiy?)
	// }

	ref := r.stack.hash
	if ref == "" {
		ref = "master"
	}
	envs = append(envs, []corev1.EnvVar{
		{
			Name:  "TFO_MAIN_MODULE_REPO",
			Value: r.stack.repo,
		},
		{
			Name:  "TFO_MAIN_MODULE_REPO_REF",
			Value: ref,
		},
	}...)

	// r.tokenSecret.Name
	// if r.token != "" {

	// }
	if len(r.stack.subdirs) > 0 {
		value := r.stack.subdirs[0]
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

	downloadsVol := "downloads"
	downloadsPath := "/tmp/downloads"
	volumes = append(volumes, []corev1.Volume{
		{
			Name: downloadsVol,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: id,
					},
				},
			},
		},
	}...)
	volumeMounts = append(volumeMounts, []corev1.VolumeMount{
		{
			Name:      downloadsVol,
			MountPath: downloadsPath,
		},
	}...)
	envs = append(envs, []corev1.EnvVar{
		{
			Name:  "TFO_DOWNLOADS",
			Value: downloadsPath,
		},
	}...)

	optional := true
	xmode := int32(0775)
	volumes = append(volumes, corev1.Volume{
		Name: "gitaskpass",
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: id,
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
					SecretName:  id,
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

	annotations := make(map[string]string)
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
	labels["tfGeneration"] = fmt.Sprintf("%d", generation)

	// Schedule a job that will execute the terraform plan
	initContainers := []corev1.Container{}
	containers := []corev1.Container{}

	// Make sure to use the same uid for containers so the dir in the
	// PersistentVolume have the correct permissions for the user
	user := int64(1000)
	securityContext := &corev1.SecurityContext{
		RunAsUser: &user,
	}

	if isTFRunner {
		envs = append(envs, corev1.EnvVar{
			Name:  "TFO_RUNNER",
			Value: string(podType),
		})
		containers = append(containers, corev1.Container{
			SecurityContext: securityContext,
			Name:            "tf",
			Image:           r.terraformRunner + ":" + r.terraformVersion,
			ImagePullPolicy: r.terraformRunnerPullPolicy,
			EnvFrom:         envFrom,
			Env:             envs,
			VolumeMounts:    volumeMounts,
		})

		if podType == tfv1alpha1.PodInit {
			// setup once per generation

			initContainers = append(initContainers, corev1.Container{
				Name:            "tfo-init",
				SecurityContext: securityContext,
				Image:           r.setupRunner + ":" + r.setupRunnerVersion,
				ImagePullPolicy: r.setupRunnerPullPolicy,
				EnvFrom:         envFrom,
				Env:             envs,
				VolumeMounts:    volumeMounts,
			})
		}

		if preScriptPodType != "" {
			envs = append(envs, corev1.EnvVar{
				Name:  "TFO_SCRIPT",
				Value: string(preScriptPodType),
			})
			initContainers = append(initContainers, corev1.Container{
				SecurityContext: securityContext,
				Name:            "pre-script",
				Image:           r.scriptRunner + ":" + r.scriptRunnerVersion,
				ImagePullPolicy: r.scriptRunnerPullPolicy,
				EnvFrom:         envFrom,
				Env:             envs,
				VolumeMounts:    volumeMounts,
			})
		}

	} else {
		envs = append(envs, corev1.EnvVar{
			Name:  "TFO_SCRIPT",
			Value: string(podType),
		})
		containers = append(containers, corev1.Container{
			SecurityContext: securityContext,
			Name:            "script",
			Image:           r.scriptRunner + ":" + r.scriptRunnerVersion,
			ImagePullPolicy: r.scriptRunnerPullPolicy,
			EnvFrom:         envFrom,
			Env:             envs,
			VolumeMounts:    volumeMounts,
		})
	}

	pod = &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: generateName,
			Namespace:    r.namespace,
			Labels:       labels,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: r.serviceAccount,
			RestartPolicy:      corev1.RestartPolicyNever,
			InitContainers:     initContainers,
			Containers:         containers,
			Volumes:            volumes,
		},
	}

	return pod
}

// getOrCreateEnv will check if an env exists. If it does not, it will be
// created with the value
func (r *RunOptions) getOrCreateEnv(name, value string) {
	for _, i := range r.envVars {
		if i.Name == name {
			return
		}
	}
	r.envVars = append(r.envVars, corev1.EnvVar{
		Name:  name,
		Value: value,
	})
}

func (r ReconcileTerraform) run(ctx context.Context, reqLogger logr.Logger, tf *tfv1alpha1.Terraform, runOpts RunOptions) (err error) {

	n := len(tf.Status.Stages)
	isNewGeneration := tf.Status.Stages[n-1].Reason == "GENERATION_CHANGE"
	isFirstInstall := tf.Status.Stages[n-1].Reason == "TF_RESOURCE_CREATED"

	if isFirstInstall || isNewGeneration {
		if isFirstInstall {
			if err := r.createPVC(ctx, tf, runOpts); err != nil {
				return err
			}
		}
		if err := r.createSecret(ctx, tf, runOpts); err != nil {
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

		if _, found, err := r.checkConfigMapExists(ctx, lookupKey); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("could not find ConfigMap '%s'", lookupKey)
		}

		if _, found, err := r.checkSecretExists(ctx, lookupKey); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("could not find Secret '%s'", lookupKey)
		}

		if _, found, err := r.checkRoleBindingExists(ctx, lookupKey); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("could not find RoleBinding '%s'", lookupKey)
		}

		if _, found, err := r.checkRoleExists(ctx, lookupKey); err != nil {
			return err
		} else if !found {
			return fmt.Errorf("could not find Role '%s'", lookupKey)
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

	// generateName := id + "-" + string(podType) + "-"
	// pod := runOpts.generatePod(podType, preScriptPodType, isTFRunner, id, generateName, generation)
	// controllerutil.SetControllerReference(tf, pod, r.Scheme)

	//
	// TODO recreate resources for new inits podTypes
	//
	// createResources := false
	// if podType == tfv1alpha1.PodInit || podType == tfv1alpha1.PodInitDelete {
	// 	createResources = true
	// }
	// _ = createResources

	// if serviceAccount != nil {
	// 	controllerutil.SetControllerReference(tf, serviceAccount, r.Scheme)

	// 	err = r.Client.Create(ctx, serviceAccount)
	// 	if err != nil && errors.IsNotFound(err) {
	// 		return "", err
	// 	} else if err != nil {
	// 		reqLogger.Info(err.Error())
	// 	}
	// }

	// err = r.Client.Create(ctx, role)
	// if err != nil && errors.IsNotFound(err) {
	// 	return "", err
	// } else if err != nil {
	// 	reqLogger.Info(err.Error())
	// }

	// err = r.Client.Create(ctx, roleBinding)
	// if err != nil && errors.IsNotFound(err) {
	// 	return "", err
	// } else if err != nil {
	// 	reqLogger.Info(err.Error())
	// }

	// err = r.Client.Create(ctx, secret)
	// if err != nil && errors.IsNotFound(err) {
	// 	return "", err
	// } else if err != nil {
	// 	reqLogger.Info(fmt.Sprintf("Secret %s already exists", secret.Name))
	// 	updateErr := r.Client.Update(ctx, secret)
	// 	if updateErr != nil && errors.IsNotFound(updateErr) {
	// 		return "", updateErr
	// 	} else if updateErr != nil {
	// 		reqLogger.Info(err.Error())
	// 	}
	// }

	// err = r.Client.Create(ctx, job)
	// if err != nil && errors.IsNotFound(err) {
	// 	return "", err
	// } else if err != nil {
	// 	reqLogger.Info(err.Error())
	// }
	// return job.Name, nil

	// err = r.Client.Create(ctx, pod)
	// if err != nil && errors.IsNotFound(err) {
	// 	return "", err
	// } else if err != nil {
	// 	reqLogger.Info(err.Error())
	// }

	return nil
}

func (r ReconcileTerraform) createGitAskpass(ctx context.Context, tokenSecret tfv1alpha1.TokenSecretRef) ([]byte, error) {
	gitAskpass := []byte{}
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
	gitAskpass = []byte(s)
	return gitAskpass, nil

}

func (r ReconcileTerraform) loadSecret(ctx context.Context, name, namespace string) (*corev1.Secret, error) {
	if namespace == "" {
		namespace = "default"
	}
	lookupKey := types.NamespacedName{Name: name, Namespace: namespace}
	fmt.Println(lookupKey)
	secret := &corev1.Secret{}
	err := r.Client.Get(ctx, lookupKey, secret)
	if err != nil {
		return secret, err
	}
	return secret, nil
}

func newGitRepoAccessOptionsFromSpec(instance *tfv1alpha1.Terraform, address string, extras []string) (GitRepoAccessOptions, error) {
	d := GitRepoAccessOptions{}
	var sshProxyOptions tfv1alpha1.ProxyOpts

	// var tfAuthOptions []tfv1alpha1.AuthOpts

	// TODO allow configmaps as a source. This has to be parsed differently
	// before being passed to terraform's parsing mechanism

	temp, err := ioutil.TempDir("", "repo")
	if err != nil {
		return d, fmt.Errorf("Unable to make directory: %v", err)
	}
	// defer os.RemoveAll(temp) // clean up

	d = GitRepoAccessOptions{
		Address:   address,
		Extras:    extras,
		Directory: temp,
	}
	d.SCMAuthMethods = instance.Spec.SCMAuthMethods

	if instance.Spec.SSHTunnel != nil {
		sshProxyOptions = *instance.Spec.SSHTunnel
	}
	d.SSHProxy = sshProxyOptions

	return d, nil
}

func getHostKey(host string) (ssh.PublicKey, error) {
	file, err := os.Open(filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts"))
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var hostKey ssh.PublicKey
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), " ")
		if len(fields) != 3 {
			continue
		}
		if strings.Contains(fields[0], host) {
			var err error
			hostKey, _, _, _, err = ssh.ParseAuthorizedKey(scanner.Bytes())
			if err != nil {
				return nil, fmt.Errorf("error parsing %q: %v", fields[2], err)
			}
			break
		}
	}

	if hostKey == nil {
		return nil, fmt.Errorf("no hostkey for %s", host)
	}
	return hostKey, nil
}

func (d GitRepoAccessOptions) tfvarFiles() (string, error) {
	// dump contents of tfvar files into a var
	tfvars := ""

	// TODO Should path definitions walk the path?
	if utils.ListContainsStr(d.Extras, "is-file") {
		for _, filename := range d.subdirs {
			if !strings.HasSuffix(filename, ".tfvars") {
				continue
			}
			file := filepath.Join(d.Directory, filename)
			content, err := ioutil.ReadFile(file)
			if err != nil {
				return "", fmt.Errorf("error reading file: %v", err)
			}
			tfvars += string(content) + "\n"
		}
	} else if len(d.subdirs) > 0 {
		for _, s := range d.subdirs {
			subdir := filepath.Join(d.Directory, s)
			lsdir, err := ioutil.ReadDir(subdir)
			if err != nil {
				return "", fmt.Errorf("error listing dir: %v", err)
			}

			for _, f := range lsdir {
				if strings.Contains(f.Name(), ".tfvars") {
					file := filepath.Join(subdir, f.Name())

					content, err := ioutil.ReadFile(file)
					if err != nil {
						return "", fmt.Errorf("error reading file: %v", err)
					}

					tfvars += string(content) + "\n"
				}
			}
		}
	} else {
		lsdir, err := ioutil.ReadDir(d.Directory)
		if err != nil {
			return "", fmt.Errorf("error listing dir: %v", err)
		}

		for _, f := range lsdir {
			if strings.Contains(f.Name(), ".tfvars") {
				file := filepath.Join(d.Directory, f.Name())

				content, err := ioutil.ReadFile(file)
				if err != nil {
					return "", fmt.Errorf("error reading file: %v", err)
				}

				tfvars += string(content) + "\n"
			}
		}
	}
	// TODO validate tfvars
	return tfvars, nil
}

// TODO combine this with the tfvars and make it a generic  get configs method
func (d GitRepoAccessOptions) otherConfigFiles() (map[string]string, error) {
	// create a configmap entry per source file
	configFiles := make(map[string]string)

	// TODO Should path definitions walk the path?
	if utils.ListContainsStr(d.Extras, "is-file") {
		for _, filename := range d.subdirs {
			file := filepath.Join(d.Directory, filename)
			content, err := ioutil.ReadFile(file)
			if err != nil {
				return configFiles, fmt.Errorf("error reading file: %v", err)
			}
			configFiles[filepath.Base(filename)] = string(content)
		}
	} else if len(d.subdirs) > 0 {
		for _, s := range d.subdirs {
			subdir := filepath.Join(d.Directory, s)
			lsdir, err := ioutil.ReadDir(subdir)
			if err != nil {
				return configFiles, fmt.Errorf("error listing dir: %v", err)
			}

			for _, f := range lsdir {

				file := filepath.Join(subdir, f.Name())

				content, err := ioutil.ReadFile(file)
				if err != nil {
					return configFiles, fmt.Errorf("error reading file: %v", err)
				}

				configFiles[f.Name()] = string(content)

			}
		}
	} else {
		lsdir, err := ioutil.ReadDir(d.Directory)
		if err != nil {
			return configFiles, fmt.Errorf("error listing dir: %v", err)
		}

		for _, f := range lsdir {

			file := filepath.Join(d.Directory, f.Name())

			content, err := ioutil.ReadFile(file)
			if err != nil {
				return configFiles, fmt.Errorf("error reading file: %v", err)
			}

			configFiles[f.Name()] = string(content)

		}
	}
	// TODO validate tfvars
	return configFiles, nil
}

// downloadFromSource will downlaod the files locally. It will also download
// tf modules locally if the user opts to. TF module downloading
// is probably going to be used in the event that go-getter cannot fetch the
// modules, perhaps becuase of a firewall. Check for proxy settings to send
// to the download command.
func downloadFromSource(src, moduleDir string) error {

	// Check for global proxy

	ds := getter.Detectors
	output, err := getter.Detect(src, moduleDir, ds)
	if err != nil {
		return fmt.Errorf("Could not Detect source: %v", err)
	}

	if strings.HasPrefix(output, "git::") {
		// send to gitSource
		return fmt.Errorf("There isn't an error, reading output as %v", output)
	} else if strings.HasPrefix(output, "https://") {
		return fmt.Errorf("downloadFromSource does not yet support http(s)")
	} else if strings.HasPrefix(output, "file://") {
		return fmt.Errorf("downloadFromSource does not yet support file")
	} else if strings.HasPrefix(output, "s3::") {
		return fmt.Errorf("downloadFromSource does not yet support s3")
	}

	// TODO If the total size of the stacks configmap is too large, it will have
	// to uploaded else where.

	return nil
}

func configureGitSSHString(user, host, port, uri string) string {
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	return fmt.Sprintf("ssh://%s@%s:%s%s", user, host, port, uri)
}

func tarBinaryData(fullpath, filename string) (map[string][]byte, error) {
	binaryData := make(map[string][]byte)
	// Archive the file and send to configmap
	// First remove the .git file if exists in Path
	gitFile := filepath.Join(fullpath, ".git")
	_, err := os.Stat(gitFile)
	if err == nil {
		if err = os.RemoveAll(gitFile); err != nil {
			return binaryData, fmt.Errorf("Could not find or remove .git: %v", err)
		}
	}

	tardir, err := ioutil.TempDir("", "tarball")
	if err != nil {
		return binaryData, fmt.Errorf("unable making tardir: %v", err)
	}
	defer os.RemoveAll(tardir) // clean up

	tarTarget := filepath.Join(tardir, "tarball")
	tarSource := filepath.Join(tardir, filename)

	err = os.Mkdir(tarTarget, 0755)
	if err != nil {
		return binaryData, fmt.Errorf("Could not create tarTarget: %v", err)
	}
	err = os.Mkdir(tarSource, 0755)
	if err != nil {
		return binaryData, fmt.Errorf("Could not create tarTarget: %v", err)
	}

	// expect result of untar to be same as filename. Copy src to a
	// "filename" dir instead of it's current dir
	// targetSrc := filepath.Join(target, fmt.Sprintf("%s", filename))
	err = utils.CopyDirectory(fullpath, tarSource)
	if err != nil {
		return binaryData, err
	}

	err = tarit("repo", tarSource, tarTarget)
	if err != nil {
		return binaryData, fmt.Errorf("error archiving '%s': %v", tarSource, err)
	}
	// files := make(map[string][]byte)
	tarballs, err := ioutil.ReadDir(tarTarget)
	if err != nil {
		return binaryData, fmt.Errorf("error listing tardir: %v", err)
	}
	for _, f := range tarballs {
		content, err := ioutil.ReadFile(filepath.Join(tarTarget, f.Name()))
		if err != nil {
			return binaryData, fmt.Errorf("error reading tarball: %v", err)
		}

		binaryData[f.Name()] = content
	}

	return binaryData, nil
}

// func readConfigMap(k8sclient client.Client, name, namespace string) (*corev1.ConfigMap, error) {
// 	configMap := &corev1.ConfigMap{}
// 	namespacedName := types.NamespacedName{Namespace: namespace, Name: name}
// 	err := k8sclient.Get(ctx, namespacedName, configMap)
// 	// configMap, err := c.clientset.CoreV1().ConfigMaps(namespace).Get(name, metav1.GetOptions{})
// 	if err != nil {
// 		return &corev1.ConfigMap{}, fmt.Errorf("error reading configmap: %v", err)
// 	}

// 	return configMap, nil
// }

// func (c *k8sClient) createConfigMap(name, namespace string, binaryData map[string][]byte, data map[string]string) error {

// 	configMapObject := &corev1.ConfigMap{
// 		ObjectMeta: metav1.ObjectMeta{
// 			Name:      name,
// 			Namespace: namespace,
// 		},
// 		Data:       data,
// 		BinaryData: binaryData,
// 	}

// 	// TODO Make the terraform the referenced Owner of this resource
// 	_, err := c.clientset.CoreV1().ConfigMaps(namespace).Create(configMapObject)
// 	if err != nil {
// 		// fmt.Printf("The first create error... %v\n", err.Error())
// 		_, err = c.clientset.CoreV1().ConfigMaps(namespace).Update(configMapObject)
// 		if err != nil {
// 			return fmt.Errorf("error creating configmap: %v", err)
// 		}
// 	}

// 	return nil
// }

func (r RunOptions) generateSecret() *corev1.Secret {
	secretObject := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.name,
			Namespace: r.namespace,
		},
		Data: r.secretData,
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
		return "", fmt.Errorf("Could not get secret: %v", err)
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

func loadPrivateKey(ctx context.Context, k8sclient client.Client, key, name, namespace string) (*os.File, error) {

	secret := &corev1.Secret{}
	namespacedName := types.NamespacedName{Namespace: namespace, Name: name}
	err := k8sclient.Get(ctx, namespacedName, secret)
	if err != nil {
		return nil, fmt.Errorf("Could not get id_rsa secret: %v", err)
	}

	var privateKey []byte
	for k, value := range secret.Data {
		if k == key {
			privateKey = value
		}
	}

	if len(privateKey) == 0 {
		return nil, fmt.Errorf("unable to locate '%s' in secret: %v", key, err)
	}

	content := []byte(privateKey)
	tmpfile, err := ioutil.TempFile("", "id_rsa")
	if err != nil {
		return nil, fmt.Errorf("error creating tmpfile: %v", err)
	}

	if _, err := tmpfile.Write(content); err != nil {
		return nil, fmt.Errorf("unable to write tempfile: %v", err)
	}

	var mode os.FileMode
	mode = 0600
	os.Chmod(tmpfile.Name(), mode)

	return tmpfile, nil
}

func unique(s []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range s {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func tarit(filename, source, target string) error {
	target = filepath.Join(target, fmt.Sprintf("%s.tar", filename))
	tarfile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer tarfile.Close()

	tarball := tar.NewWriter(tarfile)
	defer tarball.Close()

	info, err := os.Stat(source)
	if err != nil {
		return nil
	}

	var baseDir string
	if info.IsDir() {
		baseDir = filepath.Base(source)
	}

	return filepath.Walk(source,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			header, err := tar.FileInfoHeader(info, info.Name())
			if err != nil {
				return err
			}

			if baseDir != "" {
				header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source))
			}

			if err := tarball.WriteHeader(header); err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(tarball, file)
			return err
		})
}

func untar(tarball, target string) error {
	reader, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer reader.Close()
	tarReader := tar.NewReader(reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		path := filepath.Join(target, header.Name)
		info := header.FileInfo()
		if info.IsDir() {
			if err = os.MkdirAll(path, info.Mode()); err != nil {
				return err
			}
			continue
		}

		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(file, tarReader)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *GitRepoAccessOptions) download(ctx context.Context, k8sclient client.Client, namespace string) error {
	// This function only supports git modules. There's no explicit check
	// for this yet.
	// TODO document available options for sources
	reqLogger := logf.WithValues("Download", d.Address, "Namespace", namespace, "Function", "download")
	reqLogger.V(1).Info(fmt.Sprintf("Getting ready to download source %s", d.repo))

	var gitRepo gitclient.GitRepo
	if d.protocol == "ssh" {
		filename, err := d.getGitSSHKey(ctx, k8sclient, namespace, d.protocol, reqLogger)
		if err != nil {
			return fmt.Errorf("Download failed for '%s': %v", d.repo, err)
		}
		defer os.Remove(filename)
		gitRepo, err = gitclient.GitSSHDownload(d.repo, d.Directory, filename, d.hash, reqLogger)
		if err != nil {
			return fmt.Errorf("Download failed for '%s': %v", d.repo, err)
		}
	} else {
		// TODO find out and support any other protocols
		// Just assume http is the only other protocol for now
		token, err := d.getGitToken(ctx, k8sclient, namespace, d.protocol, reqLogger)
		if err != nil {
			// Maybe we don't need to exit if no creds are used here
			reqLogger.Info(fmt.Sprintf("%v", err))
		}

		gitRepo, err = gitclient.GitHTTPDownload(d.repo, d.Directory, "git", token, d.hash)
		if err != nil {
			return fmt.Errorf("Download failed for '%s': %v", d.repo, err)
		}
	}

	// Set the hash and return
	var err error
	d.Client = gitRepo
	d.hash, err = gitRepo.HashString()
	if err != nil {
		return err
	}
	reqLogger.Info(fmt.Sprintf("Hash: %v", d.hash))
	return nil
}

func (d *GitRepoAccessOptions) startHTTPSProxy(ctx context.Context, k8sclient client.Client, namespace string, reqLogger logr.Logger) error {
	proxyAuthMethod, err := d.getProxyAuthMethod(ctx, k8sclient, namespace)
	if err != nil {
		return fmt.Errorf("Error getting proxyAuthMethod: %v", err)
	}

	reqLogger.V(1).Info("Setting up http proxy")
	proxyServer := ""
	if strings.Contains(d.host, ":") {
		proxyServer = d.SSHProxy.Host
	} else {
		// fmt.Sprintf("%s:22", d.SSHProxy.Host)
	}

	hostKey := goSocks5.NewHostKey()
	duration := time.Duration(60 * time.Second)
	socks5Proxy := goSocks5.NewSocks5Proxy(hostKey, nil, duration)

	err = socks5Proxy.Start(d.SSHProxy.User, proxyServer, proxyAuthMethod)
	if err != nil {
		return fmt.Errorf("unable to start socks5: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	socks5Addr, err := socks5Proxy.Addr()
	if err != nil {
		return fmt.Errorf("unable to get socks5Addr: %v", err)
	}

	dialer, err := proxy.SOCKS5("tcp", socks5Addr, nil, proxy.Direct)
	if err != nil {
		return fmt.Errorf("unable to get dialer: %v", err)
	}

	httpTransport := &http.Transport{Dial: dialer.Dial}
	// set our socks5 as the dialer
	// httpTransport.Dial = dialer.Dial
	httpClient := &http.Client{Transport: httpTransport}

	gitTransportClient.InstallProtocol("http", githttp.NewClient(httpClient))
	gitTransportClient.InstallProtocol("https", githttp.NewClient(httpClient))
	return nil
}

func (d *GitRepoAccessOptions) startSSHProxy(ctx context.Context, k8sclient client.Client, namespace string, reqLogger logr.Logger) error {
	uri := d.uri

	reqLogger.V(1).Info(fmt.Sprintf("Setting up ssh proxy for %s with job: %+v", namespace, d))
	port, tunnel, err := d.setupSSHProxy(ctx, k8sclient, namespace)
	if err != nil {
		return err
	}

	if os.Getenv("DEBUG_SSHTUNNEL") != "" {
		tunnel.Log = log.New(os.Stdout, "", log.Ldate|log.Lmicroseconds)
	}
	d.tunnel = tunnel

	if strings.Index(uri, "/") != 0 {
		uri = "/" + uri
	}
	reqLogger.V(1).Info("SSH proxy is ready for usage")
	// configure auth with go git options
	d.repo = fmt.Sprintf("ssh://%s@127.0.0.1:%s%s", d.user, port, uri)
	return nil

}

func (d *GitRepoAccessOptions) setupSSHProxy(ctx context.Context, k8sclient client.Client, namespace string) (string, *sshtunnel.SSHTunnel, error) {
	var port string
	var tunnel *sshtunnel.SSHTunnel
	proxyAuthMethod, err := d.getProxyAuthMethod(ctx, k8sclient, namespace)
	if err != nil {
		return port, tunnel, fmt.Errorf("Error getting proxyAuthMethod: %v", err)
	}
	proxyServerWithUser := fmt.Sprintf("%s@%s", d.SSHProxy.User, d.SSHProxy.Host)
	destination := ""
	if strings.Contains(d.host, ":") {
		destination = d.host
	} else {
		destination = fmt.Sprintf("%s:%s", d.host, d.port)
	}

	// Setup the tunnel, but do not yet start it yet.
	// // User and host of tunnel server, it will default to port 22
	// // if not specified.
	// proxyServerWithUser,

	// // Pick ONE of the following authentication methods:
	// // sshtunnel.PrivateKeyFile(filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa")), // 1. private key
	// proxyAuthMethod,

	// // The destination host and port of the actual server.
	// destination,
	tunnel = sshtunnel.NewSSHTunnel(proxyServerWithUser, proxyAuthMethod, destination, "0")

	// NewSSHTunnel will bind to a random port so that you can have
	// multiple SSH tunnels available. The port is available through:
	//   tunnel.Local.Port

	// You can use any normal Go code to connect to the destination server
	// through localhost. You may need to use 127.0.0.1 for some libraries.

	// You can provide a logger for debugging, or remove this line to
	// make it silent.
	// tunnel.Log = log.New(os.Stdout, "", log.Ldate|log.Lmicroseconds)
	// reqLogger.Info(tunnel.Log)

	// Start the server in the background. You will need to wait a
	// small amount of time for it to bind to the localhost port
	// before you can start sending connections.
	go tunnel.Start()
	time.Sleep(1000 * time.Millisecond)
	port = strconv.Itoa(tunnel.Local.Port)

	return port, tunnel, nil
}

func (d *GitRepoAccessOptions) getParsedAddress() error {
	sourcedir, subdirstr := getter.SourceDirSubdir(d.Address)
	// subdir can contain a list seperated by double slashes
	subdirs := strings.Split(subdirstr, "//")
	src := strings.TrimPrefix(sourcedir, "git::")
	var hash string
	if strings.Contains(sourcedir, "?") {
		for i, v := range strings.Split(sourcedir, "?") {
			if i > 0 {
				if strings.Contains(v, "&") {
					for _, w := range strings.Split(v, "&") {
						if strings.Contains(w, "ref=") {
							hash = strings.Split(w, "ref=")[1]
						}
					}

				} else if strings.Contains(v, "ref=") {
					hash = strings.Split(v, "ref=")[1]
				}
			}

		}
	}

	// strip out the url args
	repo := strings.Split(src, "?")[0]
	u, err := giturl.Parse(repo)
	if err != nil {
		return fmt.Errorf("unable to parse giturl: %v", err)
	}
	protocol := u.Scheme
	uri := strings.Split(u.RequestURI(), "?")[0]
	host := u.Host
	port := u.Port()
	if port == "" {
		if protocol == "ssh" {
			port = "22"
		} else if protocol == "https" {
			port = "443"
		}
	}

	user := u.User.Username()
	if user == "" {
		user = "git"
	}

	d.ParsedAddress = ParsedAddress{
		sourcedir: sourcedir,
		subdirs:   subdirs,
		hash:      hash,
		protocol:  protocol,
		uri:       uri,
		host:      host,
		port:      port,
		user:      user,
		repo:      repo,
	}
	return nil
}

func (d GitRepoAccessOptions) getProxyAuthMethod(ctx context.Context, k8sclient client.Client, namespace string) (ssh.AuthMethod, error) {
	var proxyAuthMethod ssh.AuthMethod

	name := d.SSHProxy.SSHKeySecretRef.Name
	key := d.SSHProxy.SSHKeySecretRef.Key
	if key == "" {
		key = "id_rsa"
	}
	ns := d.SSHProxy.SSHKeySecretRef.Namespace
	if ns == "" {
		ns = namespace
	}

	sshKey, err := loadPrivateKey(ctx, k8sclient, key, name, ns)
	if err != nil {
		return proxyAuthMethod, fmt.Errorf("unable to get privkey: %v", err)
	}
	defer os.Remove(sshKey.Name())
	defer sshKey.Close()
	proxyAuthMethod = sshtunnel.PrivateKeyFile(sshKey.Name())

	return proxyAuthMethod, nil
}

func (d *GitRepoAccessOptions) getGitSSHKey(ctx context.Context, k8sclient client.Client, namespace, protocol string, reqLogger logr.Logger) (string, error) {
	var filename string
	for _, m := range d.SCMAuthMethods {
		if m.Host == d.ParsedAddress.host && m.Git.SSH != nil {
			reqLogger.Info("Using Git over SSH with a key")
			name := m.Git.SSH.SSHKeySecretRef.Name
			key := m.Git.SSH.SSHKeySecretRef.Key
			if key == "" {
				key = "id_rsa"
			}
			ns := m.Git.SSH.SSHKeySecretRef.Namespace
			if ns == "" {
				ns = namespace
			}
			sshKey, err := loadPrivateKey(ctx, k8sclient, key, name, ns)
			if err != nil {
				return filename, err
			}
			defer sshKey.Close()
			filename = sshKey.Name()
		}
	}
	if filename == "" {
		return filename, fmt.Errorf("Failed to find Git SSH Key for %v\n", d.ParsedAddress.host)
	}
	return filename, nil
}

func (d *GitRepoAccessOptions) getGitToken(ctx context.Context, k8sclient client.Client, namespace, protocol string, reqLogger logr.Logger) (string, error) {
	var token string
	var err error
	for _, m := range d.SCMAuthMethods {
		if m.Host == d.ParsedAddress.host && m.Git.HTTPS != nil {
			reqLogger.Info("Using Git over HTTPS with a token")
			name := m.Git.HTTPS.TokenSecretRef.Name
			key := m.Git.HTTPS.TokenSecretRef.Key
			if key == "" {
				key = "token"
			}
			ns := m.Git.HTTPS.TokenSecretRef.Namespace
			if ns == "" {
				ns = namespace
			}
			token, err = loadPassword(ctx, k8sclient, key, name, ns)
			if err != nil {
				return token, fmt.Errorf("unable to get token: %v", err)
			}
		}
	}
	if token == "" {
		return token, fmt.Errorf("Failed to find Git token Key for %v\n", d.ParsedAddress.host)
	}
	return token, nil
}

func (d GitRepoAccessOptions) commitTfvars(ctx context.Context, k8sclient client.Client, tfvars, tfvarsFile, confFile, namespace, customBackend string, runOpts RunOptions, reqLogger logr.Logger) {
	filesToCommit := []string{}

	reqLogger.V(1).Info("Setting up download options for export")
	if (tfv1alpha1.ProxyOpts{}) != d.SSHProxy {
		if strings.Contains(d.protocol, "http") {
			err := d.startHTTPSProxy(ctx, k8sclient, namespace, reqLogger)
			if err != nil {
				reqLogger.Error(err, "failed to start ssh proxy")
				return
			}
		} else if d.protocol == "ssh" {
			err := d.startSSHProxy(ctx, k8sclient, namespace, reqLogger)
			if err != nil {
				reqLogger.Error(err, "failed to start ssh proxy")
				return
			}
			defer d.TunnelClose(reqLogger.WithValues("Spec", "exportRepo"))
		}
	}
	err := d.download(ctx, k8sclient, namespace)
	if err != nil {
		errString := fmt.Sprintf("Could not download repo %v", err)
		reqLogger.Info(errString)
		return
	}

	// Create a file in the external repo
	err = d.Client.CheckoutBranch("")
	if err != nil {
		errString := fmt.Sprintf("Could not check out new branch %v", err)
		reqLogger.Info(errString)
		return
	}

	// Format TFVars File
	// First read in the tfvar file that gets created earlier. This tfvar
	// file should have already concatenated all the tfvars found
	// from the git repos
	tfvarsFileContent := tfvars
	for _, i := range runOpts.envVars {
		k := i.Name
		v := i.Value
		// TODO Attempt to resolve other kinds of TF_VAR_ values via
		// 		an EnvVarSource other than `Value`
		if v == "" {
			continue
		}
		if !strings.Contains(k, "TF_VAR") {
			continue
		}
		k = strings.ReplaceAll(k, "TF_VAR_", "")
		if string(v[0]) != "{" && string(v[0]) != "[" {
			v = fmt.Sprintf("\"%s\"", v)
		}
		tfvarsFileContent = tfvarsFileContent + fmt.Sprintf("\n%s = %s", k, v)
	}

	// Remove Duplicates
	// TODO replace this code with a more terraform native method of merging tfvars
	var c bytes.Buffer
	var currentKey string
	var currentValue string
	keyIndexer := make(map[string]string)
	var openBrackets int
	for _, line := range strings.Split(tfvarsFileContent, "\n") {
		lineArr := strings.Split(line, "=")
		// ignore blank lines
		if strings.TrimSpace(lineArr[0]) == "" {
			continue
		}

		if openBrackets > 0 {
			currentValue += "\n" + strings.ReplaceAll(line, "\t", "  ")
			// Check for more open brackets and close brackets
			trimmedLine := strings.TrimSpace(line)
			lastCharIdx := len(trimmedLine) - 1
			lastChar := string(trimmedLine[lastCharIdx])
			lastTwoChar := ""
			if lastCharIdx > 0 {
				lastTwoChar = string(trimmedLine[lastCharIdx-1:])
			}

			if lastChar == "{" || lastChar == "[" {
				openBrackets++
			} else if lastChar == "}" || lastChar == "]" || lastTwoChar == "}," || lastTwoChar == "]," {
				openBrackets--
			}
			if openBrackets == 0 {
				keyIndexer[currentKey] = currentValue
			}
			continue
		}
		currentKey = strings.TrimSpace(lineArr[0])

		if len(lineArr) > 1 {
			lastLineArrIdx := len(lineArr) - 1
			trimmedLine := lineArr[lastLineArrIdx]
			lastCharIdx := len(trimmedLine) - 1
			lastChar := string(trimmedLine[lastCharIdx])
			if lastChar == "{" || lastChar == "[" {
				openBrackets++
			}
		} else {
			errString := fmt.Sprintf("Error in parsing tfvars string: %s", line)
			reqLogger.Info(errString)
			return
		}

		currentValue = line
		if openBrackets > 0 {
			continue
		}
		keyIndexer[currentKey] = currentValue
	}

	keys := make([]string, 0, len(keyIndexer))
	for k := range keyIndexer {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&c, "%s\n\n", keyIndexer[k])

	}

	// Write HCL to file
	// Create the path if not exists
	err = os.MkdirAll(filepath.Dir(filepath.Join(d.Directory, tfvarsFile)), 0755)
	if err != nil {
		errString := fmt.Sprintf("Could not create path: %v", err)
		reqLogger.Info(errString)
		return
	}
	err = ioutil.WriteFile(filepath.Join(d.Directory, tfvarsFile), c.Bytes(), 0644)
	if err != nil {
		errString := fmt.Sprintf("Could not write file %v", err)
		reqLogger.Info(errString)
		return
	}

	// Write to file

	filesToCommit = append(filesToCommit, tfvarsFile)

	// Format Conf File
	if confFile != "" {
		confFileContent := ""
		// The backend-configs for tf-operator are actually written
		// as a complete tf resource. We need to extract only the key
		// and values from the conf file only.
		if customBackend != "" {

			configsOnly := strings.Split(customBackend, "\n")
			for _, line := range configsOnly {
				// Assuming that config lines contain an equal sign
				// All other lines are discarded
				if strings.Contains(line, "=") {
					if confFileContent == "" {
						confFileContent = strings.TrimSpace(line)
					} else {
						confFileContent = confFileContent + "\n" + strings.TrimSpace(line)
					}
				}
			}
		}

		// Write to file
		err = os.MkdirAll(filepath.Dir(filepath.Join(d.Directory, confFile)), 0755)
		if err != nil {
			errString := fmt.Sprintf("Could not create path: %v", err)
			reqLogger.Info(errString)
			return
		}
		err = ioutil.WriteFile(filepath.Join(d.Directory, confFile), []byte(confFileContent), 0644)
		if err != nil {
			errString := fmt.Sprintf("Could not write file %v", err)
			reqLogger.Info(errString)
			return
		}
		filesToCommit = append(filesToCommit, confFile)
	}

	// Commit and push to repo
	commitMsg := fmt.Sprintf("automatic update via terraform-operator\nupdates to:\n%s", strings.Join(filesToCommit, "\n"))
	err = d.Client.Commit(filesToCommit, commitMsg)
	if err != nil {
		errString := fmt.Sprintf("Could not commit to repo %v", err)
		reqLogger.V(1).Info(errString)
		return
	}
	err = d.Client.Push("refs/heads/master")
	if err != nil {
		errString := fmt.Sprintf("Could not push to repo %v", err)
		reqLogger.V(1).Info(errString)
		return
	}

}
