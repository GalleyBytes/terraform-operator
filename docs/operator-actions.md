# Operator Actions

Terraform-operator has several options it can handle when it comes to running `terraform apply` on a given deployment. 

## Options

These options are configurable directly under `spec.config` in Terraform Kubernetes manifest.

`applyOnCreate` -  Automatically apply Terraform the first time the Kubernetes resource is created. A "first time run" is when the Kubernetes manifest for the Terraform resource has a `metadata.generation` equal to 1.
    

`applyOnUpdate` - Automatically apply Terraform when the Kubernetes resource is updated. An "update" is when the Kubernetes manifest for the Terraform resource has a `metadata.generation` greater than 1.
    

`applyOnDelete` - Automatically apply a `terraform -destroy` when the Kubernetes resource is deleted. _Terraform-operator will not run a destroy command when `ignoreDelete` is set to `true`._
    
`ignoreDelete` - Do not execute a destroy when the Kubernetes resource gets deleted.
    