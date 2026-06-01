#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

derive_vllm_realtime_url() {
  local base="${1:-http://127.0.0.1:8000}"
  base="${base%/}"
  case "$base" in
    */v1/realtime)
      printf '%s\n' "$base"
      ;;
    https://*)
      printf 'wss://%s/v1/realtime\n' "${base#https://}"
      ;;
    http://*)
      printf 'ws://%s/v1/realtime\n' "${base#http://}"
      ;;
    ws://*|wss://*)
      printf '%s/v1/realtime\n' "$base"
      ;;
    *)
      printf 'ws://%s/v1/realtime\n' "$base"
      ;;
  esac
}

AWS_PROFILE_NAME="${AWS_PROFILE_NAME:-strln}"
AWS_REGION_NAME="${AWS_REGION_NAME:-us-west-2}"
EXPECTED_ACCOUNT_ID="${EXPECTED_ACCOUNT_ID:-562304353751}"
PROJECT_NAME="${PROJECT_NAME:-cubicle-transcription}"
IMAGE_TAG="${IMAGE_TAG:-$(date +%Y%m%d%H%M%S)}"
DIARIZATION_PROVIDER_WAS_SET="${DIARIZATION_PROVIDER+x}"
INCLUDE_VOXTRAL_REALTIME="${INCLUDE_VOXTRAL_REALTIME:-true}"
INCLUDE_DIARIZATION="${INCLUDE_DIARIZATION:-true}"
SELF_HOSTED_MODELS="${SELF_HOSTED_MODELS:-false}"
INCLUDE_SELF_HOSTED_MODELS="${INCLUDE_SELF_HOSTED_MODELS:-$SELF_HOSTED_MODELS}"
ENABLE_VOXTRAL_RUNTIME="${ENABLE_VOXTRAL_RUNTIME:-}"
ENABLE_GPU_CAPACITY="${ENABLE_GPU_CAPACITY:-}"
ECS_LAUNCH_TYPE="${ECS_LAUNCH_TYPE:-}"
GPU_DESIRED_CAPACITY="${GPU_DESIRED_CAPACITY:-}"
GPU_INSTANCE_TYPE="${GPU_INSTANCE_TYPE:-g5.xlarge}"
GPU_MIN_SIZE="${GPU_MIN_SIZE:-0}"
GPU_MAX_SIZE="${GPU_MAX_SIZE:-1}"
REQUIRE_GPU="${REQUIRE_GPU:-}"
MODELS_OFFLINE="${MODELS_OFFLINE:-false}"
PRELOAD_MODEL_WEIGHTS="${PRELOAD_MODEL_WEIGHTS:-false}"
VLLM_BASE_URL="${VLLM_BASE_URL:-http://127.0.0.1:8000}"
VLLM_REALTIME_URL="${VLLM_REALTIME_URL:-$(derive_vllm_realtime_url "$VLLM_BASE_URL")}"
VOXTRAL_MODEL="${VOXTRAL_MODEL:-${VLLM_MODEL:-mistralai/Voxtral-Mini-4B-Realtime-2602}}"
VOXTRAL_MODEL_VERSION="${VOXTRAL_MODEL_VERSION:-self-hosted-vllm-2602}"
VOXTRAL_REALTIME_URL="${VOXTRAL_REALTIME_URL:-$VLLM_REALTIME_URL}"
VOXTRAL_FINAL_RESPONSE_TIMEOUT_SECONDS="${VOXTRAL_FINAL_RESPONSE_TIMEOUT_SECONDS:-30}"
WHISPER_MODEL="${WHISPER_MODEL:-h2oai/faster-whisper-large-v3-turbo}"
WHISPER_DEVICE="${WHISPER_DEVICE:-cuda}"
WHISPER_COMPUTE_TYPE="${WHISPER_COMPUTE_TYPE:-float16}"
PYANNOTE_MODEL_ID="${PYANNOTE_MODEL_ID:-pyannote/speaker-diarization-community-1}"
PYANNOTE_DEVICE="${PYANNOTE_DEVICE:-cpu}"
PYANNOTE_MIN_SPEAKERS="${PYANNOTE_MIN_SPEAKERS:-0}"
PYANNOTE_MAX_SPEAKERS="${PYANNOTE_MAX_SPEAKERS:-0}"
PYANNOTE_AUTH_TOKEN_SECRET_ARN="${PYANNOTE_AUTH_TOKEN_SECRET_ARN:-}"
PYANNOTE_AUTH_TOKEN_SECRET_NAME="${PYANNOTE_AUTH_TOKEN_SECRET_NAME:-}"
DOCKER_PLATFORM="${DOCKER_PLATFORM:-linux/amd64}"
DOCKER_LOGOUT_ON_EXIT="${DOCKER_LOGOUT_ON_EXIT:-true}"
TRANSCRIPTION_AUTH_MODE="${TRANSCRIPTION_AUTH_MODE:-shared_token}"
TRANSCRIPTION_ALLOWED_USERS="${TRANSCRIPTION_ALLOWED_USERS:-}"
TRANSCRIPTION_REVOKED_TOKEN_IDS="${TRANSCRIPTION_REVOKED_TOKEN_IDS:-}"
TRANSCRIPTION_TOKEN_ISSUER="${TRANSCRIPTION_TOKEN_ISSUER:-cubicle-transcription}"
TRANSCRIPTION_TOKEN_AUDIENCE="${TRANSCRIPTION_TOKEN_AUDIENCE:-cubicle-macos}"
TRANSCRIPTION_REQUIRED_SCOPE="${TRANSCRIPTION_REQUIRED_SCOPE:-transcription:stream}"
ENABLE_PUBLIC_ADMIN_CONSOLE="${ENABLE_PUBLIC_ADMIN_CONSOLE:-true}"
PUBLIC_ADMIN_REQUEST_CERTIFICATE="${PUBLIC_ADMIN_REQUEST_CERTIFICATE:-true}"
PUBLIC_ADMIN_CERTIFICATE_ARN="${PUBLIC_ADMIN_CERTIFICATE_ARN:-arn:aws:acm:us-west-2:562304353751:certificate/4e87bf99-c142-4033-868e-703db7d60c61}"
ADMIN_DESIRED_COUNT="${ADMIN_DESIRED_COUNT:-1}"
ADMIN_IMAGE_URI="${ADMIN_IMAGE_URI:-}"
VOICENOTES_COGNITO_USER_POOL_ID="${VOICENOTES_COGNITO_USER_POOL_ID:-}"
VOICENOTES_COGNITO_REGION="${VOICENOTES_COGNITO_REGION:-$AWS_REGION_NAME}"
VOICENOTES_ADMIN_LAMBDA_NAME="${VOICENOTES_ADMIN_LAMBDA_NAME:-}"
VOICENOTES_ADMIN_LAMBDA_REGION="${VOICENOTES_ADMIN_LAMBDA_REGION:-$AWS_REGION_NAME}"
ENABLE_DIARIZATION_WORKER="${ENABLE_DIARIZATION_WORKER:-false}"
DIARIZATION_WORKER_DESIRED_COUNT="${DIARIZATION_WORKER_DESIRED_COUNT:-0}"
DIARIZATION_WORKER_LAUNCH_TYPE="${DIARIZATION_WORKER_LAUNCH_TYPE:-FARGATE}"
DIARIZATION_WORKER_TASK_CPU="${DIARIZATION_WORKER_TASK_CPU:-4096}"
DIARIZATION_WORKER_TASK_MEMORY="${DIARIZATION_WORKER_TASK_MEMORY:-8192}"
DIARIZATION_WORKER_ASSIGN_PUBLIC_IP="${DIARIZATION_WORKER_ASSIGN_PUBLIC_IP:-true}"
DIARIZATION_WORKER_PROVIDER="${DIARIZATION_WORKER_PROVIDER:-pyannote}"
DIARIZATION_WORKER_TIMEOUT_SECONDS="${DIARIZATION_WORKER_TIMEOUT_SECONDS:-90}"
DIARIZATION_WORKER_GPU_COUNT="${DIARIZATION_WORKER_GPU_COUNT:-1}"
DIARIZATION_WORKER_PYANNOTE_DEVICE="${DIARIZATION_WORKER_PYANNOTE_DEVICE:-}"
ENABLE_DIARIZATION_WORKER_GPU_CAPACITY="${ENABLE_DIARIZATION_WORKER_GPU_CAPACITY:-false}"
DIARIZATION_WORKER_GPU_INSTANCE_TYPE="${DIARIZATION_WORKER_GPU_INSTANCE_TYPE:-$GPU_INSTANCE_TYPE}"
DIARIZATION_WORKER_GPU_MIN_SIZE="${DIARIZATION_WORKER_GPU_MIN_SIZE:-0}"
DIARIZATION_WORKER_GPU_DESIRED_CAPACITY="${DIARIZATION_WORKER_GPU_DESIRED_CAPACITY:-0}"
DIARIZATION_WORKER_GPU_MAX_SIZE="${DIARIZATION_WORKER_GPU_MAX_SIZE:-1}"
DIARIZATION_WORKER_IMAGE_URI="${DIARIZATION_WORKER_IMAGE_URI:-}"
DIARIZATION_WORKER_URL="${DIARIZATION_WORKER_URL:-}"
DIARIZATION_WORKER_PRELOAD_MODEL_WEIGHTS="${DIARIZATION_WORKER_PRELOAD_MODEL_WEIGHTS:-false}"
DIARIZATION_WORKER_TORCH_VERSION="${DIARIZATION_WORKER_TORCH_VERSION:-2.11.0}"
DIARIZATION_WORKER_TORCHAUDIO_VERSION="${DIARIZATION_WORKER_TORCHAUDIO_VERSION:-2.11.0}"
DIARIZATION_WORKER_TORCH_INDEX_URL="${DIARIZATION_WORKER_TORCH_INDEX_URL:-}"
SKIP_SERVICE_IMAGE_BUILD="${SKIP_SERVICE_IMAGE_BUILD:-false}"
SKIP_DIARIZATION_WORKER_IMAGE_BUILD="${SKIP_DIARIZATION_WORKER_IMAGE_BUILD:-false}"
DIARIZATION_WARMUP_ENABLED="${DIARIZATION_WARMUP_ENABLED:-false}"
ENABLE_TEXT_INTELLIGENCE_WORKER="${ENABLE_TEXT_INTELLIGENCE_WORKER:-false}"
TEXT_INTELLIGENCE_WORKER_DESIRED_COUNT="${TEXT_INTELLIGENCE_WORKER_DESIRED_COUNT:-0}"
TEXT_INTELLIGENCE_WORKER_LAUNCH_TYPE="${TEXT_INTELLIGENCE_WORKER_LAUNCH_TYPE:-EC2}"
TEXT_INTELLIGENCE_WORKER_TASK_CPU="${TEXT_INTELLIGENCE_WORKER_TASK_CPU:-4096}"
TEXT_INTELLIGENCE_WORKER_TASK_MEMORY="${TEXT_INTELLIGENCE_WORKER_TASK_MEMORY:-15360}"
TEXT_INTELLIGENCE_WORKER_ASSIGN_PUBLIC_IP="${TEXT_INTELLIGENCE_WORKER_ASSIGN_PUBLIC_IP:-true}"
TEXT_INTELLIGENCE_WORKER_PROVIDER="${TEXT_INTELLIGENCE_WORKER_PROVIDER:-vllm}"
TEXT_INTELLIGENCE_MODEL="${TEXT_INTELLIGENCE_MODEL:-Qwen/Qwen2.5-7B-Instruct}"
TEXT_INTELLIGENCE_RUNTIME_IMAGE_URI="${TEXT_INTELLIGENCE_RUNTIME_IMAGE_URI:-vllm/vllm-openai:v0.21.0-ubuntu2404}"
TEXT_INTELLIGENCE_WORKER_AUTH_ENABLED="${TEXT_INTELLIGENCE_WORKER_AUTH_ENABLED:-true}"
TEXT_INTELLIGENCE_ALLOWED_SECURITY_GROUP_IDS="${TEXT_INTELLIGENCE_ALLOWED_SECURITY_GROUP_IDS:-[]}"
TEXT_INTELLIGENCE_REQUEST_TIMEOUT_SECONDS="${TEXT_INTELLIGENCE_REQUEST_TIMEOUT_SECONDS:-20}"
TEXT_INTELLIGENCE_SUMMARY_TIMEOUT_SECONDS="${TEXT_INTELLIGENCE_SUMMARY_TIMEOUT_SECONDS:-60}"
TEXT_INTELLIGENCE_MAX_TRANSLATION_TOKENS="${TEXT_INTELLIGENCE_MAX_TRANSLATION_TOKENS:-160}"
TEXT_INTELLIGENCE_MAX_SUMMARY_TOKENS="${TEXT_INTELLIGENCE_MAX_SUMMARY_TOKENS:-1200}"
TEXT_INTELLIGENCE_TEMPERATURE="${TEXT_INTELLIGENCE_TEMPERATURE:-0}"
TEXT_INTELLIGENCE_RUNTIME_GPU_COUNT="${TEXT_INTELLIGENCE_RUNTIME_GPU_COUNT:-1}"
TEXT_INTELLIGENCE_RUNTIME_MAX_MODEL_LEN="${TEXT_INTELLIGENCE_RUNTIME_MAX_MODEL_LEN:-8192}"
TEXT_INTELLIGENCE_RUNTIME_GPU_MEMORY_UTILIZATION="${TEXT_INTELLIGENCE_RUNTIME_GPU_MEMORY_UTILIZATION:-0.90}"
ENABLE_TEXT_INTELLIGENCE_WORKER_GPU_CAPACITY="${ENABLE_TEXT_INTELLIGENCE_WORKER_GPU_CAPACITY:-false}"
REUSE_DIARIZATION_WORKER_GPU_CAPACITY_FOR_TEXT_INTELLIGENCE="${REUSE_DIARIZATION_WORKER_GPU_CAPACITY_FOR_TEXT_INTELLIGENCE:-true}"
TEXT_INTELLIGENCE_WORKER_GPU_INSTANCE_TYPE="${TEXT_INTELLIGENCE_WORKER_GPU_INSTANCE_TYPE:-g5.xlarge}"
TEXT_INTELLIGENCE_WORKER_GPU_MIN_SIZE="${TEXT_INTELLIGENCE_WORKER_GPU_MIN_SIZE:-0}"
TEXT_INTELLIGENCE_WORKER_GPU_DESIRED_CAPACITY="${TEXT_INTELLIGENCE_WORKER_GPU_DESIRED_CAPACITY:-0}"
TEXT_INTELLIGENCE_WORKER_GPU_MAX_SIZE="${TEXT_INTELLIGENCE_WORKER_GPU_MAX_SIZE:-1}"
TEXT_INTELLIGENCE_WORKER_IMAGE_URI="${TEXT_INTELLIGENCE_WORKER_IMAGE_URI:-}"
TEXT_INTELLIGENCE_WORKER_URL="${TEXT_INTELLIGENCE_WORKER_URL:-}"

REGISTRY_AUTHORITY_ENABLED="false"
if [[ "${ENABLE_PUBLIC_ADMIN_CONSOLE:-false}" == "true" ]]; then
  REGISTRY_AUTHORITY_ENABLED="true"
fi

if [[ "$TRANSCRIPTION_AUTH_MODE" == "signed_user_token" && -z "$TRANSCRIPTION_ALLOWED_USERS" && "${ALLOW_ANY_SIGNED_TRANSCRIPTION_USER:-false}" != "true" && "$REGISTRY_AUTHORITY_ENABLED" != "true" ]]; then
  cat >&2 <<'EOF'
TRANSCRIPTION_AUTH_MODE=signed_user_token requires TRANSCRIPTION_ALLOWED_USERS.
Set TRANSCRIPTION_ALLOWED_USERS to a comma-separated list of allowed user ids/emails,
enable the admin console registry/token ledger, or explicitly set
ALLOW_ANY_SIGNED_TRANSCRIPTION_USER=true when another token issuer is authoritative.
EOF
  exit 2
fi

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
Refusing to deploy while ambient AWS credential/profile variables are set:
  ${conflicting_aws_env[*]}

This deployment is pinned to AWS profile '$AWS_PROFILE_NAME' and account '$EXPECTED_ACCOUNT_ID'.
Unset the conflicting variables first, for example:

  unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN AWS_SECURITY_TOKEN AWS_WEB_IDENTITY_TOKEN_FILE AWS_ROLE_ARN AWS_CONTAINER_CREDENTIALS_FULL_URI AWS_CONTAINER_CREDENTIALS_RELATIVE_URI

EOF
  exit 2
fi

if [[ "$SELF_HOSTED_MODELS" == "true" ]]; then
  INCLUDE_VOXTRAL_REALTIME=false
  ASR_PROVIDER="${ASR_PROVIDER:-voxtral_self_hosted}"
  ECS_LAUNCH_TYPE="${ECS_LAUNCH_TYPE:-EC2}"
  ENABLE_VOXTRAL_RUNTIME="${ENABLE_VOXTRAL_RUNTIME:-true}"
  ENABLE_GPU_CAPACITY="${ENABLE_GPU_CAPACITY:-true}"
  GPU_DESIRED_CAPACITY="${GPU_DESIRED_CAPACITY:-1}"
  REQUIRE_GPU="${REQUIRE_GPU:-true}"
fi

ENABLE_VOXTRAL_RUNTIME="${ENABLE_VOXTRAL_RUNTIME:-false}"
ENABLE_GPU_CAPACITY="${ENABLE_GPU_CAPACITY:-false}"
ECS_LAUNCH_TYPE="${ECS_LAUNCH_TYPE:-FARGATE}"
GPU_DESIRED_CAPACITY="${GPU_DESIRED_CAPACITY:-0}"
REQUIRE_GPU="${REQUIRE_GPU:-false}"
if [[ -z "$DIARIZATION_WORKER_TORCH_INDEX_URL" ]]; then
  if [[ "$DIARIZATION_WORKER_LAUNCH_TYPE" == "EC2" && "${DIARIZATION_WORKER_PYANNOTE_DEVICE:-cuda}" == "cuda" ]]; then
    DIARIZATION_WORKER_TORCH_INDEX_URL="https://download.pytorch.org/whl/cu128"
  else
    DIARIZATION_WORKER_TORCH_INDEX_URL="https://download.pytorch.org/whl/cpu"
  fi
fi

aws_cli() {
  aws --profile "$AWS_PROFILE_NAME" --region "$AWS_REGION_NAME" "$@"
}

ACCOUNT_ID="$(aws_cli sts get-caller-identity --query Account --output text)"
if [[ "$ACCOUNT_ID" != "$EXPECTED_ACCOUNT_ID" ]]; then
  echo "Refusing to deploy to AWS account $ACCOUNT_ID; expected $EXPECTED_ACCOUNT_ID." >&2
  exit 2
fi

terraform -chdir="$SCRIPT_DIR" init -input=false
ENABLE_MISTRAL_SECRET=false
ASR_PROVIDER="${ASR_PROVIDER:-mock}"
if [[ "$SELF_HOSTED_MODELS" != "true" && -n "${MISTRAL_API_KEY:-}" ]]; then
  ENABLE_MISTRAL_SECRET=true
  ASR_PROVIDER="voxtral_realtime"
fi

ENABLE_PYANNOTE_SECRET=false
if [[ "$ENABLE_DIARIZATION_WORKER" == "true" && -z "$DIARIZATION_PROVIDER_WAS_SET" ]]; then
  DIARIZATION_PROVIDER="remote_http"
else
  DIARIZATION_PROVIDER="${DIARIZATION_PROVIDER:-disabled}"
fi
DIARIZATION_STOP_TIMEOUT_SECONDS="${DIARIZATION_STOP_TIMEOUT_SECONDS:-45}"
if [[ -n "${PYANNOTE_AUTH_TOKEN:-}" ]]; then
  ENABLE_PYANNOTE_SECRET=true
  if [[ "$ENABLE_DIARIZATION_WORKER" != "true" && -z "$DIARIZATION_PROVIDER_WAS_SET" ]]; then
    DIARIZATION_PROVIDER="pyannote"
  fi
fi
if [[ -z "$PYANNOTE_AUTH_TOKEN_SECRET_ARN" && -n "$PYANNOTE_AUTH_TOKEN_SECRET_NAME" ]]; then
  PYANNOTE_AUTH_TOKEN_SECRET_ARN="$(aws_cli secretsmanager describe-secret \
    --secret-id "$PYANNOTE_AUTH_TOKEN_SECRET_NAME" \
    --query ARN \
    --output text)"
fi
if [[ -n "$PYANNOTE_AUTH_TOKEN_SECRET_ARN" ]]; then
  if [[ "$ENABLE_DIARIZATION_WORKER" != "true" && -z "$DIARIZATION_PROVIDER_WAS_SET" ]]; then
    DIARIZATION_PROVIDER="pyannote"
  fi
fi
if [[ "$DIARIZATION_PROVIDER" == "pyannote" && "$ENABLE_PYANNOTE_SECRET" != "true" && -z "$PYANNOTE_AUTH_TOKEN_SECRET_ARN" ]]; then
  cat >&2 <<'EOF'
TRANSCRIPTION_DIARIZATION_PROVIDER=pyannote requires a token source.
Set PYANNOTE_AUTH_TOKEN, PYANNOTE_AUTH_TOKEN_SECRET_ARN, or PYANNOTE_AUTH_TOKEN_SECRET_NAME.
EOF
  exit 2
fi
if [[ "$ENABLE_TEXT_INTELLIGENCE_WORKER" == "true" && "$REUSE_DIARIZATION_WORKER_GPU_CAPACITY_FOR_TEXT_INTELLIGENCE" == "true" ]]; then
  if [[ "$ENABLE_DIARIZATION_WORKER" == "true" ]]; then
    cat >&2 <<'EOF'
Text-intelligence GPU reuse requires ENABLE_DIARIZATION_WORKER=false.
Do not run the diarization worker and Qwen text-intelligence worker on the same g5.xlarge.
EOF
    exit 2
  fi
  ENABLE_DIARIZATION_WORKER_GPU_CAPACITY=true
  if [[ "${DIARIZATION_WORKER_GPU_DESIRED_CAPACITY:-0}" == "0" && "${TEXT_INTELLIGENCE_WORKER_GPU_DESIRED_CAPACITY:-0}" != "0" ]]; then
    DIARIZATION_WORKER_GPU_DESIRED_CAPACITY="$TEXT_INTELLIGENCE_WORKER_GPU_DESIRED_CAPACITY"
  fi
fi
if [[ "$ENABLE_DIARIZATION_WORKER" == "true" && "$DIARIZATION_WORKER_PROVIDER" == "pyannote" && "$ENABLE_PYANNOTE_SECRET" != "true" && -z "$PYANNOTE_AUTH_TOKEN_SECRET_ARN" ]]; then
  cat >&2 <<'EOF'
ENABLE_DIARIZATION_WORKER=true with DIARIZATION_WORKER_PROVIDER=pyannote requires a token source.
Set PYANNOTE_AUTH_TOKEN, PYANNOTE_AUTH_TOKEN_SECRET_ARN, or PYANNOTE_AUTH_TOKEN_SECRET_NAME.
EOF
  exit 2
fi

terraform -chdir="$SCRIPT_DIR" apply -input=false -auto-approve \
  -target=aws_ecr_repository.service \
  -target=aws_ecr_repository.voxtral_runtime \
  -target=aws_ecr_repository.diarization_worker \
  -target=aws_secretsmanager_secret.service_token \
  -target=aws_secretsmanager_secret.user_token_signing_key \
  -target=aws_secretsmanager_secret.diarization_worker_auth_token \
  -target=aws_secretsmanager_secret.text_intelligence_worker_auth_token \
  -target=aws_secretsmanager_secret.mistral_api_key \
  -target=aws_secretsmanager_secret.pyannote_auth_token \
  -var "aws_profile=$AWS_PROFILE_NAME" \
  -var "aws_region=$AWS_REGION_NAME" \
  -var "expected_account_id=$EXPECTED_ACCOUNT_ID" \
  -var "project_name=$PROJECT_NAME" \
  -var "auth_mode=$TRANSCRIPTION_AUTH_MODE" \
  -var "allowed_users=$TRANSCRIPTION_ALLOWED_USERS" \
  -var "revoked_token_ids=$TRANSCRIPTION_REVOKED_TOKEN_IDS" \
  -var "token_issuer=$TRANSCRIPTION_TOKEN_ISSUER" \
  -var "token_audience=$TRANSCRIPTION_TOKEN_AUDIENCE" \
  -var "required_scope=$TRANSCRIPTION_REQUIRED_SCOPE" \
  -var "enable_mistral_secret=$ENABLE_MISTRAL_SECRET" \
  -var "enable_pyannote_secret=$ENABLE_PYANNOTE_SECRET" \
  -var "pyannote_auth_token_secret_arn=$PYANNOTE_AUTH_TOKEN_SECRET_ARN" \
  -var "enable_public_admin_console=$ENABLE_PUBLIC_ADMIN_CONSOLE" \
  -var "public_admin_request_certificate=$PUBLIC_ADMIN_REQUEST_CERTIFICATE" \
  -var "public_admin_certificate_arn=$PUBLIC_ADMIN_CERTIFICATE_ARN" \
  -var "admin_desired_count=$ADMIN_DESIRED_COUNT" \
  -var "admin_image_uri=$ADMIN_IMAGE_URI" \
  -var "voicenotes_cognito_user_pool_id=$VOICENOTES_COGNITO_USER_POOL_ID" \
  -var "voicenotes_cognito_region=$VOICENOTES_COGNITO_REGION" \
  -var "voicenotes_admin_lambda_name=$VOICENOTES_ADMIN_LAMBDA_NAME" \
  -var "voicenotes_admin_lambda_region=$VOICENOTES_ADMIN_LAMBDA_REGION"

REPOSITORY_URL="$(terraform -chdir="$SCRIPT_DIR" output -raw repository_url)"
VOXTRAL_RUNTIME_REPOSITORY_URL="$(terraform -chdir="$SCRIPT_DIR" output -raw voxtral_runtime_repository_url)"
DIARIZATION_WORKER_REPOSITORY_URL="$(terraform -chdir="$SCRIPT_DIR" output -raw diarization_worker_repository_url)"
IMAGE_URI="$REPOSITORY_URL:$IMAGE_TAG"
VOXTRAL_RUNTIME_IMAGE_URI="${VOXTRAL_RUNTIME_IMAGE_URI:-$VOXTRAL_RUNTIME_REPOSITORY_URL:$IMAGE_TAG}"
DIARIZATION_WORKER_IMAGE_URI="${DIARIZATION_WORKER_IMAGE_URI:-$DIARIZATION_WORKER_REPOSITORY_URL:$IMAGE_TAG}"
TEXT_INTELLIGENCE_WORKER_IMAGE_URI="${TEXT_INTELLIGENCE_WORKER_IMAGE_URI:-$IMAGE_URI}"
ECR_REGISTRY="$ACCOUNT_ID.dkr.ecr.$AWS_REGION_NAME.amazonaws.com"

HF_TOKEN_SECRET_FILE=""
cleanup() {
  if [[ -n "$HF_TOKEN_SECRET_FILE" ]]; then
    rm -f "$HF_TOKEN_SECRET_FILE"
  fi
  if [[ "$DOCKER_LOGOUT_ON_EXIT" == "true" ]]; then
    docker logout "$ECR_REGISTRY" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

aws_cli ecr get-login-password | docker login --username AWS --password-stdin "$ECR_REGISTRY"

MODEL_SECRET_ARGS=()
HF_TOKEN_INPUT_FILE="${HF_TOKEN_FILE:-${HUGGINGFACE_TOKEN_FILE:-}}"
if [[ -n "$HF_TOKEN_INPUT_FILE" ]]; then
  if [[ ! -r "$HF_TOKEN_INPUT_FILE" ]]; then
    echo "HF token file is set but is not readable: $HF_TOKEN_INPUT_FILE" >&2
    exit 2
  fi
  MODEL_SECRET_ARGS+=(--secret id=hf_token,src="$HF_TOKEN_INPUT_FILE")
elif [[ -n "${HF_TOKEN:-}" ]]; then
  HF_TOKEN_SECRET_FILE="$(mktemp "${TMPDIR:-/tmp}/cubicle-hf-token.XXXXXX")"
  chmod 600 "$HF_TOKEN_SECRET_FILE"
  printf '%s' "$HF_TOKEN" > "$HF_TOKEN_SECRET_FILE"
  unset HF_TOKEN
  MODEL_SECRET_ARGS+=(--secret id=hf_token,src="$HF_TOKEN_SECRET_FILE")
elif [[ -n "${HUGGINGFACE_TOKEN:-}" ]]; then
  HF_TOKEN_SECRET_FILE="$(mktemp "${TMPDIR:-/tmp}/cubicle-hf-token.XXXXXX")"
  chmod 600 "$HF_TOKEN_SECRET_FILE"
  printf '%s' "$HUGGINGFACE_TOKEN" > "$HF_TOKEN_SECRET_FILE"
  unset HUGGINGFACE_TOKEN
  MODEL_SECRET_ARGS+=(--secret id=hf_token,src="$HF_TOKEN_SECRET_FILE")
fi

if (( ${#MODEL_SECRET_ARGS[@]} == 0 )) && [[ "$DIARIZATION_WORKER_PRELOAD_MODEL_WEIGHTS" == "true" ]]; then
  HF_TOKEN_SECRET_ID="${PYANNOTE_AUTH_TOKEN_SECRET_ARN:-$PYANNOTE_AUTH_TOKEN_SECRET_NAME}"
  if [[ -n "$HF_TOKEN_SECRET_ID" ]]; then
    HF_TOKEN_SECRET_FILE="$(mktemp "${TMPDIR:-/tmp}/cubicle-hf-token.XXXXXX")"
    chmod 600 "$HF_TOKEN_SECRET_FILE"
    aws_cli secretsmanager get-secret-value \
      --secret-id "$HF_TOKEN_SECRET_ID" \
      --query SecretString \
      --output text > "$HF_TOKEN_SECRET_FILE"
    MODEL_SECRET_ARGS+=(--secret id=hf_token,src="$HF_TOKEN_SECRET_FILE")
  fi
fi

if [[ -n "${EXTRA_CA_CERT_FILE:-}" ]]; then
  if [[ ! -r "$EXTRA_CA_CERT_FILE" ]]; then
    echo "EXTRA_CA_CERT_FILE is set but is not readable: $EXTRA_CA_CERT_FILE" >&2
    exit 2
  fi
  MODEL_SECRET_ARGS+=(--secret id=extra_ca,src="$EXTRA_CA_CERT_FILE")
fi

if [[ "$SKIP_SERVICE_IMAGE_BUILD" != "true" ]]; then
  docker buildx build \
    --platform "$DOCKER_PLATFORM" \
    --build-arg "INSTALL_VOXTRAL_REALTIME=$INCLUDE_VOXTRAL_REALTIME" \
    --build-arg "INSTALL_DIARIZATION=$INCLUDE_DIARIZATION" \
    --build-arg "INSTALL_SELF_HOSTED_MODELS=$INCLUDE_SELF_HOSTED_MODELS" \
    --build-arg "PRELOAD_SERVICE_MODELS=$PRELOAD_MODEL_WEIGHTS" \
    --build-arg "WHISPER_MODEL_ID=$WHISPER_MODEL" \
    --build-arg "PYANNOTE_MODEL_ID=$PYANNOTE_MODEL_ID" \
    ${MODEL_SECRET_ARGS[@]+"${MODEL_SECRET_ARGS[@]}"} \
    -t "$IMAGE_URI" \
    --push \
    "$REPO_ROOT/aws/transcription-service"
fi

if [[ "$ENABLE_VOXTRAL_RUNTIME" == "true" ]]; then
  docker buildx build \
    --platform "$DOCKER_PLATFORM" \
    --file "$REPO_ROOT/aws/transcription-service/Dockerfile.voxtral-runtime" \
    --build-arg "VOXTRAL_MODEL_ID=$VOXTRAL_MODEL" \
    --build-arg "PRELOAD_VOXTRAL_MODEL=$PRELOAD_MODEL_WEIGHTS" \
    ${MODEL_SECRET_ARGS[@]+"${MODEL_SECRET_ARGS[@]}"} \
    -t "$VOXTRAL_RUNTIME_IMAGE_URI" \
    --push \
    "$REPO_ROOT/aws/transcription-service"
fi

if [[ "$ENABLE_DIARIZATION_WORKER" == "true" && "$SKIP_DIARIZATION_WORKER_IMAGE_BUILD" != "true" ]]; then
  docker buildx build \
    --platform "$DOCKER_PLATFORM" \
    --file "$REPO_ROOT/aws/transcription-service/Dockerfile.diarization-worker" \
    --build-arg "TORCH_VERSION=$DIARIZATION_WORKER_TORCH_VERSION" \
    --build-arg "TORCHAUDIO_VERSION=$DIARIZATION_WORKER_TORCHAUDIO_VERSION" \
    --build-arg "TORCH_INDEX_URL=$DIARIZATION_WORKER_TORCH_INDEX_URL" \
    --build-arg "PYANNOTE_MODEL_ID=$PYANNOTE_MODEL_ID" \
    --build-arg "PRELOAD_PYANNOTE_MODEL=$DIARIZATION_WORKER_PRELOAD_MODEL_WEIGHTS" \
    ${MODEL_SECRET_ARGS[@]+"${MODEL_SECRET_ARGS[@]}"} \
    -t "$DIARIZATION_WORKER_IMAGE_URI" \
    --push \
    "$REPO_ROOT/aws/transcription-service"
fi

if [[ -n "${TRANSCRIPTION_SERVICE_TOKEN:-}" ]]; then
  SERVICE_TOKEN="$TRANSCRIPTION_SERVICE_TOKEN"
else
  SERVICE_TOKEN="$(aws_cli secretsmanager get-secret-value \
    --secret-id "$PROJECT_NAME/service-token" \
    --query SecretString \
    --output text 2>/dev/null || true)"
  if [[ -z "$SERVICE_TOKEN" || "$SERVICE_TOKEN" == "None" ]]; then
    SERVICE_TOKEN="$(openssl rand -base64 36)"
  fi
fi

aws_cli secretsmanager put-secret-value \
  --secret-id "$PROJECT_NAME/service-token" \
  --secret-string "$SERVICE_TOKEN" >/dev/null

if [[ -n "${TRANSCRIPTION_TOKEN_SIGNING_SECRET:-}" ]]; then
  USER_TOKEN_SIGNING_SECRET="$TRANSCRIPTION_TOKEN_SIGNING_SECRET"
else
  USER_TOKEN_SIGNING_SECRET="$(aws_cli secretsmanager get-secret-value \
    --secret-id "$PROJECT_NAME/user-token-signing-key" \
    --query SecretString \
    --output text 2>/dev/null || true)"
  if [[ -z "$USER_TOKEN_SIGNING_SECRET" || "$USER_TOKEN_SIGNING_SECRET" == "None" ]]; then
    USER_TOKEN_SIGNING_SECRET="$(openssl rand -base64 48)"
  fi
fi

aws_cli secretsmanager put-secret-value \
  --secret-id "$PROJECT_NAME/user-token-signing-key" \
  --secret-string "$USER_TOKEN_SIGNING_SECRET" >/dev/null

if [[ "$ENABLE_DIARIZATION_WORKER" == "true" || "$DIARIZATION_PROVIDER" == "remote_http" || "$DIARIZATION_PROVIDER" == "worker_http" ]]; then
  if [[ -n "${TRANSCRIPTION_DIARIZATION_WORKER_AUTH_TOKEN:-}" ]]; then
    DIARIZATION_WORKER_AUTH_TOKEN="$TRANSCRIPTION_DIARIZATION_WORKER_AUTH_TOKEN"
  else
    DIARIZATION_WORKER_AUTH_TOKEN="$(aws_cli secretsmanager get-secret-value \
      --secret-id "$PROJECT_NAME/diarization-worker-auth-token" \
      --query SecretString \
      --output text 2>/dev/null || true)"
    if [[ -z "$DIARIZATION_WORKER_AUTH_TOKEN" || "$DIARIZATION_WORKER_AUTH_TOKEN" == "None" ]]; then
      DIARIZATION_WORKER_AUTH_TOKEN="$(openssl rand -base64 36)"
    fi
  fi

  aws_cli secretsmanager put-secret-value \
    --secret-id "$PROJECT_NAME/diarization-worker-auth-token" \
    --secret-string "$DIARIZATION_WORKER_AUTH_TOKEN" >/dev/null
fi

if [[ "$ENABLE_TEXT_INTELLIGENCE_WORKER" == "true" || -n "$TEXT_INTELLIGENCE_WORKER_URL" ]]; then
  if [[ -n "${TEXT_INTELLIGENCE_WORKER_AUTH_TOKEN:-}" ]]; then
    TEXT_WORKER_AUTH_TOKEN="$TEXT_INTELLIGENCE_WORKER_AUTH_TOKEN"
  else
    TEXT_WORKER_AUTH_TOKEN="$(aws_cli secretsmanager get-secret-value \
      --secret-id "$PROJECT_NAME/text-intelligence-worker-auth-token" \
      --query SecretString \
      --output text 2>/dev/null || true)"
    if [[ -z "$TEXT_WORKER_AUTH_TOKEN" || "$TEXT_WORKER_AUTH_TOKEN" == "None" ]]; then
      TEXT_WORKER_AUTH_TOKEN="$(openssl rand -base64 36)"
    fi
  fi

  aws_cli secretsmanager put-secret-value \
    --secret-id "$PROJECT_NAME/text-intelligence-worker-auth-token" \
    --secret-string "$TEXT_WORKER_AUTH_TOKEN" >/dev/null
fi

terraform -chdir="$SCRIPT_DIR" apply -input=false -auto-approve \
  -var "aws_profile=$AWS_PROFILE_NAME" \
  -var "aws_region=$AWS_REGION_NAME" \
  -var "expected_account_id=$EXPECTED_ACCOUNT_ID" \
  -var "project_name=$PROJECT_NAME" \
  -var "image_uri=$IMAGE_URI" \
  -var "auth_mode=$TRANSCRIPTION_AUTH_MODE" \
  -var "allowed_users=$TRANSCRIPTION_ALLOWED_USERS" \
  -var "revoked_token_ids=$TRANSCRIPTION_REVOKED_TOKEN_IDS" \
  -var "token_issuer=$TRANSCRIPTION_TOKEN_ISSUER" \
  -var "token_audience=$TRANSCRIPTION_TOKEN_AUDIENCE" \
  -var "required_scope=$TRANSCRIPTION_REQUIRED_SCOPE" \
  -var "asr_provider=$ASR_PROVIDER" \
  -var "diarization_provider=$DIARIZATION_PROVIDER" \
  -var "enable_diarization_worker=$ENABLE_DIARIZATION_WORKER" \
  -var "diarization_worker_image_uri=$DIARIZATION_WORKER_IMAGE_URI" \
  -var "diarization_worker_provider=$DIARIZATION_WORKER_PROVIDER" \
  -var "diarization_worker_desired_count=$DIARIZATION_WORKER_DESIRED_COUNT" \
  -var "diarization_worker_launch_type=$DIARIZATION_WORKER_LAUNCH_TYPE" \
  -var "diarization_worker_task_cpu=$DIARIZATION_WORKER_TASK_CPU" \
  -var "diarization_worker_task_memory=$DIARIZATION_WORKER_TASK_MEMORY" \
  -var "diarization_worker_assign_public_ip=$DIARIZATION_WORKER_ASSIGN_PUBLIC_IP" \
  -var "diarization_worker_url=$DIARIZATION_WORKER_URL" \
  -var "diarization_worker_timeout_seconds=$DIARIZATION_WORKER_TIMEOUT_SECONDS" \
  -var "diarization_worker_gpu_count=$DIARIZATION_WORKER_GPU_COUNT" \
  -var "diarization_worker_pyannote_device=$DIARIZATION_WORKER_PYANNOTE_DEVICE" \
  -var "enable_diarization_worker_gpu_capacity=$ENABLE_DIARIZATION_WORKER_GPU_CAPACITY" \
  -var "diarization_worker_gpu_instance_type=$DIARIZATION_WORKER_GPU_INSTANCE_TYPE" \
  -var "diarization_worker_gpu_min_size=$DIARIZATION_WORKER_GPU_MIN_SIZE" \
  -var "diarization_worker_gpu_desired_capacity=$DIARIZATION_WORKER_GPU_DESIRED_CAPACITY" \
  -var "diarization_worker_gpu_max_size=$DIARIZATION_WORKER_GPU_MAX_SIZE" \
  -var "diarization_stop_timeout_seconds=$DIARIZATION_STOP_TIMEOUT_SECONDS" \
  -var "diarization_warmup_enabled=$DIARIZATION_WARMUP_ENABLED" \
  -var "enable_text_intelligence_worker=$ENABLE_TEXT_INTELLIGENCE_WORKER" \
  -var "text_intelligence_worker_image_uri=$TEXT_INTELLIGENCE_WORKER_IMAGE_URI" \
  -var "text_intelligence_runtime_image_uri=$TEXT_INTELLIGENCE_RUNTIME_IMAGE_URI" \
  -var "text_intelligence_worker_provider=$TEXT_INTELLIGENCE_WORKER_PROVIDER" \
  -var "text_intelligence_model=$TEXT_INTELLIGENCE_MODEL" \
  -var "text_intelligence_worker_desired_count=$TEXT_INTELLIGENCE_WORKER_DESIRED_COUNT" \
  -var "text_intelligence_worker_launch_type=$TEXT_INTELLIGENCE_WORKER_LAUNCH_TYPE" \
  -var "text_intelligence_worker_task_cpu=$TEXT_INTELLIGENCE_WORKER_TASK_CPU" \
  -var "text_intelligence_worker_task_memory=$TEXT_INTELLIGENCE_WORKER_TASK_MEMORY" \
  -var "text_intelligence_worker_assign_public_ip=$TEXT_INTELLIGENCE_WORKER_ASSIGN_PUBLIC_IP" \
  -var "text_intelligence_worker_url=$TEXT_INTELLIGENCE_WORKER_URL" \
  -var "text_intelligence_allowed_security_group_ids=$TEXT_INTELLIGENCE_ALLOWED_SECURITY_GROUP_IDS" \
  -var "text_intelligence_worker_auth_enabled=$TEXT_INTELLIGENCE_WORKER_AUTH_ENABLED" \
  -var "text_intelligence_request_timeout_seconds=$TEXT_INTELLIGENCE_REQUEST_TIMEOUT_SECONDS" \
  -var "text_intelligence_summary_timeout_seconds=$TEXT_INTELLIGENCE_SUMMARY_TIMEOUT_SECONDS" \
  -var "text_intelligence_max_translation_tokens=$TEXT_INTELLIGENCE_MAX_TRANSLATION_TOKENS" \
  -var "text_intelligence_max_summary_tokens=$TEXT_INTELLIGENCE_MAX_SUMMARY_TOKENS" \
  -var "text_intelligence_temperature=$TEXT_INTELLIGENCE_TEMPERATURE" \
  -var "text_intelligence_runtime_gpu_count=$TEXT_INTELLIGENCE_RUNTIME_GPU_COUNT" \
  -var "text_intelligence_runtime_max_model_len=$TEXT_INTELLIGENCE_RUNTIME_MAX_MODEL_LEN" \
  -var "text_intelligence_runtime_gpu_memory_utilization=$TEXT_INTELLIGENCE_RUNTIME_GPU_MEMORY_UTILIZATION" \
  -var "enable_text_intelligence_worker_gpu_capacity=$ENABLE_TEXT_INTELLIGENCE_WORKER_GPU_CAPACITY" \
  -var "reuse_diarization_worker_gpu_capacity_for_text_intelligence=$REUSE_DIARIZATION_WORKER_GPU_CAPACITY_FOR_TEXT_INTELLIGENCE" \
  -var "text_intelligence_worker_gpu_instance_type=$TEXT_INTELLIGENCE_WORKER_GPU_INSTANCE_TYPE" \
  -var "text_intelligence_worker_gpu_min_size=$TEXT_INTELLIGENCE_WORKER_GPU_MIN_SIZE" \
  -var "text_intelligence_worker_gpu_desired_capacity=$TEXT_INTELLIGENCE_WORKER_GPU_DESIRED_CAPACITY" \
  -var "text_intelligence_worker_gpu_max_size=$TEXT_INTELLIGENCE_WORKER_GPU_MAX_SIZE" \
  -var "ecs_launch_type=$ECS_LAUNCH_TYPE" \
  -var "enable_gpu_capacity=$ENABLE_GPU_CAPACITY" \
  -var "gpu_instance_type=$GPU_INSTANCE_TYPE" \
  -var "gpu_min_size=$GPU_MIN_SIZE" \
  -var "gpu_desired_capacity=$GPU_DESIRED_CAPACITY" \
  -var "gpu_max_size=$GPU_MAX_SIZE" \
  -var "enable_voxtral_runtime=$ENABLE_VOXTRAL_RUNTIME" \
  -var "voxtral_runtime_image_uri=$VOXTRAL_RUNTIME_IMAGE_URI" \
  -var "voxtral_model=$VOXTRAL_MODEL" \
  -var "voxtral_model_version=$VOXTRAL_MODEL_VERSION" \
  -var "voxtral_realtime_url=$VOXTRAL_REALTIME_URL" \
  -var "voxtral_final_response_timeout_seconds=$VOXTRAL_FINAL_RESPONSE_TIMEOUT_SECONDS" \
  -var "whisper_model=$WHISPER_MODEL" \
  -var "whisper_device=$WHISPER_DEVICE" \
  -var "whisper_compute_type=$WHISPER_COMPUTE_TYPE" \
  -var "require_gpu=$REQUIRE_GPU" \
  -var "models_offline=$MODELS_OFFLINE" \
  -var "enable_mistral_secret=$ENABLE_MISTRAL_SECRET" \
  -var "enable_pyannote_secret=$ENABLE_PYANNOTE_SECRET" \
  -var "pyannote_auth_token_secret_arn=$PYANNOTE_AUTH_TOKEN_SECRET_ARN" \
  -var "pyannote_model=$PYANNOTE_MODEL_ID" \
  -var "pyannote_device=$PYANNOTE_DEVICE" \
  -var "pyannote_min_speakers=$PYANNOTE_MIN_SPEAKERS" \
  -var "pyannote_max_speakers=$PYANNOTE_MAX_SPEAKERS" \
  -var "enable_public_admin_console=$ENABLE_PUBLIC_ADMIN_CONSOLE" \
  -var "public_admin_request_certificate=$PUBLIC_ADMIN_REQUEST_CERTIFICATE" \
  -var "public_admin_certificate_arn=$PUBLIC_ADMIN_CERTIFICATE_ARN" \
  -var "admin_desired_count=$ADMIN_DESIRED_COUNT" \
  -var "admin_image_uri=$ADMIN_IMAGE_URI" \
  -var "voicenotes_cognito_user_pool_id=$VOICENOTES_COGNITO_USER_POOL_ID" \
  -var "voicenotes_cognito_region=$VOICENOTES_COGNITO_REGION" \
  -var "voicenotes_admin_lambda_name=$VOICENOTES_ADMIN_LAMBDA_NAME" \
  -var "voicenotes_admin_lambda_region=$VOICENOTES_ADMIN_LAMBDA_REGION"

if [[ "$ENABLE_MISTRAL_SECRET" == "true" ]]; then
  aws_cli secretsmanager put-secret-value \
    --secret-id "$PROJECT_NAME/mistral-api-key" \
    --secret-string "$MISTRAL_API_KEY" >/dev/null
fi

if [[ "$ENABLE_PYANNOTE_SECRET" == "true" ]]; then
  aws_cli secretsmanager put-secret-value \
    --secret-id "$PROJECT_NAME/pyannote-auth-token" \
    --secret-string "$PYANNOTE_AUTH_TOKEN" >/dev/null
fi

terraform -chdir="$SCRIPT_DIR" apply -input=false -auto-approve \
  -var "aws_profile=$AWS_PROFILE_NAME" \
  -var "aws_region=$AWS_REGION_NAME" \
  -var "expected_account_id=$EXPECTED_ACCOUNT_ID" \
  -var "project_name=$PROJECT_NAME" \
  -var "image_uri=$IMAGE_URI" \
  -var "auth_mode=$TRANSCRIPTION_AUTH_MODE" \
  -var "allowed_users=$TRANSCRIPTION_ALLOWED_USERS" \
  -var "revoked_token_ids=$TRANSCRIPTION_REVOKED_TOKEN_IDS" \
  -var "token_issuer=$TRANSCRIPTION_TOKEN_ISSUER" \
  -var "token_audience=$TRANSCRIPTION_TOKEN_AUDIENCE" \
  -var "required_scope=$TRANSCRIPTION_REQUIRED_SCOPE" \
  -var "asr_provider=$ASR_PROVIDER" \
  -var "diarization_provider=$DIARIZATION_PROVIDER" \
  -var "enable_diarization_worker=$ENABLE_DIARIZATION_WORKER" \
  -var "diarization_worker_image_uri=$DIARIZATION_WORKER_IMAGE_URI" \
  -var "diarization_worker_provider=$DIARIZATION_WORKER_PROVIDER" \
  -var "diarization_worker_desired_count=$DIARIZATION_WORKER_DESIRED_COUNT" \
  -var "diarization_worker_launch_type=$DIARIZATION_WORKER_LAUNCH_TYPE" \
  -var "diarization_worker_task_cpu=$DIARIZATION_WORKER_TASK_CPU" \
  -var "diarization_worker_task_memory=$DIARIZATION_WORKER_TASK_MEMORY" \
  -var "diarization_worker_assign_public_ip=$DIARIZATION_WORKER_ASSIGN_PUBLIC_IP" \
  -var "diarization_worker_url=$DIARIZATION_WORKER_URL" \
  -var "diarization_worker_timeout_seconds=$DIARIZATION_WORKER_TIMEOUT_SECONDS" \
  -var "diarization_worker_gpu_count=$DIARIZATION_WORKER_GPU_COUNT" \
  -var "diarization_worker_pyannote_device=$DIARIZATION_WORKER_PYANNOTE_DEVICE" \
  -var "enable_diarization_worker_gpu_capacity=$ENABLE_DIARIZATION_WORKER_GPU_CAPACITY" \
  -var "diarization_worker_gpu_instance_type=$DIARIZATION_WORKER_GPU_INSTANCE_TYPE" \
  -var "diarization_worker_gpu_min_size=$DIARIZATION_WORKER_GPU_MIN_SIZE" \
  -var "diarization_worker_gpu_desired_capacity=$DIARIZATION_WORKER_GPU_DESIRED_CAPACITY" \
  -var "diarization_worker_gpu_max_size=$DIARIZATION_WORKER_GPU_MAX_SIZE" \
  -var "diarization_stop_timeout_seconds=$DIARIZATION_STOP_TIMEOUT_SECONDS" \
  -var "diarization_warmup_enabled=$DIARIZATION_WARMUP_ENABLED" \
  -var "enable_text_intelligence_worker=$ENABLE_TEXT_INTELLIGENCE_WORKER" \
  -var "text_intelligence_worker_image_uri=$TEXT_INTELLIGENCE_WORKER_IMAGE_URI" \
  -var "text_intelligence_runtime_image_uri=$TEXT_INTELLIGENCE_RUNTIME_IMAGE_URI" \
  -var "text_intelligence_worker_provider=$TEXT_INTELLIGENCE_WORKER_PROVIDER" \
  -var "text_intelligence_model=$TEXT_INTELLIGENCE_MODEL" \
  -var "text_intelligence_worker_desired_count=$TEXT_INTELLIGENCE_WORKER_DESIRED_COUNT" \
  -var "text_intelligence_worker_launch_type=$TEXT_INTELLIGENCE_WORKER_LAUNCH_TYPE" \
  -var "text_intelligence_worker_task_cpu=$TEXT_INTELLIGENCE_WORKER_TASK_CPU" \
  -var "text_intelligence_worker_task_memory=$TEXT_INTELLIGENCE_WORKER_TASK_MEMORY" \
  -var "text_intelligence_worker_assign_public_ip=$TEXT_INTELLIGENCE_WORKER_ASSIGN_PUBLIC_IP" \
  -var "text_intelligence_worker_url=$TEXT_INTELLIGENCE_WORKER_URL" \
  -var "text_intelligence_allowed_security_group_ids=$TEXT_INTELLIGENCE_ALLOWED_SECURITY_GROUP_IDS" \
  -var "text_intelligence_worker_auth_enabled=$TEXT_INTELLIGENCE_WORKER_AUTH_ENABLED" \
  -var "text_intelligence_request_timeout_seconds=$TEXT_INTELLIGENCE_REQUEST_TIMEOUT_SECONDS" \
  -var "text_intelligence_summary_timeout_seconds=$TEXT_INTELLIGENCE_SUMMARY_TIMEOUT_SECONDS" \
  -var "text_intelligence_max_translation_tokens=$TEXT_INTELLIGENCE_MAX_TRANSLATION_TOKENS" \
  -var "text_intelligence_max_summary_tokens=$TEXT_INTELLIGENCE_MAX_SUMMARY_TOKENS" \
  -var "text_intelligence_temperature=$TEXT_INTELLIGENCE_TEMPERATURE" \
  -var "text_intelligence_runtime_gpu_count=$TEXT_INTELLIGENCE_RUNTIME_GPU_COUNT" \
  -var "text_intelligence_runtime_max_model_len=$TEXT_INTELLIGENCE_RUNTIME_MAX_MODEL_LEN" \
  -var "text_intelligence_runtime_gpu_memory_utilization=$TEXT_INTELLIGENCE_RUNTIME_GPU_MEMORY_UTILIZATION" \
  -var "enable_text_intelligence_worker_gpu_capacity=$ENABLE_TEXT_INTELLIGENCE_WORKER_GPU_CAPACITY" \
  -var "reuse_diarization_worker_gpu_capacity_for_text_intelligence=$REUSE_DIARIZATION_WORKER_GPU_CAPACITY_FOR_TEXT_INTELLIGENCE" \
  -var "text_intelligence_worker_gpu_instance_type=$TEXT_INTELLIGENCE_WORKER_GPU_INSTANCE_TYPE" \
  -var "text_intelligence_worker_gpu_min_size=$TEXT_INTELLIGENCE_WORKER_GPU_MIN_SIZE" \
  -var "text_intelligence_worker_gpu_desired_capacity=$TEXT_INTELLIGENCE_WORKER_GPU_DESIRED_CAPACITY" \
  -var "text_intelligence_worker_gpu_max_size=$TEXT_INTELLIGENCE_WORKER_GPU_MAX_SIZE" \
  -var "ecs_launch_type=$ECS_LAUNCH_TYPE" \
  -var "enable_gpu_capacity=$ENABLE_GPU_CAPACITY" \
  -var "gpu_instance_type=$GPU_INSTANCE_TYPE" \
  -var "gpu_min_size=$GPU_MIN_SIZE" \
  -var "gpu_desired_capacity=$GPU_DESIRED_CAPACITY" \
  -var "gpu_max_size=$GPU_MAX_SIZE" \
  -var "enable_voxtral_runtime=$ENABLE_VOXTRAL_RUNTIME" \
  -var "voxtral_runtime_image_uri=$VOXTRAL_RUNTIME_IMAGE_URI" \
  -var "voxtral_model=$VOXTRAL_MODEL" \
  -var "voxtral_model_version=$VOXTRAL_MODEL_VERSION" \
  -var "voxtral_realtime_url=$VOXTRAL_REALTIME_URL" \
  -var "voxtral_final_response_timeout_seconds=$VOXTRAL_FINAL_RESPONSE_TIMEOUT_SECONDS" \
  -var "whisper_model=$WHISPER_MODEL" \
  -var "whisper_device=$WHISPER_DEVICE" \
  -var "whisper_compute_type=$WHISPER_COMPUTE_TYPE" \
  -var "require_gpu=$REQUIRE_GPU" \
  -var "models_offline=$MODELS_OFFLINE" \
  -var "enable_mistral_secret=$ENABLE_MISTRAL_SECRET" \
  -var "enable_pyannote_secret=$ENABLE_PYANNOTE_SECRET" \
  -var "pyannote_auth_token_secret_arn=$PYANNOTE_AUTH_TOKEN_SECRET_ARN" \
  -var "pyannote_model=$PYANNOTE_MODEL_ID" \
  -var "pyannote_device=$PYANNOTE_DEVICE" \
  -var "pyannote_min_speakers=$PYANNOTE_MIN_SPEAKERS" \
  -var "pyannote_max_speakers=$PYANNOTE_MAX_SPEAKERS" \
  -var "enable_public_admin_console=$ENABLE_PUBLIC_ADMIN_CONSOLE" \
  -var "public_admin_request_certificate=$PUBLIC_ADMIN_REQUEST_CERTIFICATE" \
  -var "public_admin_certificate_arn=$PUBLIC_ADMIN_CERTIFICATE_ARN" \
  -var "admin_desired_count=$ADMIN_DESIRED_COUNT" \
  -var "admin_image_uri=$ADMIN_IMAGE_URI" \
  -var "voicenotes_cognito_user_pool_id=$VOICENOTES_COGNITO_USER_POOL_ID" \
  -var "voicenotes_cognito_region=$VOICENOTES_COGNITO_REGION" \
  -var "voicenotes_admin_lambda_name=$VOICENOTES_ADMIN_LAMBDA_NAME" \
  -var "voicenotes_admin_lambda_region=$VOICENOTES_ADMIN_LAMBDA_REGION"

echo "AWS account: $ACCOUNT_ID"
echo "Image: $IMAGE_URI"
if [[ "$ENABLE_VOXTRAL_RUNTIME" == "true" ]]; then
  echo "Voxtral runtime image: $VOXTRAL_RUNTIME_IMAGE_URI"
fi
if [[ "$ENABLE_DIARIZATION_WORKER" == "true" ]]; then
  echo "Diarization worker image: $DIARIZATION_WORKER_IMAGE_URI"
  echo "Diarization worker private URL: $(terraform -chdir="$SCRIPT_DIR" output -raw diarization_worker_private_url)"
  echo "Diarization worker launch type: $(terraform -chdir="$SCRIPT_DIR" output -raw diarization_worker_launch_type)"
  DIARIZATION_WORKER_GPU_ASG="$(terraform -chdir="$SCRIPT_DIR" output -raw diarization_worker_gpu_autoscaling_group_name 2>/dev/null || true)"
  echo "Diarization worker GPU ASG: ${DIARIZATION_WORKER_GPU_ASG:-none}"
fi
if [[ "$ENABLE_TEXT_INTELLIGENCE_WORKER" == "true" ]]; then
  echo "Text-intelligence worker image: $TEXT_INTELLIGENCE_WORKER_IMAGE_URI"
  echo "Text-intelligence runtime image: $TEXT_INTELLIGENCE_RUNTIME_IMAGE_URI"
  echo "Text-intelligence worker private URL: $(terraform -chdir="$SCRIPT_DIR" output -raw text_intelligence_worker_private_url)"
  echo "Text-intelligence worker launch type: $(terraform -chdir="$SCRIPT_DIR" output -raw text_intelligence_worker_launch_type)"
  echo "Text-intelligence reuses diarization GPU capacity: $(terraform -chdir="$SCRIPT_DIR" output -raw text_intelligence_reuses_diarization_worker_gpu_capacity)"
  TEXT_WORKER_GPU_ASG="$(terraform -chdir="$SCRIPT_DIR" output -raw text_intelligence_worker_gpu_autoscaling_group_name 2>/dev/null || true)"
  echo "Text-intelligence worker GPU ASG: ${TEXT_WORKER_GPU_ASG:-none}"
fi
echo "WebSocket endpoint: $(terraform -chdir="$SCRIPT_DIR" output -raw websocket_endpoint)"
echo "Health endpoint: $(terraform -chdir="$SCRIPT_DIR" output -raw health_endpoint)"
if [[ "$TRANSCRIPTION_AUTH_MODE" == "signed_user_token" || "$TRANSCRIPTION_AUTH_MODE" == "signed_or_shared" ]]; then
  echo "Signed per-user token auth enabled. Mint user tokens with aws/transcription-service/scripts/mint-user-token.py and store only the user token in Cubicle Settings."
else
  echo "Service token saved in Secrets Manager. For local Cubicle testing, save the same token in Settings or export TRANSCRIPTION_SERVICE_TOKEN before running this script."
fi
