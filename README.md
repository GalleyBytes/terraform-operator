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

Requires:

- go v0.13.x
- operator-sdk v0.15.1

Backend configs are overridden with a consul backend that must be installed in the cluster at `hashicorp-consul-server.default:8500`. This is a WIP and will probalby be enviromentalized for production. Set up a simple consule server by running the following commands:

```
git clone --single-branch --branch v0.8.1 https://github.com/hashicorp/consul-helm.git
helm3 install --namespace tf-system hashicorp ./consul-helm --set server.replicas=1 --set server.bootstrapExpect=1
```

**Hashicorp console must be installed into tf-system because [backend.tf](docker/terraform/backend.tf) has this hard-coded.**

## Install the Operator and CRDs

To get started, install the files in `deploy`

```
kubectl apply -f deploy/crds/tf.isaaguilar.com_terraforms_crd.yaml
kubectl apply -f deploy
```

### Cloud Provider Instructions

For deployments to a cloudProvider, terraform installing resources in the 
cloud will need credentials. These will be consumed via `cloudProfile`. 
For kiam and possibly other cloud credential providers, this poses an issue. 
Since a mechanism to install the kiam that `cloudProfile` will consume can't
be done via the operator, this depends on an external method to install the 
roles that will be used. I propose creating just one master role externally, 
and then a "create/update namespace" job can spawn namespaced kiam roles using 
the master credentials.  

### Simple Usage

The simplest form to use the operator is to download a simple tfmodule from git and pass in your vars through envs

```
apiVersion: tf.isaaguilar.com/v1alpha1
kind: Terraform
metadata:
  name: example
spec:

  stack:
    source:
      address: https://github.com/isaaguilar/simple-aws-s3-bucket.git

  config:
    cloudProfile: kiam-terraform
    env:
      - name: TF_VAR_profile
        value: isaaguilar
      - name: TF_VAR_region
        value: us-east-1                   # Bucket Region
      - name: TF_VAR_bucket
        value: terraform-operator-bucket-example
```


