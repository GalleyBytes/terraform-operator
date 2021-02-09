# Install the Controller and CRDs

## Install using Helm

```console
$ helm repo add isaaguilar https://isaaguilar.github.io/helm-charts
$ helm install terraform-operator isaaguilar/terraform-operator --namespace tf-system --create-namespace
```

> See [terraform-operator's helm chart](https://github.com/isaaguilar/helm-charts/tree/master/charts/terraform-operator) for options

## Install using kubectl

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

## Hello Terraform Operator Example

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

> See lots more example in the [example directory](../examples)!
