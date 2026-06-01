# Cubicle Transcription Service

This is the AWS service module for Cubicle live transcription. It is a separate deployable module from the macOS app target.

## What Works In This Slice

- WebSocket ingress shape at `/v1/transcription`.
- Bearer token authentication gate, including optional signed per-user tokens.
- Versioned `start_session` validation for `transcription.v1`.
- Binary WebSocket frames for PCM audio chunks.
- Mock ASR provider with English, Japanese-to-English, and auto-source transcription routes.
- Self-hosted Voxtral Realtime provider boundary that streams to a VPC-local vLLM Realtime runtime.
- Legacy hosted Voxtral Realtime provider boundary using Mistral's realtime SDK, retained for rollback/testing only.
- Optional faster-whisper/CTranslate2 fallback ASR provider behind the same service interface.
- Separate diarization provider boundary with a default mock provider and an optional pyannote.audio provider.
- WhisperX-style segment/speaker glue that assigns finalized transcript segment IDs to diarized speaker turns by timestamp overlap.
- Provider route handling:
  - `english_to_english` -> Voxtral Realtime live transcription.
  - `japanese_to_english` -> Voxtral Realtime route with Cubicle mode metadata preserved.
  - `multilingual_to_english` -> Voxtral Realtime auto/source transcription for downstream English translation.
  - `english_to_english` -> Whisper transcribe with `language=en`.
  - `japanese_to_english` -> Whisper translate with `language=ja`.
  - `multilingual_to_english` -> Whisper transcribe with language autodetection.
- Model warmup at session start.
- Provider/runtime health metadata in `/healthz` and `session_started`, including diarization readiness.
- Separate diarization-enabled and diarization-disabled event behavior.
- Partial and final transcript event contracts.
- `speaker_update` events that preserve segment identity and do not include transcript text.
- Backpressure guard for pending audio frames.
- `/healthz` endpoint.
- Default structured logging helpers that redact transcript text, audio, tokens, word timestamps, and speaker embeddings.
- Docker container scaffold.

## What Is Still Mocked

- VAD/segmentation.
- Final correction worker.
- Session metadata persistence.
- Raw audio retention. Retention is disabled by default.

Voxtral Realtime is used only for ASR/translation. Realtime is not combined with a hosted diarization parameter; diarization is handled by the separate pyannote/whisperX-style pipeline.

The mock provider remains the default for local development. Select the AWS self-hosted Voxtral provider with:

```bash
TRANSCRIPTION_ASR_PROVIDER=voxtral_self_hosted
TRANSCRIPTION_VOXTRAL_MODEL=mistralai/Voxtral-Mini-4B-Realtime-2602
TRANSCRIPTION_VOXTRAL_MODEL_VERSION=self-hosted-vllm-2602
TRANSCRIPTION_VOXTRAL_RUNTIME=vllm
TRANSCRIPTION_VOXTRAL_REALTIME_URL=ws://127.0.0.1:8000/v1/realtime
TRANSCRIPTION_REQUIRE_GPU=true
```

This mode does not use `MISTRAL_API_KEY`. The service container sends Cubicle audio to an internal vLLM Realtime runtime running in the same ECS task or VPC. The vLLM runtime hosts the Voxtral weights in your AWS account.

For the currently verified AWS GPU instance, keep the SSM port forward open on
the same Mac that runs the service and use the local forwarded vLLM aliases:

```bash
aws ssm start-session \
  --profile strln \
  --region us-west-2 \
  --target i-02b84c39f9912a77a \
  --document-name AWS-StartPortForwardingSession \
  --parameters '{"portNumber":["8000"],"localPortNumber":["8000"]}'
```

```bash
python3 -m pip install -r services/transcription/requirements.txt
set -a
. services/transcription/env.vllm-forwarded.example
set +a
TRANSCRIPTION_SERVICE_TOKEN_FILE=/path/to/0600-token-file \
PYTHONPATH=services/transcription python3 -m transcription_service.main
```

The vLLM endpoint values are:

```bash
VLLM_BASE_URL=http://localhost:8000
VLLM_REALTIME_URL=ws://localhost:8000/v1/realtime
VLLM_MODEL=mistralai/Voxtral-Mini-4B-Realtime-2602
```

Cubicle should still point to this service's Cubicle protocol endpoint, for
example `ws://127.0.0.1:8080/v1/transcription`, not directly to vLLM's
`/v1/realtime` route. The service adapts Cubicle session JSON/audio frames to
the OpenAI-compatible vLLM Realtime protocol and keeps diarization/session
events separate.

The legacy hosted Mistral path is still available with:

```bash
TRANSCRIPTION_ASR_PROVIDER=voxtral_realtime
TRANSCRIPTION_VOXTRAL_MODEL=voxtral-mini-transcribe-realtime-2602
TRANSCRIPTION_VOXTRAL_TARGET_DELAY_MS=480
MISTRAL_API_KEY=<short-lived-or-secret-managed-key>
```

Install Voxtral dependencies only in the service runtime image:

```bash
pip install -r requirements-voxtral.txt
```

Use faster-whisper as the local/GPU fallback:

```bash
TRANSCRIPTION_ASR_PROVIDER=faster_whisper
TRANSCRIPTION_WHISPER_MODEL=large-v3-turbo
TRANSCRIPTION_WHISPER_DEVICE=cuda
TRANSCRIPTION_WHISPER_COMPUTE_TYPE=float16
TRANSCRIPTION_REQUIRE_GPU=true
```

Use `TRANSCRIPTION_WHISPER_DEVICE=cpu` and `TRANSCRIPTION_REQUIRE_GPU=false` only for local functional testing. Production fallback GPU workers should require a visible CUDA GPU.

## Local Development

Run unit tests without installing FastAPI:

```bash
PYTHONPATH=services/transcription python3 -m unittest discover services/transcription/tests -v
```

Run the service:

```bash
cd services/transcription
python3 -m venv .venv
. .venv/bin/activate
pip install -r requirements.txt
TRANSCRIPTION_SERVICE_TOKEN=dev-token python -m transcription_service.main
```

Install the optional Voxtral or fallback dependencies only when running those providers:

```bash
pip install -r requirements-voxtral.txt
pip install -r requirements-asr.txt
pip install -r requirements-diarization.txt
```

Enable the pyannote diarization provider only in a runtime that has accepted the model conditions and has a token supplied through secure secret injection:

```bash
TRANSCRIPTION_DIARIZATION_PROVIDER=pyannote
TRANSCRIPTION_PYANNOTE_MODEL=pyannote/speaker-diarization-community-1
TRANSCRIPTION_PYANNOTE_MODEL_VERSION=pyannote-audio-4.x
TRANSCRIPTION_PYANNOTE_DEVICE=cuda
PYANNOTE_AUTH_TOKEN=<from-secrets-manager-or-ssm>
PYANNOTE_METRICS_ENABLED=0
```

`PYANNOTE_METRICS_ENABLED=0` is the container default so local/provider telemetry stays off unless the deployment owner explicitly changes it. If the legacy pyannote.audio 3.x pipeline is required, set `TRANSCRIPTION_PYANNOTE_MODEL=pyannote/speaker-diarization-3.1` and use a compatible dependency image.

For the production AWS path, do not run pyannote inside the public WebSocket adapter. Run a private diarization worker and point the adapter at it:

```bash
# Public app-facing adapter
TRANSCRIPTION_DIARIZATION_PROVIDER=remote_http
TRANSCRIPTION_DIARIZATION_WORKER_URL=http://<private-worker-host>:8080
TRANSCRIPTION_DIARIZATION_WORKER_AUTH_TOKEN=<shared-worker-call-token>

# Private worker task
TRANSCRIPTION_SERVICE_ROLE=diarization_worker
TRANSCRIPTION_WORKER_DIARIZATION_PROVIDER=pyannote
TRANSCRIPTION_PYANNOTE_MODEL=pyannote/speaker-diarization-community-1
TRANSCRIPTION_PYANNOTE_DEVICE=cuda
PYANNOTE_AUTH_TOKEN=<from-secrets-manager>
TRANSCRIPTION_DIARIZATION_WORKER_AUTH_TOKEN=<same-shared-worker-call-token>
PYANNOTE_METRICS_ENABLED=0
```

The adapter sends PCM audio and session metadata to `/v1/diarization` and receives only speaker turns. The worker writes a temporary WAV for pyannote and deletes it when the request completes; raw audio retention remains disabled. If the worker is unavailable or times out, transcription still completes and the adapter emits `diarization_status=failed` rather than failing the WebSocket session.

For long sessions, run that private worker on its own ECS EC2 GPU capacity rather than inside the app-facing adapter task:

```bash
ENABLE_DIARIZATION_WORKER=true
DIARIZATION_WORKER_DESIRED_COUNT=1
DIARIZATION_WORKER_LAUNCH_TYPE=EC2
ENABLE_DIARIZATION_WORKER_GPU_CAPACITY=true
DIARIZATION_WORKER_GPU_INSTANCE_TYPE=g5.xlarge
DIARIZATION_WORKER_GPU_DESIRED_CAPACITY=1
DIARIZATION_WORKER_PYANNOTE_DEVICE=cuda
DIARIZATION_STOP_TIMEOUT_SECONDS=180
```

The app-facing adapter remains on its existing ASR path and only sends stop-time diarization jobs to the private worker. Terraform keeps this worker on a separate capacity provider so pyannote cannot consume the Voxtral runtime GPU.

Point Cubicle Settings to:

```text
ws://127.0.0.1:8080/v1/transcription
```

For authenticated local testing, the client must send:

```text
Authorization: Bearer dev-token
```

## Direct AWS Authentication

The production direct path is:

```text
Cubicle.app
  -> wss://<cloudfront-host>/v1/transcription
  -> CloudFront TLS
  -> ALB restricted to CloudFront origin-facing ranges
  -> ECS transcription service
  -> private vLLM Voxtral runtime on 127.0.0.1:8000 inside the same task or VPC
```

Cubicle never connects directly to vLLM. It connects to this service because
the service enforces Cubicle's protocol, auth, diarization events, safe logs,
and future persistence/correction boundaries.

The legacy `TRANSCRIPTION_SERVICE_TOKEN` shared-token mode still works, but
per-user control should use signed user tokens:

```bash
TRANSCRIPTION_AUTH_MODE=signed_user_token
TRANSCRIPTION_ALLOWED_USERS=
TRANSCRIPTION_TOKEN_ISSUER=cubicle-transcription
TRANSCRIPTION_TOKEN_AUDIENCE=cubicle-macos
TRANSCRIPTION_REQUIRED_SCOPE=transcription:stream
```

Leave `TRANSCRIPTION_ALLOWED_USERS` empty to allow any correctly signed,
unexpired user token with the expected issuer, audience, and scope. Set
`enforce_service_user_registry=true` in Terraform only when the transcription
service should also require an active DynamoDB user row and non-revoked
token-ledger entry.

The HMAC signing secret is injected through
`TRANSCRIPTION_TOKEN_SIGNING_SECRET` or `TRANSCRIPTION_TOKEN_SIGNING_SECRET_FILE`
and should come from AWS Secrets Manager. Do not put the signing secret in
Cubicle Settings. Cubicle only stores the short-lived signed user token in
Keychain and sends it as a bearer header.

Mint a short-lived token:

```bash
umask 077
aws secretsmanager get-secret-value \
  --profile strln \
  --region us-west-2 \
  --secret-id cubicle-transcription/user-token-signing-key \
  --query SecretString \
  --output text > /tmp/cubicle-user-token-signing-key

services/transcription/scripts/mint-user-token.py \
  --secret-file /tmp/cubicle-user-token-signing-key \
  --subject prabhat7@cisco.com \
  --email prabhat7@cisco.com \
  --ttl-seconds 3600

rm -f /tmp/cubicle-user-token-signing-key
```

To invalidate a specific issued token before its expiry, add its `jti` to
`TRANSCRIPTION_REVOKED_TOKEN_IDS` and redeploy. If
`enforce_service_user_registry=true`, removing or disabling a user in the
registry also blocks new sessions.

For the private admin console path, the service can also enforce a dynamic
DynamoDB user registry after the tables and task IAM permissions exist:

```bash
TRANSCRIPTION_USER_REGISTRY_BACKEND=dynamodb
TRANSCRIPTION_USER_REGISTRY_TABLE=cubicle-transcription-users
TRANSCRIPTION_TOKEN_LEDGER_TABLE=cubicle-transcription-token-ledger
TRANSCRIPTION_USER_REGISTRY_REQUIRE_TOKEN_LEDGER=true
TRANSCRIPTION_USER_REGISTRY_CACHE_TTL_SECONDS=30
```

The registry lookup happens after JWT signature, issuer, audience, expiry,
scope, and env revocation checks. It rejects missing users, disabled users,
unissued token ids when the ledger is required, revoked token-ledger entries,
and DynamoDB lookup failures. This lets the private admin console add, disable,
issue, and revoke users without redeploying the transcription adapter. The
current deployed stack can continue using the env allow list until the private
admin console infrastructure is provisioned.

## Admin Console

The service now includes a disabled-by-default admin console/API scaffold for
token-based user administration. Do not enable it on the public app-facing
CloudFront transcription service. Run it as a separate ECS service behind the
protected admin ALB for `cubicle.agenticisolation.com`.

Enable only in that control-plane service:

```bash
TRANSCRIPTION_ADMIN_ENABLED=true
TRANSCRIPTION_ADMIN_STORE_BACKEND=dynamodb
TRANSCRIPTION_ADMIN_EXTERNAL_AUTH_PROVIDER=cognito_alb
TRANSCRIPTION_ADMIN_REQUIRED_GROUP=CubicleTranscriptionAdmins
TRANSCRIPTION_ADMIN_SESSION_SECRET_FILE=/run/secrets/admin-session-secret
TRANSCRIPTION_TOKEN_SIGNING_SECRET_FILE=/run/secrets/user-token-signing-key
TRANSCRIPTION_USER_REGISTRY_TABLE=cubicle-transcription-users
TRANSCRIPTION_TOKEN_LEDGER_TABLE=cubicle-transcription-token-ledger
TRANSCRIPTION_ADMIN_AUDIT_TABLE=cubicle-transcription-admin-audit
TRANSCRIPTION_ADMIN_COOKIE_SECURE=true
```

Admin routes live under `/admin`. The browser console uses an HttpOnly,
Secure, SameSite admin session cookie and CSRF-protected mutation forms.
Public deployments use Cognito username/password/MFA at the ALB and the admin
app validates the `CubicleTranscriptionAdmins` group from ALB/Cognito identity
headers. There is no second credential prompt after Cognito. Token issuance returns the
signed user token once and writes only token metadata to the ledger. The
plaintext token is not stored. Usage lookup is metadata-only and does not store
audio or transcript text.

See `docs/transcription-admin-console.md` for the
`cubicle.agenticisolation.com` hosting plan and GoDaddy DNS guidance.

## Docker

```bash
docker build -t cubicle-transcription-service:local services/transcription
docker run --rm -p 8080:8080 -e TRANSCRIPTION_SERVICE_TOKEN=dev-token cubicle-transcription-service:local
```

Build an image that includes the legacy hosted Voxtral Realtime dependencies:

```bash
docker build \
  --build-arg INSTALL_VOXTRAL_REALTIME=true \
  -t cubicle-transcription-service:voxtral \
  services/transcription
```

Build the service image with self-hosted Whisper/pyannote dependencies:

```bash
docker build \
  --build-arg INSTALL_SELF_HOSTED_MODELS=true \
  -t cubicle-transcription-service:selfhost \
  services/transcription
```

Build the self-hosted Voxtral vLLM runtime image:

```bash
docker build \
  -f services/transcription/Dockerfile.voxtral-runtime \
  -t cubicle-transcription-voxtral-runtime:local \
  services/transcription
```

Optional model preloading uses file-backed BuildKit secrets so Hugging Face
tokens are not stored in image layers or exposed through Docker process
environment inspection:

```bash
umask 077
security find-generic-password -a cubicle-transcription -s cubicle-hf-token -w > /tmp/cubicle-hf-token
docker build \
  --secret id=hf_token,src=/tmp/cubicle-hf-token \
  --build-arg INSTALL_SELF_HOSTED_MODELS=true \
  --build-arg PRELOAD_SERVICE_MODELS=true \
  -t cubicle-transcription-service:selfhost-preloaded \
  services/transcription
rm -f /tmp/cubicle-hf-token

docker build \
  -f services/transcription/Dockerfile.voxtral-runtime \
  --build-arg PRELOAD_VOXTRAL_MODEL=true \
  -t cubicle-transcription-voxtral-runtime:preloaded \
  services/transcription
```

Build an image that includes fallback faster-whisper dependencies:

```bash
docker build \
  --build-arg INSTALL_REAL_ASR=true \
  -t cubicle-transcription-service:asr \
  services/transcription
```

Build an image that includes pyannote diarization dependencies:

```bash
docker build \
  --build-arg INSTALL_DIARIZATION=true \
  -t cubicle-transcription-service:diarization \
  services/transcription
```

Build the dedicated private diarization worker image:

```bash
docker build \
  -f services/transcription/Dockerfile.diarization-worker \
  -t cubicle-transcription-diarization-worker:pyannote \
  services/transcription
```

The self-hosted Voxtral runtime does not contain a Mistral API key. The pyannote image does not contain a Hugging Face token unless you explicitly preload gated weights with a BuildKit secret. For production, preload or cache Voxtral, Whisper, and pyannote weights into ECR image layers or an encrypted AWS-owned model volume, then run with `TRANSCRIPTION_MODELS_OFFLINE=true`. The faster-whisper and pyannote images may download configured model weights during runtime warmup unless the deployment pre-populates the model cache.
