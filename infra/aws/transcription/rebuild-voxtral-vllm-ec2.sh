#!/usr/bin/env bash
set -euo pipefail

AWS_PROFILE_NAME="${AWS_PROFILE_NAME:-strln}"
AWS_REGION_NAME="${AWS_REGION_NAME:-us-west-2}"
EXPECTED_ACCOUNT_ID="${EXPECTED_ACCOUNT_ID:-562304353751}"
ASG_NAME="${ASG_NAME:-cubicle-transcription-gpu}"
INSTANCE_ROLE_NAME="${INSTANCE_ROLE_NAME:-cubicle-transcription-ecs-gpu-instance}"
HF_SECRET_NAME="${HF_SECRET_NAME:-cubicle-transcription/huggingface-token}"
IMAGE_URI="${IMAGE_URI:-vllm/vllm-openai:v0.21.0-ubuntu2404}"
VOXTRAL_MODEL="${VOXTRAL_MODEL:-mistralai/Voxtral-Mini-4B-Realtime-2602}"
CONTAINER_NAME="${CONTAINER_NAME:-voxtral-vllm}"
HOST_PORT="${HOST_PORT:-8000}"
LOCAL_PORT="${LOCAL_PORT:-8000}"
MODEL_LEN="${MODEL_LEN:-45000}"
HF_CACHE_DIR="${HF_CACHE_DIR:-/home/ssm-user/.cache/huggingface}"
READINESS_ATTEMPTS="${READINESS_ATTEMPTS:-120}"
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
  infra/transcription/rebuild-voxtral-vllm-ec2.sh status
  infra/transcription/rebuild-voxtral-vllm-ec2.sh ensure-secret-access
  infra/transcription/rebuild-voxtral-vllm-ec2.sh ensure-instance
  infra/transcription/rebuild-voxtral-vllm-ec2.sh start-vllm
  infra/transcription/rebuild-voxtral-vllm-ec2.sh full
  infra/transcription/rebuild-voxtral-vllm-ec2.sh port-forward
  CONFIRM_TERMINATE=replace-i-understand infra/transcription/rebuild-voxtral-vllm-ec2.sh replace-instance

Defaults are pinned to the verified Cubicle transcription AWS account and region:
  AWS_PROFILE_NAME=strln
  AWS_REGION_NAME=us-west-2
  EXPECTED_ACCOUNT_ID=562304353751
  ASG_NAME=cubicle-transcription-gpu
  VOXTRAL_MODEL=mistralai/Voxtral-Mini-4B-Realtime-2602

The script uses SSM only. It does not require or create an EC2 SSH key.
EOF
}

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

This runbook is pinned to AWS profile '$AWS_PROFILE_NAME' and account '$EXPECTED_ACCOUNT_ID'.
Unset conflicting variables first.
EOF
  exit 2
fi

aws_cli() {
  aws --profile "$AWS_PROFILE_NAME" --region "$AWS_REGION_NAME" "$@"
}

require_account() {
  local account_id
  account_id="$(aws_cli sts get-caller-identity --query Account --output text)"
  if [[ "$account_id" != "$EXPECTED_ACCOUNT_ID" ]]; then
    echo "Refusing to run in AWS account $account_id; expected $EXPECTED_ACCOUNT_ID." >&2
    exit 2
  fi
}

hf_secret_arn() {
  aws_cli secretsmanager describe-secret \
    --secret-id "$HF_SECRET_NAME" \
    --query ARN \
    --output text
}

ensure_secret_access() {
  require_account

  local secret_arn policy_file
  secret_arn="$(hf_secret_arn)"
  policy_file="$(mktemp "${TMPDIR:-/tmp}/cubicle-hf-secret-policy.XXXXXX.json")"
  trap 'rm -f "$policy_file"' RETURN

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

  echo "Granted $INSTANCE_ROLE_NAME read access to $HF_SECRET_NAME."
}

asg_instance_id() {
  aws_cli autoscaling describe-auto-scaling-groups \
    --auto-scaling-group-names "$ASG_NAME" \
    --query 'AutoScalingGroups[0].Instances[?LifecycleState==`InService`].InstanceId | [0]' \
    --output text
}

ensure_instance() {
  require_account

  local instance_id
  instance_id="$(asg_instance_id)"

  if [[ -z "$instance_id" || "$instance_id" == "None" ]]; then
    echo "No InService GPU instance found. Setting $ASG_NAME desired capacity to 1." >&2
    aws_cli autoscaling update-auto-scaling-group \
      --auto-scaling-group-name "$ASG_NAME" \
      --desired-capacity 1 >/dev/null
  fi

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

wait_for_ssm() {
  local instance_id="$1"

  for _ in $(seq 1 90); do
    local status
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

send_ssm_script() {
  local instance_id="$1"
  local script_path="$2"
  local params_file command_id

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
    --comment "Cubicle Voxtral vLLM runtime rebuild" \
    --parameters "file://$params_file" \
    --query Command.CommandId \
    --output text)"

  rm -f "$params_file"
  echo "SSM command: $command_id"

  for _ in $(seq 1 180); do
    local status
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

  aws_cli ssm get-command-invocation \
    --command-id "$command_id" \
    --instance-id "$instance_id" \
    --query '{Status:Status,ResponseCode:ResponseCode,Stdout:StandardOutputContent,Stderr:StandardErrorContent}' \
    --output json

  local final_status
  final_status="$(aws_cli ssm get-command-invocation \
    --command-id "$command_id" \
    --instance-id "$instance_id" \
    --query Status \
    --output text)"
  if [[ "$final_status" != "Success" ]]; then
    echo "SSM command failed with status $final_status." >&2
    exit 5
  fi
}

start_vllm() {
  require_account

  local instance_id remote_script
  instance_id="${INSTANCE_ID:-$(ensure_instance)}"
  wait_for_ssm "$instance_id"

  remote_script="$(mktemp "${TMPDIR:-/tmp}/cubicle-start-vllm.XXXXXX.sh")"
  trap 'rm -f "$remote_script"' RETURN

  cat > "$remote_script" <<REMOTE
set -euo pipefail

AWS_REGION_NAME='$AWS_REGION_NAME'
HF_SECRET_NAME='$HF_SECRET_NAME'
IMAGE_URI='$IMAGE_URI'
VOXTRAL_MODEL='$VOXTRAL_MODEL'
CONTAINER_NAME='$CONTAINER_NAME'
HOST_PORT='$HOST_PORT'
MODEL_LEN='$MODEL_LEN'
HF_CACHE_DIR='$HF_CACHE_DIR'
READINESS_ATTEMPTS='$READINESS_ATTEMPTS'
READINESS_SLEEP_SECONDS='$READINESS_SLEEP_SECONDS'

echo "[info] instance metadata"
metadata_token="\$(curl -fsS --max-time 2 -X PUT -H 'X-aws-ec2-metadata-token-ttl-seconds: 60' http://169.254.169.254/latest/api/token || true)"
if [[ -n "\$metadata_token" ]]; then
  curl -fsS --max-time 2 -H "X-aws-ec2-metadata-token: \$metadata_token" http://169.254.169.254/latest/meta-data/instance-id || true
  echo
fi

echo "[info] gpu"
nvidia-smi --query-gpu=name,memory.total,driver_version --format=csv,noheader
echo "[info] docker=\$(docker --version)"

if ! command -v aws >/dev/null 2>&1; then
  echo "[info] installing awscli"
  yum install -y awscli >/tmp/cubicle-awscli-install.log 2>&1 || {
    cat /tmp/cubicle-awscli-install.log
    exit 12
  }
fi

mkdir -p "\$HF_CACHE_DIR"
chmod 700 "\$HF_CACHE_DIR"

HF_TOKEN="\$(aws secretsmanager get-secret-value \
  --region "\$AWS_REGION_NAME" \
  --secret-id "\$HF_SECRET_NAME" \
  --query SecretString \
  --output text)"
export HF_TOKEN

docker pull "\$IMAGE_URI"
docker rm -f "\$CONTAINER_NAME" >/dev/null 2>&1 || true

docker run -d \
  --name "\$CONTAINER_NAME" \
  --gpus all \
  --ipc=host \
  --restart unless-stopped \
  -p "\$HOST_PORT:8000" \
  -v "\$HF_CACHE_DIR:/root/.cache/huggingface" \
  -e HF_TOKEN \
  -e VLLM_DISABLE_COMPILE_CACHE=1 \
  -e VLLM_NO_USAGE_STATS=1 \
  -e DO_NOT_TRACK=1 \
  --entrypoint /bin/bash \
  "\$IMAGE_URI" \
  -lc "python3 -m pip install --no-cache-dir 'mistral-common[soundfile]' soundfile && exec vllm serve '\$VOXTRAL_MODEL' --host 0.0.0.0 --port 8000 --tokenizer-mode mistral --max-model-len '\$MODEL_LEN' --gpu-memory-utilization 0.90 --compilation_config '{\"cudagraph_mode\":\"PIECEWISE\"}'"

unset HF_TOKEN

echo "[info] waiting for vLLM readiness"
ready=0
for attempt in \$(seq 1 "\$READINESS_ATTEMPTS"); do
  if curl -fsS "http://127.0.0.1:\$HOST_PORT/health" >/tmp/cubicle-vllm-health.txt \
    && curl -fsS "http://127.0.0.1:\$HOST_PORT/v1/models" >/tmp/cubicle-vllm-models.json; then
    ready=1
    break
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

if [[ "\$ready" != "1" ]]; then
  echo "[error] vLLM did not become ready"
  docker logs --tail 240 "\$CONTAINER_NAME" || true
  exit 21
fi

cat /tmp/cubicle-vllm-health.txt
echo
cat /tmp/cubicle-vllm-models.json
echo
docker ps --filter "name=\$CONTAINER_NAME"
docker logs --tail 80 "\$CONTAINER_NAME" || true
echo "[success] Voxtral vLLM runtime is ready"
REMOTE

  send_ssm_script "$instance_id" "$remote_script"
}

status() {
  require_account
  aws_cli autoscaling describe-auto-scaling-groups \
    --auto-scaling-group-names "$ASG_NAME" \
    --query 'AutoScalingGroups[0].{AutoScalingGroupName:AutoScalingGroupName,MinSize:MinSize,MaxSize:MaxSize,DesiredCapacity:DesiredCapacity,LaunchTemplate:LaunchTemplate,Instances:Instances[].{InstanceId:InstanceId,LifecycleState:LifecycleState,HealthStatus:HealthStatus}}' \
    --output json
}

replace_instance() {
  require_account

  if [[ "${CONFIRM_TERMINATE:-}" != "replace-i-understand" ]]; then
    cat >&2 <<'EOF'
Refusing to terminate an EC2 instance without explicit confirmation.
Run:
  CONFIRM_TERMINATE=replace-i-understand infra/transcription/rebuild-voxtral-vllm-ec2.sh replace-instance
EOF
    exit 2
  fi

  local instance_id
  instance_id="$(asg_instance_id)"
  if [[ -z "$instance_id" || "$instance_id" == "None" ]]; then
    echo "No InService instance to replace. Creating one instead."
    ensure_instance >/dev/null
    return 0
  fi

  aws_cli autoscaling update-auto-scaling-group \
    --auto-scaling-group-name "$ASG_NAME" \
    --desired-capacity 1 >/dev/null
  aws_cli autoscaling set-instance-protection \
    --auto-scaling-group-name "$ASG_NAME" \
    --instance-ids "$instance_id" \
    --no-protected-from-scale-in >/dev/null
  aws_cli ec2 terminate-instances \
    --instance-ids "$instance_id" >/dev/null

  echo "Terminated $instance_id. Waiting for replacement."
  ensure_instance
}

port_forward() {
  require_account

  local instance_id
  instance_id="${INSTANCE_ID:-$(ensure_instance)}"
  wait_for_ssm "$instance_id"

  echo "Opening SSM port forward: localhost:$LOCAL_PORT -> $instance_id:127.0.0.1:$HOST_PORT"
  exec aws ssm start-session \
    --profile "$AWS_PROFILE_NAME" \
    --region "$AWS_REGION_NAME" \
    --target "$instance_id" \
    --document-name AWS-StartPortForwardingSession \
    --parameters "{\"portNumber\":[\"$HOST_PORT\"],\"localPortNumber\":[\"$LOCAL_PORT\"]}"
}

cmd="${1:-}"
case "$cmd" in
  status)
    status
    ;;
  ensure-secret-access)
    ensure_secret_access
    ;;
  ensure-instance)
    ensure_instance
    ;;
  start-vllm)
    start_vllm
    ;;
  full)
    ensure_secret_access
    start_vllm
    ;;
  replace-instance)
    replace_instance
    ;;
  port-forward)
    port_forward
    ;;
  -h|--help|help|"")
    usage
    ;;
  *)
    echo "Unknown command: $cmd" >&2
    usage >&2
    exit 2
    ;;
esac
