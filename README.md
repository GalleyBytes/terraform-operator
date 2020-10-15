<p align="center">
<img src="https://s3.amazonaws.com/classic.isaaguilar.com/terraform-operator-logo.gif" alt="Terraform Operator" width="50%"></img>
</p>

> A Kubernetes CRD and Controller to handle Terraform operations by generating k8s jobs catered to perform Terraform workflows

## What is terraform-operator?

This project is:

- A way to run Terraform in Kubernetes by defining Terraform deployments as Kubernetes manifests
- A controller that configures and starts Kubernetes Jobs when it sees changes to the Kubernetes manifest
- A Terraform runner which runs Terraform plan/apply, and can also perform pre and post scripts

This project is not:

- An HCL to YAML converter or vice versa
- A Terraform module definition

## Docs

- [Installing Terraform-operator](#install-the-controller-and-crds)
- [Hello Terraform Operator](#hello-terraform-operator)
- [Terraform-state](docs/terraform-state.md) (Pushing State to consul, S3, etc.)
- [Terraform-provider credentials](docs/provider-credentials.md) (ie Cloud Credentials)
- [Operator Actions](docs/operator-actions.md) (Configuring when to run `terraform apply`)
- [Exporting TFvars](docs/extra-features.md#exporting-tfvars-to-git) (Saving your tfvars for reference elsewhere)
- [Pre/Post Run Scripts](docs/extra-features.md#the-pre-run-script) (Scripts that run before and after Terraform commands)

**Advanced Topics**

- [Git Authentication](docs/advanced/authentication-for-git.md) (Using SSH Keys and or Tokens with Git)
- [Using an SSH Proxy](docs/advanced/proxy.md) (Getting to Private and Enterprise Git Servers)


## Architecture

Below is a diagram of the basic idea of the project

![](tfop_1.png)

The controller is responsible for fetching tfvars or other files, and then creates a Kubernetes Job to perform the actual terraform execution. By default, the Terraform-operator will save state in a Consul on the same cluster. Even though Consul is the default, other state backends can be configured.

## Install the Controller and CRDs

#### Install using Helm

```console
$ helm repo add isaaguilar https://isaaguilar.github.io/helm-charts
$ helm install isaaguilar/terraform-operator --namespace tf-system
```

> See [terraform-operator's helm chart](https://github.com/isaaguilar/helm-charts/tree/master/charts/terraform-operator) for options

#### Install using kubectl

First install the CRDs

```console
$ kubectl apply -f deploy/crds/tf.isaaguilar.com_terraforms_crd.yaml
```

Then install the controller

```console
$ kubectl apply -f deploy --namespace tf-system
```

Once the operator is installed, terraform resources are ready to be deployed.

Check out the [examples](examples) directory to see the different options tf-operator handles. See [complete-examples](examples/complete-examples) for realistic examples.

## Hello Terraform Operator 

> Create your first Terraform resource using Terraform-operator

Apply your first Terraform resource by running this _hello_world_ example:

```bash
printf 'apiVersion: tf.isaaguilar.com/v1alpha1
kind: Terraform
metadata:
  name: tf-operator-test
spec:
  
  terraformVersion: 0.12.23
  terraformModule:
    address: https://github.com/cloudposse/terraform-aws-test-module.git
  
  customBackend: |-
    terraform {
      backend "local" {
        path = "relative/path/to/terraform.tfstate"
      }
    }
  applyOnCreate: true
  applyOnUpdate: true
  ignoreDelete: true
'|kubectl apply -f-
```

Check the kubectl pod logs:

```
$ kubectl logs -f job/tf-operator-test
```

Delete the resource:

```bash
$ kubectl delete terraform tf-operator-test
```

> More examples coming soon!


## Development

Requires the following installed on your system:

- go v1.13.3
- operator-sdk v0.15.1

