# terraform-operator: **WIP**
Kubernetes CRD to handle terraform operations.

This project is not ready for usage. The concept is still being developed.  Lots of work needs to be done into the following:

- status of a deployment to be used in reconciliation
- `terraform.state` file management
    - This would be perfect with consul deployed in the same cluster. This can be deployed as part of the terraform-operator, otherwise a pre-requisite to the terraform-operator installation. 
- cloud provider permissions to deploy cloud resources
- Use a shared filesystem so pods could read/write modules to it
- and lots lots more to consider...

Below, however, is a functional example of the basic idea of the project.

## Setting up Development Cluster (WIP/POC)

Backend configs are overridden with a consul backend that must be installed in the cluster at `hashicorp-consul-server.default:8500`. This is a WIP and will probalby be enviromentalized for production. Set up a simple consule server by running the following commands: 

```
git clone --single-branch --branch v0.8.1 https://github.com/hashicorp/consul-helm.git
helm install --name hashicorp ./consul-helm --set server.replicas=1 --set server.bootstrapExpect=1
```


## Install the Operator and CRDs

To get started, install the files in `deploy`

```

kubectl apply -f deploy
kubectl apply -f deploy/crds/tf_v1alpha1_terraform_crd.yaml
```

### Simple Usage

The simplest form to use the operator is to download a simple tfmodule from git and pass in your vars through envs

```
apiVersion: tf.isaaguilar.com/v1alpha1
kind: Terraform
metadata:
  name: example-terraform
  labels:
    type: example-tfapply
spec:
  stack:
    source:
      address: https://github.com/isaaguilar/Terraform_create_file.git?ref=5668434b4f73ccdc0c62509b78e3a1d3612c4711

  config:
    env:
      - name: TF_VAR_identifier
        value: tfops
```

The state file is saved to `example-terraform-tfstate` as a configmap. Right now, the operator never returns a `success` in order for the operation on the terraform CR to continue going while it's in development. Will add statuses soon (I think).

