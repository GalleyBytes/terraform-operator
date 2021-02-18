<p align="center">
<img src="https://s3.amazonaws.com/classic.isaaguilar.com/terraform-operator-logo.gif" alt="Terraform Operator"></img>
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

**Installation**

- [Installing Terraform-operator](docs/README.md#install-the-controller-and-crds) (Install using helm or kubectl)
- [Hello Terraform Operator](docs/README.md#hello-terraform-operator-example) (A very quick example of defining a resource)

**Configurations**

- [Terraform-state](docs/terraform-state.md) (Pushing State to consul, S3, etc.)
- [Terraform-provider credentials](docs/provider-credentials.md) (ie Cloud Credentials)
- [Operator Actions](docs/operator-actions.md) (Configuring when to run `terraform apply`)
- [Exporting TFvars](docs/extra-features.md#exporting-tfvars-to-git) (Saving your tfvars for reference elsewhere)
- [Pre/Post Run Scripts](docs/extra-features.md#the-pre-run-script) (Scripts that run before and after Terraform commands)
- [Terraform Runner Versions](docs/terraform-runners.md) (A list of officially supported Terraform Runners)

**Advanced Topics**

- [Git Authentication](docs/advanced/authentication-for-git.md) (Using SSH Keys and or Tokens with Git)
- [Using an SSH Proxy](docs/advanced/proxy.md) (Getting to Private and Enterprise Git Servers)

**Architecture**

- [Terraform Operator Design](docs/architecture.md) (The design overview of the Project)
- [Terraform Outputs](docs/architecture.md#outputs) (Finding Terraform outputs after running terraform)


## Development

Requires the following installed on your system:

- go >= v1.15.0
- operator-sdk ~ v1.4.0

