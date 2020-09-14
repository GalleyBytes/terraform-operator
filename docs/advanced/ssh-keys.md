# SSH Key Credentials

Some actions by Terraform-operator will require Authentication via SSH Keys. This could include:

- Git authentication over SSH (eg pulling `git@github.com:user/repo.git`)
- Using terraform-operator's [SSH Proxy Server](proxy.md) authentication

## Kubernetes Secret Requirements for the SSH Key

SSH Key Secrets should be:

1. created before the Terraform Kubernetes resource is created. 
2. in the same namespace as the Terraform Kubernetes resource.


### Example Secret

```yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:  
  name: my-ssh-key
data:
  id_rsa: <base64-encoded-ssh-key>
```

## Usage

Currently the Terraform Kubernetes resource can only use SSH Keys for Git and Proxy Servers. Here's an example of these configurations:

```yaml
# terraform.yaml
(...)
spec:
  sshProxy:
    host: 172.18.0.1
    user: root
    sshKeySecretRef:
      name: my-ssh-key
      key: id_rsa

  scmAuthMethods:
  - host: github.com
    git:
      ssh:
        sshKeySecretRef:
          name: my-git-ssh-key
          key: id_rsa
```

The default `sshKeySecretRef.key` lookup in the Secret is `id_rsa`. If `sshKeySecretRef.key` is not `id_rsa`, then it must be specified.

See the authentication example for [Git over SSH](authentication-for-git.md#ssh) and [SSH Proxy](proxy.md) for more details.