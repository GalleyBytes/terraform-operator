package convert

import (
	"encoding/json"
	"fmt"

	tfv1alpha1 "github.com/isaaguilar/terraform-operator/pkg/apis/tf/v1alpha1"
	tfv1alpha2 "github.com/isaaguilar/terraform-operator/pkg/apis/tf/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func ConvertV1alpha1ToV1alpha2(rawRequest []byte) ([]byte, runtime.Object, error) {

	want := tfv1alpha2.Terraform{}
	have := tfv1alpha1.Terraform{}

	err := json.Unmarshal(rawRequest, &have)
	if err != nil {
		return []byte{}, &want, err
	}

	fmt.Printf("Should convert %s/%s from %s to %s\n", have.Namespace, have.Name, tfv1alpha1.SchemeGroupVersion, tfv1alpha2.SchemeGroupVersion)

	want.TypeMeta = metav1.TypeMeta{
		Kind:       "Terraform",
		APIVersion: tfv1alpha2.SchemeGroupVersion.String(),
	}
	want.ObjectMeta = have.ObjectMeta
	want.Spec.TerraformVersion = have.Spec.TerraformVersion
	want.Spec.TerraformModule.Source = have.Spec.TerraformModule
	want.Spec.TerraformModule.Inline = have.Spec.TerraformModuleInline
	want.Spec.TerraformModule.ConfigMapSelector = (*tfv1alpha2.ConfigMapSelector)(have.Spec.TerraformModuleConfigMap)
	want.Spec.Backend = have.Spec.CustomBackend
	want.Spec.IgnoreDelete = have.Spec.IgnoreDelete
	want.Spec.KeepCompletedPods = have.Spec.KeepCompletedPods
	want.Spec.KeepLatestPodsOnly = have.Spec.KeepLatestPodsOnly
	want.Spec.WriteOutputsToStatus = have.Spec.WriteOutputsToStatus
	want.Spec.OutputsSecret = have.Spec.OutputsSecret
	want.Spec.PersistentVolumeSize = have.Spec.PersistentVolumeSize
	want.Spec.OutputsToInclude = have.Spec.OutputsToInclude
	want.Spec.OutputsToOmit = have.Spec.OutputsToOmit
	want.Spec.ServiceAccount = have.Spec.ServiceAccount

	scriptImageConfig := convertImageConfig("script", have.Spec.ScriptRunner, have.Spec.ScriptRunnerVersion, have.Spec.ScriptRunnerPullPolicy)
	terraformImageConfig := convertImageConfig("terraform", have.Spec.TerraformRunner, "", have.Spec.TerraformRunnerPullPolicy)
	setupImageConfig := convertImageConfig("setup", have.Spec.SetupRunner, have.Spec.SetupRunnerVersion, have.Spec.SetupRunnerPullPolicy)
	images := tfv1alpha2.Images{
		Terraform: terraformImageConfig,
		Setup:     setupImageConfig,
		Script:    scriptImageConfig,
	}
	if scriptImageConfig != nil || terraformImageConfig != nil || setupImageConfig != nil {
		want.Spec.Images = &images
	}

	for _, credentials := range have.Spec.Credentials {
		want.Spec.Credentials = append(want.Spec.Credentials, tfv1alpha2.Credentials{
			SecretNameRef:             tfv1alpha2.SecretNameRef(credentials.SecretNameRef),
			AWSCredentials:            tfv1alpha2.AWSCredentials(credentials.AWSCredentials),
			ServiceAccountAnnotations: credentials.ServiceAccountAnnotations,
		})
	}

	if have.Spec.SSHTunnel != nil {
		want.Spec.SSHTunnel = &tfv1alpha2.ProxyOpts{
			Host:            have.Spec.SSHTunnel.Host,
			User:            have.Spec.SSHTunnel.User,
			SSHKeySecretRef: tfv1alpha2.SSHKeySecretRef(have.Spec.SSHTunnel.SSHKeySecretRef),
		}
	}

	for _, scmAuthMethod := range have.Spec.SCMAuthMethods {
		var gitScmAuthMethod *tfv1alpha2.GitSCM
		if scmAuthMethod.Git != nil {
			var gitSSH *tfv1alpha2.GitSSH
			if scmAuthMethod.Git.SSH != nil {
				gitSSH = &tfv1alpha2.GitSSH{
					RequireProxy:    scmAuthMethod.Git.SSH.RequireProxy,
					SSHKeySecretRef: (*tfv1alpha2.SSHKeySecretRef)(scmAuthMethod.Git.SSH.SSHKeySecretRef),
				}
			}
			var gitHTTPS *tfv1alpha2.GitHTTPS
			if scmAuthMethod.Git.HTTPS != nil {
				gitHTTPS = &tfv1alpha2.GitHTTPS{
					RequireProxy:   scmAuthMethod.Git.HTTPS.RequireProxy,
					TokenSecretRef: (*tfv1alpha2.TokenSecretRef)(scmAuthMethod.Git.HTTPS.TokenSecretRef),
				}
			}
			gitScmAuthMethod = &tfv1alpha2.GitSCM{
				SSH:   gitSSH,
				HTTPS: gitHTTPS,
			}
		}
		want.Spec.SCMAuthMethods = append(want.Spec.SCMAuthMethods, tfv1alpha2.SCMAuthMethod{
			Host: scmAuthMethod.Host,
			Git:  gitScmAuthMethod,
		})
	}

	want.Spec.TaskOptions = []tfv1alpha2.TaskOption{}

	if len(have.Spec.Env) > 0 ||
		len(have.Spec.RunnerRules) > 0 ||
		len(have.Spec.RunnerAnnotations) > 0 ||
		len(have.Spec.RunnerLabels) > 0 {

		taskOption := tfv1alpha2.TaskOption{
			TaskTypes: []tfv1alpha2.TaskType{"*"},
		}

		if len(have.Spec.Env) > 0 {
			taskOption.Env = have.Spec.Env
		}
		if len(have.Spec.RunnerRules) > 0 {
			taskOption.PolicyRules = have.Spec.RunnerRules
		}
		if len(have.Spec.RunnerAnnotations) > 0 {
			taskOption.Annotations = have.Spec.RunnerAnnotations
		}
		if len(have.Spec.RunnerLabels) > 0 {
			taskOption.Labels = have.Spec.RunnerLabels
		}
		want.Spec.TaskOptions = append(want.Spec.TaskOptions, taskOption)
	}

	// NOTICE: ScriptRunnerExecutionScriptConfigMap is not supported. Instead,
	// use a different Container Image to change the ENTRYPOINT. Changing the
	// ENRYPOINT was the intended behaviour of *ExecutionScriptConfigMap
	// options.
	//
	// Note that both
	// - terraformRunnerExecutionScriptConfigMap &
	// - setupRunnerExecutionScriptConfigMap
	// will be used as as scripts that their respective Containers execute.
	//
	// In practice, the use of the *ExecutionScriptConfigMap was not adopted and
	// will lose support in favor of TaskOptions Script.
	if have.Spec.TerraformRunnerExecutionScriptConfigMap != nil {
		terraformRunnerExecutionScriptConfigMap := &tfv1alpha1.ConfigMapSelector{
			Name: have.Spec.TerraformRunnerExecutionScriptConfigMap.Name,
			Key:  have.Spec.TerraformRunnerExecutionScriptConfigMap.Key,
		}
		convertRunScriptsToTaskConfigMapSelector(terraformRunnerExecutionScriptConfigMap, tfv1alpha2.RunInit, &want.Spec.TaskOptions)
		convertRunScriptsToTaskConfigMapSelector(terraformRunnerExecutionScriptConfigMap, tfv1alpha2.RunPlan, &want.Spec.TaskOptions)
		convertRunScriptsToTaskConfigMapSelector(terraformRunnerExecutionScriptConfigMap, tfv1alpha2.RunApply, &want.Spec.TaskOptions)
		convertRunScriptsToTaskConfigMapSelector(terraformRunnerExecutionScriptConfigMap, tfv1alpha2.RunInitDelete, &want.Spec.TaskOptions)
		convertRunScriptsToTaskConfigMapSelector(terraformRunnerExecutionScriptConfigMap, tfv1alpha2.RunPlanDelete, &want.Spec.TaskOptions)
		convertRunScriptsToTaskConfigMapSelector(terraformRunnerExecutionScriptConfigMap, tfv1alpha2.RunApplyDelete, &want.Spec.TaskOptions)
	}

	if have.Spec.SetupRunnerExecutionScriptConfigMap != nil {
		setupRunnerExecutionScriptConfigMap := &tfv1alpha1.ConfigMapSelector{
			Name: have.Spec.SetupRunnerExecutionScriptConfigMap.Name,
			Key:  have.Spec.SetupRunnerExecutionScriptConfigMap.Key,
		}
		convertRunScriptsToTaskConfigMapSelector(setupRunnerExecutionScriptConfigMap, tfv1alpha2.RunSetup, &want.Spec.TaskOptions)
		convertRunScriptsToTaskConfigMapSelector(setupRunnerExecutionScriptConfigMap, tfv1alpha2.RunSetupDelete, &want.Spec.TaskOptions)
	}

	convertRunScriptsToTaskInlineScripts(have.Spec.PreInitScript, tfv1alpha2.RunPreInit, &want.Spec.TaskOptions)
	convertRunScriptsToTaskInlineScripts(have.Spec.PostInitScript, tfv1alpha2.RunPostInit, &want.Spec.TaskOptions)
	convertRunScriptsToTaskInlineScripts(have.Spec.PrePlanScript, tfv1alpha2.RunPrePlan, &want.Spec.TaskOptions)
	convertRunScriptsToTaskInlineScripts(have.Spec.PostPlanScript, tfv1alpha2.RunPostPlan, &want.Spec.TaskOptions)
	convertRunScriptsToTaskInlineScripts(have.Spec.PreApplyScript, tfv1alpha2.RunPreApply, &want.Spec.TaskOptions)
	convertRunScriptsToTaskInlineScripts(have.Spec.PostApplyScript, tfv1alpha2.RunPostApply, &want.Spec.TaskOptions)
	convertRunScriptsToTaskInlineScripts(have.Spec.PreInitDeleteScript, tfv1alpha2.RunPreInitDelete, &want.Spec.TaskOptions)
	convertRunScriptsToTaskInlineScripts(have.Spec.PostInitDeleteScript, tfv1alpha2.RunPostInitDelete, &want.Spec.TaskOptions)
	convertRunScriptsToTaskInlineScripts(have.Spec.PrePlanDeleteScript, tfv1alpha2.RunPrePlanDelete, &want.Spec.TaskOptions)
	convertRunScriptsToTaskInlineScripts(have.Spec.PostPlanDeleteScript, tfv1alpha2.RunPostPlanDelete, &want.Spec.TaskOptions)
	convertRunScriptsToTaskInlineScripts(have.Spec.PreApplyDeleteScript, tfv1alpha2.RunPreApplyDelete, &want.Spec.TaskOptions)
	convertRunScriptsToTaskInlineScripts(have.Spec.PostApplyDeleteScript, tfv1alpha2.RunPostApplyDelete, &want.Spec.TaskOptions)

	for _, resourceDownload := range have.Spec.ResourceDownloads {
		if resourceDownload != nil {
			if want.Spec.Setup == nil {
				want.Spec.Setup = &tfv1alpha2.Setup{}
			}
			want.Spec.Setup.ResourceDownloads = append(want.Spec.Setup.ResourceDownloads, tfv1alpha2.ResourceDownload(*resourceDownload))
		}
	}

	// Status is very important so TFO can continue where it left from last version
	want.Status.PodNamePrefix = have.Status.PodNamePrefix
	want.Status.Stages = []tfv1alpha2.Stage{}
	for _, stage := range have.Status.Stages {
		want.Status.Stages = append(want.Status.Stages, tfv1alpha2.Stage{
			Generation:    stage.Generation,
			State:         tfv1alpha2.StageState(stage.State),
			TaskType:      runTypeFromPodType(stage.PodType),
			Interruptible: tfv1alpha2.Interruptible(stage.Interruptible),
			Reason:        stage.Reason,
			StartTime:     stage.StartTime,
			StopTime:      stage.StopTime,
		})
	}
	if len(have.Status.Stages) > 0 {
		lastStage := have.Status.Stages[len(have.Status.Stages)-1]
		want.Status.Stage = tfv1alpha2.Stage{
			Generation:    lastStage.Generation,
			State:         tfv1alpha2.StageState(lastStage.State),
			TaskType:      runTypeFromPodType(lastStage.PodType),
			Interruptible: tfv1alpha2.Interruptible(lastStage.Interruptible),
			Reason:        lastStage.Reason,
			StartTime:     lastStage.StartTime,
			StopTime:      lastStage.StopTime,
			PodName:       "",
			Message:       "",
		}
	}
	want.Status.Phase = tfv1alpha2.StatusPhase(have.Status.Phase)
	want.Status.Exported = tfv1alpha2.Exported(have.Status.Exported)
	want.Status.LastCompletedGeneration = have.Status.LastCompletedGeneration

	rawResponse, _ := json.Marshal(want)

	return rawResponse, &want, nil
}

func runTypeFromPodType(podType tfv1alpha1.PodType) tfv1alpha2.TaskType {

	conversionMap := map[tfv1alpha1.PodType]tfv1alpha2.TaskType{
		tfv1alpha1.PodSetupDelete:     tfv1alpha2.RunSetupDelete,
		tfv1alpha1.PodPreInitDelete:   tfv1alpha2.RunPreInitDelete,
		tfv1alpha1.PodInitDelete:      tfv1alpha2.RunInitDelete,
		tfv1alpha1.PodPostInitDelete:  tfv1alpha2.RunPostInitDelete,
		tfv1alpha1.PodPrePlanDelete:   tfv1alpha2.RunPrePlanDelete,
		tfv1alpha1.PodPlanDelete:      tfv1alpha2.RunPlanDelete,
		tfv1alpha1.PodPostPlanDelete:  tfv1alpha2.RunPostPlanDelete,
		tfv1alpha1.PodPreApplyDelete:  tfv1alpha2.RunPreApplyDelete,
		tfv1alpha1.PodApplyDelete:     tfv1alpha2.RunApplyDelete,
		tfv1alpha1.PodPostApplyDelete: tfv1alpha2.RunPostApplyDelete,

		tfv1alpha1.PodSetup:     tfv1alpha2.RunSetup,
		tfv1alpha1.PodPreInit:   tfv1alpha2.RunPreInit,
		tfv1alpha1.PodInit:      tfv1alpha2.RunInit,
		tfv1alpha1.PodPostInit:  tfv1alpha2.RunPostInit,
		tfv1alpha1.PodPrePlan:   tfv1alpha2.RunPrePlan,
		tfv1alpha1.PodPlan:      tfv1alpha2.RunPlan,
		tfv1alpha1.PodPostPlan:  tfv1alpha2.RunPostPlan,
		tfv1alpha1.PodPreApply:  tfv1alpha2.RunPreApply,
		tfv1alpha1.PodApply:     tfv1alpha2.RunApply,
		tfv1alpha1.PodPostApply: tfv1alpha2.RunPostApply,
		tfv1alpha1.PodNil:       tfv1alpha2.RunNil,
	}

	return conversionMap[podType]

}

func convertRunScriptsToTaskConfigMapSelector(configMapSelector *tfv1alpha1.ConfigMapSelector, runType tfv1alpha2.TaskType, taskOptions *[]tfv1alpha2.TaskOption) {
	if configMapSelector != nil {
		*taskOptions = append(*taskOptions, tfv1alpha2.TaskOption{
			TaskTypes: []tfv1alpha2.TaskType{runType},
			Script: tfv1alpha2.StageScript{
				ConfigMapSelector: (*tfv1alpha2.ConfigMapSelector)(configMapSelector),
			},
		})
	}
}

func convertRunScriptsToTaskInlineScripts(inlineScript string, runType tfv1alpha2.TaskType, taskOptions *[]tfv1alpha2.TaskOption) {
	if inlineScript != "" {
		*taskOptions = append(*taskOptions, tfv1alpha2.TaskOption{
			TaskTypes: []tfv1alpha2.TaskType{runType},
			Script: tfv1alpha2.StageScript{
				Inline: inlineScript,
			},
		})
	}
}

func convertImageConfig(imageType, repo, version string, imagePullPolicy corev1.PullPolicy) *tfv1alpha2.ImageConfig {
	defaults := map[string]map[string]string{
		"script": {
			"tag":   "1.0.0",
			"image": "ghcr.io/galleybytes/terraform-operator-script",
		},
		"terraform": {
			"image": "ghcr.io/galleybytes/terraform-operator-tftaskv1",
		},
		"setup": {
			"image": "ghcr.io/galleybytes/terraform-operator-setup",
			"tag":   "1.0.0",
		},
	}
	var imageConfig *tfv1alpha2.ImageConfig
	if repo != "" || version != "" || imagePullPolicy != "" {
		image := ""
		var imagePullPolicy corev1.PullPolicy
		if repo != "" {
			image = repo
		} else {
			if value, ok := defaults[imageType]["image"]; ok {
				image = value
			}
		}
		if version != "" {
			image += ":" + version
		} else {
			if value, ok := defaults[imageType]["tag"]; ok {
				image += ":" + value
			}
		}
		if imagePullPolicy == "" {
			imagePullPolicy = "Always"
		}
		imageConfig = &tfv1alpha2.ImageConfig{
			Image:           image,
			ImagePullPolicy: imagePullPolicy,
		}
	}

	return imageConfig
}
