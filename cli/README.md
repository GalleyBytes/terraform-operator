# Terraform Operator CLI [tfo]

A CLI tool to aid in discovering TFO managed pods and running debug sessions



## Installation

**Binary files will be created when the cli is released**

You can manually build the project by:

0. Requires golang 1.17
1. Download the repo https://github.com/isaaguilar/terraform-operator
2. Checkout the `kubectl-tfo` branch
3. Change directories into `cli`
4. Run the `go build` command

```bash
git clone https://github.com/isaaguilar/terraform-operator.git
cd terraform-operator
git checkout kubectl-tfo
cd cli
go build -o tfo main.go
mv tfo /usr/local/bin
```


## Usage

Run `tfo help` for all options.

### Show tfo resources

See the tfo resources in a namespace by running `tfo show`

```bash
tfo show
```

or

```bash
tfo show --namespace foo
```

### Open a **debug** session

```bash
tfo debug <tf-resource-name>
```

**Example:**

```bash
kubectl apply --namespace default -f - << EOF
apiVersion: tf.isaaguilar.com/v1alpha1
kind: Terraform
metadata:
  name: stable
spec:
  terraformModule: https://github.com/isaaguilar/simple-aws-tf-modules.git//create_file
  customBackend: |-
    terraform {
      backend "kubernetes" {
        secret_suffix    = "stable"
        in_cluster_config  = true
      }
    }
  ignoreDelete: false
EOF
```

Output should look like:

```
terraform.tf.isaaguilar.com/stable configured
```

Then run a debug pod:

```bash
tfo debug --namespace default stable
```

This command will create a pod on the cluster using the tfo resource for configuration. The pod puts the user in the terraform module.
```
Connecting to stable-2huxns3o-v3-debug-xhhtg.....

Try running 'terraform init'

/home/tfo-runner/generations/3/main$
```

Finally, debug and exit.

```bash
/home/tfo-runner/generations/3/main$ terraform plan
null_resource.write_file: Refreshing state... [id=2066474016391370391]

No changes. Your infrastructure matches the configuration.

Terraform has compared your real infrastructure against your configuration and found no differences, so no changes are needed.

/home/tfo-runner/generations/3/main$ exit
exit
```

Notice the debug pod terminates as soon as the user exits.

```bash
kubectl get po | grep stable-2huxns3o-v3-debug-xhhtg
stable-2huxns3o-v3-debug-xhhtg                 1/1     Terminating   0          4m20s
```

