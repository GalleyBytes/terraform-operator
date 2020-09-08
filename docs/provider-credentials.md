# Provider Credentials

Cloud resources will require some form of credentials. The Terraform providers maintained by the large clouds generally have some form of authentication using environment variables. The terraform-operator can add credentials using environment variables via _Kubernetes Secrets_. 

Some Terraform Providers can use other methods of authentication. Terraform-operator currently supports the use of AWS IRSA or KIAM when deploying.

## Credentials using Kubernetes Secrets

Add provider credentials from environment variables by first creating the secret in Kubernetes. 

First, [configure all key-value pairs in a secret as container environment variables](https://kubernetes.io/docs/tasks/inject-data-application/distribute-credentials-secure/#configure-all-key-value-pairs-in-a-secret-as-container-environment-variables). Then add the following to the terraform-operator resource configuration to use these secrets:  


```yaml
apiVersion: tf.isaaguilar.com/v1alpha1
kind: Terraform
# (...)
spec:
  config:
    credentials: 
    - secretNameRef:
        name: aws-session-credentials
```


## AWS Credentials via KIAM

[KIAM](https://github.com/uswitch/kiam) runs as an agent on each node in your Kubernetes cluster and allows cluster users to associate IAM roles to Pods. 

Add the KIAM annotation to the terraform execution pod by adding the following config to the terraform-operator resource configuration:

```yaml
apiVersion: tf.isaaguilar.com/v1alpha1
kind: Terraform
# (...)
spec:
  config:
    credentials: 
    - aws:
        kiam: my-kiam-rolename
``` 

## AWS Credentials via IRSA (IAM Roles for Service Accounts)

With [IAM roles for service accounts](https://docs.aws.amazon.com/eks/latest/userguide/iam-roles-for-service-accounts.html) (IRSA) on Amazon EKS clusters, you can associate an IAM role with a Kubernetes service account. This service account can then provide AWS permissions to the containers in any pod that uses that service account. 

In order for the pod to be able to use this role, the "Trusted Entity" of the IAM role must allow this service account name and namespace.
	
The most secure way to use IRSA is by adding the specific `ServiceAccount` to the AWS TrustEntity config  with a "StringEquals" policy.
	
However, for a reusable policy consider "StringLike" with a few wildcards to make the IRSA role usable by pods created by terraform-operator. The example below is pretty liberal, but will work for any pod created by the terraform-operator.

```json
{
    "Version": "2012-10-17",
    "Statement": [
    {
        "Effect": "Allow",
        "Principal": {
        "Federated": "${OIDC_ARN}"
        },
        "Action": "sts:AssumeRoleWithWebIdentity",
        "Condition": {
        "StringLike": {
            "${OIDC_URL}:sub": "system:serviceaccount:*:tf-*"
        }
        }
    }
    ]
}
```

Add the following configuration to the terraform-operator resource configuration to use IRSA:

```yaml
apiVersion: tf.isaaguilar.com/v1alpha1
kind: Terraform
# (...)
spec:
  config:
    credentials: 
    - aws:
        irsa: arn:aws:iam::111222333444:role/my-irsa-role
```
