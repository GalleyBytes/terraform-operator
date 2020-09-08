# SSH-key and Token management

Tokens and SSH keys can be stored in Kubernetes as Secrets and then pulled into terraform-operator when fetching resources. terraform-operator does not create these secrets so work must be done before terraform-operator can use it. 

## Creating a SSH-key Secret

The SSH key Secret will require: 

- the private key and
- that the Secret is created in the same namespace as the terraform resource. 

It is also recommended that the private key is the value of `data.id_rsa`, although, this is not required.  Here's an example of a Kubernetes Secret that can be used by terraform-operator:


```
apiVersion: v1
kind: Secret
type: Opaque
metadata:  
  name: terraform-operator-git-sshkey
  namespace: dev
data:
  id_rsa: LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQpNSUlFcEFJQkFBS0NBUUVBeWM1OXVlRVVBUjRscERFT2I3REl3U0FmZFpQY0ZIVis4UDYzT2g1SE03QlJIN0FrCkNzdTd0MmQ0RTdUN0tVU3EwQ1h0cW5kdjdnZFg4QXN5V1g5eW5XRDFaS0xiNG1QcnVmWFJCMnBUVDIyay9IRWsKUlpnVHZFMGhUUUV6UVR0NlFtLzk3bVpuZDdzc3hTRFNCa0FuVUlEQjhqOEFPQlp2alRFbnduUDVYcmVoOTVaQQpyeHpKVEs5ZTlGaG5VMmhsV3Rud01wOTFYTGJwSnBHcUlxcE5QMzdlK2Y4RE9Rb3N2L3ZzKzR1RWRUSDVFNmF4CjhMeTNQdmh3QTNGVlpVTnhXYVl5WGRpQTRodWtXRmZqNndNKy9sTHFETFMxbzM4T2RQRGllS3UxeUFDOUl1WGsKejhIejJxWjYrc1hRNTdqMkpoSFZHM2ZOa0V1N2ZPUUNyeW5ldXdJREFRQUJBb0lCQVFDWGQzR3NJd0JsdWwvYwpOYW0xTVFYczFoUm1wbnpIcWt5RnkxaHd1YXNOWTZmdjFiK25qclNzK204SXM0elRzNk5WS1RLU0FLVTFEYlAyCkNpRlhSUzRjYTFxamx3emNoY3kydllhUFAwR2FXeHc3RVJ4OVU2QjBjNXVyOVZ1bitXRlJIa2VFT0w0dUFvR2UKejN4empwRXpmZ0NUdHErT2FXQitvOGRJenN6N1JoaU1sVnQzR2xkZlhBR0UzajFxWkkzeXA3QThhQzRvU3pKLwpjRFhKMFVtZXVxa3UyZ3dWYTBNVHVQQmR2UXZQejFCc200ZXlEbStsWERBTUN1RWZwMUZmeG5WcnhyeXVmWnVpCmhVblpJUjQwVVZUU3VURUw1aXJUZGc1VlQ0R3pDUzJxQUVIT2NTVFpwUXc0RG9BZko0YkdjVExKTFB4cStqZWwKY3lYQlR2SFJBb0dCQVBWVmNTaFA5aVpXRlBLdzZZT3A3RmcrblByb1hJTDNtQmRBUkk5WGxXN0M3NkZWL2JvcQppaU91Wi9CT0N4MUZwOXFEWHhGYjBzaWZvSkhWM0Q2TFBvaXNBOGhWTW1uazl1dktoYzVsYnU1WE5uQk5idHBHCjRzVlUxQVZnSDR5NjVwTlh3MWVkb1l3NzQ4aytmQStldHNaaFVGcDE5a0drYVZ2MW9VNnJZSlBqQW9HQkFOS1UKbDlUYzRWVUVzSWwvVFFIUUtmaDVSTmsvaWNxb0pXNDhxTm9rMWlOVFJYMU51RVhoS0UvNm1ib2V3Z2Nqa3V5TwpCWFR0NEp6UXRwTmlXd1I4MlBFZE44ejc3djcrekFRbHY1TkRkUm81QlBvV2Q3Tmxoa2htRk43cEN0eWZHNFh1CmVTNFhoWmtRTGs2RFJSM3c4a3JzNWNkWTVKMWZQS0ZOc2hKSHJmRkpBb0dCQU01VXM3eWhzM1YraEZPd01sU0gKZnJ5Z3ZFblJUcXpmSzB5eXduYUR4S3ZJeXR5M2c1TWszOVV1Z3ovNWd5TjFSN3hoTEgxZTZxSE1qckRZV2tsSAp0cW9mY1hiMUlGY3JOL2dLOWdvbUNPdnU4VnYxNDdzMFR0aURoV1dYK0REVnA4SlgxM1JDb0hGZWxTN1ZuR1ZPCnFJMmpubjdXSXV3R0tJNHN3U04yd3R6ZEFvR0FHcmJURkNQNVNnblFRNEVzeWNBWXN2YmZieGdLYVBVdjJtNUQKbFhqNjJYeGs0bUtMc0FIQ1ZYTWJNV3RaZmdKYlR6c3RJZ3BUWmxGcitBS1FQVitCUGdWUTRPWk5DWGhWZFdrOApobmdXVVA5T3pGTXhXRWJXNURSZkRYQk8rbklNMGM3US9MSHJOdUhBbmlFMUVYbFJvNE91R3I0Q01weTBXbG82Cjd1cTgvRkVDZ1lBREduQTMrWDR1QVRpSWo4T3MrSUlrSmFIbmtoM0JsOHZEbzV1YjdDT2dsYnN6NU1xZHRMN3QKZFhSTWxkUWFtd2d5U1Bma2V0OUowdFdXSlFGVUEzQjQvQUJXZU9JaTdKek9EdWtVeHc2eWlFNkxoTkFjNGJxdApkZ3h0aW9iYjVqVzV2b2didDJGelFrU2doNlFERktJUkZpY1pJU1lGL1h5Nkx3TGlLU2ZNUEE9PQotLS0tLUVORCBSU0EgUFJJVkFURSBLRVktLS0tLQo=
```

Both the Git and Tunnel SSH-key follow the same guidelines.

#### Configuration of `sshKeySecretRef` in the terraform-resource

For both Git and Tunnel, the `sshKeySecretRef` is used to reference an existing Kubernetes Secret. `sshKeySecretRef` is added to Git and Tunnel configs separately, but they are configured the same. 

```
# sshKeySecretRef snippet
...
sshKeySecretRef:
  name: example  # the name of the Secret
  key: id_rsa    # the data.[key] of the Secret which contains the ssh key. 
                 # If "id_rsa" is the data.[key], this field can be omitted
...
```


## Creating a Git Token Secret

The Git token Secret will require:

- that the Secret is created in the same namespace as the terraform resource.

If is also recommended that the token is the value of `data.token`, although, this is not required. Here's an example of a Kubernetes secret that can be used by terraform-operator:

```
apiVersion: v1
kind: Secret
type: Opaque
metadata:  
  name: terraform-operator-git-token
  namespace: dev
data:
  token: WERBTUN1RWZwMUZmeG5WcnhyeXVmWnVp=
```

#### Configuration of `tokenSecretRef` in the terraform-resource

When configuring a git-token in the terraform-resource, the `tokenSecretRef` would be configured with the Secret name and optionally the data key. 

```
# tokenSecretRef snippet
...
tokenSecretRef:
  name: example  # the name of the Secret
  key: token     # the data.[key] of the Secret which contains the git token.
                 # If "token" is the data.[key], this field can be omitted
...
```

# SSH-key and Token Usage

Keys and tokens are can be used by SSH Tunnel and Git repo pulls. 

## SSH Tunnel

The terraform-operator can be configured to fetch resources through an SSH tunnel which is sometimes required to fetch git repos. To configure this, first make sure the Kubernetes Secret exists, then add `spec.sshProxy` configuration in the terraform resource: 

```
apiVersion: tf.isaaguilar.com/v1alpha1
kind: Terraform
# (...)
spec:
  sshProxy:
    host: 10.151.36.97
    user: ec2-user
    sshKeySecretRef:    # See Configuration of `sshKeySecretRef` above
      name: proxysshkey
```

#### Limitations of SSH Tunnel

The SSH tunnel is fully compatible with fetch Git over SSH for pulling tf-modules (stacks) and configs. SSH tunnel is not compatible with Git over HTTPS when pulling tf-modules. 

## Git over SSH or HTTPS via `scmAuthMethods` 

The configuration for terraform-operator to pull tf-moudles, tfvars, or other files over secure repos require some kind of authentication. `scmAuthMethods` allows the user to define authentication methods to different resources. This configuration block has fields for `host` and repo-specific fields for that host. 

### Git over SSH

When pulling any Git repo over SSH, a key must always be provided. To configure terraform-operator to pull Git over SSH, first make sure the Kubernetes Secret exists. Then add an appropriate `scmAuthMethod` in the terraform-resource. 

```
apiVersion: tf.isaaguilar.com/v1alpha1
kind: Terraform
# (...)
spec:
  scmAuthMethods:
  - host: github.com
    git:
      ssh:
        requireProxy: true  # Require proxy true will need an sshProxy config (see docs above)
        sshKeySecretRef:
          name: gitsshkey
```

When a proxy is required, configure the `sshProxy` field in spec and set `requireProxy` to `true`.

### Git over HTTPS

When pulling Git over HTTPS, a token must be provided when the git repo is private. This token must be configured in `scmAuthMethods`. To configure terraform-operator to pull Git over HTTPS to a private repo, first make sure the Kubernetes Secret exists. Then add an appropriate `scmAuthMethod`.

```
apiVersion: tf.isaaguilar.com/v1alpha1
kind: Terraform
# (...)
spec:
  scmAuthMethods:
  - host: github.com
    git:
      https:
        requireProxy: false
        tokenSecretRef:
          name: gittoken
```

