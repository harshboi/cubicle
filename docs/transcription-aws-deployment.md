# Cubicle AWS Transcription Deployment

Status: self-hosted model runtime implementation path added. The service can validate Cubicle WebSocket session contracts, run the mock provider by default, route live audio to a self-hosted Voxtral Realtime runtime, keep faster-whisper/CTranslate2 as the fallback for English transcription and Japanese-to-English translation, run a separate optional pyannote.audio diarization provider, and enforce either legacy shared-token auth or short-lived signed per-user tokens. A Docker-based Voxtral vLLM runtime has been verified on AWS EC2 g5.xlarge instance `i-02b84c39f9912a77a` in account `562304353751`. The secure direct path is Cubicle -> CloudFront -> ALB -> ECS transcription service -> private vLLM runtime, with no public vLLM port and no Mac SSM tunnel. Final-correction workers are not provisioned yet.

## Architecture

- Client: Cubicle macOS app connects to a WebSocket endpoint configured in Settings and sends the Keychain-stored bearer token in the `Authorization` header.
- Ingress: CloudFront provides TLS for `wss://dcabsri6ekziv.cloudfront.net/v1/transcription`, forwarding `/v1/transcription` WebSocket traffic to an Application Load Balancer and ECS service. The ALB security group is restricted to AWS's CloudFront origin-facing managed prefix list so callers cannot bypass CloudFront and hit the ALB directly.
- Service: `aws/transcription-service` FastAPI/uvicorn container. This is the auth, protocol, transcript aggregation, diarization, and logging boundary. Cubicle should not connect directly to vLLM.
- ASR / translation provider: `TRANSCRIPTION_ASR_PROVIDER=voxtral_self_hosted`, which connects to a self-hosted vLLM Realtime endpoint such as `ws://<private-vllm-ip>:8000/v1/realtime`. This path does not use `MISTRAL_API_KEY`.
- Self-hosted inference runtime: the verified path is Docker on EC2 using `vllm/vllm-openai:v0.21.0-ubuntu2404` plus the Voxtral audio dependency install inside the container. `aws/transcription-service/Dockerfile.voxtral-runtime` and Terraform keep the same settings for a future ECS EC2 sidecar path.
- Legacy hosted ASR / translation provider: `TRANSCRIPTION_ASR_PROVIDER=voxtral_realtime` via Mistral's realtime SDK is retained for rollback/testing only.
- Fallback ASR provider: faster-whisper/CTranslate2 with Whisper large-v3 or large-v3-turbo.
- Diarization provider: separate `TRANSCRIPTION_DIARIZATION_PROVIDER` boundary. Local default is `mock`; production can use `pyannote` with `pyannote/speaker-diarization-community-1` on pyannote.audio 4.x, or the legacy `pyannote/speaker-diarization-3.1` pipeline when pinned to a compatible 3.x image.
- Alignment/glue: Slice 5 assigns finalized transcript segment IDs to pyannote speaker turns by timestamp overlap, matching the whisperX-style separation between ASR timestamps and diarization turns. Future word-level timestamps can extend the same assignment boundary without changing the Cubicle client event contract.
- Storage: disabled by default. If raw audio retention is enabled later, use S3 with SSE-KMS and explicit lifecycle retention.
- Metadata: future production deployment should persist session and segment metadata in DynamoDB.
- Async correction/finalization: future production deployment should use SQS jobs and a finalization worker.

## AWS Components

- ECR repository for the service image.
- ECR repository for the self-hosted Voxtral vLLM runtime image.
- ECS Fargate for the current secure mock/Voxtral-managed API staging service.
- ECS on EC2 GPU capacity or EKS with GPU node groups for production local fallback ASR and pyannote GPU diarization.
- Do not use Fargate for GPU inference.
- ALB with WebSocket support and idle timeout tuned for long sessions.
- CloudFront default TLS certificate for the current staging endpoint.
- ACM certificate and Route53 DNS record for a production WebSocket host when a domain is available in the deployment account.
- VPC with public ALB subnets, private worker subnets, ALB ingress restricted to CloudFront origin-facing ranges, and task ingress restricted to the ALB.
- CloudWatch log group with transcript/audio content excluded from default logs.
- Secrets Manager or SSM Parameter Store for `TRANSCRIPTION_SERVICE_TOKEN`, `TRANSCRIPTION_TOKEN_SIGNING_SECRET`, and model/provider secrets.
- IAM task role with least-privilege access to only required stores.
- Optional S3 bucket with KMS key for encrypted audio retention. Keep retention disabled by default.
- Optional DynamoDB tables for sessions and transcript segment metadata.
- Optional SQS queues for final correction jobs.

## Self-Hosted Model Policy

The production target is to keep customer audio and model inference inside the AWS account:

1. Voxtral Realtime runs in a GPU vLLM runtime container in ECS EC2 or EKS GPU nodes.
2. Whisper large-v3-turbo fallback runs through faster-whisper/CTranslate2 in the service container or a future worker container.
3. pyannote.audio runs locally in the service container or a future diarization worker.
4. `MISTRAL_API_KEY` is not required for `voxtral_self_hosted`.
5. Hugging Face tokens are only bootstrap/download credentials for gated pyannote weights. They should be passed as BuildKit secrets or Secrets Manager values, never committed, logged, or placed in Cubicle Settings.
6. For fully offline runtime, preload weights into ECR images or an encrypted AWS-owned model volume and set `TRANSCRIPTION_MODELS_OFFLINE=true`.

Default model IDs:

```text
Voxtral Realtime: mistralai/Voxtral-Mini-4B-Realtime-2602
Whisper fallback: h2oai/faster-whisper-large-v3-turbo
Diarization:      pyannote/speaker-diarization-community-1
```

`pyannote/speaker-diarization-community-1` is freely accessible after accepting the Hugging Face model conditions. The token is still required for the initial gated download unless the model has already been mirrored into the AWS-owned image/cache.

## Direct App-To-AWS Auth

The secure direct client path is:

```text
Cubicle.app
  -> wss://<cloudfront-host>/v1/transcription
  -> CloudFront TLS
  -> ALB restricted to CloudFront origin-facing ranges
  -> ECS transcription service
  -> private self-hosted vLLM Voxtral runtime
```

The app does not need to speak to EC2 or vLLM directly. The service remains the
policy point because it validates Cubicle's protocol, authenticates users,
controls diarization, emits stable transcript events, and keeps audio/transcript
content out of default logs.

For the current verified EC2 vLLM runtime, deploy the direct adapter with:

```bash
TRANSCRIPTION_ALLOWED_USERS=prabhat7@cisco.com \
AWS_PROFILE_NAME=strln \
EXPECTED_ACCOUNT_ID=562304353751 \
./infra/transcription/deploy-direct-aws-adapter.sh
```

The wrapper discovers the private IP for `i-02b84c39f9912a77a`, deploys the
Fargate adapter behind CloudFront/ALB, enables `signed_user_token`, and points
the adapter at `ws://<private-ip>:8000/v1/realtime`. Terraform allows port
`8000` only between members of the transcription task security group, so the
GPU runtime remains private while client laptops connect directly to the WSS
CloudFront endpoint. The wrapper preserves the existing GPU capacity resources
for the manually verified EC2 runtime while keeping the app-facing adapter on
Fargate and not starting a new vLLM sidecar.

For production, use signed per-user tokens instead of a single shared service
token:

```bash
TRANSCRIPTION_AUTH_MODE=signed_user_token
TRANSCRIPTION_ALLOWED_USERS=prabhat7@cisco.com
TRANSCRIPTION_TOKEN_ISSUER=cubicle-transcription
TRANSCRIPTION_TOKEN_AUDIENCE=cubicle-macos
TRANSCRIPTION_REQUIRED_SCOPE=transcription:stream
```

The service validates token signature, expiry, not-before time, issuer,
audience, required scope, optional token id revocation, and optional user/email
allow-list membership. The signing secret is injected from Secrets Manager as
`TRANSCRIPTION_TOKEN_SIGNING_SECRET`; Cubicle never stores that signing secret.
Cubicle stores only the short-lived user token in Keychain and sends it as:

```text
Authorization: Bearer <signed-user-token>
```

Mint a token from the AWS signing secret:

```bash
umask 077
aws secretsmanager get-secret-value \
  --profile strln \
  --region us-west-2 \
  --secret-id cubicle-transcription/user-token-signing-key \
  --query SecretString \
  --output text > /tmp/cubicle-user-token-signing-key

aws/transcription-service/scripts/mint-user-token.py \
  --secret-file /tmp/cubicle-user-token-signing-key \
  --subject prabhat7@cisco.com \
  --email prabhat7@cisco.com \
  --ttl-seconds 3600

rm -f /tmp/cubicle-user-token-signing-key
```

To add or remove users, update `TRANSCRIPTION_ALLOWED_USERS` and redeploy. To
revoke a specific token before expiry, add its `jti` to
`TRANSCRIPTION_REVOKED_TOKEN_IDS` and redeploy. Use short TTLs until this is
fronted by an internal identity-provider or Cognito/OIDC token broker.

## Verified g5 Docker Runtime And SSM Fallback

The known-good Voxtral runtime is Docker-based on Amazon Linux 2. Do not retry
native Python, Conda, or pyenv vLLM installs on this instance; the native path
failed on system Python, OpenSSL, SciPy/NumPy/xformers builds, and the old GCC
toolchain. Keep port 8000 private. The production app path reaches this runtime
through the ECS adapter over private VPC networking. SSM port forwarding is only
for operator debugging or the old local-adapter fallback.

Current runtime:

```text
Region: us-west-2
Instance: i-02b84c39f9912a77a
Instance type: g5.xlarge
GPU: NVIDIA A10G
Container: voxtral-vllm
Image: vllm/vllm-openai:v0.21.0-ubuntu2404
Model: mistralai/Voxtral-Mini-4B-Realtime-2602
Local EC2 port: 8000
```

For debugging, a Mac can still open an SSM port forward:

```bash
aws ssm start-session \
  --profile strln \
  --region us-west-2 \
  --target i-02b84c39f9912a77a \
  --document-name AWS-StartPortForwardingSession \
  --parameters '{"portNumber":["8000"],"localPortNumber":["8000"]}'
```

Then verify the forwarded vLLM server:

```bash
curl -fsS http://localhost:8000/health
curl -fsS http://localhost:8000/v1/models | python3 -m json.tool
```

Expected model ID:

```text
mistralai/Voxtral-Mini-4B-Realtime-2602
```

Run the Cubicle transcription service as the adapter:

```bash
python3 -m pip install -r aws/transcription-service/requirements.txt
set -a
. aws/transcription-service/env.vllm-forwarded.example
set +a
TRANSCRIPTION_SERVICE_TOKEN_FILE=/path/to/0600-token-file \
PYTHONPATH=aws/transcription-service python3 -m transcription_service.main
```

The important service-side endpoint variables are:

```bash
VLLM_BASE_URL=http://localhost:8000
VLLM_REALTIME_URL=ws://localhost:8000/v1/realtime
VLLM_MODEL=mistralai/Voxtral-Mini-4B-Realtime-2602
```

Cubicle Settings should point at the Cubicle service WebSocket endpoint, such
as `ws://127.0.0.1:8080/v1/transcription`, and should store the service bearer
token in Keychain. Do not point Cubicle directly at `ws://localhost:8000/v1/realtime`
unless a separate direct OpenAI Realtime client is implemented.

## Build The Service Image

```bash
docker build -t cubicle-transcription-service:local aws/transcription-service
```

The default image keeps the mock provider path so local builds and contract tests do not require external ASR dependencies. To include the self-hosted service dependencies for Whisper fallback and pyannote:

```bash
docker build \
  --build-arg INSTALL_SELF_HOSTED_MODELS=true \
  -t cubicle-transcription-service:selfhost \
  aws/transcription-service
```

To preload Whisper and pyannote weights into the service image, use a temporary
file-backed BuildKit secret. Do not pass the token with `env=HF_TOKEN` because
that can expose it through local process inspection while Docker is running:

```bash
umask 077
security find-generic-password -a cubicle-transcription -s cubicle-hf-token -w > /tmp/cubicle-hf-token
docker build \
  --secret id=hf_token,src=/tmp/cubicle-hf-token \
  --build-arg INSTALL_SELF_HOSTED_MODELS=true \
  --build-arg PRELOAD_SERVICE_MODELS=true \
  --build-arg WHISPER_MODEL_ID=h2oai/faster-whisper-large-v3-turbo \
  --build-arg PYANNOTE_MODEL_ID=pyannote/speaker-diarization-community-1 \
  -t cubicle-transcription-service:selfhost-preloaded \
  aws/transcription-service
rm -f /tmp/cubicle-hf-token
```

To build the self-hosted Voxtral runtime:

```bash
docker build \
  -f aws/transcription-service/Dockerfile.voxtral-runtime \
  --build-arg VOXTRAL_MODEL_ID=mistralai/Voxtral-Mini-4B-Realtime-2602 \
  -t cubicle-transcription-voxtral-runtime:local \
  aws/transcription-service
```

To preload Voxtral weights into the vLLM runtime image:

```bash
docker build \
  -f aws/transcription-service/Dockerfile.voxtral-runtime \
  --build-arg PRELOAD_VOXTRAL_MODEL=true \
  --build-arg VOXTRAL_MODEL_ID=mistralai/Voxtral-Mini-4B-Realtime-2602 \
  -t cubicle-transcription-voxtral-runtime:preloaded \
  aws/transcription-service
```

Configure the self-hosted provider at runtime:

```bash
TRANSCRIPTION_ASR_PROVIDER=voxtral_self_hosted
TRANSCRIPTION_VOXTRAL_MODEL=mistralai/Voxtral-Mini-4B-Realtime-2602
TRANSCRIPTION_VOXTRAL_MODEL_VERSION=self-hosted-vllm-2602
TRANSCRIPTION_VOXTRAL_RUNTIME=vllm
TRANSCRIPTION_VOXTRAL_REALTIME_URL=ws://127.0.0.1:8000/v1/realtime
VLLM_BASE_URL=http://127.0.0.1:8000
VLLM_REALTIME_URL=ws://127.0.0.1:8000/v1/realtime
VLLM_MODEL=mistralai/Voxtral-Mini-4B-Realtime-2602
TRANSCRIPTION_REQUIRE_GPU=true
TRANSCRIPTION_MODELS_OFFLINE=true
```

The legacy hosted Voxtral Realtime dependencies remain available for rollback:

```bash
docker build \
  --build-arg INSTALL_VOXTRAL_REALTIME=true \
  -t cubicle-transcription-service:voxtral \
  aws/transcription-service
```

Configure the legacy hosted provider at runtime:

```bash
TRANSCRIPTION_ASR_PROVIDER=voxtral_realtime
TRANSCRIPTION_VOXTRAL_MODEL=voxtral-mini-transcribe-realtime-2602
TRANSCRIPTION_VOXTRAL_MODEL_VERSION=mistral-realtime-2602
TRANSCRIPTION_VOXTRAL_TARGET_DELAY_MS=480
MISTRAL_API_KEY=<from-secrets-manager-or-ssm>
MISTRAL_BASE_URL=wss://api.mistral.ai
```

Do not place `MISTRAL_API_KEY` in the container image, Cubicle Settings endpoint, or WebSocket query string. Store it in AWS Secrets Manager or SSM Parameter Store and inject it as an environment secret into the service task.

To include fallback faster-whisper dependencies:

```bash
docker build \
  --build-arg INSTALL_REAL_ASR=true \
  -t cubicle-transcription-service:asr \
  aws/transcription-service
```

Configure the fallback provider at runtime:

```bash
TRANSCRIPTION_ASR_PROVIDER=faster_whisper
TRANSCRIPTION_WHISPER_MODEL=large-v3-turbo
TRANSCRIPTION_WHISPER_MODEL_VERSION=faster-whisper
TRANSCRIPTION_WHISPER_DEVICE=cuda
TRANSCRIPTION_WHISPER_COMPUTE_TYPE=float16
TRANSCRIPTION_REQUIRE_GPU=true
TRANSCRIPTION_PARTIAL_MIN_AUDIO_MS=1000
```

For CPU-only fallback functional testing, use `TRANSCRIPTION_WHISPER_DEVICE=cpu`, `TRANSCRIPTION_WHISPER_COMPUTE_TYPE=int8`, and `TRANSCRIPTION_REQUIRE_GPU=false`. Do not use the CPU path for production latency targets.

To include pyannote diarization dependencies:

```bash
docker build \
  --build-arg INSTALL_DIARIZATION=true \
  -t cubicle-transcription-service:diarization \
  aws/transcription-service
```

Configure the pyannote provider at runtime:

```bash
TRANSCRIPTION_DIARIZATION_PROVIDER=pyannote
TRANSCRIPTION_PYANNOTE_MODEL=pyannote/speaker-diarization-community-1
TRANSCRIPTION_PYANNOTE_MODEL_VERSION=pyannote-audio-4.x
TRANSCRIPTION_PYANNOTE_DEVICE=cuda
TRANSCRIPTION_DIARIZATION_MIN_SPEAKERS=
TRANSCRIPTION_DIARIZATION_MAX_SPEAKERS=
PYANNOTE_AUTH_TOKEN=<from-secrets-manager-or-ssm>
PYANNOTE_METRICS_ENABLED=0
```

The pyannote model card requires accepting model conditions and creating a Hugging Face access token before production use. Do not place `PYANNOTE_AUTH_TOKEN`, `HF_TOKEN`, or `HUGGINGFACE_TOKEN` in image layers, Cubicle Settings, logs, or WebSocket query strings.

Push to ECR:

```bash
AWS_REGION=us-west-2
AWS_ACCOUNT_ID=<account-id>
aws ecr create-repository --repository-name cubicle-transcription-service --region "$AWS_REGION"
aws ecr get-login-password --region "$AWS_REGION" | docker login --username AWS --password-stdin "$AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com"
docker buildx build \
  --platform linux/amd64 \
  -t "$AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com/cubicle-transcription-service:<tag>" \
  --push \
  aws/transcription-service
```

This checkout is not a git repo in the runtime copy, so use an explicit image tag when building from `/Volumes/Webex/getwebexspace-data/GetWebexSpaceMac`.

## Provision Infrastructure

Slice 5 adds a Terraform staging stack under `infra/transcription`. It deploys the secure ingress and service path into account `562304353751` using AWS profile `strln` by default:

```bash
AWS_PROFILE_NAME=strln EXPECTED_ACCOUNT_ID=562304353751 ./infra/transcription/deploy.sh
```

The helper refuses to run when shell-level AWS credential variables such as `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, or `AWS_SESSION_TOKEN` are set. This prevents temporary credentials for another account from shadowing the intended `strln` profile. Clear those variables and verify the account before deployment:

```bash
unset AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_SESSION_TOKEN AWS_SECURITY_TOKEN
unset AWS_WEB_IDENTITY_TOKEN_FILE AWS_ROLE_ARN
unset AWS_CONTAINER_CREDENTIALS_FULL_URI AWS_CONTAINER_CREDENTIALS_RELATIVE_URI
AWS_PROFILE=strln aws sts get-caller-identity
```

The returned account must be `562304353751`.

The helper enforces the expected account, initializes Terraform, creates ECR and Secrets Manager resources, builds and pushes a `linux/amd64` image for ECS Fargate or ECS EC2, stores the legacy service token and user-token signing key in Secrets Manager, and applies the stack. Use `DOCKER_PLATFORM=linux/amd64` when building from Apple Silicon; this is the script default.

Current staging outputs:

```text
WebSocket endpoint: wss://dcabsri6ekziv.cloudfront.net/v1/transcription
Health endpoint: https://dcabsri6ekziv.cloudfront.net/healthz
ECR image: 562304353751.dkr.ecr.us-west-2.amazonaws.com/cubicle-transcription-service:20260517114915
```

The current Fargate stack includes:

1. Dedicated VPC and public subnets for the ALB and Fargate service.
2. ECR repository.
3. ECS Fargate cluster and service for the secure mock/Voxtral-managed API staging service.
4. Task definition running the container on port `8080` with `X86_64` Linux runtime platform.
5. ALB listener plus CloudFront TLS distribution.
6. Target group with health check path `/healthz`.
7. WebSocket route from `/v1/transcription` to the service target group.
8. Secrets Manager entries for `TRANSCRIPTION_SERVICE_TOKEN` and `TRANSCRIPTION_TOKEN_SIGNING_SECRET`.
9. CloudWatch log group and alarms for 5xx errors, target health, CPU/GPU utilization, and memory.
10. Account guard preventing accidental deployment outside account `562304353751`.

The self-hosted GPU path added in Terraform includes:

1. Optional `cubicle-transcription-voxtral-runtime` ECR repository.
2. Optional ECS EC2 GPU Auto Scaling group using the ECS GPU-optimized AMI.
3. Optional ECS capacity provider attached to the transcription cluster.
4. Optional vLLM sidecar container in the task definition with a one-GPU reservation.
5. Service environment wiring for `voxtral_self_hosted`, local Realtime URL, Whisper fallback model, model cache path, and offline model flags.

To publish the Voxtral model-weight image into private ECR without changing the
live service or starting GPU capacity, run:

```bash
umask 077
security find-generic-password -a cubicle-transcription -s cubicle-hf-token -w > /tmp/cubicle-hf-token
HF_TOKEN_FILE=/tmp/cubicle-hf-token \
EXTRA_CA_CERT_FILE=/tmp/cubicle-transcription-macos-ca.pem \
AWS_PROFILE_NAME=strln \
EXPECTED_ACCOUNT_ID=562304353751 \
./infra/transcription/publish-model-images.sh
rm -f /tmp/cubicle-hf-token
```

For the preferred AWS-side download path, store the Hugging Face token in
Secrets Manager and run the CodeBuild publisher:

```bash
SECRET_NAME="cubicle-transcription/huggingface-token"
aws secretsmanager create-secret \
  --profile strln \
  --region us-west-2 \
  --name "$SECRET_NAME" \
  --secret-string file:///tmp/cubicle-hf-token

IMAGE_TAG=models-$(date +%Y%m%d%H%M) \
HF_SECRET_NAME="$SECRET_NAME" \
AWS_PROFILE_NAME=strln \
EXPECTED_ACCOUNT_ID=562304353751 \
./infra/transcription/start-model-publish-codebuild.sh
```

The CodeBuild publisher uses a no-source CPU build project with privileged
Docker, a constrained IAM service role, seven-day build log retention, and
Secrets Manager token retrieval inside AWS. It downloads the Voxtral weights in
AWS, pushes `cubicle-transcription-voxtral-runtime:<tag>` to private ECR, and
does not change the ECS service, task definition, Auto Scaling group, or GPU
desired capacity.

The helper prefers `HF_TOKEN_FILE`/`HUGGINGFACE_TOKEN_FILE` and passes the token
as `--secret id=hf_token,src=<file>`, so the token is not placed in Docker
process arguments or environment. If only `HF_TOKEN`/`HUGGINGFACE_TOKEN` is
provided, the helper copies it into a temporary `0600` file, unsets the
environment variable before invoking Docker, removes the file on exit, and logs
out of private ECR by default. If the build environment uses a corporate TLS
intercepting CA, set `EXTRA_CA_CERT_FILE` to a PEM bundle and it will be passed
as a BuildKit secret instead of being copied into source control.

This helper only creates/verifies the ECR repositories and, by default, pushes:

1. `cubicle-transcription-voxtral-runtime:<tag>` with Voxtral Realtime weights.

It does not update the ECS service, task definition, Auto Scaling group, or GPU
desired capacity. Use this as the last no-GPU step before approving a runtime
deployment.

The fallback/diarization service image is intentionally opt-in because pyannote
pulls Torch and Triton/CUDA dependencies. Set `PUSH_SERVICE_IMAGE=true` only
when publishing that heavier image, or split pyannote into a dedicated
diarization worker image.

Enable it with explicit variables or through the helper. For a secure direct
app-to-AWS deployment with per-user controls:

```bash
TRANSCRIPTION_AUTH_MODE=signed_user_token \
TRANSCRIPTION_ALLOWED_USERS=prabhat7@cisco.com \
SELF_HOSTED_MODELS=true \
PRELOAD_MODEL_WEIGHTS=true \
HF_TOKEN_FILE=/tmp/cubicle-hf-token \
AWS_PROFILE_NAME=strln \
EXPECTED_ACCOUNT_ID=562304353751 \
./infra/transcription/deploy.sh
```

This will build and push both the service image and Voxtral runtime image, switch ECS to EC2 GPU launch type, create GPU capacity, set `ASR_PROVIDER=voxtral_self_hosted`, and point the service container at the sidecar's `ws://127.0.0.1:8000/v1/realtime` endpoint. The default GPU desired capacity for this mode is one instance; set `GPU_DESIRED_CAPACITY=0` to stage infrastructure without running GPU compute.

Production follow-up workers still need:

1. Private ECS EC2 capacity provider or EKS GPU node group.
2. NVIDIA driver and container runtime setup.
3. Private worker subnets and scaling policy based on active sessions and GPU utilization.
4. SQS finalization/correction workers if async final correction is enabled.
5. DynamoDB session/segment metadata tables if local persistence is enabled.

Before using GPU instances, verify availability in the target region:

```bash
aws ec2 describe-instance-type-offerings \
  --region us-west-2 \
  --location-type availability-zone \
  --filters Name=instance-type,Values=g5.xlarge,g5.2xlarge,g6.xlarge
```

The primary self-hosted Voxtral Realtime provider runs on NVIDIA GPU capacity through vLLM. ECS EC2 hosts or GPU-enabled EKS nodes need NVIDIA drivers and NVIDIA container runtime. The ECS GPU-optimized AMI path in Terraform handles this for ECS EC2. Set `TRANSCRIPTION_REQUIRE_GPU=true` for GPU providers; `/healthz` reports `status: degraded` when a required CUDA GPU is not visible. Pyannote readiness is reported under `diarization` and top-level `diarization_provider`.

## Deploy Or Update

For the current Terraform stack:

```bash
./infra/transcription/deploy.sh
terraform -chdir=infra/transcription output -raw websocket_endpoint
terraform -chdir=infra/transcription output -raw health_endpoint
```

To enable the self-hosted Voxtral runtime during deployment:

```bash
TRANSCRIPTION_AUTH_MODE=signed_user_token \
TRANSCRIPTION_ALLOWED_USERS=prabhat7@cisco.com \
SELF_HOSTED_MODELS=true \
PRELOAD_MODEL_WEIGHTS=true \
HF_TOKEN_FILE=/tmp/cubicle-hf-token \
./infra/transcription/deploy.sh
```

To stage the self-hosted repositories and task definitions without starting GPU compute, add:

```bash
GPU_DESIRED_CAPACITY=0 SELF_HOSTED_MODELS=true ./infra/transcription/deploy.sh
```

To enable pyannote diarization through runtime Secrets Manager injection instead of image preloading, supply `PYANNOTE_AUTH_TOKEN`; the helper stores it in Secrets Manager and sets `TRANSCRIPTION_DIARIZATION_PROVIDER=pyannote`:

```bash
PYANNOTE_AUTH_TOKEN=... SELF_HOSTED_MODELS=true ./infra/transcription/deploy.sh
```

If neither self-hosted mode nor model token is supplied, the deployed service remains on mock ASR and mock diarization for secure contract and app integration testing. `MISTRAL_API_KEY` is only used by the legacy hosted provider when `SELF_HOSTED_MODELS` is not enabled. For production, keep `TRANSCRIPTION_AUTH_MODE=signed_user_token`; `signed_or_shared` is only for migration while existing clients are moved from the old shared token.

ECS outline:

```bash
aws ecs register-task-definition --cli-input-json file://task-definition.json
aws ecs update-service \
  --cluster cubicle-transcription \
  --service cubicle-transcription-service \
  --task-definition cubicle-transcription-service:<revision>
```

EKS outline:

```bash
kubectl set image deployment/cubicle-transcription-service \
  app="$AWS_ACCOUNT_ID.dkr.ecr.$AWS_REGION.amazonaws.com/cubicle-transcription-service:<tag>"
kubectl rollout status deployment/cubicle-transcription-service
```

Rollback:

```bash
aws ecs update-service --cluster cubicle-transcription --service cubicle-transcription-service --task-definition cubicle-transcription-service:<previous-revision>
```

or:

```bash
kubectl rollout undo deployment/cubicle-transcription-service
```

## Verify Connectivity

Health check:

```bash
curl -fsS https://transcription.example.com/healthz
```

Expected mock-provider health includes `asr_provider: mock` and `diarization_provider: mock`. Expected self-hosted Voxtral health includes `asr_provider: voxtral_self_hosted`, `model_name: mistralai/Voxtral-Mini-4B-Realtime-2602`, `runtime: vllm`, `external_api_dependency:false`, `realtime_url_configured:true`, `target_streaming_delay_ms`, and `dependencies_available`. Expected fallback health includes `asr_provider: faster_whisper`, `model_name`, `device`, `compute_type`, `gpu_available`, and `gpu_required`. Expected pyannote health includes `diarization_provider: pyannote`, `diarization.model_name`, `diarization.auth_token_configured`, `diarization.telemetry_enabled:false`, and `diarization.ready`.

WebSocket smoke with `websocat`:

```bash
websocat -H='Authorization: Bearer <signed-user-token>' wss://transcription.example.com/v1/transcription
```

Send a `start_session` JSON message:

```json
{
  "type": "start_session",
  "protocol_version": "transcription.v1",
  "session_id": "smoke-english-1",
  "transcription_enabled": true,
  "diarization_enabled": false,
  "language_mode": "english_to_english",
  "sample_rate": 16000,
  "channel_count": 1,
  "audio_encoding": "pcm_s16le",
  "client_timestamp": "2026-05-17T17:00:00.000Z"
}
```

Expected first events:

- `session_started`
- `auth_mode: signed_user_token`
- `authenticated_user_id` set to the signed token email or subject
- `diarization_status` with `disabled`

For Japanese-to-English smoke, set:

```json
"language_mode": "japanese_to_english"
```

Expected transcript events use `language_mode: japanese_to_english`. With `TRANSCRIPTION_ASR_PROVIDER=voxtral_self_hosted`, the service uses the local vLLM Voxtral Realtime stream and preserves Cubicle's mode metadata in every transcript event. With `TRANSCRIPTION_ASR_PROVIDER=faster_whisper`, the fallback path sends Whisper `task=translate` with `language=ja`; English mode sends `task=transcribe` with `language=en`. When diarization is enabled, transcript text events remain text-only and the service emits separate `speaker_update` events keyed by `segment_id`.

## Real ASR Smoke

For local GPU development, run the verified vLLM runtime container and the service container on the same Docker network:

```bash
docker network create cubicle-transcription || true

docker run --rm --gpus all --network cubicle-transcription --name voxtral-runtime \
  --ipc=host \
  -v "$HOME/.cache/huggingface:/root/.cache/huggingface" \
  -e HF_TOKEN="${HF_TOKEN:-}" \
  -e VLLM_DISABLE_COMPILE_CACHE=1 \
  --entrypoint /bin/bash \
  vllm/vllm-openai:v0.21.0-ubuntu2404 \
  -lc 'python3 -m pip install --no-cache-dir "mistral-common[soundfile]" soundfile && exec vllm serve mistralai/Voxtral-Mini-4B-Realtime-2602 --host 0.0.0.0 --port 8000 --tokenizer-mode mistral --max-model-len 45000 --gpu-memory-utilization 0.90 --compilation_config '\''{"cudagraph_mode":"PIECEWISE"}'\'''
```

In a second terminal:

```bash
docker run --rm --network cubicle-transcription -p 8080:8080 \
  -e TRANSCRIPTION_SERVICE_TOKEN=dev-token \
  -e TRANSCRIPTION_ASR_PROVIDER=voxtral_self_hosted \
  -e TRANSCRIPTION_VOXTRAL_MODEL=mistralai/Voxtral-Mini-4B-Realtime-2602 \
  -e TRANSCRIPTION_VOXTRAL_MODEL_VERSION=self-hosted-vllm-2602 \
  -e VLLM_BASE_URL=http://voxtral-runtime:8000 \
  -e VLLM_REALTIME_URL=ws://voxtral-runtime:8000/v1/realtime \
  -e VLLM_MODEL=mistralai/Voxtral-Mini-4B-Realtime-2602 \
  -e TRANSCRIPTION_VOXTRAL_TARGET_DELAY_MS=480 \
  -e TRANSCRIPTION_REQUIRE_GPU=true \
  cubicle-transcription-service:selfhost
```

For the fallback faster-whisper provider on a GPU worker after installing the ASR image:

```bash
docker run --rm --gpus all -p 8080:8080 \
  -e TRANSCRIPTION_SERVICE_TOKEN=dev-token \
  -e TRANSCRIPTION_ASR_PROVIDER=faster_whisper \
  -e TRANSCRIPTION_WHISPER_MODEL=h2oai/faster-whisper-large-v3-turbo \
  -e TRANSCRIPTION_WHISPER_DEVICE=cuda \
  -e TRANSCRIPTION_WHISPER_COMPUTE_TYPE=float16 \
  -e TRANSCRIPTION_REQUIRE_GPU=true \
  cubicle-transcription-service:asr
```

Verify the runtime check:

```bash
curl -fsS http://127.0.0.1:8080/healthz
```

Then run a WebSocket smoke with a short PCM 16 kHz mono fixture. The service should emit `session_started` with `asr_provider: voxtral_self_hosted` or `asr_provider: faster_whisper`, `model_warmed: true`, and the provider-specific runtime metadata, followed by partial/final transcript events. Do not log the fixture audio or transcript text in CloudWatch.

For pyannote diarization after installing the diarization image:

```bash
docker run --rm --gpus all -p 8080:8080 \
  -e TRANSCRIPTION_SERVICE_TOKEN=dev-token \
  -e TRANSCRIPTION_DIARIZATION_PROVIDER=pyannote \
  -e TRANSCRIPTION_PYANNOTE_MODEL=pyannote/speaker-diarization-community-1 \
  -e TRANSCRIPTION_PYANNOTE_DEVICE=cuda \
  -e PYANNOTE_AUTH_TOKEN=<secret> \
  -e PYANNOTE_METRICS_ENABLED=0 \
  cubicle-transcription-service:diarization
```

Start a WebSocket session with `"diarization_enabled": true`, send a short PCM fixture, then stop the session. Expected event order is `final_transcript`, one or more `speaker_update` events for finalized `segment_id` values, `diarization_status` with `completed`, and `session_stopped`. With `"diarization_enabled": false`, no `speaker_update` event should be emitted.

## Logs And Privacy

- Do not log raw audio.
- Do not log transcript text by default.
- Do not place auth tokens in query strings.
- Use `Authorization: Bearer <signed-user-token>` for production sessions.
- Keep TLS on for every non-local endpoint.
- Keep raw audio retention disabled unless a privacy review explicitly enables it.
- If retention is enabled later, store audio chunks in S3 with SSE-KMS, session-scoped object prefixes, and lifecycle deletion.

Inspect logs without exposing content:

```bash
aws logs tail /aws/ecs/cubicle-transcription-service --follow
```

Slice 5 service logs use metadata-only safe logging helpers. Voxtral, faster-whisper, pyannote, and alignment providers must preserve that boundary: default logs include session IDs, mode, provider, runtime status, and error classes, not raw audio, transcript content, word timestamps, or speaker embeddings.

Pyannote telemetry is disabled by default in the container with `PYANNOTE_METRICS_ENABLED=0`. If a deployment owner explicitly enables it, document that decision in the service privacy review and keep transcript/audio logging disabled.

## Point Cubicle To Staging Or Production

In Cubicle Settings:

1. Enable Live Transcription.
2. Set Speaker Diarization as needed.
3. Select `English -> English` or `Japanese -> English`.
4. Set AWS endpoint to the ALB WebSocket URL, for example:

```text
wss://transcription.example.com/v1/transcription
```

5. Paste the short-lived signed user token into the transcription service token field. Cubicle stores it in Keychain and sends it as the bearer header.

The app rejects query-string tokens in the endpoint. Use the Keychain-backed token field only.

## Remaining Production Work

- Add repo-approved Terraform or CDK.
- Add VAD/segmentation.
- Add final correction worker and queue.
- Add DynamoDB metadata persistence.
- Add optional encrypted S3 audio retention with default disabled.
- Add load/concurrency tests against GPU nodes.
