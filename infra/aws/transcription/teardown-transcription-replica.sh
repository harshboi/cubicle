#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

COMMAND="${1:-plan}"
if [[ "$COMMAND" != "plan" && "$COMMAND" != "destroy" && "$COMMAND" != "-h" && "$COMMAND" != "--help" && "$COMMAND" != "help" ]]; then
  PROJECT_NAME="${PROJECT_NAME:-$COMMAND}"
  COMMAND="${2:-plan}"
else
  PROJECT_NAME="${PROJECT_NAME:-cubicle-transcript-rp}"
fi

AWS_PROFILE_NAME="${AWS_PROFILE_NAME:-strln}"
AWS_REGION_NAME="${AWS_REGION_NAME:-us-west-2}"
EXPECTED_ACCOUNT_ID="${EXPECTED_ACCOUNT_ID:-562304353751}"
TERRAFORM_WORKSPACE_NAME="${TERRAFORM_WORKSPACE_NAME:-replica-${PROJECT_NAME//[^[:alnum:]_-]/-}}"
PUBLIC_ADMIN_DOMAIN_NAME="${PUBLIC_ADMIN_DOMAIN_NAME:-cubicle-replica.agenticisolation.com}"
KEEP_HF_SECRET="${KEEP_HF_SECRET:-false}"
DELETE_TERRAFORM_WORKSPACE="${DELETE_TERRAFORM_WORKSPACE:-false}"

AWS_CREDENTIAL_ENV_VARS=(
  AWS_ACCESS_KEY_ID
  AWS_SECRET_ACCESS_KEY
  AWS_SESSION_TOKEN
  AWS_SECURITY_TOKEN
  AWS_WEB_IDENTITY_TOKEN_FILE
  AWS_ROLE_ARN
  AWS_CONTAINER_CREDENTIALS_FULL_URI
  AWS_CONTAINER_CREDENTIALS_RELATIVE_URI
)

usage() {
  cat <<'EOF'
Usage:
  infra/transcription/teardown-transcription-replica.sh plan

  CONFIRM_DESTROY=cubicle-transcript-rp \
  infra/transcription/teardown-transcription-replica.sh destroy

  CONFIRM_DESTROY=cubicle-transcript-rp \
  infra/transcription/teardown-transcription-replica.sh cubicle-transcript-rp destroy

What it destroys for PROJECT_NAME:
  - CloudFront transcription endpoint
  - ALB listeners, target groups, and load balancers
  - ECS services, tasks, cluster, and task definitions
  - GPU Auto Scaling Group, launch template, instance profile, and EC2 host
  - Cognito admin pool/client/domain/group when enabled
  - DynamoDB admin users, token ledger, and audit tables
  - Secrets Manager secrets managed by Terraform
  - ECR repositories and images
  - WAF, security groups, VPC, subnets, route tables, and log groups

Safety:
  - Refuses PROJECT_NAME=cubicle-transcription unless ALLOW_PRIMARY_PROJECT_NAME=true.
  - Requires CONFIRM_DESTROY=<PROJECT_NAME> for destroy.
  - Uses the dedicated Terraform workspace replica-<PROJECT_NAME> by default.
  - Disables DynamoDB deletion protection before destroy.
  - Empties ECR repositories before destroy.
  - Unprotects and scales down GPU ASG instances before destroy.

External DNS note:
  If you created a GoDaddy/Route53 CNAME such as cubicle-replica.agenticisolation.com,
  remove that DNS record manually after destroy. This Terraform stack only outputs
  that record; it does not manage your external DNS provider.
EOF
}

if [[ "$COMMAND" == "-h" || "$COMMAND" == "--help" || "$COMMAND" == "help" ]]; then
  usage
  exit 0
fi

if [[ "$COMMAND" != "plan" && "$COMMAND" != "destroy" ]]; then
  echo "Unknown command: $COMMAND" >&2
  usage >&2
  exit 2
fi

if [[ "$PROJECT_NAME" == "cubicle-transcription" && "${ALLOW_PRIMARY_PROJECT_NAME:-false}" != "true" ]]; then
  cat >&2 <<'EOF'
Refusing to tear down PROJECT_NAME=cubicle-transcription.
That is the existing primary service. Use a replica PROJECT_NAME, or set
ALLOW_PRIMARY_PROJECT_NAME=true only if you intentionally want to destroy the
primary service.
EOF
  exit 2
fi

if (( ${#PROJECT_NAME} > 22 )); then
  cat >&2 <<EOF
PROJECT_NAME is too long for this stack naming scheme: '$PROJECT_NAME' (${#PROJECT_NAME} chars).
Use the exact short replica name, for example:
  PROJECT_NAME=cubicle-transcript-rp
EOF
  exit 2
fi

for tool in aws terraform python3; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "Missing required tool: $tool" >&2
    exit 2
  fi
done

conflicting_aws_env=()
for env_var in "${AWS_CREDENTIAL_ENV_VARS[@]}"; do
  if [[ -n "${!env_var:-}" ]]; then
    conflicting_aws_env+=("$env_var")
  fi
done

if [[ -n "${AWS_PROFILE:-}" && "$AWS_PROFILE" != "$AWS_PROFILE_NAME" ]]; then
  conflicting_aws_env+=("AWS_PROFILE=$AWS_PROFILE")
fi

if [[ -n "${AWS_DEFAULT_PROFILE:-}" && "$AWS_DEFAULT_PROFILE" != "$AWS_PROFILE_NAME" ]]; then
  conflicting_aws_env+=("AWS_DEFAULT_PROFILE=$AWS_DEFAULT_PROFILE")
fi

if (( ${#conflicting_aws_env[@]} > 0 )); then
  cat >&2 <<EOF
Refusing to run while ambient AWS credential/profile variables are set:
  ${conflicting_aws_env[*]}

This teardown is pinned to AWS profile '$AWS_PROFILE_NAME' and account '$EXPECTED_ACCOUNT_ID'.
Unset conflicting variables first.
EOF
  exit 2
fi

aws_cli() {
  aws --profile "$AWS_PROFILE_NAME" --region "$AWS_REGION_NAME" "$@"
}

aws_global() {
  aws --profile "$AWS_PROFILE_NAME" "$@"
}

terraform_cli() {
  terraform -chdir="$SCRIPT_DIR" "$@"
}

require_account() {
  local account_id
  account_id="$(aws_cli sts get-caller-identity --query Account --output text)"
  if [[ "$account_id" != "$EXPECTED_ACCOUNT_ID" ]]; then
    echo "Refusing to tear down AWS account $account_id; expected $EXPECTED_ACCOUNT_ID." >&2
    exit 2
  fi
  echo "$account_id"
}

select_workspace() {
  terraform_cli init -input=false
  if ! terraform_cli workspace list | sed 's/^[* ]*//' | grep -Fxq "$TERRAFORM_WORKSPACE_NAME"; then
    echo "Terraform workspace does not exist: $TERRAFORM_WORKSPACE_NAME" >&2
    exit 3
  fi
  terraform_cli workspace select "$TERRAFORM_WORKSPACE_NAME" >/dev/null
  export TF_WORKSPACE="$TERRAFORM_WORKSPACE_NAME"
}

build_terraform_vars() {
  TF_VARS=(
    -var "aws_profile=$AWS_PROFILE_NAME"
    -var "aws_region=$AWS_REGION_NAME"
    -var "expected_account_id=$EXPECTED_ACCOUNT_ID"
    -var "project_name=$PROJECT_NAME"
  )
}

disable_dynamodb_deletion_protection() {
  local tables=(
    "$PROJECT_NAME-users"
    "$PROJECT_NAME-token-ledger"
    "$PROJECT_NAME-admin-audit"
  )
  local table enabled

  for table in "${tables[@]}"; do
    enabled="$(aws_cli dynamodb describe-table \
      --table-name "$table" \
      --query 'Table.DeletionProtectionEnabled' \
      --output text 2>/dev/null || true)"
    if [[ "$enabled" == "True" || "$enabled" == "true" ]]; then
      echo "Disabling DynamoDB deletion protection: $table"
      aws_cli dynamodb update-table \
        --table-name "$table" \
        --no-deletion-protection-enabled >/dev/null
      for _ in $(seq 1 60); do
        enabled="$(aws_cli dynamodb describe-table \
          --table-name "$table" \
          --query 'Table.DeletionProtectionEnabled' \
          --output text 2>/dev/null || true)"
        [[ "$enabled" == "False" || "$enabled" == "false" ]] && break
        sleep 2
      done
    fi
  done
}

empty_ecr_repository() {
  local repo="$1" image_ids_file
  if ! aws_cli ecr describe-repositories --repository-names "$repo" >/dev/null 2>&1; then
    return 0
  fi

  image_ids_file="$(mktemp "${TMPDIR:-/tmp}/cubicle-ecr-images.XXXXXX.json")"
  aws_cli ecr list-images \
    --repository-name "$repo" \
    --query 'imageIds' \
    --output json > "$image_ids_file"

  if python3 - "$image_ids_file" <<'PY'
import json
import sys
with open(sys.argv[1], "r", encoding="utf-8") as handle:
    raise SystemExit(0 if json.load(handle) else 1)
PY
  then
    echo "Deleting ECR images: $repo"
    aws_cli ecr batch-delete-image \
      --repository-name "$repo" \
      --image-ids "file://$image_ids_file" >/dev/null || true
  fi
  rm -f "$image_ids_file"
}

empty_ecr_repositories() {
  empty_ecr_repository "$PROJECT_NAME"
  empty_ecr_repository "$PROJECT_NAME-voxtral-runtime"
}

unprotect_gpu_asg() {
  local asg_name="$PROJECT_NAME-gpu"
  local instance_ids

  instance_ids="$(aws_cli autoscaling describe-auto-scaling-groups \
    --auto-scaling-group-names "$asg_name" \
    --query 'AutoScalingGroups[0].Instances[].InstanceId' \
    --output text 2>/dev/null || true)"
  if [[ -z "$instance_ids" || "$instance_ids" == "None" ]]; then
    return 0
  fi

  echo "Removing scale-in protection from GPU ASG instances: $instance_ids"
  aws_cli autoscaling set-instance-protection \
    --auto-scaling-group-name "$asg_name" \
    --instance-ids $instance_ids \
    --no-protected-from-scale-in >/dev/null || true

  echo "Scaling GPU ASG to zero before destroy: $asg_name"
  aws_cli autoscaling update-auto-scaling-group \
    --auto-scaling-group-name "$asg_name" \
    --min-size 0 \
    --desired-capacity 0 >/dev/null || true
}

delete_replica_hf_secret() {
  local secret_name="$PROJECT_NAME/huggingface-token"
  if [[ "$KEEP_HF_SECRET" == "true" ]]; then
    echo "Keeping replica Hugging Face secret: $secret_name"
    return 0
  fi
  if aws_cli secretsmanager describe-secret --secret-id "$secret_name" >/dev/null 2>&1; then
    echo "Deleting replica Hugging Face secret: $secret_name"
    aws_cli secretsmanager delete-secret \
      --secret-id "$secret_name" \
      --force-delete-without-recovery >/dev/null || true
  fi
}

destroy_replica() {
  if [[ "${CONFIRM_DESTROY:-}" != "$PROJECT_NAME" ]]; then
    cat >&2 <<EOF
Refusing to destroy without explicit confirmation.
Set:
  CONFIRM_DESTROY=$PROJECT_NAME
EOF
    exit 2
  fi

  disable_dynamodb_deletion_protection
  empty_ecr_repositories
  unprotect_gpu_asg

  build_terraform_vars
  terraform_cli destroy -input=false -auto-approve "${TF_VARS[@]}"

  delete_replica_hf_secret

  if [[ "$DELETE_TERRAFORM_WORKSPACE" == "true" ]]; then
    terraform_cli workspace select default >/dev/null
    terraform_cli workspace delete "$TERRAFORM_WORKSPACE_NAME" >/dev/null
    echo "Deleted Terraform workspace: $TERRAFORM_WORKSPACE_NAME"
  fi

  cat <<EOF

Replica teardown complete for PROJECT_NAME=$PROJECT_NAME.

If you created external DNS, remove it manually:
  $PUBLIC_ADMIN_DOMAIN_NAME CNAME -> replica admin ALB
EOF
}

account_id="$(require_account)"
echo "AWS account:       $account_id"
echo "Region:            $AWS_REGION_NAME"
echo "Project:           $PROJECT_NAME"
echo "Terraform state:   workspace $TERRAFORM_WORKSPACE_NAME"

select_workspace

case "$COMMAND" in
  plan)
    build_terraform_vars
    terraform_cli plan -destroy -input=false "${TF_VARS[@]}"
    ;;
  destroy)
    destroy_replica
    ;;
esac
