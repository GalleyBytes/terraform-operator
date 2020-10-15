# Fetching Modules and Other Files

> THIS DOC IS A WORK IN PROGRESS

Every Terraform-operator resource requires a `spec.terraformModule`. The Terraform-operator resource also has [optional configurations](#config-sources) to get other files from Git.

See the Terraform definition of a [module](https://learn.hashicorp.com/tutorials/terraform/module#what-is-a-terraform-module).

## Terraform Module Source

Define which module to use in the spec by defining the address:
```yaml
(...)
spec:
  
    terraformVersion: 0.12.23
    terraformModule:
      address: https://github.com/cloudposse/terraform-aws-test-module.git
```

where:

- `spec.terraformVersion` - Version of Terraform used to deploy with
- `spec.terraformModule.address` - A git repo to the root Terraform module. See more on [source addresses](#source-address) below.


## Other Sources

When organizing Terraform, it is common to keep files like templates, tfvars, or other files outside of the Terraform module. Terraform-operator can fetch these files from Git by using `spec.sources`.

In this example, Terraform-operator will fetch files within the "policy" directory of the Git repo. 
```yaml
spec:
  sources:
  - address: https://github.com/isaaguilar/scratchspace.git//policy
```

where:

- `spec.sources[]` - List of Git sources to fetch files. See more on [source addresses](#source-address) below.

What Terraform-operator does is that it fetches the repo and reads files from the specified directory (or root if no directory is specified) into a Kubernetes ConfigMap. Then when the Terraform runner pod gets executed, it mounts the ConfigMap as a Volume and dumps the files into the root of the Terraform module inside the pod. 



## Source Address

Each `source` object must contain an `address`. Each `address` uses a single string URL as an input. The `address` input can also contain [query parameter](#query-parameters) and [subdirectory](#subdirectories) components.

### Query Parameter

The only query parameter supported at this time is `ref` which is a git parameter that tells it what ref to checkout for that Git repository.

```
https://github.com/cloudposse/terraform-aws-s3-bucket?ref=247b2877252af095c50e9c88be6de8ee359a45b7
```

Branches can also be used as refs.

```
https://github.com/cloudposse/terraform-aws-s3-bucket?ref=0.12/master
```

### Subdirectories

If you want to download only a specific subdirectory from a downloaded repo, you can specify a subdirectory after a double-slash `//`.

```
https://github.com/isaaguilar/simple-aws-tf-modules.git//s3-bucket
```

When defining `terraformModule.address`, the [query parameter](#query-parameters) and [subdirectory](#subdirectories) are the only available components to use. However, the "config" sources can have a few [extra](#source-extras) options. 

## Source Extras

...