# Git Tokens

Terraform-operator allows tokens to Authenticate with Git private repos.

## Kubernetes Secret Requirements for the Token

"Token" Secrets should be:

1. created before the Terraform Kubernetes resource is created. 
2. in the same namespace as the Terraform Kubernetes resource.

### Example Secret

```yaml
apiVersion: v1
kind: Secret
type: Opaque
metadata:  
  name: my-git-token
data:
  id_rsa: <base64-encoded-git-token>
```

## Usage

See the authentication example for [Git over HTTPS](authentication-for-git.md#https).