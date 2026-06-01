#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

AWS_PROFILE_NAME="${AWS_PROFILE_NAME:-strln}"
AWS_REGION_NAME="${AWS_REGION_NAME:-us-west-2}"
COMMAND="provision"
if [[ "${1:-}" == "preflight" ]]; then
  COMMAND="preflight"
  shift
fi

EXPECTED_ACCOUNT_ID="${EXPECTED_ACCOUNT_ID:-${1:-}}"
PROJECT_NAME="${PROJECT_NAME:-cubicle-transcript-rp}"
TERRAFORM_WORKSPACE_NAME="${TERRAFORM_WORKSPACE_NAME:-replica-${PROJECT_NAME//[^[:alnum:]_-]/-}}"
TRANSCRIPTION_ALLOWED_USERS="${TRANSCRIPTION_ALLOWED_USERS:-}"
TRANSCRIPTION_AUTH_MODE="${TRANSCRIPTION_AUTH_MODE:-signed_user_token}"
TRANSCRIPTION_TOKEN_ISSUER="${TRANSCRIPTION_TOKEN_ISSUER:-cubicle-transcription}"
TRANSCRIPTION_TOKEN_AUDIENCE="${TRANSCRIPTION_TOKEN_AUDIENCE:-cubicle-macos}"
TRANSCRIPTION_REQUIRED_SCOPE="${TRANSCRIPTION_REQUIRED_SCOPE:-transcription:stream}"
DIARIZATION_PROVIDER="${DIARIZATION_PROVIDER:-mock}"

HF_SECRET_NAME="${HF_SECRET_NAME:-$PROJECT_NAME/huggingface-token}"
SOURCE_HF_SECRET_NAME="${SOURCE_HF_SECRET_NAME:-cubicle-transcription/huggingface-token}"
HF_TOKEN_FILE="${HF_TOKEN_FILE:-${HUGGINGFACE_TOKEN_FILE:-}}"
VLLM_IMAGE_URI="${VLLM_IMAGE_URI:-vllm/vllm-openai:v0.21.0-ubuntu2404}"
VOXTRAL_MODEL="${VOXTRAL_MODEL:-mistralai/Voxtral-Mini-4B-Realtime-2602}"
VOXTRAL_MODEL_VERSION="${VOXTRAL_MODEL_VERSION:-self-hosted-vllm-2602}"
VLLM_CONTAINER_NAME="${VLLM_CONTAINER_NAME:-voxtral-vllm}"
VLLM_HOST_PORT="${VLLM_HOST_PORT:-8000}"
VLLM_MODEL_LEN="${VLLM_MODEL_LEN:-45000}"
VLLM_GPU_MEMORY_UTILIZATION="${VLLM_GPU_MEMORY_UTILIZATION:-0.90}"
HF_CACHE_DIR="${HF_CACHE_DIR:-/home/ssm-user/.cache/huggingface}"
READINESS_ATTEMPTS="${READINESS_ATTEMPTS:-240}"
READINESS_SLEEP_SECONDS="${READINESS_SLEEP_SECONDS:-10}"

GPU_INSTANCE_TYPE="${GPU_INSTANCE_TYPE:-g5.xlarge}"
GPU_DESIRED_CAPACITY="${GPU_DESIRED_CAPACITY:-1}"
GPU_MIN_SIZE="${GPU_MIN_SIZE:-0}"
GPU_MAX_SIZE="${GPU_MAX_SIZE:-1}"
ASG_NAME="${ASG_NAME:-$PROJECT_NAME-gpu}"
INSTANCE_ROLE_NAME="${INSTANCE_ROLE_NAME:-$PROJECT_NAME-ecs-gpu-instance}"

IMAGE_TAG_PREFIX="${IMAGE_TAG_PREFIX:-replica-$(date +%Y%m%d%H%M%S)}"
BOOTSTRAP_IMAGE_TAG="${BOOTSTRAP_IMAGE_TAG:-$IMAGE_TAG_PREFIX-bootstrap}"
FINAL_IMAGE_TAG="${FINAL_IMAGE_TAG:-$IMAGE_TAG_PREFIX-direct}"

ENABLE_PUBLIC_ADMIN_CONSOLE="${ENABLE_PUBLIC_ADMIN_CONSOLE:-false}"
PUBLIC_ADMIN_DOMAIN_NAME="${PUBLIC_ADMIN_DOMAIN_NAME:-cubicle-replica.agenticisolation.com}"
PUBLIC_ADMIN_CERTIFICATE_ARN="${PUBLIC_ADMIN_CERTIFICATE_ARN:-}"
PUBLIC_ADMIN_ALLOWED_CIDR_BLOCKS_HCL="${PUBLIC_ADMIN_ALLOWED_CIDR_BLOCKS_HCL:-[\"0.0.0.0/0\"]}"
PUBLIC_ADMIN_COGNITO_DOMAIN_PREFIX="${PUBLIC_ADMIN_COGNITO_DOMAIN_PREFIX:-}"
PUBLIC_ADMIN_WAF_RATE_LIMIT="${PUBLIC_ADMIN_WAF_RATE_LIMIT:-500}"
ADMIN_DESIRED_COUNT="${ADMIN_DESIRED_COUNT:-1}"
ADMIN_EMAIL="${ADMIN_EMAIL:-}"

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
  EXPECTED_ACCOUNT_ID=562304353751 \
  AWS_PROFILE_NAME=strln \
  AWS_REGION_NAME=us-west-2 \
  PROJECT_NAME=cubicle-transcript-rp \
  TRANSCRIPTION_ALLOWED_USERS=prabhat7@cisco.com \
  HF_TOKEN_FILE=/path/to/0600-hf-token \
  infra/transcription/provision-transcription-replica.sh

Or pass the account id as the first argument:
  infra/transcription/provision-transcription-replica.sh 562304353751

Non-destructive validation:
  EXPECTED_ACCOUNT_ID=562304353751 \
  TRANSCRIPTION_ALLOWED_USERS=prabhat7@cisco.com \
  infra/transcription/provision-transcription-replica.sh preflight

What it does:
  1. Verifies the AWS caller matches EXPECTED_ACCOUNT_ID.
  2. Selects a dedicated Terraform workspace for PROJECT_NAME.
  3. Creates or updates the Hugging Face token secret.
  4. Runs the existing Terraform/deploy helper once to create the stack, ECR,
     service secrets, CloudFront/ALB/ECS, and one GPU EC2 ASG instance.
  5. Grants the GPU EC2 role least-privilege read access to the HF secret.
  6. Uses SSM to install/verify Docker, pull vLLM, download/load Voxtral, and
     run the voxtral-vllm container on port 8000.
  7. Redeploys the ECS transcription adapter to the GPU instance private
     ws://<ip>:8000/v1/realtime endpoint.
  8. Optionally enables the public Cognito/WAF admin console when
     ENABLE_PUBLIC_ADMIN_CONSOLE=true and PUBLIC_ADMIN_CERTIFICATE_ARN is set.

Important inputs:
  EXPECTED_ACCOUNT_ID              Required AWS account guard.
  PROJECT_NAME                     Defaults to cubicle-transcript-rp. Keep at
                                  22 characters or fewer so AWS ALB names fit.
  TERRAFORM_WORKSPACE_NAME         Defaults to replica-<PROJECT_NAME>.
  TRANSCRIPTION_ALLOWED_USERS      Required unless ALLOW_ANY_SIGNED_TRANSCRIPTION_USER=true.
  HF_TOKEN_FILE or HF_TOKEN        Required unless HF_SECRET_NAME already exists.
  SOURCE_HF_SECRET_NAME            Optional source secret to copy into HF_SECRET_NAME
                                  when no HF token input is supplied. Defaults to
                                  cubicle-transcription/huggingface-token.
  ENABLE_PUBLIC_ADMIN_CONSOLE      false by default. Set true with a regional ACM cert ARN.
  PUBLIC_ADMIN_CERTIFICATE_ARN     Required to expose the admin console.
  PUBLIC_ADMIN_ALLOWED_CIDR_BLOCKS_HCL
                                  Terraform list literal, default ["0.0.0.0/0"].
  ADMIN_EMAIL                      Optional Cognito admin user to invite/add to group.

The script never prints the Hugging Face token, signing secret, service token,
or issued transcription bearer tokens.
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ -z "$EXPECTED_ACCOUNT_ID" ]]; then
  echo "EXPECTED_ACCOUNT_ID is required, or pass the AWS account id as the first argument." >&2
  usage >&2
  exit 2
fi

if [[ "$TRANSCRIPTION_AUTH_MODE" == "signed_user_token" && -z "$TRANSCRIPTION_ALLOWED_USERS" && "${ALLOW_ANY_SIGNED_TRANSCRIPTION_USER:-false}" != "true" ]]; then
  cat >&2 <<'EOF'
TRANSCRIPTION_ALLOWED_USERS is required for signed_user_token mode.
Set it to a comma-separated list of allowed users, or explicitly set
ALLOW_ANY_SIGNED_TRANSCRIPTION_USER=true for a broad staging-only mode.
EOF
  exit 2
fi

if [[ "$PROJECT_NAME" == "cubicle-transcription" && "${ALLOW_PRIMARY_PROJECT_NAME:-false}" != "true" ]]; then
  cat >&2 <<'EOF'
Refusing to provision a replica with PROJECT_NAME=cubicle-transcription.
That name is reserved for the existing primary service. Use a unique
PROJECT_NAME such as cubicle-transcription-replica, or set
ALLOW_PRIMARY_PROJECT_NAME=true if you intentionally want to operate on the
primary service.
EOF
  exit 2
fi

if (( ${#PROJECT_NAME} > 22 )); then
  cat >&2 <<EOF
PROJECT_NAME is too long for a complete replica: '$PROJECT_NAME' (${#PROJECT_NAME} chars).
Use 22 characters or fewer so AWS ALB names fit, for example:
  PROJECT_NAME=cubicle-transcript-rp
EOF
  exit 2
fi

require_tool() {
  local tool="$1"
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "Missing required tool: $tool" >&2
    exit 2
  fi
}

for tool in aws terraform docker openssl python3; do
  require_tool "$tool"
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

This replica deployment is pinned to AWS profile '$AWS_PROFILE_NAME' and account '$EXPECTED_ACCOUNT_ID'.
Unset conflicting variables first.
EOF
  exit 2
fi

aws_cli() {
  aws --profile "$AWS_PROFILE_NAME" --region "$AWS_REGION_NAME" "$@"
}

terraform_cli() {
  terraform -chdir="$SCRIPT_DIR" "$@"
}

require_account() {
  local account_id
  account_id="$(aws_cli sts get-caller-identity --query Account --output text)"
  if [[ "$account_id" != "$EXPECTED_ACCOUNT_ID" ]]; then
    echo "Refusing to deploy to AWS account $account_id; expected $EXPECTED_ACCOUNT_ID." >&2
    exit 2
  fi
  echo "$account_id"
}

check_build_tooling() {
  docker version >/dev/null
  docker buildx version >/dev/null
  aws_cli ecr get-login-password >/dev/null
  echo "Docker, buildx, and ECR auth checks passed."
}

check_gpu_capacity() {
  local offering_count quota_value
  offering_count="$(aws_cli ec2 describe-instance-type-offerings \
    --location-type region \
    --filters "Name=instance-type,Values=$GPU_INSTANCE_TYPE" "Name=location,Values=$AWS_REGION_NAME" \
    --query 'length(InstanceTypeOfferings)' \
    --output text)"
  if [[ "$offering_count" == "0" ]]; then
    echo "Instance type $GPU_INSTANCE_TYPE is not offered in $AWS_REGION_NAME." >&2
    exit 2
  fi

  quota_value="$(aws_cli service-quotas get-service-quota \
    --service-code ec2 \
    --quota-code L-DB2E81BA \
    --query 'Quota.Value' \
    --output text 2>/dev/null || true)"
  if [[ -n "$quota_value" && "$quota_value" != "None" ]]; then
    python3 - "$quota_value" <<'PY'
import sys
quota = float(sys.argv[1])
if quota < 4:
    raise SystemExit("On-Demand G/VT quota is below the 4 vCPU needed for g5.xlarge")
PY
    echo "GPU capacity checks passed: $GPU_INSTANCE_TYPE is offered and G/VT quota is $quota_value vCPU."
  else
    echo "GPU instance offering check passed; quota lookup unavailable, continuing."
  fi
}

check_public_admin_certificate() {
  if [[ "$ENABLE_PUBLIC_ADMIN_CONSOLE" != "true" ]]; then
    return 0
  fi
  if [[ -z "$PUBLIC_ADMIN_CERTIFICATE_ARN" ]]; then
    echo "Public admin console is enabled but PUBLIC_ADMIN_CERTIFICATE_ARN is empty." >&2
    echo "Provisioning can request DNS validation records, but the full admin endpoint will not be complete until an issued cert is supplied." >&2
    return 0
  fi

  local cert_json
  cert_json="$(aws_cli acm describe-certificate \
    --certificate-arn "$PUBLIC_ADMIN_CERTIFICATE_ARN" \
    --output json)"
  CERT_JSON="$cert_json" python3 - "$PUBLIC_ADMIN_DOMAIN_NAME" <<'PY'
import json
import os
import sys

wanted = sys.argv[1].strip().lower()
cert = json.loads(os.environ["CERT_JSON"])["Certificate"]
status = cert.get("Status")
if status != "ISSUED":
    raise SystemExit(f"certificate status is {status}, expected ISSUED")
names = [cert.get("DomainName", ""), *cert.get("SubjectAlternativeNames", [])]
names = [name.lower() for name in names if name]

def matches(pattern: str, host: str) -> bool:
    if pattern == host:
        return True
    if pattern.startswith("*."):
        suffix = pattern[1:]
        return host.endswith(suffix) and host.count(".") == pattern.count(".")
    return False

if not any(matches(name, wanted) for name in names):
    raise SystemExit(f"certificate does not cover {wanted}; names={names}")
print(f"Public admin certificate check passed: {wanted}")
PY
}

check_name_collisions() {
  local tagged_count dynamodb_count secret_count role_count alb_count asg_count
  tagged_count="$(aws_cli resourcegroupstaggingapi get-resources \
    --tag-filters "Key=Project,Values=$PROJECT_NAME" \
    --query 'length(ResourceTagMappingList)' \
    --output text)"
  dynamodb_count="$(aws_cli dynamodb list-tables \
    --query "length(TableNames[?starts_with(@, \`$PROJECT_NAME\`)])" \
    --output text)"
  secret_count="$(aws_cli secretsmanager list-secrets \
    --filters "Key=name,Values=$PROJECT_NAME" \
    --query 'length(SecretList)' \
    --output text)"
  role_count="$(aws --profile "$AWS_PROFILE_NAME" iam list-roles \
    --query "length(Roles[?starts_with(RoleName, \`$PROJECT_NAME\`)])" \
    --output text)"
  alb_count="$(aws_cli elbv2 describe-load-balancers \
    --query "length(LoadBalancers[?starts_with(LoadBalancerName, \`$PROJECT_NAME\`)])" \
    --output text)"
  asg_count="$(aws_cli autoscaling describe-auto-scaling-groups \
    --query "length(AutoScalingGroups[?starts_with(AutoScalingGroupName, \`$PROJECT_NAME\`)])" \
    --output text)"

  if (( tagged_count + dynamodb_count + secret_count + role_count + alb_count + asg_count > 0 )); then
    cat >&2 <<EOF
Existing AWS resources already use PROJECT_NAME=$PROJECT_NAME.
Counts: tagged=$tagged_count dynamodb=$dynamodb_count secrets=$secret_count roles=$role_count albs=$alb_count asgs=$asg_count
Choose a different PROJECT_NAME or clean up the previous replica first.
EOF
    exit 2
  fi
  echo "Name collision checks passed for PROJECT_NAME=$PROJECT_NAME."
}

select_terraform_workspace() {
  terraform_cli init -input=false
  if terraform_cli workspace list | sed 's/^[* ]*//' | grep -Fxq "$TERRAFORM_WORKSPACE_NAME"; then
    terraform_cli workspace select "$TERRAFORM_WORKSPACE_NAME" >/dev/null
  else
    terraform_cli workspace new "$TERRAFORM_WORKSPACE_NAME" >/dev/null
  fi
  export TF_WORKSPACE="$TERRAFORM_WORKSPACE_NAME"
  echo "Terraform workspace: $TERRAFORM_WORKSPACE_NAME"
}

preflight() {
  local account_id
  account_id="$(require_account)"
  echo "AWS account verified: $account_id"
  echo "Project name: $PROJECT_NAME"
  echo "Region: $AWS_REGION_NAME"
  echo "Terraform workspace: $TERRAFORM_WORKSPACE_NAME"

  if secret_exists; then
    echo "Hugging Face secret exists: $HF_SECRET_NAME"
  elif [[ -n "$SOURCE_HF_SECRET_NAME" ]] && aws_cli secretsmanager describe-secret --secret-id "$SOURCE_HF_SECRET_NAME" >/dev/null 2>&1; then
    echo "Source Hugging Face secret exists and will be copied to the replica secret: $SOURCE_HF_SECRET_NAME -> $HF_SECRET_NAME"
  elif [[ -n "$HF_TOKEN_FILE" || -n "${HF_TOKEN:-}" || -n "${HUGGINGFACE_TOKEN:-}" ]]; then
    echo "Hugging Face token input is present. Secret will be created during provisioning."
  else
    echo "Hugging Face token is missing and secret '$HF_SECRET_NAME' does not exist." >&2
    exit 2
  fi

  select_terraform_workspace
  terraform_cli validate
  check_build_tooling
  check_gpu_capacity
  check_public_admin_certificate
  check_name_collisions
  echo "Preflight passed. No AWS resources were created or changed."
}

secret_exists() {
  aws_cli secretsmanager describe-secret --secret-id "$HF_SECRET_NAME" >/dev/null 2>&1
}

create_or_update_hf_secret() {
  local secret_value=""
  local source=""

  if [[ -n "$HF_TOKEN_FILE" ]]; then
    if [[ ! -r "$HF_TOKEN_FILE" ]]; then
      echo "HF_TOKEN_FILE is set but is not readable: $HF_TOKEN_FILE" >&2
      exit 2
    fi
    secret_value="$(tr -d '\r\n' < "$HF_TOKEN_FILE")"
    source="file"
  elif [[ -n "${HF_TOKEN:-}" ]]; then
    secret_value="$HF_TOKEN"
    source="HF_TOKEN"
  elif [[ -n "${HUGGINGFACE_TOKEN:-}" ]]; then
    secret_value="$HUGGINGFACE_TOKEN"
    source="HUGGINGFACE_TOKEN"
  fi

  if [[ -z "$secret_value" && -n "$SOURCE_HF_SECRET_NAME" && "$SOURCE_HF_SECRET_NAME" != "$HF_SECRET_NAME" ]]; then
    secret_value="$(aws_cli secretsmanager get-secret-value \
      --secret-id "$SOURCE_HF_SECRET_NAME" \
      --query SecretString \
      --output text 2>/dev/null || true)"
    if [[ -n "$secret_value" && "$secret_value" != "None" ]]; then
      source="source secret $SOURCE_HF_SECRET_NAME"
    else
      secret_value=""
    fi
  fi

  if [[ -z "$secret_value" ]]; then
    if secret_exists; then
      echo "Using existing Hugging Face secret: $HF_SECRET_NAME"
      return 0
    fi
    cat >&2 <<EOF
No Hugging Face token was supplied and secret '$HF_SECRET_NAME' does not exist.
Provide HF_TOKEN_FILE, HF_TOKEN, HUGGINGFACE_TOKEN, pre-create the secret, or
set SOURCE_HF_SECRET_NAME to an existing source secret to copy.
EOF
    exit 2
  fi

  if secret_exists; then
    aws_cli secretsmanager put-secret-value \
      --secret-id "$HF_SECRET_NAME" \
      --secret-string "$secret_value" >/dev/null
    echo "Updated Hugging Face secret from $source: $HF_SECRET_NAME"
  else
    aws_cli secretsmanager create-secret \
      --name "$HF_SECRET_NAME" \
      --description "Hugging Face token for Cubicle transcription replica $PROJECT_NAME" \
      --secret-string "$secret_value" \
      --tags Key=Project,Value="$PROJECT_NAME" Key=Application,Value=Cubicle Key=Component,Value=transcription >/dev/null
    echo "Created Hugging Face secret from $source: $HF_SECRET_NAME"
  fi

  unset HF_TOKEN HUGGINGFACE_TOKEN
}

run_deploy_pass() {
  local pass_name="$1"
  local image_tag="$2"
  local asr_provider="$3"
  local vllm_realtime_url="$4"

  echo
  echo "==> Deploy pass: $pass_name"
  echo "    project: $PROJECT_NAME"
  echo "    image tag: $image_tag"
  echo "    ASR provider: $asr_provider"
  if [[ -n "$vllm_realtime_url" ]]; then
    echo "    vLLM realtime URL: $vllm_realtime_url"
  fi

  (
    export AWS_PROFILE_NAME
    export AWS_REGION_NAME
    export EXPECTED_ACCOUNT_ID
    export PROJECT_NAME
    export IMAGE_TAG="$image_tag"
    export TRANSCRIPTION_AUTH_MODE
    export TRANSCRIPTION_ALLOWED_USERS
    export TRANSCRIPTION_TOKEN_ISSUER
    export TRANSCRIPTION_TOKEN_AUDIENCE
    export TRANSCRIPTION_REQUIRED_SCOPE
    export ASR_PROVIDER="$asr_provider"
    export DIARIZATION_PROVIDER
    export INCLUDE_VOXTRAL_REALTIME=false
    export INCLUDE_DIARIZATION=false
    export INCLUDE_SELF_HOSTED_MODELS=false
    export ENABLE_VOXTRAL_RUNTIME=false
    export ENABLE_GPU_CAPACITY=true
    export ECS_LAUNCH_TYPE=FARGATE
    export GPU_MIN_SIZE
    export GPU_DESIRED_CAPACITY
    export GPU_MAX_SIZE
    export REQUIRE_GPU=false
    export MODELS_OFFLINE=false
    export VOXTRAL_MODEL
    export VOXTRAL_MODEL_VERSION
    export GPU_INSTANCE_TYPE
    if [[ -n "$vllm_realtime_url" ]]; then
      export VLLM_BASE_URL="http://${vllm_realtime_url#ws://}"
      VLLM_BASE_URL="${VLLM_BASE_URL%/v1/realtime}"
      export VLLM_REALTIME_URL="$vllm_realtime_url"
      export VOXTRAL_REALTIME_URL="$vllm_realtime_url"
    fi
    "$SCRIPT_DIR/deploy.sh"
  )
}

asg_instance_id() {
  aws_cli autoscaling describe-auto-scaling-groups \
    --auto-scaling-group-names "$ASG_NAME" \
    --query 'AutoScalingGroups[0].Instances[?LifecycleState==`InService`].InstanceId | [0]' \
    --output text
}

wait_for_gpu_instance() {
  local instance_id
  for _ in $(seq 1 120); do
    instance_id="$(asg_instance_id)"
    if [[ -n "$instance_id" && "$instance_id" != "None" ]]; then
      echo "$instance_id"
      return 0
    fi
    sleep 10
  done

  echo "Timed out waiting for an InService instance in $ASG_NAME." >&2
  exit 3
}

instance_private_ip() {
  local instance_id="$1"
  aws_cli ec2 describe-instances \
    --instance-ids "$instance_id" \
    --query 'Reservations[0].Instances[0].PrivateIpAddress' \
    --output text
}

wait_for_ssm() {
  local instance_id="$1"
  local status
  for _ in $(seq 1 90); do
    status="$(aws_cli ssm describe-instance-information \
      --filters "Key=InstanceIds,Values=$instance_id" \
      --query 'InstanceInformationList[0].PingStatus' \
      --output text 2>/dev/null || true)"
    if [[ "$status" == "Online" ]]; then
      echo "SSM Online: $instance_id"
      return 0
    fi
    sleep 10
  done

  echo "Timed out waiting for SSM Online on $instance_id." >&2
  exit 4
}

grant_hf_secret_access() {
  local secret_arn policy_file
  secret_arn="$(aws_cli secretsmanager describe-secret \
    --secret-id "$HF_SECRET_NAME" \
    --query ARN \
    --output text)"

  policy_file="$(mktemp "${TMPDIR:-/tmp}/cubicle-hf-secret-policy.XXXXXX.json")"
  cat > "$policy_file" <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "secretsmanager:GetSecretValue",
      "Resource": "$secret_arn"
    }
  ]
}
EOF

  aws --profile "$AWS_PROFILE_NAME" iam put-role-policy \
    --role-name "$INSTANCE_ROLE_NAME" \
    --policy-name cubicle-transcription-hf-secret-read \
    --policy-document "file://$policy_file" >/dev/null

  rm -f "$policy_file"
  echo "Granted $INSTANCE_ROLE_NAME read access to $HF_SECRET_NAME."
}

send_ssm_script() {
  local instance_id="$1"
  local script_path="$2"
  local params_file command_id status stdout stderr

  params_file="$(mktemp "${TMPDIR:-/tmp}/cubicle-ssm-params.XXXXXX.json")"
  python3 - "$script_path" "$params_file" <<'PY'
import json
import sys
from pathlib import Path

script_path = Path(sys.argv[1])
params_path = Path(sys.argv[2])
params_path.write_text(json.dumps({"commands": [script_path.read_text()]}))
PY

  command_id="$(aws_cli ssm send-command \
    --instance-ids "$instance_id" \
    --document-name AWS-RunShellScript \
    --comment "Cubicle transcription replica vLLM bring-up" \
    --parameters "file://$params_file" \
    --query Command.CommandId \
    --output text)"
  rm -f "$params_file"

  echo "SSM command: $command_id"

  for _ in $(seq 1 240); do
    status="$(aws_cli ssm get-command-invocation \
      --command-id "$command_id" \
      --instance-id "$instance_id" \
      --query Status \
      --output text 2>/dev/null || true)"
    case "$status" in
      Success|Cancelled|TimedOut|Failed|Cancelling)
        break
        ;;
    esac
    sleep 10
  done

  stdout="$(aws_cli ssm get-command-invocation \
    --command-id "$command_id" \
    --instance-id "$instance_id" \
    --query StandardOutputContent \
    --output text 2>/dev/null || true)"
  stderr="$(aws_cli ssm get-command-invocation \
    --command-id "$command_id" \
    --instance-id "$instance_id" \
    --query StandardErrorContent \
    --output text 2>/dev/null || true)"

  if [[ -n "$stdout" && "$stdout" != "None" ]]; then
    echo "$stdout"
  fi
  if [[ -n "$stderr" && "$stderr" != "None" ]]; then
    echo "$stderr" >&2
  fi

  if [[ "$status" != "Success" ]]; then
    echo "SSM command $command_id ended with status $status." >&2
    exit 5
  fi
}

start_vllm_on_instance() {
  local instance_id="$1"
  local remote_script

  remote_script="$(mktemp "${TMPDIR:-/tmp}/cubicle-vllm-remote.XXXXXX.sh")"
  cat > "$remote_script" <<EOF
#!/bin/bash
set -euo pipefail

export AWS_DEFAULT_REGION="$AWS_REGION_NAME"
CONTAINER_NAME="$VLLM_CONTAINER_NAME"
IMAGE_URI="$VLLM_IMAGE_URI"
MODEL_ID="$VOXTRAL_MODEL"
HOST_PORT="$VLLM_HOST_PORT"
MODEL_LEN="$VLLM_MODEL_LEN"
GPU_MEMORY_UTILIZATION="$VLLM_GPU_MEMORY_UTILIZATION"
HF_CACHE_DIR="$HF_CACHE_DIR"
HF_SECRET_NAME="$HF_SECRET_NAME"
READINESS_ATTEMPTS="$READINESS_ATTEMPTS"
READINESS_SLEEP_SECONDS="$READINESS_SLEEP_SECONDS"

if ! command -v docker >/dev/null 2>&1; then
  yum install -y docker
fi

systemctl enable --now docker
mkdir -p "\$HF_CACHE_DIR"
chmod 700 "\$HF_CACHE_DIR"

if ! docker info >/dev/null 2>&1; then
  echo "Docker is not healthy." >&2
  exit 10
fi

if command -v nvidia-smi >/dev/null 2>&1; then
  nvidia-smi
else
  echo "nvidia-smi is not present; continuing because ECS GPU AMI may expose GPU only after driver service start." >&2
fi

HF_TOKEN_VALUE="\$(aws secretsmanager get-secret-value \
  --region "$AWS_REGION_NAME" \
  --secret-id "\$HF_SECRET_NAME" \
  --query SecretString \
  --output text)"

if [[ -z "\$HF_TOKEN_VALUE" || "\$HF_TOKEN_VALUE" == "None" ]]; then
  echo "Hugging Face token secret is empty." >&2
  exit 11
fi

env_file="/root/cubicle-vllm.env"
umask 077
printf 'HF_TOKEN=%s\nVLLM_DISABLE_COMPILE_CACHE=1\n' "\$HF_TOKEN_VALUE" > "\$env_file"
unset HF_TOKEN_VALUE

docker pull "\$IMAGE_URI"
docker rm -f "\$CONTAINER_NAME" >/dev/null 2>&1 || true

START_CMD='python3 -m pip install --no-cache-dir "mistral-common[soundfile]" soundfile && exec vllm serve '"\$MODEL_ID"' --host 0.0.0.0 --port '"\$HOST_PORT"' --tokenizer-mode mistral --max-model-len '"\$MODEL_LEN"' --gpu-memory-utilization '"\$GPU_MEMORY_UTILIZATION"' --compilation_config '\''{"cudagraph_mode":"PIECEWISE"}'\'''

docker run -d \
  --name "\$CONTAINER_NAME" \
  --gpus all \
  --ipc=host \
  --restart unless-stopped \
  -p "\$HOST_PORT:\$HOST_PORT" \
  -v "\$HF_CACHE_DIR:/root/.cache/huggingface" \
  --env-file "\$env_file" \
  --entrypoint /bin/bash \
  "\$IMAGE_URI" \
  -lc "\$START_CMD"

rm -f "\$env_file"

for _ in \$(seq 1 "\$READINESS_ATTEMPTS"); do
  if curl -fsS "http://127.0.0.1:\$HOST_PORT/health" >/dev/null; then
    curl -fsS "http://127.0.0.1:\$HOST_PORT/v1/models" | head -c 1000
    echo
    echo "[success] vLLM Voxtral runtime is ready on 127.0.0.1:\$HOST_PORT"
    exit 0
  fi
  docker logs --tail 20 "\$CONTAINER_NAME" || true
  sleep "\$READINESS_SLEEP_SECONDS"
done

echo "Timed out waiting for vLLM health." >&2
docker logs --tail 200 "\$CONTAINER_NAME" >&2 || true
exit 12
EOF

  send_ssm_script "$instance_id" "$remote_script"
  rm -f "$remote_script"
}

deploy_public_admin_console_if_requested() {
  local vllm_realtime_url="$1"

  if [[ "$ENABLE_PUBLIC_ADMIN_CONSOLE" != "true" ]]; then
    return 0
  fi

  local repository_url image_uri pool_id
  repository_url="$(terraform_cli output -raw repository_url)"
  image_uri="$repository_url:$FINAL_IMAGE_TAG"

  if [[ -z "$PUBLIC_ADMIN_CERTIFICATE_ARN" ]]; then
    echo
    echo "ENABLE_PUBLIC_ADMIN_CONSOLE=true but PUBLIC_ADMIN_CERTIFICATE_ARN is empty."
    echo "Requesting ACM validation records only; add the DNS validation CNAME, wait for ISSUED, then rerun with PUBLIC_ADMIN_CERTIFICATE_ARN."
    terraform_cli apply -input=false -auto-approve \
      -var "aws_profile=$AWS_PROFILE_NAME" \
      -var "aws_region=$AWS_REGION_NAME" \
      -var "expected_account_id=$EXPECTED_ACCOUNT_ID" \
      -var "project_name=$PROJECT_NAME" \
      -var "image_uri=$image_uri" \
      -var "public_admin_domain_name=$PUBLIC_ADMIN_DOMAIN_NAME" \
      -var "public_admin_request_certificate=true"
    terraform_cli output admin_public_requested_certificate_arn || true
    terraform_cli output admin_public_certificate_validation_records || true
    cat >&2 <<'EOF'

Replica is not complete yet: the public admin console needs an issued ACM
certificate before users/tokens can be managed through the admin UI.
Add the DNS validation record above, wait for the certificate to become
ISSUED, then rerun this script with PUBLIC_ADMIN_CERTIFICATE_ARN set.
EOF
    exit 9
  fi

  echo
  echo "==> Enabling public Cognito/WAF admin console"
  terraform_cli apply -input=false -auto-approve \
    -var "aws_profile=$AWS_PROFILE_NAME" \
    -var "aws_region=$AWS_REGION_NAME" \
    -var "expected_account_id=$EXPECTED_ACCOUNT_ID" \
    -var "project_name=$PROJECT_NAME" \
    -var "image_uri=$image_uri" \
    -var "admin_image_uri=$image_uri" \
    -var "auth_mode=$TRANSCRIPTION_AUTH_MODE" \
    -var "allowed_users=$TRANSCRIPTION_ALLOWED_USERS" \
    -var "token_issuer=$TRANSCRIPTION_TOKEN_ISSUER" \
    -var "token_audience=$TRANSCRIPTION_TOKEN_AUDIENCE" \
    -var "required_scope=$TRANSCRIPTION_REQUIRED_SCOPE" \
    -var "asr_provider=voxtral_self_hosted" \
    -var "voxtral_realtime_url=$vllm_realtime_url" \
    -var "diarization_provider=$DIARIZATION_PROVIDER" \
    -var "enable_gpu_capacity=true" \
    -var "gpu_min_size=$GPU_MIN_SIZE" \
    -var "gpu_desired_capacity=$GPU_DESIRED_CAPACITY" \
    -var "gpu_max_size=$GPU_MAX_SIZE" \
    -var "gpu_instance_type=$GPU_INSTANCE_TYPE" \
    -var "enable_public_admin_console=true" \
    -var "public_admin_domain_name=$PUBLIC_ADMIN_DOMAIN_NAME" \
    -var "public_admin_certificate_arn=$PUBLIC_ADMIN_CERTIFICATE_ARN" \
    -var "public_admin_allowed_cidr_blocks=$PUBLIC_ADMIN_ALLOWED_CIDR_BLOCKS_HCL" \
    -var "public_admin_cognito_domain_prefix=$PUBLIC_ADMIN_COGNITO_DOMAIN_PREFIX" \
    -var "public_admin_waf_rate_limit=$PUBLIC_ADMIN_WAF_RATE_LIMIT" \
    -var "admin_desired_count=$ADMIN_DESIRED_COUNT"

  if [[ -n "$ADMIN_EMAIL" ]]; then
    pool_id="$(terraform_cli output -raw admin_public_cognito_user_pool_id)"
    aws_cli cognito-idp admin-create-user \
      --user-pool-id "$pool_id" \
      --username "$ADMIN_EMAIL" \
      --user-attributes Name=email,Value="$ADMIN_EMAIL" Name=email_verified,Value=true \
      >/dev/null || true
    aws_cli cognito-idp admin-add-user-to-group \
      --user-pool-id "$pool_id" \
      --username "$ADMIN_EMAIL" \
      --group-name CubicleTranscriptionAdmins \
      >/dev/null
    echo "Ensured Cognito admin user and group membership for $ADMIN_EMAIL."
  fi
}

main() {
  local account_id instance_id private_ip realtime_url websocket_endpoint health_endpoint

  account_id="$(require_account)"
  echo "AWS account verified: $account_id"
  echo "Project name: $PROJECT_NAME"
  echo "Region: $AWS_REGION_NAME"
  echo "Terraform workspace: $TERRAFORM_WORKSPACE_NAME"

  select_terraform_workspace

  create_or_update_hf_secret

  run_deploy_pass "bootstrap-stack-and-gpu" "$BOOTSTRAP_IMAGE_TAG" "mock" ""

  instance_id="$(wait_for_gpu_instance)"
  private_ip="$(instance_private_ip "$instance_id")"
  if [[ -z "$private_ip" || "$private_ip" == "None" ]]; then
    echo "Could not determine private IP for $instance_id." >&2
    exit 6
  fi

  echo "GPU instance: $instance_id"
  echo "GPU private IP: $private_ip"

  grant_hf_secret_access
  wait_for_ssm "$instance_id"
  start_vllm_on_instance "$instance_id"

  realtime_url="ws://$private_ip:$VLLM_HOST_PORT/v1/realtime"
  run_deploy_pass "direct-aws-adapter" "$FINAL_IMAGE_TAG" "voxtral_self_hosted" "$realtime_url"

  deploy_public_admin_console_if_requested "$realtime_url"

  websocket_endpoint="$(terraform_cli output -raw websocket_endpoint)"
  health_endpoint="$(terraform_cli output -raw health_endpoint)"

  cat <<EOF

Replica provisioning complete.

AWS account:       $account_id
Project:           $PROJECT_NAME
Region:            $AWS_REGION_NAME
Terraform state:   workspace $TERRAFORM_WORKSPACE_NAME
GPU instance:      $instance_id
GPU private IP:    $private_ip
vLLM realtime URL: $realtime_url
Client WSS URL:    $websocket_endpoint
Health URL:        $health_endpoint

Store the client WSS URL in Cubicle Settings > Live Transcription > AWS endpoint.
Issue per-user transcription tokens through the admin console when enabled, or
mint signed tokens from $PROJECT_NAME/user-token-signing-key and save them in
Cubicle's local Keychain through Settings.
EOF
}

case "$COMMAND" in
  preflight)
    preflight
    ;;
  provision)
    main "$@"
    ;;
  *)
    echo "Unknown command: $COMMAND" >&2
    usage >&2
    exit 2
    ;;
esac
