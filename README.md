<p align="center">
<img src="https://s3.amazonaws.com/classic.isaaguilar.com/terraform-operator-logo.gif" alt="Terraform Operator"></img>
</p>

> A Kubernetes CRD and Controller to handle Terraform operations by generating k8s jobs catered to perform Terraform workflows

<hr/>
<center>:warning: :warning: :warning:</center>

**master branch is currently under heavy development.** The Official documentation page is moving to [http://tf.isaaguilar.com](http://tf.isaaguilar.com). Docs and examples for version of Terraform Operator < `v0.4.0` can be found in branch: https://github.com/isaaguilar/terraform-operator/tree/master-v0.3.x/docs

Still trying to get a road map.

**Stay tuned!**

<hr/>


## What is terraform-operator?

This project is:

- A way to run Terraform in Kubernetes by defining Terraform deployments as Kubernetes manifests
- A controller that configures and starts [Terraform Workflows](docs/architecture.md) when it sees changes to the Kubernetes manifest
- Workflow runner pods that execute Terraform plan/apply and other user-defined scripts

This project is not:

- An HCL to YAML converter or vice versa
- A Terraform Module or Registry


## Docs

Visit [http://tf.isaaguilar.com](http://tf.isaaguilar.com) for docs for version >= `v0.4.x`.
