#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

AWS_PROFILE_NAME="${AWS_PROFILE_NAME:-strln}"
AWS_REGION_NAME="${AWS_REGION_NAME:-us-west-2}"
EXPECTED_ACCOUNT_ID="${EXPECTED_ACCOUNT_ID:-562304353751}"
ASG_NAME="${ASG_NAME:-cubicle-transcription-gpu}"
IMAGE_URI="${IMAGE_URI:-vllm/vllm-openai:v0.21.0-ubuntu2404}"
VOXTRAL_MODEL="${VOXTRAL_MODEL:-mistralai/Voxtral-Mini-4B-Realtime-2602}"
MODEL_DIR_NAME="${VOXTRAL_MODEL##*/}"
MODEL_PATH="${VOXTRAL_MODEL_PATH:-/models/voxtral/$MODEL_DIR_NAME}"
SERVED_MODEL_NAME="${SERVED_MODEL_NAME:-$VOXTRAL_MODEL}"
MODEL_LEN="${MODEL_LEN:-45000}"
HOST_PORT="${HOST_PORT:-8000}"
CONTAINER_NAME="${CONTAINER_NAME:-voxtral-vllm}"
HF_CACHE_DIR="${HF_CACHE_DIR:-/home/ssm-user/.cache/huggingface}"
READINESS_ATTEMPTS="${READINESS_ATTEMPTS:-120}"
READINESS_SLEEP_SECONDS="${READINESS_SLEEP_SECONDS:-10}"
SSM_TIMEOUT_SECONDS="${SSM_TIMEOUT_SECONDS:-2400}"
KEEP_CONTAINER="${KEEP_CONTAINER:-true}"

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
Refusing to run the Voxtral vLLM smoke test while ambient AWS credential/profile variables are set:
  ${conflicting_aws_env[*]}

This path is pinned to AWS profile '$AWS_PROFILE_NAME' and account '$EXPECTED_ACCOUNT_ID'.
Unset the conflicting variables first.
EOF
  exit 2
fi

aws_cli() {
  aws --profile "$AWS_PROFILE_NAME" --region "$AWS_REGION_NAME" "$@"
}

ACCOUNT_ID="$(aws_cli sts get-caller-identity --query Account --output text)"
if [[ "$ACCOUNT_ID" != "$EXPECTED_ACCOUNT_ID" ]]; then
  echo "Refusing to run in AWS account $ACCOUNT_ID; expected $EXPECTED_ACCOUNT_ID." >&2
  exit 2
fi

RUNTIME_IMAGE_URI="$IMAGE_URI"

if [[ -n "${INSTANCE_ID:-}" ]]; then
  TARGET_INSTANCE_ID="$INSTANCE_ID"
else
  TARGET_INSTANCE_ID="$(aws_cli autoscaling describe-auto-scaling-groups \
    --auto-scaling-group-names "$ASG_NAME" \
    --query 'AutoScalingGroups[0].Instances[?LifecycleState==`InService`].InstanceId | [0]' \
    --output text)"
fi

if [[ -z "$TARGET_INSTANCE_ID" || "$TARGET_INSTANCE_ID" == "None" ]]; then
  echo "No InService instance found in Auto Scaling group $ASG_NAME." >&2
  exit 2
fi

PING_STATUS="$(aws_cli ssm describe-instance-information \
  --filters "Key=InstanceIds,Values=$TARGET_INSTANCE_ID" \
  --query 'InstanceInformationList[0].PingStatus' \
  --output text 2>/dev/null || true)"
if [[ "$PING_STATUS" != "Online" ]]; then
  echo "Instance $TARGET_INSTANCE_ID is not SSM Online; current status: ${PING_STATUS:-unknown}." >&2
  exit 2
fi

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/cubicle-voxtral-smoke.XXXXXX")"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

cat > "$TMP_DIR/remote-smoke.sh" <<REMOTE
set -euo pipefail

AWS_REGION_NAME='$AWS_REGION_NAME'
ACCOUNT_ID='$ACCOUNT_ID'
RUNTIME_IMAGE_URI='$RUNTIME_IMAGE_URI'
VOXTRAL_MODEL='$VOXTRAL_MODEL'
MODEL_PATH='$MODEL_PATH'
SERVED_MODEL_NAME='$SERVED_MODEL_NAME'
MODEL_LEN='$MODEL_LEN'
HOST_PORT='$HOST_PORT'
CONTAINER_NAME='$CONTAINER_NAME'
HF_CACHE_DIR='$HF_CACHE_DIR'
READINESS_ATTEMPTS='$READINESS_ATTEMPTS'
READINESS_SLEEP_SECONDS='$READINESS_SLEEP_SECONDS'
KEEP_CONTAINER='$KEEP_CONTAINER'
ECR_REGISTRY="\$ACCOUNT_ID.dkr.ecr.\$AWS_REGION_NAME.amazonaws.com"

cleanup_remote() {
  if [[ "\$KEEP_CONTAINER" != "true" ]]; then
    docker rm -f "\$CONTAINER_NAME" >/dev/null 2>&1 || true
  fi
  docker logout "\$ECR_REGISTRY" >/dev/null 2>&1 || true
}
trap cleanup_remote EXIT

metadata_token="\$(curl -fsS --max-time 2 -X PUT -H 'X-aws-ec2-metadata-token-ttl-seconds: 60' http://169.254.169.254/latest/api/token || true)"
if [[ -n "\$metadata_token" ]]; then
  echo "[info] instance=\$(curl -fsS --max-time 2 -H "X-aws-ec2-metadata-token: \$metadata_token" http://169.254.169.254/latest/meta-data/instance-id || true)"
else
  echo "[info] instance=unknown"
fi
echo "[info] gpu"
nvidia-smi --query-gpu=name,memory.total,driver_version --format=csv,noheader
echo "[info] docker=\$(docker --version)"

if ! command -v aws >/dev/null 2>&1; then
  echo "[info] installing awscli on disposable GPU host"
  if command -v yum >/dev/null 2>&1; then
    yum install -y awscli >/tmp/cubicle-awscli-install.log 2>&1 || {
      cat /tmp/cubicle-awscli-install.log
      exit 12
    }
  else
    echo "[error] no supported package manager found for awscli install"
    exit 12
  fi
fi

if [[ "\$RUNTIME_IMAGE_URI" == "\$ECR_REGISTRY/"* ]]; then
  aws ecr get-login-password --region "\$AWS_REGION_NAME" | docker login --username AWS --password-stdin "\$ECR_REGISTRY" >/dev/null
fi
docker pull "\$RUNTIME_IMAGE_URI"
docker rm -f "\$CONTAINER_NAME" >/dev/null 2>&1 || true

docker run -d \
  --gpus all \
  --ipc=host \
  --name "\$CONTAINER_NAME" \
  --restart unless-stopped \
  -p "127.0.0.1:\$HOST_PORT:8000" \
  -v "\$HF_CACHE_DIR:/root/.cache/huggingface" \
  -e "HF_TOKEN=\${HF_TOKEN:-}" \
  -e VLLM_DISABLE_COMPILE_CACHE=1 \
  -e VLLM_NO_USAGE_STATS=1 \
  -e DO_NOT_TRACK=1 \
  --entrypoint /bin/bash \
  "\$RUNTIME_IMAGE_URI" \
  -lc "python3 -m pip install --no-cache-dir 'mistral-common[soundfile]' soundfile && exec vllm serve '\$VOXTRAL_MODEL' --host 0.0.0.0 --port 8000 --tokenizer-mode mistral --max-model-len '\$MODEL_LEN' --gpu-memory-utilization 0.90 --compilation_config '{\"cudagraph_mode\":\"PIECEWISE\"}'"

echo "[info] waiting for vLLM readiness on 127.0.0.1:\$HOST_PORT"
ready=0
for attempt in \$(seq 1 "\$READINESS_ATTEMPTS"); do
  if curl -fsS "http://127.0.0.1:\$HOST_PORT/v1/models" >/tmp/cubicle-vllm-models.json; then
    ready=1
    break
  fi

  if ! docker ps --format '{{.Names}}' | grep -qx "\$CONTAINER_NAME"; then
    echo "[error] vLLM container exited before readiness"
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

curl -fsS "http://127.0.0.1:\$HOST_PORT/health" >/tmp/cubicle-vllm-health.txt
cat /tmp/cubicle-vllm-health.txt
echo
cat /tmp/cubicle-vllm-models.json
echo
docker logs --tail 80 "\$CONTAINER_NAME" || true
echo "[success] vLLM readiness passed model_len=\$MODEL_LEN image=\$RUNTIME_IMAGE_URI"
REMOTE

python3 - "$TMP_DIR/remote-smoke.sh" "$TMP_DIR/ssm-parameters.json" <<'PY'
import json
import sys
from pathlib import Path

script = Path(sys.argv[1]).read_text()
Path(sys.argv[2]).write_text(json.dumps({"commands": [script]}))
PY

COMMAND_ID="$(aws_cli ssm send-command \
  --instance-ids "$TARGET_INSTANCE_ID" \
  --document-name AWS-RunShellScript \
  --comment "Cubicle Voxtral vLLM smoke model_len=$MODEL_LEN" \
  --parameters "file://$TMP_DIR/ssm-parameters.json" \
  --timeout-seconds "$SSM_TIMEOUT_SECONDS" \
  --query 'Command.CommandId' \
  --output text)"

echo "Started SSM command $COMMAND_ID on $TARGET_INSTANCE_ID"
deadline=$((SECONDS + SSM_TIMEOUT_SECONDS + 60))
while true; do
  STATUS="$(aws_cli ssm get-command-invocation \
    --command-id "$COMMAND_ID" \
    --instance-id "$TARGET_INSTANCE_ID" \
    --query 'Status' \
    --output text 2>/dev/null || true)"
  case "$STATUS" in
    Success|Failed|Cancelled|TimedOut)
      break
      ;;
  esac

  if (( SECONDS >= deadline )); then
    echo "SSM command $COMMAND_ID did not finish before local polling deadline." >&2
    break
  fi

  sleep 20
done

aws_cli ssm get-command-invocation \
  --command-id "$COMMAND_ID" \
  --instance-id "$TARGET_INSTANCE_ID" \
  --query '{Status:Status,ResponseCode:ResponseCode,Stdout:StandardOutputContent,Stderr:StandardErrorContent}' \
  --output json > "$TMP_DIR/invocation.json"

python3 - "$TMP_DIR/invocation.json" <<'PY'
import json
import sys
from pathlib import Path

payload = json.loads(Path(sys.argv[1]).read_text())
print(f"Status: {payload.get('Status')} ResponseCode: {payload.get('ResponseCode')}")
stdout = payload.get("Stdout") or ""
stderr = payload.get("Stderr") or ""
if stdout:
    print("--- stdout ---")
    print(stdout)
if stderr:
    print("--- stderr ---", file=sys.stderr)
    print(stderr, file=sys.stderr)
if payload.get("Status") != "Success":
    sys.exit(int(payload.get("ResponseCode") or 1) or 1)
PY
