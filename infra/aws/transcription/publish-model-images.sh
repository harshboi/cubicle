#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

AWS_PROFILE_NAME="${AWS_PROFILE_NAME:-strln}"
AWS_REGION_NAME="${AWS_REGION_NAME:-us-west-2}"
EXPECTED_ACCOUNT_ID="${EXPECTED_ACCOUNT_ID:-562304353751}"
PROJECT_NAME="${PROJECT_NAME:-cubicle-transcription}"
IMAGE_TAG="${IMAGE_TAG:-models-$(date +%Y%m%d%H%M%S)}"
DOCKER_PLATFORM="${DOCKER_PLATFORM:-linux/amd64}"

PRELOAD_MODEL_WEIGHTS="${PRELOAD_MODEL_WEIGHTS:-true}"
PUSH_SERVICE_IMAGE="${PUSH_SERVICE_IMAGE:-false}"
PUSH_VOXTRAL_RUNTIME_IMAGE="${PUSH_VOXTRAL_RUNTIME_IMAGE:-true}"
DOCKER_LOGOUT_ON_EXIT="${DOCKER_LOGOUT_ON_EXIT:-true}"

VOXTRAL_MODEL="${VOXTRAL_MODEL:-mistralai/Voxtral-Mini-4B-Realtime-2602}"
WHISPER_MODEL="${WHISPER_MODEL:-h2oai/faster-whisper-large-v3-turbo}"
PYANNOTE_MODEL_ID="${PYANNOTE_MODEL_ID:-pyannote/speaker-diarization-community-1}"

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
Refusing to publish model images while ambient AWS credential/profile variables are set:
  ${conflicting_aws_env[*]}

This publishing path is pinned to AWS profile '$AWS_PROFILE_NAME' and account '$EXPECTED_ACCOUNT_ID'.
Unset the conflicting variables first, for example:

  unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN AWS_SECURITY_TOKEN AWS_WEB_IDENTITY_TOKEN_FILE AWS_ROLE_ARN AWS_CONTAINER_CREDENTIALS_FULL_URI AWS_CONTAINER_CREDENTIALS_RELATIVE_URI

EOF
  exit 2
fi

if [[ "$PRELOAD_MODEL_WEIGHTS" == "true" && -z "${HF_TOKEN:-}" && -z "${HUGGINGFACE_TOKEN:-}" && -z "${HF_TOKEN_FILE:-}" && -z "${HUGGINGFACE_TOKEN_FILE:-}" ]]; then
  cat >&2 <<EOF
PRELOAD_MODEL_WEIGHTS=true requires HF_TOKEN_FILE, HUGGINGFACE_TOKEN_FILE, HF_TOKEN, or HUGGINGFACE_TOKEN.

Prefer HF_TOKEN_FILE or HUGGINGFACE_TOKEN_FILE so the token is not visible in a
local process environment. The token is passed to Docker BuildKit as a secret so
it is not committed, printed, or persisted as an image environment variable. It
must have access to the selected Hugging Face models, including gated pyannote
model access.
EOF
  exit 2
fi

aws_cli() {
  aws --profile "$AWS_PROFILE_NAME" --region "$AWS_REGION_NAME" "$@"
}

ACCOUNT_ID="$(aws_cli sts get-caller-identity --query Account --output text)"
if [[ "$ACCOUNT_ID" != "$EXPECTED_ACCOUNT_ID" ]]; then
  echo "Refusing to publish to AWS account $ACCOUNT_ID; expected $EXPECTED_ACCOUNT_ID." >&2
  exit 2
fi

terraform -chdir="$SCRIPT_DIR" init -input=false
terraform -chdir="$SCRIPT_DIR" apply -input=false -auto-approve \
  -target=aws_ecr_repository.service \
  -target=aws_ecr_repository.voxtral_runtime \
  -var "aws_profile=$AWS_PROFILE_NAME" \
  -var "aws_region=$AWS_REGION_NAME" \
  -var "expected_account_id=$EXPECTED_ACCOUNT_ID" \
  -var "project_name=$PROJECT_NAME"

SERVICE_REPOSITORY_URL="$(terraform -chdir="$SCRIPT_DIR" output -raw repository_url)"
VOXTRAL_RUNTIME_REPOSITORY_URL="$(terraform -chdir="$SCRIPT_DIR" output -raw voxtral_runtime_repository_url)"
SERVICE_IMAGE_URI="${SERVICE_IMAGE_URI:-$SERVICE_REPOSITORY_URL:$IMAGE_TAG}"
VOXTRAL_RUNTIME_IMAGE_URI="${VOXTRAL_RUNTIME_IMAGE_URI:-$VOXTRAL_RUNTIME_REPOSITORY_URL:$IMAGE_TAG}"
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

if [[ -n "${EXTRA_CA_CERT_FILE:-}" ]]; then
  if [[ ! -r "$EXTRA_CA_CERT_FILE" ]]; then
    echo "EXTRA_CA_CERT_FILE is set but is not readable: $EXTRA_CA_CERT_FILE" >&2
    exit 2
  fi
  MODEL_SECRET_ARGS+=(--secret id=extra_ca,src="$EXTRA_CA_CERT_FILE")
fi

if [[ "$PUSH_SERVICE_IMAGE" == "true" ]]; then
  docker buildx build \
    --platform "$DOCKER_PLATFORM" \
    --build-arg INSTALL_VOXTRAL_REALTIME=false \
    --build-arg INSTALL_DIARIZATION=true \
    --build-arg INSTALL_SELF_HOSTED_MODELS=true \
    --build-arg "PRELOAD_SERVICE_MODELS=$PRELOAD_MODEL_WEIGHTS" \
    --build-arg "WHISPER_MODEL_ID=$WHISPER_MODEL" \
    --build-arg "PYANNOTE_MODEL_ID=$PYANNOTE_MODEL_ID" \
    "${MODEL_SECRET_ARGS[@]}" \
    -t "$SERVICE_IMAGE_URI" \
    --push \
    "$REPO_ROOT/aws/transcription-service"
else
  echo "Skipping service image build. Set PUSH_SERVICE_IMAGE=true to preload Whisper/pyannote dependencies into a service image."
fi

if [[ "$PUSH_VOXTRAL_RUNTIME_IMAGE" == "true" ]]; then
  docker buildx build \
    --platform "$DOCKER_PLATFORM" \
    --file "$REPO_ROOT/aws/transcription-service/Dockerfile.voxtral-runtime" \
    --build-arg "VOXTRAL_MODEL_ID=$VOXTRAL_MODEL" \
    --build-arg "PRELOAD_VOXTRAL_MODEL=$PRELOAD_MODEL_WEIGHTS" \
    "${MODEL_SECRET_ARGS[@]}" \
    -t "$VOXTRAL_RUNTIME_IMAGE_URI" \
    --push \
    "$REPO_ROOT/aws/transcription-service"
else
  echo "Skipping Voxtral runtime image build. Set PUSH_VOXTRAL_RUNTIME_IMAGE=true to publish the vLLM runtime image."
fi

echo "AWS account: $ACCOUNT_ID"
if [[ "$PUSH_SERVICE_IMAGE" == "true" ]]; then
  echo "Service model image: $SERVICE_IMAGE_URI"
fi
if [[ "$PUSH_VOXTRAL_RUNTIME_IMAGE" == "true" ]]; then
  echo "Voxtral runtime model image: $VOXTRAL_RUNTIME_IMAGE_URI"
fi
echo "No ECS service, task definition, Auto Scaling group, or GPU desired capacity was changed."
