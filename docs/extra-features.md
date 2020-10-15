# Extra Features

In addition to running Terraform, the Terraform-operator can perform additional tasks that come in handy when dealing with Terraform. 


## Exporting "tfvars" to Git

When working with Terraform, there is sometimes a need to do "Terraform" operations outside of the standard init/plan/apply workflow. Terraform-operator makes it easy to export an aggregate of the tfvars that are used in a deployment to a Git repo. 

```yaml
(...)
spec:

  exportRepo:
    address: git@<repo>.git
    tfvarsFile: path/to/tfvars/export.tfvars
    confFile: path/to/tfvars/export.conf
```

Where:

- `spec.exportRepo.address` - (required) The git url to reach the target git repo
- `spec.exportRepo.tfvarsFile` - (optional) The full-path where to save the tfvars file. The extension `.tfvars` is not automatically added in case the user has their own convention for this.
- `spec.exportRepo.conf` - (optional) This is the backend-config used for the resource. This is usually going to be the same same as the [`backendOverride` definition](terraform-state.md#custom-terraform-backend).

## The Pre-run Script

Sometimes it is necessary to run a script before any Terraform commands get run. This can be accomplished by writing an inline script into the Kubernetes manifest file and following a few guidelines. This runs before Terraform init/plan/apply.

Here's a few guidelines for an inline script: 

- Make sure to include the shebang. 
- It's a good idea to test scripts before running this in tf-operator to make sure all the tools are installed on the image. Try pulling the terraform-execution image. (Eg isaaguilar/tfops:0.12.23) 
- You may have to install your own packages to run some commands. 

```yaml
(...)
spec:
  
    prerunScript: |-
      #!/usr/bin/env bash
      echo "Setting up the lambda deployment by pulling in the zip from S3"
      if [ -z `which pip` ];then 
        apk add --update-cache python python-dev py-pip build-base 
      fi
      if [ -z `which aws` ];then
        pip install awscli
      fi
      aws s3 cp s3://my-lambda-builds/app-v1.0.0.zip app-v1.0.0.zip
```

## The Post-run Script

Similar to the pre-run script except this runs after the Terraform commands. The same guidelines apply. This runs after Terraform init/plan/apply. 

```yaml
(...)
spec:

  postrunScript: |-
    #!/usr/bin/env bash
    echo "Saving output to S3"
    if [ -z `which pip` ];then 
      apk add --update-cache python python-dev py-pip build-base 
    fi
    if [ -z `which aws` ];then
      pip install awscli
    fi

    ### Let's pretend there is an output.txt that the tf module creates
    #
    aws s3 cp output.txt s3://my-terraform-bucket/output.txt
```

## Advanced Discussion about Pre/Post Run Scripts

Prerun and postrun scripts are executing by running `./prerun.sh` and `./postrun.sh` respectively. So by adding values to `spec.prerunScript` or `spec.postrunScript`, the operator creates the `prerun.sh` and `postrun.sh` files respectively. 

However, a user can opt to "import" these files from Git by understanding how `spec.sources` work. 

For example, if this config was added to the Kubernetes manifest: 

```yaml
(...)
spec:

  sources:
  - address: https://github.com/mygit/repo.git//path/to/prerun.sh
    extras:
    - is-file
```

The Terraform-operator will add `prerun.sh` exactly to the same directory that the inline script would have added `prerun.sh`. 

This might be a little farfetched to reason about, but it is available for users who want to include very long and complicated prerun and postrun scripts. Essentially, a user can use `spec.sources` to import many scripts and string them together as long as there is a script called `prerun.sh` or `postrun.sh` to use as the entry-point. 