# Authenticated Git over SSH or HTTPS

Configure Terraform-operator to use a specific SSH key or a Token by specifying the `spec.scmAuthMethod` in the Terraform Kubernetes resource. 

## SSH

```yaml
# terraform.yaml
(...)
spec:
  scmAuthMethods:
  - host: github.com
    git:
      ssh:
        requireProxy: true  # Require proxy true will need an sshProxy config (see docs above)
        sshKeySecretRef:
          name: my-ssh-key
          key: id_rsa
```

Where:

- `spec.scmAuthMethods[].host` - the SSH key to use when the source address matches the protocol and this host
- `spec.schAuthMethods[].git.ssh.requireProxy` - will send any Git fetch or clone commands thru an [SSH proxy](proxy.md). Default is `false`.
- `spec.schAuthMethods[].git.ssh.sshKeySecretRef.name` - is the Kubernetes Secret containing the [SSH key](ssh-keys.md)
- `spec.schAuthMethods[].git.ssh.sshKeySecretRef.key` - is the key within the Kubernetes Secret which has the token. This value defaults to `id_rsa`.

## HTTPS

```yaml
# terraform.yaml
(...)
spec:
  scmAuthMethods:
  - host: github.com
    git:
      https:
        requireProxy: false
        tokenSecretRef:
          name: my-git-token
          key: token
```

Where:

- `spec.scmAuthMethods[].host` - the SSH key to use when the source address matches the protocol and this host
- `spec.schAuthMethods[].git.https.requireProxy` - will send any Git fetch or clone commands thru an [SSH proxy](proxy.md). _This option is not recommended for pulling modules_. Default is `false`.
- `spec.schAuthMethods[].git.https.tokenSecretRef.name` - is the Kubernetes Secret containing the [Git Token](git-tokens.md)
- `spec.schAuthMethods[].git.https.tokenSecretRef.key` - is the key within the Kubernetes Secret which has the token. This value defaults to `token`.