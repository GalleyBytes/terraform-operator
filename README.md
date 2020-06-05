# terraform-operator:
Kubernetes CRD to handle terraform operations. Below is a diagram of the basic idea of the project:

![](tfop_1.png)

## Docs

- [terraform-state](docs/terraform-state.md)
- [credentials](docs/credentials.md) (eg cloud credentials and ssh keys)

## Install the Operator and CRDs

To get started, install the files in `deploy`

```
kubectl apply -f deploy/crds/tf.isaaguilar.com_terraforms_crd.yaml
kubectl apply -f deploy --namespace tf-system
```


### Creating Terraform Resources

Check out the [examples](examples) directory to see the different options tf-operator handles. Many of these options can be combined for advanced deployments. See [complete-examples](examples/complete-examples) for realistic examples. 

## Development

Requires the following installed on your system:

- go v0.13.x
- operator-sdk v0.15.1


