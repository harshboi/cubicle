#!/usr/bin/env bash
set -euo pipefail

AWS_PROFILE_NAME="${AWS_PROFILE_NAME:-strln}"
AWS_REGION_NAME="${AWS_REGION_NAME:-us-west-2}"
EXPECTED_ACCOUNT_ID="${EXPECTED_ACCOUNT_ID:-562304353751}"
PROJECT_NAME="${PROJECT_NAME:-cubicle-transcription}"
ASG_NAME="${ASG_NAME:-$PROJECT_NAME-gpu}"
HF_SECRET_NAME="${HF_SECRET_NAME:-$PROJECT_NAME/huggingface-token}"

LAUNCH_TEMPLATE_ID="${LAUNCH_TEMPLATE_ID:-}"
LAUNCH_TEMPLATE_VERSION="${LAUNCH_TEMPLATE_VERSION:-\$Latest}"
SUBNET_ID="${SUBNET_ID:-}"
INSTANCE_TYPE="${INSTANCE_TYPE:-}"
INSTANCE_NAME="${INSTANCE_NAME:-$PROJECT_NAME-vllm-manual-$(date +%Y%m%d%H%M%S)}"
INSTANCE_ROLE_NAME="${INSTANCE_ROLE_NAME:-}"

IMAGE_URI="${IMAGE_URI:-vllm/vllm-openai:v0.21.0-ubuntu2404}"
VOXTRAL_MODEL="${VOXTRAL_MODEL:-mistralai/Voxtral-Mini-4B-Realtime-2602}"
CONTAINER_NAME="${CONTAINER_NAME:-voxtral-vllm}"
HOST_PORT="${HOST_PORT:-8000}"
MODEL_LEN="${MODEL_LEN:-45000}"
GPU_MEMORY_UTILIZATION="${GPU_MEMORY_UTILIZATION:-0.90}"
HF_CACHE_DIR="${HF_CACHE_DIR:-/home/ssm-user/.cache/huggingface}"
READINESS_ATTEMPTS="${READINESS_ATTEMPTS:-240}"
READINESS_SLEEP_SECONDS="${READINESS_SLEEP_SECONDS:-10}"

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
  GetWebexSpaceMac/infra/transcription/launch-vllm-ec2-instance.sh
  GetWebexSpaceMac/infra/transcription/launch-vllm-ec2-instance.sh preflight

This creates exactly one standalone EC2 GPU instance for the Voxtral/vLLM
runtime. It does not create a backup stack and does not clone CloudFront, ALB,
ECS, DynamoDB, Cognito, or the admin console.

Default target:
  AWS_PROFILE_NAME=strln
  AWS_REGION_NAME=us-west-2
  EXPECTED_ACCOUNT_ID=562304353751
  PROJECT_NAME=cubicle-transcription
  ASG_NAME=cubicle-transcription-gpu
  HF_SECRET_NAME=cubicle-transcription/huggingface-token

What it does:
  1. Verifies the AWS account guard.
  2. Discovers the existing GPU launch template and subnet from ASG_NAME.
  3. Discovers the EC2 instance role from that launch template.
  4. Ensures that role can read the Hugging Face token secret.
  5. Launches one standalone EC2 instance from the launch template.
  6. Waits for SSM.
  7. Uses SSM Run Command to start Dockerized vLLM/Voxtral on port 8000.
  8. Prints the instance id, private IP, internal vLLM URL, and terminate command.

Useful overrides:
  INSTANCE_TYPE=g5.xlarge
  INSTANCE_NAME=cubicle-transcription-vllm-test
  LAUNCH_TEMPLATE_ID=lt-...
  SUBNET_ID=subnet-...
  HF_SECRET_NAME=cubicle-transcription/huggingface-token

Important:
  This creates a billable GPU EC2 instance. It does not switch the active ECS
  adapter to the new instance. To switch the app-facing service after testing,
  redeploy the direct adapter with the printed private IP.

Run `preflight` first to validate account, launch template, subnet, IAM role,
HF secret access, regional instance-type offering, and EC2 RunInstances dry-run.
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" || "${1:-}" == "help" ]]; then
  usage
  exit 0
fi

COMMAND="${1:-launch}"

require_tool() {
  local tool="$1"
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "Missing required tool: $tool" >&2
    exit 2
  fi
}

for tool in aws python3; do
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

This EC2 launch is pinned to AWS profile '$AWS_PROFILE_NAME' and account '$EXPECTED_ACCOUNT_ID'.
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

require_account() {
  local account_id
  account_id="$(aws_cli sts get-caller-identity --query Account --output text)"
  if [[ "$account_id" != "$EXPECTED_ACCOUNT_ID" ]]; then
    echo "Refusing to launch in AWS account $account_id; expected $EXPECTED_ACCOUNT_ID." >&2
    exit 2
  fi
  echo "$account_id"
}

discover_launch_template_id() {
  if [[ -n "$LAUNCH_TEMPLATE_ID" ]]; then
    echo "$LAUNCH_TEMPLATE_ID"
    return 0
  fi

  local lt_id
  lt_id="$(aws_cli autoscaling describe-auto-scaling-groups \
    --auto-scaling-group-names "$ASG_NAME" \
    --query 'AutoScalingGroups[0].LaunchTemplate.LaunchTemplateId' \
    --output text 2>/dev/null || true)"

  if [[ -z "$lt_id" || "$lt_id" == "None" ]]; then
    lt_id="$(aws_cli autoscaling describe-auto-scaling-groups \
      --auto-scaling-group-names "$ASG_NAME" \
      --query 'AutoScalingGroups[0].MixedInstancesPolicy.LaunchTemplate.LaunchTemplateSpecification.LaunchTemplateId' \
      --output text 2>/dev/null || true)"
  fi

  if [[ -z "$lt_id" || "$lt_id" == "None" ]]; then
    echo "Could not discover a launch template from ASG $ASG_NAME. Set LAUNCH_TEMPLATE_ID." >&2
    exit 3
  fi

  echo "$lt_id"
}

discover_subnet_id() {
  if [[ -n "$SUBNET_ID" ]]; then
    echo "$SUBNET_ID"
    return 0
  fi

  local subnet_csv first_subnet
  subnet_csv="$(aws_cli autoscaling describe-auto-scaling-groups \
    --auto-scaling-group-names "$ASG_NAME" \
    --query 'AutoScalingGroups[0].VPCZoneIdentifier' \
    --output text 2>/dev/null || true)"

  first_subnet="${subnet_csv%%,*}"
  if [[ -z "$first_subnet" || "$first_subnet" == "None" ]]; then
    echo "Could not discover a subnet from ASG $ASG_NAME. Set SUBNET_ID." >&2
    exit 3
  fi

  echo "$first_subnet"
}

discover_instance_role_name() {
  if [[ -n "$INSTANCE_ROLE_NAME" ]]; then
    echo "$INSTANCE_ROLE_NAME"
    return 0
  fi

  local lt_id="$1" profile_name role_name
  profile_name="$(aws_cli ec2 describe-launch-template-versions \
    --launch-template-id "$lt_id" \
    --versions "$LAUNCH_TEMPLATE_VERSION" \
    --query 'LaunchTemplateVersions[0].LaunchTemplateData.IamInstanceProfile.Name' \
    --output text)"

  if [[ -z "$profile_name" || "$profile_name" == "None" ]]; then
    echo "Launch template $lt_id has no IAM instance profile. Set INSTANCE_ROLE_NAME or use a different launch template." >&2
    exit 3
  fi

  role_name="$(aws_global iam get-instance-profile \
    --instance-profile-name "$profile_name" \
    --query 'InstanceProfile.Roles[0].RoleName' \
    --output text)"

  if [[ -z "$role_name" || "$role_name" == "None" ]]; then
    echo "Could not discover role from instance profile $profile_name." >&2
    exit 3
  fi

  echo "$role_name"
}

ensure_secret_access() {
  local role_name="$1" secret_arn policy_file
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

  aws_global iam put-role-policy \
    --role-name "$role_name" \
    --policy-name cubicle-transcription-hf-secret-read \
    --policy-document "file://$policy_file" >/dev/null
  rm -f "$policy_file"

  echo "Granted $role_name read access to $HF_SECRET_NAME."
}

launch_instance() {
  local lt_id="$1" subnet_id="$2" account_id="$3"
  local args instance_id

  args=(
    ec2 run-instances
    --launch-template "LaunchTemplateId=$lt_id,Version=$LAUNCH_TEMPLATE_VERSION"
    --subnet-id "$subnet_id"
    --count 1
    --metadata-options "HttpTokens=required,HttpEndpoint=enabled,HttpPutResponseHopLimit=2"
    --tag-specifications
    "ResourceType=instance,Tags=[{Key=Name,Value=$INSTANCE_NAME},{Key=Project,Value=$PROJECT_NAME},{Key=Application,Value=Cubicle},{Key=Component,Value=transcription-vllm-ec2},{Key=ManagedBy,Value=launch-vllm-ec2-instance},{Key=Account,Value=$account_id}]"
    "ResourceType=volume,Tags=[{Key=Name,Value=$INSTANCE_NAME},{Key=Project,Value=$PROJECT_NAME},{Key=Application,Value=Cubicle},{Key=Component,Value=transcription-vllm-ec2},{Key=ManagedBy,Value=launch-vllm-ec2-instance}]"
  )

  if [[ -n "$INSTANCE_TYPE" ]]; then
    args+=(--instance-type "$INSTANCE_TYPE")
  fi

  args+=(--query 'Instances[0].InstanceId' --output text)

  instance_id="$(aws_cli "${args[@]}")"
  if [[ -z "$instance_id" || "$instance_id" == "None" ]]; then
    echo "EC2 run-instances did not return an instance id." >&2
    exit 4
  fi

  echo "$instance_id"
}

dry_run_launch() {
  local lt_id="$1" subnet_id="$2" account_id="$3"
  local args output rc

  args=(
    ec2 run-instances
    --dry-run
    --launch-template "LaunchTemplateId=$lt_id,Version=$LAUNCH_TEMPLATE_VERSION"
    --subnet-id "$subnet_id"
    --count 1
    --metadata-options "HttpTokens=required,HttpEndpoint=enabled,HttpPutResponseHopLimit=2"
    --tag-specifications
    "ResourceType=instance,Tags=[{Key=Name,Value=$INSTANCE_NAME},{Key=Project,Value=$PROJECT_NAME},{Key=Application,Value=Cubicle},{Key=Component,Value=transcription-vllm-ec2},{Key=ManagedBy,Value=launch-vllm-ec2-instance},{Key=Account,Value=$account_id}]"
    "ResourceType=volume,Tags=[{Key=Name,Value=$INSTANCE_NAME},{Key=Project,Value=$PROJECT_NAME},{Key=Application,Value=Cubicle},{Key=Component,Value=transcription-vllm-ec2},{Key=ManagedBy,Value=launch-vllm-ec2-instance}]"
    --query 'Instances[0].InstanceId'
    --output text
  )

  if [[ -n "$INSTANCE_TYPE" ]]; then
    args+=(--instance-type "$INSTANCE_TYPE")
  fi

  set +e
  output="$(aws_cli "${args[@]}" 2>&1)"
  rc=$?
  set -e

  if [[ "$output" == *"DryRunOperation"* ]]; then
    echo "EC2 RunInstances dry-run passed."
    return 0
  fi

  echo "$output" >&2
  return "$rc"
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
  exit 5
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
    --comment "Cubicle standalone EC2 vLLM start" \
    --parameters "file://$params_file" \
    --query Command.CommandId \
    --output text)"
  rm -f "$params_file"

  echo "SSM command: $command_id"

  for _ in $(seq 1 300); do
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
    exit 6
  fi
}

start_vllm() {
  local instance_id="$1"
  local remote_script

  remote_script="$(mktemp "${TMPDIR:-/tmp}/cubicle-start-standalone-vllm.XXXXXX.sh")"
  cat > "$remote_script" <<REMOTE
#!/bin/bash
set -euo pipefail

AWS_REGION_NAME='$AWS_REGION_NAME'
HF_SECRET_NAME='$HF_SECRET_NAME'
IMAGE_URI='$IMAGE_URI'
VOXTRAL_MODEL='$VOXTRAL_MODEL'
CONTAINER_NAME='$CONTAINER_NAME'
HOST_PORT='$HOST_PORT'
MODEL_LEN='$MODEL_LEN'
GPU_MEMORY_UTILIZATION='$GPU_MEMORY_UTILIZATION'
HF_CACHE_DIR='$HF_CACHE_DIR'
READINESS_ATTEMPTS='$READINESS_ATTEMPTS'
READINESS_SLEEP_SECONDS='$READINESS_SLEEP_SECONDS'

if ! command -v aws >/dev/null 2>&1; then
  yum install -y awscli
fi

if ! command -v docker >/dev/null 2>&1; then
  yum install -y docker
fi

systemctl enable --now docker
mkdir -p "\$HF_CACHE_DIR"
chmod 700 "\$HF_CACHE_DIR"

if command -v nvidia-smi >/dev/null 2>&1; then
  nvidia-smi
else
  echo "nvidia-smi is not available; continuing to Docker start so container logs show the failure if GPU runtime is missing." >&2
fi

HF_TOKEN_VALUE="\$(aws secretsmanager get-secret-value \
  --region "\$AWS_REGION_NAME" \
  --secret-id "\$HF_SECRET_NAME" \
  --query SecretString \
  --output text)"

if [[ -z "\$HF_TOKEN_VALUE" || "\$HF_TOKEN_VALUE" == "None" ]]; then
  echo "Hugging Face token secret is empty." >&2
  exit 11
fi

env_file="/root/cubicle-vllm.env"
umask 077
printf 'HF_TOKEN=%s\nVLLM_DISABLE_COMPILE_CACHE=1\nVLLM_NO_USAGE_STATS=1\nDO_NOT_TRACK=1\n' "\$HF_TOKEN_VALUE" > "\$env_file"
unset HF_TOKEN_VALUE

docker pull "\$IMAGE_URI"
docker rm -f "\$CONTAINER_NAME" >/dev/null 2>&1 || true

START_CMD='python3 -m pip install --no-cache-dir "mistral-common[soundfile]" soundfile && exec vllm serve '"\$VOXTRAL_MODEL"' --host 0.0.0.0 --port 8000 --tokenizer-mode mistral --max-model-len '"\$MODEL_LEN"' --gpu-memory-utilization '"\$GPU_MEMORY_UTILIZATION"' --compilation_config '\''{"cudagraph_mode":"PIECEWISE"}'\'''

docker run -d \
  --name "\$CONTAINER_NAME" \
  --gpus all \
  --ipc=host \
  --restart unless-stopped \
  -p "\$HOST_PORT:8000" \
  -v "\$HF_CACHE_DIR:/root/.cache/huggingface" \
  --env-file "\$env_file" \
  --entrypoint /bin/bash \
  "\$IMAGE_URI" \
  -lc "\$START_CMD"

rm -f "\$env_file"

for attempt in \$(seq 1 "\$READINESS_ATTEMPTS"); do
  if curl -fsS "http://127.0.0.1:\$HOST_PORT/health" >/tmp/cubicle-vllm-health.txt \
    && curl -fsS "http://127.0.0.1:\$HOST_PORT/v1/models" >/tmp/cubicle-vllm-models.json; then
    cat /tmp/cubicle-vllm-health.txt
    echo
    cat /tmp/cubicle-vllm-models.json
    echo
    docker ps --filter "name=\$CONTAINER_NAME"
    echo "[success] Voxtral vLLM runtime is ready on host port \$HOST_PORT"
    exit 0
  fi

  if ! docker ps --format '{{.Names}}' | grep -qx "\$CONTAINER_NAME"; then
    echo "[error] container exited"
    docker logs --tail 240 "\$CONTAINER_NAME" || true
    exit 20
  fi

  if (( attempt % 6 == 0 )); then
    echo "[info] attempt=\$attempt still starting"
    docker logs --tail 40 "\$CONTAINER_NAME" || true
  fi

  sleep "\$READINESS_SLEEP_SECONDS"
done

echo "[error] vLLM did not become ready"
docker logs --tail 240 "\$CONTAINER_NAME" || true
exit 21
REMOTE

  send_ssm_script "$instance_id" "$remote_script"
  rm -f "$remote_script"
}

private_ip_for_instance() {
  local instance_id="$1"
  aws_cli ec2 describe-instances \
    --instance-ids "$instance_id" \
    --query 'Reservations[0].Instances[0].PrivateIpAddress' \
    --output text
}

launch_template_instance_type() {
  local lt_id="$1"
  aws_cli ec2 describe-launch-template-versions \
    --launch-template-id "$lt_id" \
    --versions "$LAUNCH_TEMPLATE_VERSION" \
    --query 'LaunchTemplateVersions[0].LaunchTemplateData.InstanceType' \
    --output text
}

subnet_az() {
  local subnet_id="$1"
  aws_cli ec2 describe-subnets \
    --subnet-ids "$subnet_id" \
    --query 'Subnets[0].AvailabilityZone' \
    --output text
}

check_instance_type_offering() {
  local lt_id="$1" subnet_id="$2" instance_type az count
  instance_type="${INSTANCE_TYPE:-$(launch_template_instance_type "$lt_id")}"
  az="$(subnet_az "$subnet_id")"
  count="$(aws_cli ec2 describe-instance-type-offerings \
    --location-type availability-zone \
    --filters "Name=instance-type,Values=$instance_type" "Name=location,Values=$az" \
    --query 'length(InstanceTypeOfferings)' \
    --output text)"

  if [[ "$count" != "1" ]]; then
    echo "Instance type $instance_type is not offered in $az." >&2
    exit 7
  fi
  echo "Instance type offering exists: $instance_type in $az."
}

check_role_secret_access() {
  local role_name="$1" role_arn secret_arn decision
  role_arn="$(aws_global iam get-role \
    --role-name "$role_name" \
    --query 'Role.Arn' \
    --output text)"
  secret_arn="$(aws_cli secretsmanager describe-secret \
    --secret-id "$HF_SECRET_NAME" \
    --query ARN \
    --output text)"

  decision="$(aws_global iam simulate-principal-policy \
    --policy-source-arn "$role_arn" \
    --action-names secretsmanager:GetSecretValue \
    --resource-arns "$secret_arn" \
    --query 'EvaluationResults[0].EvalDecision' \
    --output text 2>/dev/null || true)"

  if [[ "$decision" == "allowed" ]]; then
    echo "Role can read HF secret: $role_name -> $HF_SECRET_NAME."
    return 0
  fi

  echo "Role secret-read simulation returned '$decision'. The launch path will call PutRolePolicy before start." >&2
}

check_role_ssm_policy() {
  local role_name="$1"
  if aws_global iam list-attached-role-policies \
    --role-name "$role_name" \
    --query 'AttachedPolicies[].PolicyName' \
    --output text | grep -q 'AmazonSSMManagedInstanceCore'; then
    echo "Role has AmazonSSMManagedInstanceCore: $role_name."
    return 0
  fi

  echo "Role is missing AmazonSSMManagedInstanceCore: $role_name." >&2
  exit 8
}

main() {
  local account_id lt_id subnet_id role_name instance_id private_ip

  account_id="$(require_account)"
  lt_id="$(discover_launch_template_id)"
  subnet_id="$(discover_subnet_id)"
  role_name="$(discover_instance_role_name "$lt_id")"

  echo "AWS account:       $account_id"
  echo "Region:            $AWS_REGION_NAME"
  echo "Launch template:   $lt_id ($LAUNCH_TEMPLATE_VERSION)"
  echo "Subnet:            $subnet_id"
  echo "Instance role:     $role_name"
  echo "Instance name:     $INSTANCE_NAME"
  echo "Model:             $VOXTRAL_MODEL"

  if [[ "$COMMAND" == "preflight" ]]; then
    aws_cli secretsmanager describe-secret --secret-id "$HF_SECRET_NAME" --query 'Name' --output text >/dev/null
    check_role_ssm_policy "$role_name"
    check_role_secret_access "$role_name"
    check_instance_type_offering "$lt_id" "$subnet_id"
    dry_run_launch "$lt_id" "$subnet_id" "$account_id"
    echo "Preflight passed. No EC2 instance was created."
    return 0
  elif [[ "$COMMAND" != "launch" ]]; then
    echo "Unknown command: $COMMAND" >&2
    usage >&2
    exit 2
  fi

  ensure_secret_access "$role_name"

  instance_id="$(launch_instance "$lt_id" "$subnet_id" "$account_id")"
  echo "Launched instance: $instance_id"

  aws_cli ec2 wait instance-running --instance-ids "$instance_id"
  private_ip="$(private_ip_for_instance "$instance_id")"
  echo "Private IP:        $private_ip"

  wait_for_ssm "$instance_id"
  start_vllm "$instance_id"

  cat <<EOF

Standalone EC2 vLLM instance is ready.

Instance id:       $instance_id
Private IP:        $private_ip
Internal vLLM URL: ws://$private_ip:$HOST_PORT/v1/realtime
Health check:      http://$private_ip:$HOST_PORT/health

This script did not switch Cubicle traffic.
To point the existing AWS adapter at this new instance, run:

  cd /Volumes/Webex/getwebexspace-data/GetWebexSpaceMac
  VLLM_INSTANCE_ID=$instance_id \\
  VLLM_PRIVATE_IP=$private_ip \\
  AWS_PROFILE_NAME=$AWS_PROFILE_NAME \\
  AWS_REGION_NAME=$AWS_REGION_NAME \\
  EXPECTED_ACCOUNT_ID=$EXPECTED_ACCOUNT_ID \\
  ./infra/transcription/deploy-direct-aws-adapter.sh

To terminate this billable instance when done:

  aws --profile $AWS_PROFILE_NAME --region $AWS_REGION_NAME ec2 terminate-instances --instance-ids $instance_id
EOF
}

main "$@"
