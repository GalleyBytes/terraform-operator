# terraform-operator:
Kubernetes CRD to handle terraform operations.

## Architecture

Below is a diagram of the basic idea of the project

![](tfop_1.png)

The controller is responsible for fetching tfvars or other files, and then creates a Kubernetes Job to perform the actual terraform execution. By default, the Terraform-operator will save state in a Consul on the same cluster. Even though Consul is the default, other state backends can be configured.

## Docs

- [terraform-state](docs/terraform-state.md)
- [credentials](docs/provider-credentials.md) (eg cloud credentials)
- [other-credentials](dos/other-credentials.md) (eg ssh key)

## Install the Operator and CRDs

To get started, install the files in `deploy`

```
kubectl apply -f deploy/crds/tf.isaaguilar.com_terraforms_crd.yaml
kubectl apply -f deploy --namespace tf-system
```

## Create Terraform Resources

Once the operator is installed, terraform resources are ready to be deployed.

Check out the [examples](examples) directory to see the different options tf-operator handles. See [complete-examples](examples/complete-examples) for realistic examples.

### Creating your first terraform

Apply your first terraform resource by running a _hello_world_ example:

```bash
$ printf 'apiVersion: tf.isaaguilar.com/v1alpha1
kind: Terraform
metadata:
  name: tf-operator-test
spec:
  stack:
    terraformVersion: 0.12.23
    source:
      address: https://github.com/cloudposse/terraform-aws-test-module.git
  config:
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


