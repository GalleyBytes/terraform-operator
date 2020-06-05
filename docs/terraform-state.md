# Terraform State 

Terraform must store state about your managed infrastructure and configuration. This state is used by Terraform to map real world resources to your configuration, keep track of metadata, and to improve performance for large infrastructures.

## Backend Default

By default, state is stored by in a Consul cluster that must be configured in the same cluster. To accomplish this, tf-operator ignores any backend configurations in a module and overwrites it with this Consul backend: 

```hcl
terraform {
  backend "consul" {
    address = "hashicorp-consul-server.tf-system:8500"
    scheme  = "http"
    path    = "terraform/${NAMESPACE}/${DEPLOYMENT}.tfstate"
  }
}
```

Organization of the tfstate files are handled by `${NAMESPACE}/${DEPLOYMENT}` name which will always be unique per resource.

### Consul

To set up a simple consule server, run the following commands:

```
git clone --single-branch --branch v0.19.0 https://github.com/hashicorp/consul-helm.git
helm upgrade --install hashicorp ./consul-helm --namespace tf-system --set server.replicas=1 --set server.bootstrapExpect=1
```

Hashicorp console must be installed with `--namespace tf-system` because it is hard-coded in [backend.tf](../docker/terraform/backend.tf).

Consul can easily be configured for HA simply by setting higher `server.replicas` and `server.bootstrapExpect` values.


## Overriding the Consul override

Often times, there is a need to use a different backend. Using the "custom backend" tf-operator configuration, users can define their own backend. This is done by simply adding the backend hcl in `spec.config.customBackend`. 

Example: 

```
apiVersion: tf.isaaguilar.com/v1alpha1
kind: Terraform
spec:
# (...)
  config:
    customBackend: |-
      terraform {
        backend "s3" {
          key            = "isaaguilar/use1/irsa-role-and-policy/tf-operator-example.tfstate"
          region         = "us-east-1"
          bucket         = "terraform-isaaguilar"
          dynamodb_table = "terraform-isaaguilar-lock"
          profile        = "isaaguilar"
        }
      }
```

In this example, the tfstate will be pushed to s3. To handle this properly, the user will also need to provide AWS credentials to the terraform-execution pod. See the [credentials.md](credentials.md) for details.

