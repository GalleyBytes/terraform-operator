# terraform-operator:
Kubernetes CRD to handle terraform operations.

This project is not ready for production usage, but is in a functional state. The following are areas that can be handled better:

- status of a deployment to be used in reconciliation
- cloud provider permissions to deploy cloud resources

Below is a diagram of the basic idea of the project:

![](tfop_1.png)

## Development

Requires:

- go v0.13.x
- operator-sdk v0.15.1

## Terraform State

The Terraform state configs are overridden with a consul backend that must be installed in the cluster at `hashicorp-consul-server.tf-system:8500`. Set up a simple consule server by running the following commands:

```
git clone --single-branch --branch v0.19.0 https://github.com/hashicorp/consul-helm.git
helm3 upgrade --install hashicorp ./consul-helm --namespace tf-system --set server.replicas=3 --set server.bootstrapExpect=3
```

**Hashicorp console must be installed into tf-system because [backend.tf](docker/terraform/backend.tf) has this hard-coded.**

Consul can easily be configured for HA by adding more servers by setting higher `server.replicas` and `server.bootstrapExpect` values. 

## Install the Operator and CRDs

To get started, install the files in `deploy`

```
kubectl apply -f deploy/crds/tf.isaaguilar.com_terraforms_crd.yaml
kubectl apply -f deploy --namespace tf-system
```

#### Cloud Provider Considerations

For deployments to a cloudProvider, terraform installing resources in the 
cloud will need credentials. These will be consumed via `cloudProfile`. 
For kiam and possibly other cloud credential providers, this poses an issue. 
Since a mechanism to install the kiam that `cloudProfile` will consume can't
be done via the operator, this depends on an external method to install the 
roles that will be used. I propose creating just one master role externally, 
and then a "create/update namespace" job can spawn namespaced kiam roles using 
the master credentials.  

### Creating Terraform Resources

One way to deploy a Terraform resource is to define a stack "git-source" and a tfvars "git-source" config.

```
apiVersion: tf.isaaguilar.com/v1alpha1
kind: Terraform
metadata:
  name: irsa-role-and-policy-example
spec:
  stack:
    source:
      address: git@<tf-module-repo>
  config:
    terraformVersion: 0.12.23
    applyOnCreate: true
    applyOnUpdate: true
    applyOnDelete: true
    cloudProfile: admin-cloud-credentials
    sources:
    - address: git@<tf-var-repo>
```


