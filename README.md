# AWS Lakehouse Terraform Module

Terraform module that provisions a multi-tenant, Apache Iceberg-based lakehouse platform on AWS: S3 Tables
storage in data zones, the federated Lake Formation catalog, tag-based governed access down to columns, and a
query workgroup per tenant. It follows Brockhoff Cloud standards and integrates with the
`terraform-external-context` module for consistent naming and tagging.

Design principles for this module are recorded in [`.specify/memory/principles.md`](.specify/memory/principles.md).
The design and its verified deviations are in [`specs/001-governed-lakehouse/plan.md`](specs/001-governed-lakehouse/plan.md).

## Features

- **Multi-Tenant Platform**: each tenant is an S3 Tables namespace (a Lake Formation database) in every zone it
  uses, provisioned by `modules/tenant` from the platform's `tenants` map or standalone by a tenant pipeline
- **Data Zones**: raw, curated, and consumption (gold) table buckets with table maintenance and fail-closed access
- **Semantic Layer Ready**: consumption-only reader grants let an external semantic layer serve published data products
- **Open Table Catalog**: Apache Iceberg tables in the federated `s3tablescatalog`, queryable from Athena and other engines
- **Governed Access**: LF-tag access by tenant, zone, sensitivity, regulation, and domain on databases, tables, and
  columns, with delegated tenant stewards and row filters
- **Query Isolation**: an Athena workgroup per tenant with encrypted, isolated results, scan limits, and alarms
- **Environment-Aware**: security, durability, monitoring, and cost defaults derived from `environment_type`
- **Encryption by Default**: a customer-managed KMS key created by default, or bring your own
- **Compliance-Ready**: SOC 2, HIPAA, PCI DSS, and GDPR support; dedicated keys and storage only for designated tenants
- **Cost Visibility**: idle monthly estimates for the platform and each tenant, compliance cost, and usage rates
- **Least-Privilege Deployment**: `modules/deployer` policies for the platform and for tenant pipelines

## Usage

The module is not yet published to the Terraform Registry. Reference it by Git source pinned to a release tag.

### Basic Example

```hcl
provider "aws" {}

# Cost estimation uses the AWS Price List API in us-east-1
provider "aws" {
  alias  = "pricing"
  region = "us-east-1"
}

module "lakehouse" {
  source = "git::https://github.com/kbrockhoff/terraform-aws-lakehouse.git?ref=vX.Y.Z"
  providers = {
    aws         = aws
    aws.pricing = aws.pricing
  }

  name_prefix      = "dev-usw2"
  environment_type = "Development"

  # Authoritative Lake Formation admins; must include the role running Terraform
  governance_config = {
    admin_principal_arns = ["arn:aws:iam::123456789012:role/lakehouse-deployer"]
  }

  tenants = {
    analytics = {
      owner       = "analytics-data@example.com"
      sensitivity = "internal"
    }
  }
}
```

After the first apply, run the [required one-time grants](#required-one-time-grants).

### More Examples

- [`examples/defaults`](examples/defaults): required inputs and one tenant
- [`examples/complete`](examples/complete): Production, three tenants (one HIPAA-designated), every persona type, a row filter
- [`examples/secondary`](examples/secondary): a second platform in an account whose catalog integration another deployment owns
- [`examples/standalone-tenant`](examples/standalone-tenant): a tenant applied by its own pipeline from the platform contract
- [`examples/deployer`](examples/deployer): the least-privilege deployer policies for both scopes

## Environment Type Configuration

`environment_type` sets resource defaults. Ephemeral, Development, and Testing are **Baseline**; UAT, Production, and
MissionCritical are **Enhanced**. `None` takes every value from the `*_config` inputs.

| Type | RPO | RTO | Writer `DROP` | Required tags enforced | Query results kept | Scan cutoff | Default table storage class |
|------|-----|-----|---------------|------------------------|--------------------|-------------|-----------------------------|
| `Ephemeral` | N/A | 48h | yes | no | 1 day | 10 GB | STANDARD |
| `Development` | 24h | 48h | yes | no | 7 days | 10 GB | STANDARD |
| `Testing` | 24h | 48h | yes | no | 7 days | 10 GB | STANDARD |
| `UAT` | 12h | 24h | no | yes | 30 days | 1 TB | INTELLIGENT_TIERING |
| `Production` | 1h | 4h | no | yes | 30 days | 1 TB | INTELLIGENT_TIERING |
| `MissionCritical` | 5m | 1h | no | yes | 30 days | 1 TB | INTELLIGENT_TIERING |

Override individual values with `query_config`, `storage_config`, `governance_config.writer_drop_enabled`,
`required_tags_config.enforced`, and `alarms_config.scanned_bytes_threshold`; tenants can override their scan
cutoff and alarm threshold. The default table storage class is published in `platform_contract.effective`, but
AWS provider 6.x cannot yet set it on table buckets, so new tables use S3 Tables' default until it can.

## Tenancy Model

- **Tenant = namespace per zone.** A tenant is the namespace `<tenant>` in each zone's table bucket, which Lake
  Formation exposes as the database `<tenant>` in the catalog `s3tablescatalog/<bucket>`.
- **Two entry points, one implementation.** The platform applies `modules/tenant` for each `tenants` entry, or a
  tenant provisioning pipeline calls it standalone with its own state. See
  [`modules/tenant/README.md`](modules/tenant/README.md). Both entry points produce the same resources and validate
  identically.
- **Platform contract.** `platform_contract` is the versioned interface tenants consume: identifiers and effective
  settings only. Publish it through remote state or an SSM parameter (see the tenant README).
- **Shared or dedicated storage.** By default a tenant's namespaces sit in the shared zone buckets. Set
  `dedicated_storage = true` for a tenant that needs its own table quota per zone, or whose sensitivity ranks above
  `governance_config.shared_storage_classification`. Compliance-designated tenants always get dedicated buckets,
  their own KMS key, and their own results bucket.
- **Pipelines are platform-operated.** Creating a tenant's LF-tag and grants needs Lake Formation admin authority, so
  tenant pipelines run as data-lake admins under the `scope = "tenant"` deployer policy and are never handed to
  tenant-operated identities (FR-001a).

### Service quotas

| Quota | Default | Effect on tenant count |
|-------|---------|------------------------|
| Tables per table bucket | 10,000 | Shared across all tenants in a zone; large tenants set `dedicated_storage = true` |
| Table buckets per Region | 100 | 3 shared buckets, plus one per zone for each dedicated or designated tenant |
| LF-tag keys per account | check Service Quotas | One key per tenant, plus the platform's sensitivity, domain, zone, and per-regulation keys |
| Athena workgroups per account | check Service Quotas | One workgroup per tenant |

## Data Zones

| Zone | Holds | Personas normally granted |
|------|-------|---------------------------|
| `raw` | Landed source data | Ingestion writers, stewards |
| `curated` | Conformed, validated data | Transformation writers, stewards, analysts |
| `consumption` (gold) | Published data products | Product writers, stewards, the semantic layer, analysts |

Zones are an access boundary: every grant is limited to the zones listed on the persona (default: all of the
tenant's zones). The external semantic layer is a reader with `zones = ["consumption"]` in each tenant it serves.
Override the zones with `zones_config`.

## Access Control with LF-Tags

### Vocabulary

| Key | Values | Set by |
|-----|--------|--------|
| `<prefix>-tenant-<tenant>` | `member` | The tenant submodule, on its databases. Only platform admins can ASSOCIATE it |
| `<prefix>-zone` | the zones | The tenant submodule, on its databases. Only platform admins can ASSOCIATE it |
| `<prefix>-sensitivity` | `governance_config.sensitivity_values`, least to most sensitive | Databases (tenant ceiling), then pipelines and stewards |
| `<prefix>-reg-<regulation>` | `no`, `yes` | One key per `regulation_values` entry |
| `<prefix>-domain` | `governance_config.domain_values` | Databases (tenant domain), optionally tables |

The `lf_tag_vocabulary` output lists the keys and values for pipelines.

### Fail-closed defaults

Each tenant database carries the tenant's most restrictive values: its sensitivity ceiling, `yes` for each of its
regulations, and its domain. New tables and columns inherit them, so a table nobody has labeled is visible only to
readers cleared for the ceiling and every one of the tenant's regulations.

### Grants per persona

Grants are account-level LF-tag policies; every expression includes `tenant = T` and `zone ∈ persona zones`.

| Persona | Databases | Tables | LF-tags |
|---------|-----------|--------|---------|
| Steward | `CREATE_TABLE`, `ALTER`, `DROP`, `DESCRIBE` (grantable) | all table permissions (grantable) | ASSOCIATE and grant within the tenant's label scope |
| Writer | `CREATE_TABLE`, `DESCRIBE` | `SELECT`, `INSERT`, `DELETE`, `ALTER`, `DESCRIBE`, plus `DROP` in Baseline | ASSOCIATE within the tenant's label scope |
| Reader | `DESCRIBE` | `SELECT`, `DESCRIBE` where sensitivity, each regulation, and optionally domain match | — |

Column-level control is built in: a reader sees only the columns whose labels fall within its expression. A
reader's `domains` list is an optional filter. Row filters (`row_filters`) are the only named-resource grants; they
need the table to exist, so add them after writers create it. The module outputs least-privilege IAM policy
documents per persona (`persona_policy_json`) but never attaches them.

### Steward runbook

- Correct a label: `aws lakeformation add-lf-tags-to-resource` on the table or columns, then recompute the table's
  roll-up (see the contract below).
- Grant within the tenant: `aws lakeformation grant-permissions` with an LF-tag expression that includes
  `<prefix>-tenant-<tenant> = member`. Lake Formation rejects anything outside the steward's tenant.
- Every label change and grant is recorded in CloudTrail (`AddLFTagsToResource`, `GrantPermissions`).

## Labeling and Roll-Up Contract

Writer pipelines (and stewards correcting labels) must, whenever they create a table or change its schema:

1. Label **every column** with sensitivity and every `reg-*` key (and optionally domain). Explicit column labels stop
   a raised table label from cascading to the columns.
2. Set the table's sensitivity to the highest column value, and each `reg-*` to `yes` if any column is `yes`.
3. Apply the table labels. On a schema change, label the new columns and recompute.

AWS CLI:

```bash
CATALOG="${ACCOUNT}:s3tablescatalog/<prefix>-curated"
aws lakeformation add-lf-tags-to-resource \
  --resource "{\"TableWithColumns\":{\"CatalogId\":\"${CATALOG}\",\"DatabaseName\":\"sales\",\"Name\":\"customers\",\"ColumnNames\":[\"email\"]}}" \
  --lf-tags '[{"TagKey":"<prefix>-sensitivity","TagValues":["confidential"]},{"TagKey":"<prefix>-reg-gdpr","TagValues":["yes"]}]'
aws lakeformation add-lf-tags-to-resource \
  --resource "{\"Table\":{\"CatalogId\":\"${CATALOG}\",\"DatabaseName\":\"sales\",\"Name\":\"customers\"}}" \
  --lf-tags '[{"TagKey":"<prefix>-sensitivity","TagValues":["confidential"]},{"TagKey":"<prefix>-reg-gdpr","TagValues":["yes"]}]'
```

boto3:

```python
import boto3

lf = boto3.client("lakeformation")
ORDER = ["public", "internal", "confidential", "restricted"]  # lf_tag_vocabulary.sensitivity.values


def label(catalog, db, table, columns, prefix, regulations):
    """columns: {name: {"sensitivity": str, "regs": set[str]}}"""
    for name, c in columns.items():
        tags = [{"TagKey": f"{prefix}-sensitivity", "TagValues": [c["sensitivity"]]}]
        tags += [{"TagKey": f"{prefix}-reg-{r}", "TagValues": ["yes" if r in c["regs"] else "no"]} for r in regulations]
        lf.add_lf_tags_to_resource(
            Resource={"TableWithColumns": {"CatalogId": catalog, "DatabaseName": db, "Name": table, "ColumnNames": [name]}},
            LFTags=tags,
        )
    top = max((c["sensitivity"] for c in columns.values()), key=ORDER.index)
    table_tags = [{"TagKey": f"{prefix}-sensitivity", "TagValues": [top]}]
    table_tags += [
        {"TagKey": f"{prefix}-reg-{r}", "TagValues": ["yes" if any(r in c["regs"] for c in columns.values()) else "no"]}
        for r in regulations
    ]
    lf.add_lf_tags_to_resource(Resource={"Table": {"CatalogId": catalog, "DatabaseName": db, "Name": table}}, LFTags=table_tags)
```

Lake Formation does not enforce the roll-up, and the module does not detect drift.

## Account-Level Integration Ownership

The catalog integration (`modules/catalog-integration`) is an account and Region singleton: the Lake Formation
data-access role, the `bucket/*` registration, the federated `s3tablescatalog`, and the data-lake settings.

- Exactly one deployment per account and Region keeps `catalog_integration_config.enabled = true`.
- Its data-lake settings are **authoritative**: admins not in `governance_config.admin_principal_arns` are removed,
  including console-added ones, and the default `IAM_ALLOWED_PRINCIPALS` permissions are cleared. List the deploying
  role, every secondary deployment's role, and every tenant pipeline role.
- Secondary deployments set `enabled = false` and pass the owner's `data_access_role_arn` output (see
  [`examples/secondary`](examples/secondary)).

## Audit Trail Prerequisites

The module creates no CloudTrail trail. Configure one in the account with:

- management events for `lakeformation` (including stewards' `GrantPermissions` and `AddLFTagsToResource`), `glue`,
  `s3tables`, and `athena`;
- data events for `AWS::S3Tables::Table`;
- retention of at least 90 days, or at least 1 year when any tenant is designated.

## Table Maintenance

- Unreferenced-file removal is configured on every table bucket from the environment defaults.
- Compaction and snapshot management are per-table S3 Tables settings; the module creates no tables, so tables keep
  the S3 Tables defaults (compaction on; at least 1 snapshot, 120-hour maximum age). Writers must not disable them.
- **GDPR erasure:** delete the subject's rows in every zone (Iceberg `DELETE`), then rebuild or re-run the
  consumption data products that derived from them. Deleted data leaves storage once its snapshots expire (up to
  5 days) and the unreferenced and noncurrent windows pass (3 and 7 days in Enhanced): about 15 days in total.

## Compliance Designation

Set `compliance_frameworks` (`SOC2`, `HIPAA`, `PCI_DSS`) on a tenant to designate it. A designated tenant gets
dedicated table buckets, its own KMS key, and its own results bucket, tagged `ComplianceFrameworks`; their cost is
reported as `compliance_monthly_cost`. A tenant with the `hipaa` or `pci_dss` regulation must carry the matching
designation. A `regulated_data_in_baseline` check warns when a designated tenant runs in a Baseline environment.

## Required Tags

In Enhanced environments (or with `required_tags_config.enforced = true`), `tags` must include the cost-center and
data-owner keys (`CostCenter` and `DataOwner` by default), or the plan fails. The module writes the
data-classification key itself: the platform's `shared_storage_classification` on shared storage, and the tenant's
ceiling on dedicated storage. Tenant resources also carry `Tenant` and the tenant's `DataOwner`.

## Bring Your Own KMS Key

Set `encryption_config = { create_kms_key = false, kms_key_id = "<key ARN>", ... }`. The module cannot edit that key's
policy, so merge the `required_kms_key_policy_json` output into it (S3 Tables maintenance, Lake Formation data access,
CloudWatch alarms, and Athena results). Tenant keys for designated tenants are always module-created.

## Query Isolation

Each tenant gets the Athena workgroup `<prefix>-<tenant>` with enforced settings: results under
`s3://<shared results bucket>/<tenant>/` (or the designated tenant's own bucket), SSE-KMS, the scan cutoff, and
CloudWatch metrics. When alarms are enabled, `query_failures` and `scanned_bytes` alarms per tenant notify the
platform SNS topic. The shared results bucket policy has no per-tenant statements; persona IAM policies limit each
persona to its own workgroup and results prefix.

## Required One-Time Grants

Athena resolves an S3 Tables bucket catalog only for principals that have Lake Formation `DESCRIBE`
on that catalog. The AWS provider cannot express this grant yet, so after the first apply, a
data-lake admin runs it once for each zone bucket and each dedicated tenant bucket:

```bash
ACCOUNT=$(aws sts get-caller-identity --query Account --output text)
for bucket in <name_prefix>-raw <name_prefix>-curated <name_prefix>-consumption; do
  aws lakeformation grant-permissions \
    --principal "DataLakePrincipalIdentifier=${ACCOUNT}:IAMPrincipals" \
    --permissions DESCRIBE \
    --resource "{\"Catalog\":{\"Id\":\"${ACCOUNT}:s3tablescatalog/${bucket}\"}}"
done
```

The grant only reveals that the catalog exists. Databases, tables, and columns remain gated by
the tenant LF-tag grants.

Tenant pipeline roles (and any other `admin_principal_arns` entry that did not create the
platform LF-tags) also need the platform tags' permissions, because Lake Formation admins do not
implicitly hold `ASSOCIATE` on tags another principal created. Run once per role:

```bash
ROLE_ARN=arn:aws:iam::${ACCOUNT}:role/<tenant-pipeline-role>
for key in $(aws lakeformation list-lf-tags --query "LFTags[?starts_with(TagKey, '<name_prefix>-') && !contains(TagKey, '-tenant-')].TagKey" --output text); do
  aws lakeformation grant-permissions \
    --principal "DataLakePrincipalIdentifier=${ROLE_ARN}" \
    --permissions DESCRIBE ASSOCIATE GRANT_WITH_LF_TAG_EXPRESSION \
    --resource "{\"LFTag\":{\"TagKey\":\"${key}\",\"TagValues\":[\"*\"]}}"
done
```

Both grants are outside Terraform only because AWS provider 6.x cannot manage them correctly yet.

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.11 |
| <a name="requirement_aws"></a> [aws](#requirement\_aws) | >= 6.59.0, < 7.0.0 |

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_aws"></a> [aws](#provider\_aws) | >= 6.59.0, < 7.0.0 |
| <a name="provider_terraform"></a> [terraform](#provider\_terraform) | n/a |

## Modules

| Name | Source | Version |
| ---- | ------ | ------- |
| <a name="module_catalog_integration"></a> [catalog\_integration](#module\_catalog\_integration) | ./modules/catalog-integration | n/a |
| <a name="module_pricing"></a> [pricing](#module\_pricing) | ./modules/cost-estimation | n/a |
| <a name="module_tenant"></a> [tenant](#module\_tenant) | ./modules/tenant | n/a |

## Resources

| Name | Type |
| ---- | ---- |
| [aws_kms_alias.main](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/kms_alias) | resource |
| [aws_kms_key.main](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/kms_key) | resource |
| [aws_lakeformation_lf_tag.domain](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_lf_tag) | resource |
| [aws_lakeformation_lf_tag.regulation](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_lf_tag) | resource |
| [aws_lakeformation_lf_tag.sensitivity](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_lf_tag) | resource |
| [aws_lakeformation_lf_tag.zone](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lakeformation_lf_tag) | resource |
| [aws_s3_bucket.query_results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket) | resource |
| [aws_s3_bucket_lifecycle_configuration.query_results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_lifecycle_configuration) | resource |
| [aws_s3_bucket_ownership_controls.query_results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_ownership_controls) | resource |
| [aws_s3_bucket_policy.query_results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_policy) | resource |
| [aws_s3_bucket_public_access_block.query_results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_public_access_block) | resource |
| [aws_s3_bucket_server_side_encryption_configuration.query_results](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3_bucket_server_side_encryption_configuration) | resource |
| [aws_s3tables_table_bucket.zone](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3tables_table_bucket) | resource |
| [aws_s3tables_table_bucket_policy.zone](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/s3tables_table_bucket_policy) | resource |
| [aws_sns_topic.alarms](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/sns_topic) | resource |
| [terraform_data.required_tags_guard](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |
| [terraform_data.secondary_admin_guard](https://registry.terraform.io/providers/hashicorp/terraform/latest/docs/resources/data) | resource |

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Organization unique prefix to use for resource names. Designed to be generated by terraform-external-context module. | `string` | n/a | yes |
| <a name="input_alarms_config"></a> [alarms\_config](#input\_alarms\_config) | Configuration object for metric alarms and notifications | <pre>object({<br/>    enabled                 = bool<br/>    create_sns_topic        = bool<br/>    sns_topic_arn           = string<br/>    scanned_bytes_threshold = optional(number)<br/>  })</pre> | <pre>{<br/>  "create_sns_topic": true,<br/>  "enabled": false,<br/>  "sns_topic_arn": ""<br/>}</pre> | no |
| <a name="input_catalog_integration_config"></a> [catalog\_integration\_config](#input\_catalog\_integration\_config) | Account-level Lake Formation integration for S3 Tables. Exactly one deployment per account and Region sets enabled = true; secondary deployments pass the owner's data access role ARN. | <pre>object({<br/>    enabled                               = bool<br/>    data_access_role_arn                  = string<br/>    allow_full_table_external_data_access = bool<br/>  })</pre> | <pre>{<br/>  "allow_full_table_external_data_access": false,<br/>  "data_access_role_arn": "",<br/>  "enabled": true<br/>}</pre> | no |
| <a name="input_cost_estimation_config"></a> [cost\_estimation\_config](#input\_cost\_estimation\_config) | Configuration object for monthly cost estimation | <pre>object({<br/>    enabled = bool<br/>  })</pre> | <pre>{<br/>  "enabled": true<br/>}</pre> | no |
| <a name="input_data_tags"></a> [data\_tags](#input\_data\_tags) | Additional tags to apply specifically to data storage resources beyond the common tags (usually generated by terraform-external-context). | `map(string)` | `{}` | no |
| <a name="input_enabled"></a> [enabled](#input\_enabled) | Set to false to prevent the module from creating any resources | `bool` | `true` | no |
| <a name="input_encryption_config"></a> [encryption\_config](#input\_encryption\_config) | Configuration object for encryption settings and KMS key management | <pre>object({<br/>    create_kms_key               = bool<br/>    kms_key_id                   = string<br/>    kms_key_deletion_window_days = number<br/>  })</pre> | <pre>{<br/>  "create_kms_key": true,<br/>  "kms_key_deletion_window_days": 14,<br/>  "kms_key_id": ""<br/>}</pre> | no |
| <a name="input_environment_type"></a> [environment\_type](#input\_environment\_type) | Environment type for resource configuration defaults. Select 'None' to use individual config values. | `string` | `"Development"` | no |
| <a name="input_governance_config"></a> [governance\_config](#input\_governance\_config) | Lake Formation governance: data-lake admins and the platform LF-tag vocabularies for sensitivity, regulations, and domains. | <pre>object({<br/>    admin_principal_arns          = list(string)<br/>    sensitivity_values            = optional(list(string), ["public", "internal", "confidential", "restricted"])<br/>    regulation_values             = optional(list(string), ["gdpr", "hipaa", "pci_dss"])<br/>    domain_values                 = optional(list(string), ["general"])<br/>    shared_storage_classification = optional(string)<br/>    writer_drop_enabled           = optional(bool)<br/>  })</pre> | <pre>{<br/>  "admin_principal_arns": []<br/>}</pre> | no |
| <a name="input_monitoring_config"></a> [monitoring\_config](#input\_monitoring\_config) | Configuration object for optional monitoring | <pre>object({<br/>    enabled = bool<br/>  })</pre> | <pre>{<br/>  "enabled": false<br/>}</pre> | no |
| <a name="input_query_config"></a> [query\_config](#input\_query\_config) | Platform query defaults that tenants may override. Null values use the environment default. | <pre>object({<br/>    bytes_scanned_cutoff_per_query = optional(number)<br/>    results_expiration_days        = optional(number)<br/>  })</pre> | `{}` | no |
| <a name="input_required_tags_config"></a> [required\_tags\_config](#input\_required\_tags\_config) | Tag keys the module requires or writes. When enforced (null uses the environment default), tags must include the cost-center and data-owner keys. | <pre>object({<br/>    enforced                = optional(bool)<br/>    cost_center_key         = optional(string, "CostCenter")<br/>    data_owner_key          = optional(string, "DataOwner")<br/>    data_classification_key = optional(string, "DataClassification")<br/>    tenant_key              = optional(string, "Tenant")<br/>  })</pre> | `{}` | no |
| <a name="input_storage_config"></a> [storage\_config](#input\_storage\_config) | Table storage settings. Null values use the environment default. | <pre>object({<br/>    table_storage_class = optional(string)<br/>  })</pre> | `{}` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Tags/labels to apply to all resources (usually generated by terraform-external-context). | `map(string)` | `{}` | no |
| <a name="input_tenants"></a> [tenants](#input\_tenants) | Tenants to provision with the platform, keyed by tenant name. Each entry is applied through modules/tenant. | <pre>map(object({<br/>    owner                 = string<br/>    sensitivity           = string<br/>    regulations           = optional(set(string), [])<br/>    domain                = optional(string)<br/>    zones                 = optional(list(string))<br/>    compliance_frameworks = optional(set(string), [])<br/>    stewards              = optional(list(string), [])<br/>    writers = optional(map(object({<br/>      principal_arn = string<br/>      zones         = optional(list(string))<br/>    })), {})<br/>    readers = optional(map(object({<br/>      principal_arn      = string<br/>      zones              = optional(list(string))<br/>      sensitivity_values = list(string)<br/>      regulations        = optional(set(string), [])<br/>      domains            = optional(list(string))<br/>    })), {})<br/>    row_filters = optional(map(object({<br/>      reader                = string<br/>      zone                  = string<br/>      table                 = string<br/>      row_filter_expression = string<br/>      column_names          = optional(list(string))<br/>    })), {})<br/>    bytes_scanned_cutoff_per_query = optional(number)<br/>    scanned_bytes_alarm_threshold  = optional(number)<br/>    dedicated_storage              = optional(bool, false)<br/>  }))</pre> | `{}` | no |
| <a name="input_zones_config"></a> [zones\_config](#input\_zones\_config) | Data zones shared by the platform. Each zone becomes an S3 table bucket named <name\_prefix>-<zone>. | <pre>object({<br/>    zones = list(string)<br/>  })</pre> | <pre>{<br/>  "zones": [<br/>    "raw",<br/>    "curated",<br/>    "consumption"<br/>  ]<br/>}</pre> | no |

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_alarm_sns_topic_arn"></a> [alarm\_sns\_topic\_arn](#output\_alarm\_sns\_topic\_arn) | ARN of the SNS topic used for alarm notifications |
| <a name="output_alarm_sns_topic_name"></a> [alarm\_sns\_topic\_name](#output\_alarm\_sns\_topic\_name) | Name of the SNS topic used for alarm notifications |
| <a name="output_catalog_name"></a> [catalog\_name](#output\_catalog\_name) | Name of the federated S3 Tables Glue catalog. Empty when this deployment does not own the catalog integration or enabled = false. |
| <a name="output_cost_breakdown"></a> [cost\_breakdown](#output\_cost\_breakdown) | Breakdown of the platform's own monthly cost by service; tenant estimates are in the tenants output |
| <a name="output_data_access_role_arn"></a> [data\_access\_role\_arn](#output\_data\_access\_role\_arn) | Lake Formation data-access role for S3 Tables. Secondary deployments pass this as catalog\_integration\_config.data\_access\_role\_arn. |
| <a name="output_kms_alias_name"></a> [kms\_alias\_name](#output\_kms\_alias\_name) | Name of the KMS key alias |
| <a name="output_kms_key_arn"></a> [kms\_key\_arn](#output\_kms\_key\_arn) | ARN of the platform KMS key |
| <a name="output_kms_key_id"></a> [kms\_key\_id](#output\_kms\_key\_id) | ID of the KMS key used for encryption |
| <a name="output_lf_tag_vocabulary"></a> [lf\_tag\_vocabulary](#output\_lf\_tag\_vocabulary) | LF-tag keys and allowed values for pipeline labeling: sensitivity (ordered least to most sensitive), one yes/no key per regulation, domain, and the tenant key prefix. Null when enabled = false. |
| <a name="output_monthly_cost_estimate"></a> [monthly\_cost\_estimate](#output\_monthly\_cost\_estimate) | Estimated idle monthly cost in USD of the platform plus every platform-applied tenant. Standalone tenants report their own estimate. |
| <a name="output_platform_contract"></a> [platform\_contract](#output\_platform\_contract) | Versioned public interface consumed by modules/tenant, both platform-applied and standalone: identifiers and effective settings only. Adding a field is a minor change; removing or renaming one is a major change. Null when enabled = false. |
| <a name="output_query_results_bucket_arn"></a> [query\_results\_bucket\_arn](#output\_query\_results\_bucket\_arn) | ARN of the shared Athena query results bucket. |
| <a name="output_required_kms_key_policy_json"></a> [required\_kms\_key\_policy\_json](#output\_required\_kms\_key\_policy\_json) | Key policy statements the platform key needs (S3 Tables maintenance, Lake Formation data access, Athena results). Merge them into a consumer-supplied key's policy. Empty when enabled = false. |
| <a name="output_tenants"></a> [tenants](#output\_tenants) | Outputs of each platform-applied tenant, keyed by tenant name. |
| <a name="output_usage_rates"></a> [usage\_rates](#output\_usage\_rates) | Usage-based list prices in USD (S3 Tables storage and maintenance, Athena per TB scanned, S3 Standard storage) for estimating data-dependent costs |
| <a name="output_zone_table_bucket_arns"></a> [zone\_table\_bucket\_arns](#output\_zone\_table\_bucket\_arns) | ARN of the shared table bucket per zone. |
<!-- END_TF_DOCS -->

## License

This project is licensed under the Apache License, Version 2.0 - see the [LICENSE](LICENSE) file for details.
