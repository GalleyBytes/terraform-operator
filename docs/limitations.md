# Limitations


## Terraform Module Permissions

Terraform runs can only be configured with a single github token (https) and key (ssh) for the module(s) you run in a single Terraform command. 

Here is a snipped from Terraform on this: 

> Permissions
>
> When you authenticate with a user token, you can access modules from any organization you are a member of. (A user is a member of an organization if they belong to any team in that organization.)
>
> Within a given Terraform configuration, you should only use modules from one organization. Mixing modules from different organizations might work on the CLI with your user token, but it will make your configuration difficult or impossible to collaborate with. If you want to use the same module in multiple organizations, you should add it to both organizations' registries. (See Sharing Modules Across Organizations.)

Ref: https://www.terraform.io/docs/cloud/registry/using.html#permissions

