#!/usr/bin/env bash
# Ensures the federated S3 Tables catalog exists in this account and Region (spec 003 FR-029).
#
# Run by terraform_data.s3tables_catalog at create time. It creates the catalog only when it is
# missing, never updates an existing one, and never deletes anything (FR-030). Terraform cannot
# test for the catalog at plan time (plan R13), so the check happens here.
#
# Every input arrives as an environment variable, and the catalog input and tags are JSON built
# by Terraform, so no value is ever interpolated into a command string:
#   ACCOUNT_ID          account the deployment targets; the credentials must belong to it
#   REGION              Region of the catalog
#   CATALOG_NAME        catalog name (s3tablescatalog)
#   CATALOG_INPUT_JSON  --catalog-input for create-catalog
#   TAGS_JSON           --tags for create-catalog ({} for none)
set -euo pipefail

: "${ACCOUNT_ID:?}" "${REGION:?}" "${CATALOG_NAME:?}" "${CATALOG_INPUT_JSON:?}" "${TAGS_JSON:?}"
export AWS_PAGER=""

log() { printf 'ensure-s3tables-catalog: %s\n' "$*" >&2; }

# The AWS CLI uses the environment's credentials, not Terraform's provider configuration, so
# refuse to act in any other account
caller=$(aws sts get-caller-identity --region "$REGION" --query Account --output text)
if [ "$caller" != "$ACCOUNT_ID" ]; then
  log "the AWS CLI credentials belong to account ${caller}, not ${ACCOUNT_ID}; refusing to act"
  exit 1
fi

# Returns 0 when the catalog exists and 1 when it does not; any other error ends the script
catalog_exists() {
  local err
  if err=$(aws glue get-catalog --region "$REGION" --catalog-id "$CATALOG_NAME" --query Catalog.Name --output text 2>&1 >/dev/null); then
    return 0
  fi
  case "$err" in
    *EntityNotFoundException*) return 1 ;;
    *)
      log "get-catalog failed: ${err}"
      exit 1
      ;;
  esac
}

if catalog_exists; then
  log "${CATALOG_NAME} exists in ${ACCOUNT_ID} ${REGION}; left unchanged"
  exit 0
fi

args=(glue create-catalog --region "$REGION" --name "$CATALOG_NAME" --catalog-input "$CATALOG_INPUT_JSON")
if [ "$TAGS_JSON" != "{}" ]; then
  args+=(--tags "$TAGS_JSON")
fi
if ! err=$(aws "${args[@]}" 2>&1 >/dev/null); then
  case "$err" in
    # Another deployment created it between the check and the create
    *AlreadyExistsException*)
      log "${CATALOG_NAME} was created concurrently; left unchanged"
      exit 0
      ;;
    *)
      log "create-catalog failed: ${err}"
      exit 1
      ;;
  esac
fi
log "created ${CATALOG_NAME} in ${ACCOUNT_ID} ${REGION}"

# Glue is eventually consistent; wait until later resources can see the catalog
for _ in $(seq 1 30); do
  if catalog_exists; then
    exit 0
  fi
  sleep 2
done
log "${CATALOG_NAME} is not visible 60 seconds after creation"
exit 1
