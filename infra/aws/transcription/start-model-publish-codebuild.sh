#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

AWS_PROFILE_NAME="${AWS_PROFILE_NAME:-strln}"
AWS_REGION_NAME="${AWS_REGION_NAME:-us-west-2}"
EXPECTED_ACCOUNT_ID="${EXPECTED_ACCOUNT_ID:-562304353751}"
PROJECT_NAME="${PROJECT_NAME:-cubicle-transcription}"
CODEBUILD_PROJECT_NAME="${CODEBUILD_PROJECT_NAME:-cubicle-transcription-model-publish}"
CODEBUILD_ROLE_NAME="${CODEBUILD_ROLE_NAME:-cubicle-transcription-model-publish-codebuild-role}"
CODEBUILD_LOG_GROUP="${CODEBUILD_LOG_GROUP:-/aws/codebuild/cubicle-transcription-model-publish}"
HF_SECRET_NAME="${HF_SECRET_NAME:-cubicle-transcription/huggingface-token}"
IMAGE_TAG="${IMAGE_TAG:-models-$(date +%Y%m%d%H%M%S)}"
VOXTRAL_MODEL="${VOXTRAL_MODEL:-mistralai/Voxtral-Mini-4B-Realtime-2602}"
VLLM_BASE_IMAGE="${VLLM_BASE_IMAGE:-vllm/vllm-openai:v0.21.0-ubuntu2404}"
CODEBUILD_COMPUTE_TYPE="${CODEBUILD_COMPUTE_TYPE:-BUILD_GENERAL1_LARGE}"
CODEBUILD_IMAGE="${CODEBUILD_IMAGE:-aws/codebuild/standard:7.0}"
WAIT_FOR_BUILD="${WAIT_FOR_BUILD:-true}"
POLL_SECONDS="${POLL_SECONDS:-30}"

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
Refusing to start the AWS model publish builder while ambient AWS credential/profile variables are set:
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
  echo "Refusing to publish to AWS account $ACCOUNT_ID; expected $EXPECTED_ACCOUNT_ID." >&2
  exit 2
fi

aws_cli secretsmanager describe-secret --secret-id "$HF_SECRET_NAME" >/dev/null

terraform -chdir="$SCRIPT_DIR" init -input=false
terraform -chdir="$SCRIPT_DIR" apply -input=false -auto-approve \
  -target=aws_ecr_repository.voxtral_runtime \
  -var "aws_profile=$AWS_PROFILE_NAME" \
  -var "aws_region=$AWS_REGION_NAME" \
  -var "expected_account_id=$EXPECTED_ACCOUNT_ID" \
  -var "project_name=$PROJECT_NAME"

VOXTRAL_RUNTIME_REPOSITORY_URL="$(terraform -chdir="$SCRIPT_DIR" output -raw voxtral_runtime_repository_url)"
ECR_REGISTRY="$ACCOUNT_ID.dkr.ecr.$AWS_REGION_NAME.amazonaws.com"
ROLE_ARN="arn:aws:iam::$ACCOUNT_ID:role/$CODEBUILD_ROLE_NAME"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/cubicle-codebuild-model-publish.XXXXXX")"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

cat > "$TMP_DIR/trust-policy.json" <<'JSON'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "codebuild.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
JSON

if ! aws --profile "$AWS_PROFILE_NAME" iam get-role --role-name "$CODEBUILD_ROLE_NAME" >/dev/null 2>&1; then
  aws --profile "$AWS_PROFILE_NAME" iam create-role \
    --role-name "$CODEBUILD_ROLE_NAME" \
    --assume-role-policy-document "file://$TMP_DIR/trust-policy.json" >/dev/null
fi

cat > "$TMP_DIR/inline-policy.json" <<JSON
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "ReadHuggingFaceToken",
      "Effect": "Allow",
      "Action": "secretsmanager:GetSecretValue",
      "Resource": "arn:aws:secretsmanager:${AWS_REGION_NAME}:${ACCOUNT_ID}:secret:${HF_SECRET_NAME}*"
    },
    {
      "Sid": "PushVoxtralRuntimeImage",
      "Effect": "Allow",
      "Action": [
        "ecr:BatchCheckLayerAvailability",
        "ecr:BatchGetImage",
        "ecr:CompleteLayerUpload",
        "ecr:DescribeImages",
        "ecr:DescribeRepositories",
        "ecr:GetDownloadUrlForLayer",
        "ecr:InitiateLayerUpload",
        "ecr:ListImages",
        "ecr:PutImage",
        "ecr:UploadLayerPart"
      ],
      "Resource": "arn:aws:ecr:${AWS_REGION_NAME}:${ACCOUNT_ID}:repository/${PROJECT_NAME}-voxtral-runtime"
    },
    {
      "Sid": "AuthorizeECRLogin",
      "Effect": "Allow",
      "Action": "ecr:GetAuthorizationToken",
      "Resource": "*"
    },
    {
      "Sid": "WriteCodeBuildLogs",
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": [
        "arn:aws:logs:${AWS_REGION_NAME}:${ACCOUNT_ID}:log-group:${CODEBUILD_LOG_GROUP}",
        "arn:aws:logs:${AWS_REGION_NAME}:${ACCOUNT_ID}:log-group:${CODEBUILD_LOG_GROUP}:*"
      ]
    },
    {
      "Sid": "IdentifyCaller",
      "Effect": "Allow",
      "Action": "sts:GetCallerIdentity",
      "Resource": "*"
    }
  ]
}
JSON

aws --profile "$AWS_PROFILE_NAME" iam put-role-policy \
  --role-name "$CODEBUILD_ROLE_NAME" \
  --policy-name CubicleTranscriptionModelPublish \
  --policy-document "file://$TMP_DIR/inline-policy.json" >/dev/null

# IAM role and inline-policy propagation can lag role creation. Give CodeBuild's
# service-role validation a short window before create/update-project.
sleep "${CODEBUILD_IAM_PROPAGATION_SECONDS:-20}"

aws_cli logs create-log-group --log-group-name "$CODEBUILD_LOG_GROUP" >/dev/null 2>&1 || true
aws_cli logs put-retention-policy --log-group-name "$CODEBUILD_LOG_GROUP" --retention-in-days 7

DOCKERFILE_B64="$(base64 < "$REPO_ROOT/aws/transcription-service/Dockerfile.voxtral-runtime" | tr -d '\n')"
ENTRYPOINT_B64="$(base64 < "$REPO_ROOT/aws/transcription-service/scripts/voxtral-runtime-entrypoint.sh" | tr -d '\n')"

cat > "$TMP_DIR/buildspec.yml" <<YAML
version: 0.2

phases:
  pre_build:
    commands:
      - set -eu
      - echo "Starting Cubicle model publish build in account \$(aws sts get-caller-identity --query Account --output text)"
      - mkdir -p scripts
      - printf '%s' "$DOCKERFILE_B64" | base64 -d > Dockerfile.voxtral-runtime
      - printf '%s' "$ENTRYPOINT_B64" | base64 -d > scripts/voxtral-runtime-entrypoint.sh
      - chmod +x scripts/voxtral-runtime-entrypoint.sh
      - aws secretsmanager get-secret-value --region "$AWS_REGION_NAME" --secret-id "$HF_SECRET_NAME" --query SecretString --output text > /tmp/cubicle-hf-token
      - chmod 600 /tmp/cubicle-hf-token
      - python3 -c 'import sys; token = sys.stdin.read().strip(); assert not token.startswith("file://"), "HF secret value looks like a file URI, not the token contents"; assert len(token) >= 20, "HF secret value is too short"; print("HF token secret loaded into a temporary file")' < /tmp/cubicle-hf-token
      - aws ecr get-login-password --region "$AWS_REGION_NAME" | docker login --username AWS --password-stdin "$ECR_REGISTRY"
      - docker version
      - docker buildx version || true
      - df -h
  build:
    commands:
      - set -eu
      - docker buildx create --use --name cubicle-model-builder || docker buildx use cubicle-model-builder
      - docker buildx build --platform linux/amd64 --file Dockerfile.voxtral-runtime --build-arg VLLM_BASE_IMAGE="$VLLM_BASE_IMAGE" --build-arg VOXTRAL_MODEL_ID="$VOXTRAL_MODEL" --build-arg PRELOAD_VOXTRAL_MODEL=true --secret id=hf_token,src=/tmp/cubicle-hf-token -t "$VOXTRAL_RUNTIME_REPOSITORY_URL:$IMAGE_TAG" --push .
  post_build:
    commands:
      - rm -f /tmp/cubicle-hf-token
      - docker logout "$ECR_REGISTRY" >/dev/null 2>&1 || true
      - aws ecr describe-images --region "$AWS_REGION_NAME" --repository-name "${PROJECT_NAME}-voxtral-runtime" --image-ids imageTag="$IMAGE_TAG" --query 'imageDetails[0].{digest:imageDigest,size:imageSizeInBytes,pushedAt:imagePushedAt}' --output json
YAML

python3 - "$TMP_DIR/buildspec.yml" "$TMP_DIR/project.json" <<PY
import json
import sys
from pathlib import Path

buildspec = Path(sys.argv[1]).read_text()
project = {
    "name": "${CODEBUILD_PROJECT_NAME}",
    "description": "Builds and pushes the preloaded Cubicle Voxtral runtime image; does not change ECS service or GPU capacity.",
    "source": {
        "type": "NO_SOURCE",
        "buildspec": buildspec,
    },
    "artifacts": {"type": "NO_ARTIFACTS"},
    "environment": {
        "type": "LINUX_CONTAINER",
        "image": "${CODEBUILD_IMAGE}",
        "computeType": "${CODEBUILD_COMPUTE_TYPE}",
        "privilegedMode": True,
        "environmentVariables": [
            {"name": "DOCKER_BUILDKIT", "value": "1", "type": "PLAINTEXT"},
            {"name": "BUILDKIT_PROGRESS", "value": "plain", "type": "PLAINTEXT"},
        ],
    },
    "serviceRole": "${ROLE_ARN}",
    "timeoutInMinutes": 120,
    "queuedTimeoutInMinutes": 30,
    "logsConfig": {
        "cloudWatchLogs": {
            "status": "ENABLED",
            "groupName": "${CODEBUILD_LOG_GROUP}",
        }
    },
}
Path(sys.argv[2]).write_text(json.dumps(project, indent=2))
PY

project_exists=false
if aws_cli codebuild batch-get-projects --names "$CODEBUILD_PROJECT_NAME" --query 'projects[0].name' --output text 2>/dev/null | grep -qx "$CODEBUILD_PROJECT_NAME"; then
  project_exists=true
fi

project_mutation_succeeded=false
for attempt in 1 2 3 4; do
  if [[ "$project_exists" == "true" ]]; then
    if aws_cli codebuild update-project --cli-input-json "file://$TMP_DIR/project.json" >/dev/null 2>"$TMP_DIR/codebuild-project.err"; then
      project_mutation_succeeded=true
      break
    fi
  else
    if aws_cli codebuild create-project --cli-input-json "file://$TMP_DIR/project.json" >/dev/null 2>"$TMP_DIR/codebuild-project.err"; then
      project_mutation_succeeded=true
      break
    fi
  fi

  if grep -q 'sts:AssumeRole' "$TMP_DIR/codebuild-project.err"; then
    sleep 20
    continue
  fi

  cat "$TMP_DIR/codebuild-project.err" >&2
  exit 1
done

if [[ "$project_mutation_succeeded" != "true" ]]; then
  cat "$TMP_DIR/codebuild-project.err" >&2
  exit 1
fi

BUILD_ID="$(aws_cli codebuild start-build --project-name "$CODEBUILD_PROJECT_NAME" --query 'build.id' --output text)"
echo "Started CodeBuild model publish: $BUILD_ID"
echo "Target image: $VOXTRAL_RUNTIME_REPOSITORY_URL:$IMAGE_TAG"
echo "GPU capacity and ECS service were not changed."

if [[ "$WAIT_FOR_BUILD" != "true" ]]; then
  exit 0
fi

while true; do
  BUILD_JSON="$(aws_cli codebuild batch-get-builds --ids "$BUILD_ID" --output json)"
  STATUS="$(printf '%s' "$BUILD_JSON" | jq -r '.builds[0].buildStatus')"
  PHASE="$(printf '%s' "$BUILD_JSON" | jq -r '.builds[0].currentPhase // "UNKNOWN"')"
  echo "CodeBuild status: $STATUS phase: $PHASE"
  case "$STATUS" in
    SUCCEEDED)
      aws_cli ecr describe-images \
        --repository-name "${PROJECT_NAME}-voxtral-runtime" \
        --image-ids "imageTag=$IMAGE_TAG" \
        --query 'imageDetails[0].{digest:imageDigest,size:imageSizeInBytes,pushedAt:imagePushedAt}' \
        --output json
      exit 0
      ;;
    FAILED|FAULT|STOPPED|TIMED_OUT)
      echo "CodeBuild model publish failed: $BUILD_ID" >&2
      printf '%s' "$BUILD_JSON" | jq -r '.builds[0].phases[]? | select(.phaseStatus=="FAILED" or .phaseStatus=="FAULT" or .phaseStatus=="TIMED_OUT" or .phaseStatus=="STOPPED") | "\(.phaseType): \(.contexts[]?.message)"' >&2
      exit 1
      ;;
  esac
  sleep "$POLL_SECONDS"
done
