# ----
# Label-style expressions shared by the overlays (inject.tf)
# ----

# The metadata personas are module personas since awslkhs-lfj (platform.tf). Each tenant has its
# own tag key, so an expression reaching one tenant names that tenant's key.

locals {
  zone_tag_key = module.lakehouse.lf_tag_vocabulary.zone.key

  tenant_curated_expression = {
    for t in local.tenant_names : t => [
      { key = module.lakehouse.tenants[t].tenant_tag_key, values = ["member"] },
      { key = local.zone_tag_key, values = ["curated"] },
    ]
  }
}
