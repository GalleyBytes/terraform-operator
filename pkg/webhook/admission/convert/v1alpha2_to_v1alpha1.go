package convert

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"time"

	tfv1alpha1 "github.com/isaaguilar/terraform-operator/pkg/apis/tf/v1alpha1"
	tfv1alpha2 "github.com/isaaguilar/terraform-operator/pkg/apis/tf/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func ConvertV1alpha2ToV1alpha1(rawRequest []byte) ([]byte, runtime.Object, error) {
	start := time.Now()
	want := tfv1alpha1.Terraform{}
	have := tfv1alpha2.Terraform{}

	err := json.Unmarshal(rawRequest, &have)
	if err != nil {
		// The request to convert cannot be converted and will continue to fail because what is already
		// stored in kubernetes has been malformed or corrupted.

		// Perhaps the rawRequest accidentally has the wrong version stored. Can the request be
		// unmarshalled as the desired response?
		err := json.Unmarshal(rawRequest, &want)
		if err != nil {
			return []byte{}, &want, err
		}
		want.TypeMeta = metav1.TypeMeta{
			Kind:       "Terraform",
			APIVersion: tfv1alpha1.SchemeGroupVersion.String(),
		}
		rawResponse, _ := json.Marshal(want)
		return rawResponse, &want, nil

	}

	log.Printf("Should convert %s/%s from %s to %s\n", have.Namespace, have.Name, tfv1alpha2.SchemeGroupVersion, tfv1alpha1.SchemeGroupVersion)

	if v1alpha1TerraformJSON, found := have.Annotations["tf.isaaguilar.com/v1alpha1_terraforms"]; found {
		err := json.Unmarshal([]byte(v1alpha1TerraformJSON), &want)
		if err != nil {
			want.TypeMeta = metav1.TypeMeta{
				Kind:       "Terraform",
				APIVersion: tfv1alpha1.SchemeGroupVersion.String(),
			}
			rawResponse, _ := json.Marshal(want)
			return rawResponse, &want, nil
		}
	} else {
		// The annotation in v1alpha2 was not found and must now be manually generated. Generally
		// when this annotation is missing it means that the resource never existed as v1alpha1 and therefore
		// will not require backporting, but just in case, the following is the best attempt to translate
		// the apis.
		want.TypeMeta = metav1.TypeMeta{
			Kind:       "Terraform",
			APIVersion: tfv1alpha1.SchemeGroupVersion.String(),
		}
		want.ObjectMeta = have.ObjectMeta
		want.Spec.TerraformVersion = have.Spec.TerraformVersion
		want.Spec.TerraformModule = have.Spec.TerraformModule.Source
		want.Spec.TerraformModuleInline = have.Spec.TerraformModule.Inline
		want.Spec.TerraformModuleConfigMap = (*tfv1alpha1.ConfigMapSelector)(have.Spec.TerraformModule.ConfigMapSelector)
		want.Spec.CustomBackend = have.Spec.Backend
		want.Spec.IgnoreDelete = have.Spec.IgnoreDelete
		want.Spec.KeepCompletedPods = have.Spec.KeepCompletedPods
		want.Spec.KeepLatestPodsOnly = have.Spec.KeepLatestPodsOnly
		want.Spec.WriteOutputsToStatus = have.Spec.WriteOutputsToStatus
		want.Spec.OutputsSecret = have.Spec.OutputsSecret
		want.Spec.PersistentVolumeSize = have.Spec.PersistentVolumeSize
		want.Spec.OutputsToInclude = have.Spec.OutputsToInclude
		want.Spec.OutputsToOmit = have.Spec.OutputsToOmit
		want.Spec.ServiceAccount = have.Spec.ServiceAccount

		if have.Spec.StorageClassName != nil {
			if want.Annotations == nil {
				want.Annotations = map[string]string{}
			}
			want.Annotations["v1alpha2.tf.isaaguilar.com/storageClassName"] = *have.Spec.StorageClassName
		}

		if have.Spec.Images != nil {
			if have.Spec.Images.Script != nil {
				image := ""
				tag := ""
				var pullPolicy corev1.PullPolicy
				if have.Spec.Images.Script.Image != "" {
					// Simple regex checks for the components of the image
					a := strings.Split(have.Spec.Images.Script.Image, ":")
					i := len(a)
					if i == 1 {
						// Does not contain ":" meaning no tag present
						image = have.Spec.Images.Script.Image
						tag = tfv1alpha2.SetupTaskImageTagDefault
					} else {
						// Checks if last item behind ":" is tag-like
						re := regexp.MustCompile("^[a-zA-Z0-9._-]*$")
						if re.MatchString(a[i-1]) {
							image = strings.Join(a[:i-1], ":")
							tag = a[i-1]
						} else {
							image = have.Spec.Images.Script.Image
							tag = tfv1alpha2.SetupTaskImageTagDefault
						}
					}
				}
				if have.Spec.Images.Script.ImagePullPolicy != "" {
					pullPolicy = have.Spec.Images.Script.ImagePullPolicy
				}
				if image != "" {
					want.Spec.ScriptRunner = image
				}
				if tag != "" {
					want.Spec.ScriptRunnerVersion = tag
				}
				if pullPolicy != "" {
					want.Spec.ScriptRunnerPullPolicy = pullPolicy
				}
			}

			if have.Spec.Images.Terraform != nil {
				image := ""
				var pullPolicy corev1.PullPolicy
				if have.Spec.Images.Terraform.Image != "" {
					// Simple regex checks for the components of the image
					a := strings.Split(have.Spec.Images.Terraform.Image, ":")
					i := len(a)
					if i == 1 {
						// Does not contain ":" meaning no tag present
						image = have.Spec.Images.Terraform.Image
					} else {
						// Checks if last item behind ":" is tag-like
						re := regexp.MustCompile("^[a-zA-Z0-9._-]*$")
						if re.MatchString(a[i-1]) {
							image = strings.Join(a[:i-1], ":")
						} else {
							image = have.Spec.Images.Terraform.Image
						}
					}
				}
				if have.Spec.Images.Script.ImagePullPolicy != "" {
					pullPolicy = have.Spec.Images.Script.ImagePullPolicy
				}
				if image != "" {
					want.Spec.TerraformRunner = image
				}
				if pullPolicy != "" {
					want.Spec.TerraformRunnerPullPolicy = pullPolicy
				}
			}

			if have.Spec.Images.Setup != nil {
				image := ""
				tag := ""
				var pullPolicy corev1.PullPolicy
				if have.Spec.Images.Setup.Image != "" {
					// Simple regex checks for the components of the image
					a := strings.Split(have.Spec.Images.Setup.Image, ":")
					i := len(a)
					if i == 1 {
						// Does not contain ":" meaning no tag present
						image = have.Spec.Images.Setup.Image
						tag = tfv1alpha2.SetupTaskImageTagDefault
					} else {
						// Checks if last item behind ":" is tag-like
						re := regexp.MustCompile("^[a-zA-Z0-9._-]*$")
						if re.MatchString(a[i-1]) {
							image = strings.Join(a[:i-1], ":")
							tag = a[i-1]
						} else {
							image = have.Spec.Images.Setup.Image
							tag = tfv1alpha2.SetupTaskImageTagDefault
						}
					}
				}
				if have.Spec.Images.Setup.ImagePullPolicy != "" {
					pullPolicy = have.Spec.Images.Setup.ImagePullPolicy
				}
				if image != "" {
					want.Spec.SetupRunner = image
				}
				if tag != "" {
					want.Spec.SetupRunnerVersion = tag
				}
				if pullPolicy != "" {
					want.Spec.SetupRunnerPullPolicy = pullPolicy
				}
			}
		}

		for _, credentials := range have.Spec.Credentials {
			want.Spec.Credentials = append(want.Spec.Credentials, tfv1alpha1.Credentials{
				SecretNameRef:             tfv1alpha1.SecretNameRef(credentials.SecretNameRef),
				AWSCredentials:            tfv1alpha1.AWSCredentials(credentials.AWSCredentials),
				ServiceAccountAnnotations: credentials.ServiceAccountAnnotations,
			})
		}

		if have.Spec.SSHTunnel != nil {
			want.Spec.SSHTunnel = &tfv1alpha1.ProxyOpts{
				Host:            have.Spec.SSHTunnel.Host,
				User:            have.Spec.SSHTunnel.User,
				SSHKeySecretRef: tfv1alpha1.SSHKeySecretRef(have.Spec.SSHTunnel.SSHKeySecretRef),
			}
		}

		for _, scmAuthMethod := range have.Spec.SCMAuthMethods {
			var gitScmAuthMethod *tfv1alpha1.GitSCM
			if scmAuthMethod.Git != nil {
				var gitSSH *tfv1alpha1.GitSSH
				if scmAuthMethod.Git.SSH != nil {
					gitSSH = &tfv1alpha1.GitSSH{
						RequireProxy:    scmAuthMethod.Git.SSH.RequireProxy,
						SSHKeySecretRef: (*tfv1alpha1.SSHKeySecretRef)(scmAuthMethod.Git.SSH.SSHKeySecretRef),
					}
				}
				var gitHTTPS *tfv1alpha1.GitHTTPS
				if scmAuthMethod.Git.HTTPS != nil {
					gitHTTPS = &tfv1alpha1.GitHTTPS{
						RequireProxy:   scmAuthMethod.Git.HTTPS.RequireProxy,
						TokenSecretRef: (*tfv1alpha1.TokenSecretRef)(scmAuthMethod.Git.HTTPS.TokenSecretRef),
					}
				}
				gitScmAuthMethod = &tfv1alpha1.GitSCM{
					SSH:   gitSSH,
					HTTPS: gitHTTPS,
				}
			}
			want.Spec.SCMAuthMethods = append(want.Spec.SCMAuthMethods, tfv1alpha1.SCMAuthMethod{
				Host: scmAuthMethod.Host,
				Git:  gitScmAuthMethod,
			})
		}

		// TaskOptions is where v1alpha1 and v1alpha2 conversions break down. Some items cannot be back-ported
		// and will be omitted in this conversion. The idea here is that users that have configured v1alpha2
		// will not move back to v1alpha1 and the new features do not need to be added.
		//
		// This is an attempt to make v1alpha1 as close as possible to v1alpha2 so users can continue to use
		// v1alpha1 which will allow their resources to accept patching and updates. This is important for
		// helm users when `helm upgrade` is used.
		if len(have.Spec.TaskOptions) > 0 {
			for _, opts := range have.Spec.TaskOptions {
				if len(opts.For) > 0 {
					// only check for the global options first
					if opts.For[0] != "*" {
						continue
					}
					want.Spec.Env = opts.Env
					want.Spec.RunnerAnnotations = opts.Annotations
					want.Spec.RunnerLabels = opts.Labels
					want.Spec.RunnerRules = opts.PolicyRules
				}
			}

			//
			// The condition where there is a configmap for all terraform tasks translates to having TerraformRunnerExecutionScriptConfigMap
			//
			terraformTaskTypes := []tfv1alpha2.TaskName{
				tfv1alpha2.RunInitDelete,
				tfv1alpha2.RunPlanDelete,
				tfv1alpha2.RunApplyDelete,
				tfv1alpha2.RunInit,
				tfv1alpha2.RunPlan,
				tfv1alpha2.RunApply,
			}
			terraformTaskOptionsContainingConfigMapSelector := []tfv1alpha2.TaskName{}
			terraformTaskOptionConfigMapSelector := &tfv1alpha2.ConfigMapSelector{}
			for _, opts := range have.Spec.TaskOptions {
				if len(opts.For) == 1 {
					if tfv1alpha2.ListContainsTask(terraformTaskTypes, opts.For[0]) && opts.Script.ConfigMapSelector != nil {
						if !tfv1alpha2.ListContainsTask(terraformTaskOptionsContainingConfigMapSelector, opts.For[0]) {
							// Only true if the same configmap is used for all terraform tasks
							if terraformTaskOptionConfigMapSelector.Name != "" && terraformTaskOptionConfigMapSelector.Name != opts.Script.ConfigMapSelector.Name {
								break
							}
							if terraformTaskOptionConfigMapSelector.Key != "" && terraformTaskOptionConfigMapSelector.Key != opts.Script.ConfigMapSelector.Key {
								break
							}
							terraformTaskOptionsContainingConfigMapSelector = append(terraformTaskOptionsContainingConfigMapSelector, opts.For[0])
							terraformTaskOptionConfigMapSelector = opts.Script.ConfigMapSelector
						}
					}
				}
			}
			if len(terraformTaskOptionsContainingConfigMapSelector) == len(terraformTaskTypes) && terraformTaskOptionConfigMapSelector != nil {
				configMapKeySelector := corev1.ConfigMapKeySelector{
					Key: terraformTaskOptionConfigMapSelector.Key,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: terraformTaskOptionConfigMapSelector.Name,
					},
				}
				want.Spec.TerraformRunnerExecutionScriptConfigMap = &configMapKeySelector
			}

			//
			// The condition where there is a configmap for all setup tasks translates to having SetupRunnerExecutionScriptConfigMap
			//
			setupTaskTypes := []tfv1alpha2.TaskName{
				tfv1alpha2.RunSetupDelete,
				tfv1alpha2.RunSetup,
			}
			setupTaskOptionsContainingConfigMapSelector := []tfv1alpha2.TaskName{}
			setupTaskOptionConfigMapSelector := &tfv1alpha2.ConfigMapSelector{}
			for _, opts := range have.Spec.TaskOptions {
				if len(opts.For) == 1 {
					if tfv1alpha2.ListContainsTask(setupTaskTypes, opts.For[0]) && opts.Script.ConfigMapSelector != nil {
						if !tfv1alpha2.ListContainsTask(setupTaskOptionsContainingConfigMapSelector, opts.For[0]) {
							if setupTaskOptionConfigMapSelector.Name != "" && setupTaskOptionConfigMapSelector.Name != opts.Script.ConfigMapSelector.Name {
								break
							}
							if setupTaskOptionConfigMapSelector.Key != "" && setupTaskOptionConfigMapSelector.Key != opts.Script.ConfigMapSelector.Key {
								break
							}
							setupTaskOptionsContainingConfigMapSelector = append(setupTaskOptionsContainingConfigMapSelector, opts.For[0])
							setupTaskOptionConfigMapSelector = opts.Script.ConfigMapSelector
						}
					}
				}
			}
			if len(setupTaskOptionsContainingConfigMapSelector) == len(setupTaskTypes) && setupTaskOptionConfigMapSelector != nil {
				configMapKeySelector := corev1.ConfigMapKeySelector{
					Key: setupTaskOptionConfigMapSelector.Key,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: setupTaskOptionConfigMapSelector.Name,
					},
				}
				want.Spec.SetupRunnerExecutionScriptConfigMap = &configMapKeySelector
			}

			//
			// Find inline scripts and translate them into spec.<scripts> (eg spec.prePlanScript)
			//
			for _, opts := range have.Spec.TaskOptions {
				if len(opts.For) == 1 {
					if opts.Script.Inline != "" {
						convertInlineScriptFromTaskOptions(opts.For[0], opts.Script.Inline, &want)
					}
				}
			}

		}

		if have.Spec.Setup != nil {
			if have.Spec.Setup.ResourceDownloads != nil {
				for _, resourceDownload := range have.Spec.Setup.ResourceDownloads {
					want.Spec.ResourceDownloads = append(want.Spec.ResourceDownloads, (*tfv1alpha1.ResourceDownload)(&resourceDownload))
				}
			}
			want.Spec.CleanupDisk = have.Spec.Setup.CleanupDisk
		}
	}

	// Status is very important so TFO can continue where it left from last version
	want.Status.PodNamePrefix = have.Status.PodNamePrefix
	want.Status.Stages = []tfv1alpha1.Stage{
		{
			Generation:    have.Status.Stage.Generation,
			State:         tfv1alpha1.StageState(have.Status.Stage.State),
			PodType:       tfv1alpha1.PodType(have.Status.Stage.TaskType),
			Interruptible: tfv1alpha1.Interruptible(have.Status.Stage.Interruptible),
			Reason:        have.Status.Stage.Reason,
			StartTime:     have.Status.Stage.StartTime,
			StopTime:      have.Status.Stage.StopTime,
		},
	}
	want.Status.Phase = tfv1alpha1.StatusPhase(have.Status.Phase)
	want.Status.Exported = tfv1alpha1.ExportedFalse
	want.Status.LastCompletedGeneration = have.Status.LastCompletedGeneration
	rawResponse, _ := json.Marshal(want)
	log.Print("took ", time.Since(start).String(), " to complete conversion")
	return rawResponse, &want, nil
}

func PodTypeFromTaskName(taskName tfv1alpha2.TaskName) tfv1alpha1.PodType {

	conversionMap := map[tfv1alpha2.TaskName]tfv1alpha1.PodType{
		tfv1alpha2.RunSetupDelete:     tfv1alpha1.PodSetupDelete,
		tfv1alpha2.RunPreInitDelete:   tfv1alpha1.PodPreInitDelete,
		tfv1alpha2.RunInitDelete:      tfv1alpha1.PodInitDelete,
		tfv1alpha2.RunPostInitDelete:  tfv1alpha1.PodPostInitDelete,
		tfv1alpha2.RunPrePlanDelete:   tfv1alpha1.PodPrePlanDelete,
		tfv1alpha2.RunPlanDelete:      tfv1alpha1.PodPlanDelete,
		tfv1alpha2.RunPostPlanDelete:  tfv1alpha1.PodPostPlanDelete,
		tfv1alpha2.RunPreApplyDelete:  tfv1alpha1.PodPreApplyDelete,
		tfv1alpha2.RunApplyDelete:     tfv1alpha1.PodApplyDelete,
		tfv1alpha2.RunPostApplyDelete: tfv1alpha1.PodPostApplyDelete,

		tfv1alpha2.RunSetup:     tfv1alpha1.PodSetup,
		tfv1alpha2.RunPreInit:   tfv1alpha1.PodPreInit,
		tfv1alpha2.RunInit:      tfv1alpha1.PodInit,
		tfv1alpha2.RunPostInit:  tfv1alpha1.PodPostInit,
		tfv1alpha2.RunPrePlan:   tfv1alpha1.PodPrePlan,
		tfv1alpha2.RunPlan:      tfv1alpha1.PodPlan,
		tfv1alpha2.RunPostPlan:  tfv1alpha1.PodPostPlan,
		tfv1alpha2.RunPreApply:  tfv1alpha1.PodPreApply,
		tfv1alpha2.RunApply:     tfv1alpha1.PodApply,
		tfv1alpha2.RunPostApply: tfv1alpha1.PodPostApply,
		tfv1alpha2.RunNil:       tfv1alpha1.PodNil,
	}

	return conversionMap[taskName]

}

func convertInlineScriptFromTaskOptions(taskName tfv1alpha2.TaskName, scriptContents string, want *tfv1alpha1.Terraform) {
	switch PodTypeFromTaskName(taskName) {
	case tfv1alpha1.PodPreInit:
		want.Spec.PreInitScript = scriptContents
	case tfv1alpha1.PodPostInit:
		want.Spec.PostInitScript = scriptContents
	case tfv1alpha1.PodPrePlan:
		want.Spec.PrePlanScript = scriptContents
	case tfv1alpha1.PodPostPlan:
		want.Spec.PostPlanScript = scriptContents
	case tfv1alpha1.PodPreApply:
		want.Spec.PreApplyScript = scriptContents
	case tfv1alpha1.PodPostApply:
		want.Spec.PostApplyScript = scriptContents
	case tfv1alpha1.PodPreInitDelete:
		want.Spec.PreInitDeleteScript = scriptContents
	case tfv1alpha1.PodPostInitDelete:
		want.Spec.PostInitDeleteScript = scriptContents
	case tfv1alpha1.PodPrePlanDelete:
		want.Spec.PrePlanDeleteScript = scriptContents
	case tfv1alpha1.PodPostPlanDelete:
		want.Spec.PostPlanDeleteScript = scriptContents
	case tfv1alpha1.PodPreApplyDelete:
		want.Spec.PreApplyDeleteScript = scriptContents
	case tfv1alpha1.PodPostApplyDelete:
		want.Spec.PostApplyDeleteScript = scriptContents
	}
}
