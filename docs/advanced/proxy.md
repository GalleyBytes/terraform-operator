# Using Proxy Servers

Terraform-operator can be configured to fetch resources through an SSH Proxy. This is sometimes required to fetch repos on a private git server. Or perhaps it is getting to an Enterprise Github behind a firewall.  

Whatever the case may be, configure Terraform-operator to use a proxy server by adding the `spec.sshTunnel` to the Terraform Kubernetes resource. 

```yaml
# terraform.yaml
# (...)
spec:
  sshTunnel:
    host: 172.18.0.1
    user: root
    sshKeySecretRef:
      name: my-ssh-key
      key: id_rsa
```

This will require an [SSH Key Secret](ssh-keys.md) to exist before trying to use the proxy.

## Limitations of SSH Tunnel

The proxy only operates when fetching "sources". This includes fetching "modules" in the `terraform init` command. Here's what can and can't be done with the proxy:

| Protocol | Can pull `spec.terraformModule` | Can pull `spec.sources[]` |
|---|---|---|
| SSH (eg `git@github.com:user/repo.git`) | [x] | [x]|
| HTTPS (eg `https://github.com/user/repo.git`) | [] | [x] |


In other words, `sshTunnel` can perform pulling Git over SSH. `sshTunnel` can not pull the `terraformModule` over the HTTPS protocol. It can pull other source files, however.