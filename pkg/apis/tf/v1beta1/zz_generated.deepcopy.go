//go:build !ignore_autogenerated
// +build !ignore_autogenerated

/*
Copyright isaaguilar.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by controller-gen. DO NOT EDIT.

package v1beta1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/rbac/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AWSCredentials) DeepCopyInto(out *AWSCredentials) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AWSCredentials.
func (in *AWSCredentials) DeepCopy() *AWSCredentials {
	if in == nil {
		return nil
	}
	out := new(AWSCredentials)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ConfigMapSelector) DeepCopyInto(out *ConfigMapSelector) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ConfigMapSelector.
func (in *ConfigMapSelector) DeepCopy() *ConfigMapSelector {
	if in == nil {
		return nil
	}
	out := new(ConfigMapSelector)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Credentials) DeepCopyInto(out *Credentials) {
	*out = *in
	out.SecretNameRef = in.SecretNameRef
	out.AWSCredentials = in.AWSCredentials
	if in.ServiceAccountAnnotations != nil {
		in, out := &in.ServiceAccountAnnotations, &out.ServiceAccountAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Credentials.
func (in *Credentials) DeepCopy() *Credentials {
	if in == nil {
		return nil
	}
	out := new(Credentials)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GitHTTPS) DeepCopyInto(out *GitHTTPS) {
	*out = *in
	if in.TokenSecretRef != nil {
		in, out := &in.TokenSecretRef, &out.TokenSecretRef
		*out = new(TokenSecretRef)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GitHTTPS.
func (in *GitHTTPS) DeepCopy() *GitHTTPS {
	if in == nil {
		return nil
	}
	out := new(GitHTTPS)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GitSCM) DeepCopyInto(out *GitSCM) {
	*out = *in
	if in.SSH != nil {
		in, out := &in.SSH, &out.SSH
		*out = new(GitSSH)
		(*in).DeepCopyInto(*out)
	}
	if in.HTTPS != nil {
		in, out := &in.HTTPS, &out.HTTPS
		*out = new(GitHTTPS)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GitSCM.
func (in *GitSCM) DeepCopy() *GitSCM {
	if in == nil {
		return nil
	}
	out := new(GitSCM)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GitSSH) DeepCopyInto(out *GitSSH) {
	*out = *in
	if in.SSHKeySecretRef != nil {
		in, out := &in.SSHKeySecretRef, &out.SSHKeySecretRef
		*out = new(SSHKeySecretRef)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GitSSH.
func (in *GitSSH) DeepCopy() *GitSSH {
	if in == nil {
		return nil
	}
	out := new(GitSSH)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ImageConfig) DeepCopyInto(out *ImageConfig) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ImageConfig.
func (in *ImageConfig) DeepCopy() *ImageConfig {
	if in == nil {
		return nil
	}
	out := new(ImageConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Images) DeepCopyInto(out *Images) {
	*out = *in
	if in.Terraform != nil {
		in, out := &in.Terraform, &out.Terraform
		*out = new(ImageConfig)
		**out = **in
	}
	if in.Script != nil {
		in, out := &in.Script, &out.Script
		*out = new(ImageConfig)
		**out = **in
	}
	if in.Setup != nil {
		in, out := &in.Setup, &out.Setup
		*out = new(ImageConfig)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Images.
func (in *Images) DeepCopy() *Images {
	if in == nil {
		return nil
	}
	out := new(Images)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Module) DeepCopyInto(out *Module) {
	*out = *in
	if in.ConfigMapSelector != nil {
		in, out := &in.ConfigMapSelector, &out.ConfigMapSelector
		*out = new(ConfigMapSelector)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Module.
func (in *Module) DeepCopy() *Module {
	if in == nil {
		return nil
	}
	out := new(Module)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Plugin) DeepCopyInto(out *Plugin) {
	*out = *in
	out.ImageConfig = in.ImageConfig
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Plugin.
func (in *Plugin) DeepCopy() *Plugin {
	if in == nil {
		return nil
	}
	out := new(Plugin)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ProxyOpts) DeepCopyInto(out *ProxyOpts) {
	*out = *in
	out.SSHKeySecretRef = in.SSHKeySecretRef
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ProxyOpts.
func (in *ProxyOpts) DeepCopy() *ProxyOpts {
	if in == nil {
		return nil
	}
	out := new(ProxyOpts)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResourceDownload) DeepCopyInto(out *ResourceDownload) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResourceDownload.
func (in *ResourceDownload) DeepCopy() *ResourceDownload {
	if in == nil {
		return nil
	}
	out := new(ResourceDownload)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SCMAuthMethod) DeepCopyInto(out *SCMAuthMethod) {
	*out = *in
	if in.Git != nil {
		in, out := &in.Git, &out.Git
		*out = new(GitSCM)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SCMAuthMethod.
func (in *SCMAuthMethod) DeepCopy() *SCMAuthMethod {
	if in == nil {
		return nil
	}
	out := new(SCMAuthMethod)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SSHKeySecretRef) DeepCopyInto(out *SSHKeySecretRef) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SSHKeySecretRef.
func (in *SSHKeySecretRef) DeepCopy() *SSHKeySecretRef {
	if in == nil {
		return nil
	}
	out := new(SSHKeySecretRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *SecretNameRef) DeepCopyInto(out *SecretNameRef) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new SecretNameRef.
func (in *SecretNameRef) DeepCopy() *SecretNameRef {
	if in == nil {
		return nil
	}
	out := new(SecretNameRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Setup) DeepCopyInto(out *Setup) {
	*out = *in
	if in.ResourceDownloads != nil {
		in, out := &in.ResourceDownloads, &out.ResourceDownloads
		*out = make([]ResourceDownload, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Setup.
func (in *Setup) DeepCopy() *Setup {
	if in == nil {
		return nil
	}
	out := new(Setup)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Stage) DeepCopyInto(out *Stage) {
	*out = *in
	in.StartTime.DeepCopyInto(&out.StartTime)
	in.StopTime.DeepCopyInto(&out.StopTime)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Stage.
func (in *Stage) DeepCopy() *Stage {
	if in == nil {
		return nil
	}
	out := new(Stage)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *StageScript) DeepCopyInto(out *StageScript) {
	*out = *in
	if in.ConfigMapSelector != nil {
		in, out := &in.ConfigMapSelector, &out.ConfigMapSelector
		*out = new(ConfigMapSelector)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new StageScript.
func (in *StageScript) DeepCopy() *StageScript {
	if in == nil {
		return nil
	}
	out := new(StageScript)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TaskOption) DeepCopyInto(out *TaskOption) {
	*out = *in
	if in.For != nil {
		in, out := &in.For, &out.For
		*out = make([]TaskName, len(*in))
		copy(*out, *in)
	}
	if in.PolicyRules != nil {
		in, out := &in.PolicyRules, &out.PolicyRules
		*out = make([]v1.PolicyRule, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Annotations != nil {
		in, out := &in.Annotations, &out.Annotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.EnvFrom != nil {
		in, out := &in.EnvFrom, &out.EnvFrom
		*out = make([]corev1.EnvFromSource, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Env != nil {
		in, out := &in.Env, &out.Env
		*out = make([]corev1.EnvVar, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = new(corev1.ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
	in.Script.DeepCopyInto(&out.Script)
	if in.Volumes != nil {
		in, out := &in.Volumes, &out.Volumes
		*out = make([]corev1.Volume, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.VolumeMounts != nil {
		in, out := &in.VolumeMounts, &out.VolumeMounts
		*out = make([]corev1.VolumeMount, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TaskOption.
func (in *TaskOption) DeepCopy() *TaskOption {
	if in == nil {
		return nil
	}
	out := new(TaskOption)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Terraform) DeepCopyInto(out *Terraform) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Terraform.
func (in *Terraform) DeepCopy() *Terraform {
	if in == nil {
		return nil
	}
	out := new(Terraform)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Terraform) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TerraformList) DeepCopyInto(out *TerraformList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Terraform, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TerraformList.
func (in *TerraformList) DeepCopy() *TerraformList {
	if in == nil {
		return nil
	}
	out := new(TerraformList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *TerraformList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TerraformSpec) DeepCopyInto(out *TerraformSpec) {
	*out = *in
	if in.OutputsToInclude != nil {
		in, out := &in.OutputsToInclude, &out.OutputsToInclude
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.OutputsToOmit != nil {
		in, out := &in.OutputsToOmit, &out.OutputsToOmit
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.PersistentVolumeSize != nil {
		in, out := &in.PersistentVolumeSize, &out.PersistentVolumeSize
		x := (*in).DeepCopy()
		*out = &x
	}
	if in.StorageClassName != nil {
		in, out := &in.StorageClassName, &out.StorageClassName
		*out = new(string)
		**out = **in
	}
	if in.Credentials != nil {
		in, out := &in.Credentials, &out.Credentials
		*out = make([]Credentials, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.SSHTunnel != nil {
		in, out := &in.SSHTunnel, &out.SSHTunnel
		*out = new(ProxyOpts)
		**out = **in
	}
	if in.SCMAuthMethods != nil {
		in, out := &in.SCMAuthMethods, &out.SCMAuthMethods
		*out = make([]SCMAuthMethod, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Images != nil {
		in, out := &in.Images, &out.Images
		*out = new(Images)
		(*in).DeepCopyInto(*out)
	}
	if in.Setup != nil {
		in, out := &in.Setup, &out.Setup
		*out = new(Setup)
		(*in).DeepCopyInto(*out)
	}
	in.TerraformModule.DeepCopyInto(&out.TerraformModule)
	if in.TaskOptions != nil {
		in, out := &in.TaskOptions, &out.TaskOptions
		*out = make([]TaskOption, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Plugins != nil {
		in, out := &in.Plugins, &out.Plugins
		*out = make(map[TaskName]Plugin, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TerraformSpec.
func (in *TerraformSpec) DeepCopy() *TerraformSpec {
	if in == nil {
		return nil
	}
	out := new(TerraformSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TerraformStatus) DeepCopyInto(out *TerraformStatus) {
	*out = *in
	if in.Outputs != nil {
		in, out := &in.Outputs, &out.Outputs
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	in.Stage.DeepCopyInto(&out.Stage)
	if in.PluginsStarted != nil {
		in, out := &in.PluginsStarted, &out.PluginsStarted
		*out = make([]TaskName, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TerraformStatus.
func (in *TerraformStatus) DeepCopy() *TerraformStatus {
	if in == nil {
		return nil
	}
	out := new(TerraformStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TokenSecretRef) DeepCopyInto(out *TokenSecretRef) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TokenSecretRef.
func (in *TokenSecretRef) DeepCopy() *TokenSecretRef {
	if in == nil {
		return nil
	}
	out := new(TokenSecretRef)
	in.DeepCopyInto(out)
	return out
}
