# Operator Actions

Terraform-operator has several options it can handle when it comes to running `terraform apply` on a given deployment. 

## Options

These options are configurable directly under `spec` in the Terraform Kubernetes manifest.

`applyOnCreate` -  Automatically apply Terraform the first time the Kubernetes resource is created. A "first time run" is when the Kubernetes manifest for the Terraform resource has a `metadata.generation` equal to 1.
    

`applyOnUpdate` - Automatically apply Terraform when the Kubernetes resource is updated. An "update" is when the Kubernetes manifest for the Terraform resource has a `metadata.generation` greater than 1.
    

`applyOnDelete` - Automatically apply a `terraform -destroy` when the Kubernetes resource is deleted. _Terraform-operator will not run a destroy command when `ignoreDelete` is set to `true`._
    
`ignoreDelete` - Do not execute a destroy when the Kubernetes resource gets deleted.


## When apply is false

When any of the `applyOn` vars are set as `false`, the user must manually update a Kubernetes ConfigMap.

### 1. Getting the ConfigMap Name

The configMap name is the "`Terraform Kuberentes Resource Name`" + "`-action`". When in doubt, the logs of the pod running Terraform will print the ConfigMap name to update. 

### 2. Updating the ConfigMap

 Update the value of `data.action` with one of following two options:

 - "`apply`" - Will continue the script with `terraform apply` and any [postRun scripts](extra-features.md#the-post-run-script).
 - "`abort`" - Will exit the terraform-execution pod






