# Deployer Example

Renders the least-privilege deployer policies for both scopes:

- `scope = "platform"`: the role that applies the root module;
- `scope = "tenant"`: a standalone tenant provisioning pipeline role.

`modules/deployer` attaches the policy to an existing role (`iam_role_arn`) and creates no roles.
Both roles must also be Lake Formation data-lake admins, which IAM cannot grant (see the module
README).

<!-- BEGIN_TF_DOCS -->
<!-- END_TF_DOCS -->
