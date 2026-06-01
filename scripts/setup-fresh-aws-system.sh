#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TRANSCRIPTION_DIR="$REPO_ROOT/infra/aws/transcription"
VOICENOTES_DIR="$REPO_ROOT/apps/voicenotes-web/infra/aws"

COMMAND="${1:-full}"
if [[ "$COMMAND" == "-h" || "$COMMAND" == "--help" || "$COMMAND" == "help" ]]; then
  COMMAND="help"
fi

AWS_PROFILE_NAME="${AWS_PROFILE_NAME:-}"
AWS_REGION_NAME="${AWS_REGION_NAME:-us-west-2}"
EXPECTED_ACCOUNT_ID="${EXPECTED_ACCOUNT_ID:-}"
PROJECT_NAME="${PROJECT_NAME:-cubicle-transcript-rp}"
TERRAFORM_WORKSPACE_NAME="${TERRAFORM_WORKSPACE_NAME:-replica-${PROJECT_NAME//[^[:alnum:]_-]/-}}"
TRANSCRIPTION_ALLOWED_USERS="${TRANSCRIPTION_ALLOWED_USERS:-}"
HF_TOKEN_FILE="${HF_TOKEN_FILE:-${HUGGINGFACE_TOKEN_FILE:-}}"
DEPLOY_VOICENOTES="${DEPLOY_VOICENOTES:-false}"
VOICENOTES_PROJECT_NAME="${VOICENOTES_PROJECT_NAME:-voicenotes}"
VOICENOTES_TERRAFORM_WORKSPACE_NAME="${VOICENOTES_TERRAFORM_WORKSPACE_NAME:-${TERRAFORM_WORKSPACE_NAME}-voicenotes}"
VOICENOTES_DOMAIN_NAME="${VOICENOTES_DOMAIN_NAME:-voicenotes.example.com}"
VOICENOTES_CERTIFICATE_ARN="${VOICENOTES_CERTIFICATE_ARN:-}"
REQUEST_VOICENOTES_CERTIFICATE="${REQUEST_VOICENOTES_CERTIFICATE:-false}"
VOICENOTES_IMAGE_TAG="${VOICENOTES_IMAGE_TAG:-fresh-$(date +%Y%m%d%H%M%S)}"
VOICENOTES_DOCKER_PLATFORM="${VOICENOTES_DOCKER_PLATFORM:-linux/arm64}"
VOICENOTES_ADMIN_EMAIL="${VOICENOTES_ADMIN_EMAIL:-}"
RUN_SOURCE_BACKUP="${RUN_SOURCE_BACKUP:-true}"

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
  scripts/setup-fresh-aws-system.sh preflight
  scripts/setup-fresh-aws-system.sh backup
  scripts/setup-fresh-aws-system.sh transcription
  scripts/setup-fresh-aws-system.sh voicenotes
  scripts/setup-fresh-aws-system.sh full

Fresh-account setup wrapper for gifting the Cubicle system to another AWS
account. Run it from a laptop with AWS CLI, Terraform, Docker buildx, Python 3,
OpenSSL, and a configured AWS profile for the recipient account.

Required for any AWS-changing command:
  AWS_PROFILE_NAME                  AWS CLI profile for the recipient account.
  EXPECTED_ACCOUNT_ID               Recipient AWS account id safety guard.
  PROJECT_NAME                      Short transcription project name, <= 22 chars.
  TRANSCRIPTION_ALLOWED_USERS       Comma-separated users allowed to stream.
  HF_TOKEN_FILE                     File containing a Hugging Face token that
                                    has accepted Voxtral/pyannote model terms,
                                    unless the target secret already exists.

Optional VoiceNotes deployment:
  DEPLOY_VOICENOTES=true
  VOICENOTES_DOMAIN_NAME=notes.example.com
  VOICENOTES_CERTIFICATE_ARN=arn:aws:acm:us-west-2:<acct>:certificate/...
  VOICENOTES_ADMIN_EMAIL=user@example.com

Certificate helper:
  REQUEST_VOICENOTES_CERTIFICATE=true scripts/setup-fresh-aws-system.sh voicenotes

That requests an ACM DNS-validated certificate and prints the validation record.
Add the DNS record, wait until the certificate is ISSUED, then rerun with
VOICENOTES_CERTIFICATE_ARN set.

Commands:
  backup          Create a source-code handoff archive only.
  preflight       Validate local tools, AWS identity, GPU quota, Terraform, and
                  name collisions without creating resources.
  transcription   Provision the transcription CloudFront/ALB/ECS/EC2 GPU/vLLM
                  path by wrapping infra/aws/transcription/provision-transcription-replica.sh.
  voicenotes      Build and deploy VoiceNotes into the transcription VPC.
  full            backup + transcription + optional VoiceNotes when
                  DEPLOY_VOICENOTES=true.
EOF
}

require_tool() {
  local tool="$1"
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "Missing required tool: $tool" >&2
    exit 2
  fi
}

require_common_inputs() {
  if [[ -z "$AWS_PROFILE_NAME" ]]; then
    echo "AWS_PROFILE_NAME is required." >&2
    exit 2
  fi
  if [[ -z "$EXPECTED_ACCOUNT_ID" ]]; then
    echo "EXPECTED_ACCOUNT_ID is required." >&2
    exit 2
  fi
  if [[ -z "$PROJECT_NAME" || ${#PROJECT_NAME} -gt 22 ]]; then
    echo "PROJECT_NAME is required and must be 22 characters or fewer: '$PROJECT_NAME'." >&2
    exit 2
  fi
}

check_ambient_credentials() {
  local conflicts=()
  for env_var in "${AWS_CREDENTIAL_ENV_VARS[@]}"; do
    if [[ -n "${!env_var:-}" ]]; then
      conflicts+=("$env_var")
    fi
  done
  if [[ -n "${AWS_PROFILE:-}" && "$AWS_PROFILE" != "$AWS_PROFILE_NAME" ]]; then
    conflicts+=("AWS_PROFILE=$AWS_PROFILE")
  fi
  if [[ -n "${AWS_DEFAULT_PROFILE:-}" && "$AWS_DEFAULT_PROFILE" != "$AWS_PROFILE_NAME" ]]; then
    conflicts+=("AWS_DEFAULT_PROFILE=$AWS_DEFAULT_PROFILE")
  fi
  if (( ${#conflicts[@]} > 0 )); then
    cat >&2 <<EOF
Refusing to run while ambient AWS credential/profile variables are set:
  ${conflicts[*]}

This setup is pinned to AWS_PROFILE_NAME='$AWS_PROFILE_NAME' and account '$EXPECTED_ACCOUNT_ID'.
Unset the conflicting variables first.
EOF
    exit 2
  fi
}

aws_cli() {
  aws --profile "$AWS_PROFILE_NAME" --region "$AWS_REGION_NAME" "$@"
}

terraform_transcription() {
  env -u TF_WORKSPACE terraform -chdir="$TRANSCRIPTION_DIR" "$@"
}

terraform_voicenotes() {
  env -u TF_WORKSPACE terraform -chdir="$VOICENOTES_DIR" "$@"
}

verify_account() {
  local account_id
  account_id="$(aws_cli sts get-caller-identity --query Account --output text)"
  if [[ "$account_id" != "$EXPECTED_ACCOUNT_ID" ]]; then
    echo "Refusing to run in AWS account $account_id; expected $EXPECTED_ACCOUNT_ID." >&2
    exit 2
  fi
  echo "AWS account verified: $account_id"
}

run_backup() {
  "$REPO_ROOT/scripts/backup-source.sh"
}

run_preflight() {
  require_common_inputs
  check_ambient_credentials
  for tool in aws terraform docker openssl python3; do
    require_tool "$tool"
  done
  verify_account
  AWS_PROFILE_NAME="$AWS_PROFILE_NAME" \
  AWS_REGION_NAME="$AWS_REGION_NAME" \
  EXPECTED_ACCOUNT_ID="$EXPECTED_ACCOUNT_ID" \
  PROJECT_NAME="$PROJECT_NAME" \
  TERRAFORM_WORKSPACE_NAME="$TERRAFORM_WORKSPACE_NAME" \
  TRANSCRIPTION_ALLOWED_USERS="$TRANSCRIPTION_ALLOWED_USERS" \
  HF_TOKEN_FILE="$HF_TOKEN_FILE" \
  "$TRANSCRIPTION_DIR/provision-transcription-replica.sh" preflight
}

run_transcription() {
  require_common_inputs
  check_ambient_credentials
  if [[ -z "$TRANSCRIPTION_ALLOWED_USERS" && "${ALLOW_ANY_SIGNED_TRANSCRIPTION_USER:-false}" != "true" ]]; then
    echo "TRANSCRIPTION_ALLOWED_USERS is required unless ALLOW_ANY_SIGNED_TRANSCRIPTION_USER=true." >&2
    exit 2
  fi

  AWS_PROFILE_NAME="$AWS_PROFILE_NAME" \
  AWS_REGION_NAME="$AWS_REGION_NAME" \
  EXPECTED_ACCOUNT_ID="$EXPECTED_ACCOUNT_ID" \
  PROJECT_NAME="$PROJECT_NAME" \
  TERRAFORM_WORKSPACE_NAME="$TERRAFORM_WORKSPACE_NAME" \
  TRANSCRIPTION_ALLOWED_USERS="$TRANSCRIPTION_ALLOWED_USERS" \
  HF_TOKEN_FILE="$HF_TOKEN_FILE" \
    "$TRANSCRIPTION_DIR/provision-transcription-replica.sh"
}

select_transcription_workspace() {
  terraform_transcription init -input=false
  if terraform_transcription workspace list | sed 's/^[* ]*//' | grep -Fxq "$TERRAFORM_WORKSPACE_NAME"; then
    terraform_transcription workspace select "$TERRAFORM_WORKSPACE_NAME" >/dev/null
  else
    cat >&2 <<EOF
Transcription Terraform workspace '$TERRAFORM_WORKSPACE_NAME' does not exist.
Run the transcription command first:
  scripts/setup-fresh-aws-system.sh transcription
EOF
    exit 2
  fi
  echo "Transcription Terraform workspace: $TERRAFORM_WORKSPACE_NAME"
}

select_voicenotes_workspace() {
  terraform_voicenotes init -input=false
  if terraform_voicenotes workspace list | sed 's/^[* ]*//' | grep -Fxq "$VOICENOTES_TERRAFORM_WORKSPACE_NAME"; then
    terraform_voicenotes workspace select "$VOICENOTES_TERRAFORM_WORKSPACE_NAME" >/dev/null
  else
    terraform_voicenotes workspace new "$VOICENOTES_TERRAFORM_WORKSPACE_NAME" >/dev/null
  fi
  export TF_WORKSPACE="$VOICENOTES_TERRAFORM_WORKSPACE_NAME"
  echo "VoiceNotes Terraform workspace: $VOICENOTES_TERRAFORM_WORKSPACE_NAME"
}

request_voicenotes_certificate() {
  local cert_arn
  cert_arn="$(aws_cli acm request-certificate \
    --domain-name "$VOICENOTES_DOMAIN_NAME" \
    --validation-method DNS \
    --query CertificateArn \
    --output text)"

  echo "Requested ACM certificate: $cert_arn"
  echo "DNS validation records:"
  aws_cli acm describe-certificate \
    --certificate-arn "$cert_arn" \
    --query 'Certificate.DomainValidationOptions[].ResourceRecord' \
    --output table
  cat <<EOF

Add the DNS validation record above, wait for ACM status ISSUED, then rerun:
  VOICENOTES_CERTIFICATE_ARN=$cert_arn scripts/setup-fresh-aws-system.sh voicenotes
EOF
}

run_voicenotes() {
  require_common_inputs
  check_ambient_credentials
  for tool in aws terraform docker python3; do
    require_tool "$tool"
  done
  verify_account

  if [[ "$REQUEST_VOICENOTES_CERTIFICATE" == "true" && -z "$VOICENOTES_CERTIFICATE_ARN" ]]; then
    request_voicenotes_certificate
    exit 9
  fi

  if [[ -z "$VOICENOTES_CERTIFICATE_ARN" ]]; then
    cat >&2 <<EOF
VOICENOTES_CERTIFICATE_ARN is required to deploy VoiceNotes.
Set REQUEST_VOICENOTES_CERTIFICATE=true to request a DNS-validated ACM cert first.
EOF
    exit 2
  fi

  local vpc_id public_subnet_ids service_subnet_ids upstream_url upstream_secret repository_url image_uri alb_dns pool_id
  select_transcription_workspace
  vpc_id="$(terraform_transcription output -raw vpc_id)"
  public_subnet_ids="$(terraform_transcription output -json public_subnet_ids)"
  service_subnet_ids="$(terraform_transcription output -json service_subnet_ids)"
  upstream_url="$(terraform_transcription output -raw websocket_endpoint)"
  upstream_secret="$(terraform_transcription output -raw service_token_secret_arn)"

  select_voicenotes_workspace

  terraform_voicenotes apply -input=false -auto-approve \
    -target=aws_ecr_repository.app \
    -var "aws_profile=$AWS_PROFILE_NAME" \
    -var "aws_region=$AWS_REGION_NAME" \
    -var "project_name=$VOICENOTES_PROJECT_NAME" \
    -var "domain_name=$VOICENOTES_DOMAIN_NAME" \
    -var "certificate_arn=$VOICENOTES_CERTIFICATE_ARN" \
    -var "vpc_id=$vpc_id" \
    -var "public_subnet_ids=$public_subnet_ids" \
    -var "service_subnet_ids=$service_subnet_ids" \
    -var "container_image=bootstrap" \
    -var "upstream_transcription_url=$upstream_url" \
    -var "upstream_transcription_token_secret_arn=$upstream_secret"

  repository_url="$(terraform_voicenotes output -raw ecr_repository_url)"
  image_uri="$repository_url:$VOICENOTES_IMAGE_TAG"
  local registry="${EXPECTED_ACCOUNT_ID}.dkr.ecr.${AWS_REGION_NAME}.amazonaws.com"

  aws_cli ecr get-login-password | docker login --username AWS --password-stdin "$registry"
  docker buildx build \
    --platform "$VOICENOTES_DOCKER_PLATFORM" \
    -t "$image_uri" \
    --push \
    "$REPO_ROOT/voicenotes"

  terraform_voicenotes apply -input=false -auto-approve \
    -var "aws_profile=$AWS_PROFILE_NAME" \
    -var "aws_region=$AWS_REGION_NAME" \
    -var "project_name=$VOICENOTES_PROJECT_NAME" \
    -var "domain_name=$VOICENOTES_DOMAIN_NAME" \
    -var "certificate_arn=$VOICENOTES_CERTIFICATE_ARN" \
    -var "vpc_id=$vpc_id" \
    -var "public_subnet_ids=$public_subnet_ids" \
    -var "service_subnet_ids=$service_subnet_ids" \
    -var "container_image=$image_uri" \
    -var "upstream_transcription_url=$upstream_url" \
    -var "upstream_transcription_token_secret_arn=$upstream_secret"

  alb_dns="$(terraform_voicenotes output -raw domain_cname_target)"
  pool_id="$(terraform_voicenotes output -raw cognito_user_pool_id)"

  if [[ -n "$VOICENOTES_ADMIN_EMAIL" ]]; then
    aws_cli cognito-idp admin-create-user \
      --user-pool-id "$pool_id" \
      --username "$VOICENOTES_ADMIN_EMAIL" \
      --user-attributes Name=email,Value="$VOICENOTES_ADMIN_EMAIL" Name=email_verified,Value=true \
      >/dev/null || true
    echo "Ensured VoiceNotes Cognito user: $VOICENOTES_ADMIN_EMAIL"
  fi

  cat <<EOF

VoiceNotes deployment complete.
URL: https://$VOICENOTES_DOMAIN_NAME
Image: $image_uri
Cognito user pool: $pool_id
DNS CNAME required:
  $VOICENOTES_DOMAIN_NAME -> $alb_dns
EOF
}

case "$COMMAND" in
  help)
    usage
    ;;
  backup)
    run_backup
    ;;
  preflight)
    run_preflight
    ;;
  transcription)
    run_transcription
    ;;
  voicenotes)
    run_voicenotes
    ;;
  full)
    if [[ "$RUN_SOURCE_BACKUP" == "true" ]]; then
      run_backup
    fi
    run_transcription
    if [[ "$DEPLOY_VOICENOTES" == "true" ]]; then
      run_voicenotes
    else
      echo "Skipping VoiceNotes because DEPLOY_VOICENOTES is not true."
    fi
    ;;
  *)
    echo "Unknown command: $COMMAND" >&2
    usage >&2
    exit 2
    ;;
esac
