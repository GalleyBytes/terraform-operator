# terraform-operator

A Helm chart to deploy the terraform-operator-controller and CRD.

## Upgrading the Custom Resource Definition

Helm does not have support at this time for upgrading or deleting CRDs. Instead, update CRDs manually by running

```
kubectl apply -f crds/terraform.yaml
```

## Values

| Key | Description | Default |
|---|---|---|
| controller.affinity | `object` node/pod affinities | `{}` |
| controller.args | `list` additional arguments for the command | <a href="values.yaml#L22-L24">values.yaml</a> |
| controller.enabled | `bool` deploy the terraform-operator controller | `true` |
| controller.environmentVars | `object` key/value envs | `{}` |
| controller.image.pullPolicy | `string`  Set how kubernetes determines when to pull the docker image. | `"Always"` |
| controller.image.repository | `string` repo name without the tag | `"isaaguilar/terraform-operator"` |
| controller.image.tag | `string` tag of the image | `"v0.1.2"` |
| controller.nodeSelector | `object` node labels for pod assignment | `{}` |
| controller.replicaCount | `int` number of replicas | `1` |
| controller.resources | `object` CPU/Memory request and limit configuration | `{}` |
| controller.tolerations | `list` List of node taints to tolerate | `[]` |

