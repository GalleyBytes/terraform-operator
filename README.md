# Terraform Operator

> A Kubernetes CRD and Controller to handle Terraform operations by generating k8s pods catered to perform Terraform workflows

<p align="center">
<img src="https://s3.amazonaws.com/classic.isaaguilar.com/tfo-worm-logo-text.png" alt="Terraform Operator Logo"></img>
</p>


## What is terraform-operator?

This project is:

- A way to run Terraform in Kubernetes by defining Terraform deployments as Kubernetes manifests
- A controller that configures and starts [Terraform Workflows](http://tf.isaaguilar.com/docs/architecture/workflow/) when it sees changes to the Kubernetes manifest
- Workflow runner pods that execute Terraform plan/apply and other user-defined scripts

This project is not:

- An HCL to YAML converter or vice versa
- A Terraform Module or Registry

## Installation

The preferred method is to use helm. See [Install using Helm](http://tf.isaaguilar.com/docs/getting-started/installation/#install-using-helm) on the docs.

Another simple method is to install the resources under `deploy` & `deploy/crds`

```bash
git clone https://github.com/isaaguilar/terraform-operator.git
cd terraform-operator
apply -f deploy/crds,deploy/
```

See [more installation options](http://tf.isaaguilar.com/docs/getting-started/installation/).

## Docs

Visit [http://tf.isaaguilar.com](http://tf.isaaguilar.com) to read the docs.

<p align="center">
<img src="https://s3.amazonaws.com/classic.isaaguilar.com/tfo-workflow-diagramv2.png" alt="Terraform Operator Workflow Diagram"></img>
</p>


## Related Projects and Tools

Here are some other projects that enhance the experience of Terraform Operator.


### Debug With `tfo` CLI

Terraform is great, but every now and then, a module takes a turn for the worse and the workflow fails. When this happens, a terraform workflow will need to be "debugged."

Fortunately, the `tfo` cli (https://github.com/isaaguilar/terraform-operator-cli) can be used to start a debug pod which is connected directly to the same terraform session the workflow runs.  It does so by reading the TFO resource and generates a pod with the same environment vars, ConfigMaps, Secrets, and ServiceAccount as a regular workflow pod. Then it drops the user in a shell directly in the main module.

```bash
tfo debug my-tfo-resource --namespace default
```

The user should be ready to rock-n-roll and show off their mad debugging skills.

```bash
Connecting to my-tfo-resource-ca6ajn94-v2-debug-qmjd5.....

Try running 'terraform init'

/home/tfo-runner/generations/2/main$
```

Happy debugging!


## Join the Community

Currently, I'm experimenting with a Discord channel. It may be tough when taking into account juggling a full time job and full time parenting, but I'll see what comes of it. Join the channel https://discord.gg/J5vRmT2PWg

