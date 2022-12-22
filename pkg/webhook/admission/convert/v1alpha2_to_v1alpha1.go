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

			// TODO Extract TaskOptions for different task types

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
