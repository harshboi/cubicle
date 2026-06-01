# Cubicle Direct AWS Transcription Auth

This is the secure direct app-to-AWS path for Cubicle live transcription.

## Target Topology

```text
Cubicle.app
  wss://<cloudfront-host>/v1/transcription
    Authorization: Bearer <short-lived signed user token>
  CloudFront TLS distribution
  ALB restricted to AWS CloudFront origin-facing prefix list
  ECS transcription service on port 8080
  private self-hosted Voxtral/vLLM runtime on VPC port 8000
```

Cubicle should point to the Cubicle transcription service endpoint, not to
vLLM's `/v1/realtime` endpoint. The service adapts Cubicle's session protocol
and PCM audio frames to the vLLM Realtime API, emits `partial_transcript`,
`final_transcript`, `speaker_update`, `diarization_status`, and error events,
and keeps transcript/audio content out of default logs.

## Why This Replaces The Mac SSM Tunnel

The development path used an SSM port forward because vLLM was running on an
isolated EC2 GPU instance and port `8000` is intentionally not public. The
direct AWS path moves the transcription adapter into ECS so it can reach the
private vLLM runtime over VPC networking. The app then connects directly to
CloudFront over WSS and no laptop tunnel, local adapter, or tmux session is
needed.

Do not open EC2 port `8000` to the internet. vLLM has no Cubicle user policy,
diarization policy, transcript aggregation, or privacy logging boundary.

## Per-User Access Control

The service supports three auth modes:

- `shared_token`: legacy single service token.
- `signed_user_token`: only signed user tokens are accepted.
- `signed_or_shared`: migration mode that accepts either the shared token or a
  signed user token.

Use `signed_user_token` for production. The token is a short-lived HS256
JWT-like bearer token signed by a secret stored in AWS Secrets Manager. The
service validates:

- Signature.
- Expiration and not-before time.
- Issuer.
- Audience.
- Required scope.
- Optional token id revocation list.
- Optional user/email allow list.

The default issuer is `cubicle-transcription`, the default audience is
`cubicle-macos`, and the required scope is `transcription:stream`.

## Deploy With Signed User Tokens

Deploy or update the stack with explicit user allow-listing. For the current
verified EC2 vLLM runtime, use the direct adapter wrapper:

```bash
TRANSCRIPTION_ALLOWED_USERS=prabhat7@cisco.com \
AWS_PROFILE_NAME=strln \
EXPECTED_ACCOUNT_ID=562304353751 \
./infra/transcription/deploy-direct-aws-adapter.sh
```

The wrapper discovers the private IP for `i-02b84c39f9912a77a`, deploys the
lightweight Fargate adapter with `TRANSCRIPTION_ASR_PROVIDER=voxtral_self_hosted`,
sets `TRANSCRIPTION_VOXTRAL_REALTIME_URL` to the private `ws://<private-ip>:8000/v1/realtime`
endpoint, and leaves the GPU vLLM container on EC2. It does not open port
`8000` publicly and does not require SSM on client laptops. It also preserves
the existing GPU capacity resources that own the verified EC2 runtime while
keeping the app-facing adapter on Fargate.

For a later managed ECS EC2 sidecar rollout, use `SELF_HOSTED_MODELS=true`
with `./infra/transcription/deploy.sh` after validating the sidecar path.

The deploy helper creates or updates these Secrets Manager entries:

- `cubicle-transcription/service-token`
- `cubicle-transcription/user-token-signing-key`

The signing key is generated once and preserved unless
`TRANSCRIPTION_TOKEN_SIGNING_SECRET` is explicitly supplied. The ECS task
receives it as an injected secret named `TRANSCRIPTION_TOKEN_SIGNING_SECRET`.
It is not stored in the image and should never be stored in Cubicle Settings.

For the production app-facing path, leave `TRANSCRIPTION_ALLOWED_USERS` empty.
The service then accepts any correctly signed, unexpired token with the
expected issuer, audience, and scope. The admin console can still keep a
DynamoDB user/token ledger for issuing and auditing tokens. Set Terraform
`enforce_service_user_registry=true` only when the WebSocket service must also
require a matching active registry user and non-revoked ledger entry.

## Mint A User Token

Mint tokens from the signing secret. Keep the temporary secret file local and
delete it immediately:

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

Paste only the generated user token into Cubicle Settings. The app stores it in
Keychain and sends:

```text
Authorization: Bearer <signed-user-token>
```

## Add, Remove, Or Revoke Users

When `enforce_service_user_registry=true`, add users in the admin registry and
issue their tokens through the registry so the token ledger records the `jti`.

For static allow-list deployments, add users with:

```bash
TRANSCRIPTION_ALLOWED_USERS=prabhat7@cisco.com,new-user@example.com
```

and redeploy. To remove users from a non-registry deployment, remove them from
that comma-separated list and redeploy. This blocks new sessions for removed
users.

To revoke a specific token immediately, mint tokens with a `jti` or use the
default UUID printed into the token payload, then redeploy with:

```bash
TRANSCRIPTION_REVOKED_TOKEN_IDS=<token-jti-1>,<token-jti-2>
```

Keep token TTLs short. A one-hour TTL is a reasonable operator-issued token for
testing. For broader team use, integrate an internal identity provider or
Cognito/OIDC token broker that mints these short-lived service tokens after
corporate login.

## Dynamic User Registry Backend

The service now has an optional dynamic registry path for the private admin
console design. The current deployed stack can keep using
`TRANSCRIPTION_ALLOWED_USERS` while the admin console is built. When DynamoDB
tables and IAM permissions are provisioned, enable registry checks with:

```bash
TRANSCRIPTION_USER_REGISTRY_BACKEND=dynamodb
TRANSCRIPTION_USER_REGISTRY_TABLE=cubicle-transcription-users
TRANSCRIPTION_TOKEN_LEDGER_TABLE=cubicle-transcription-token-ledger
TRANSCRIPTION_USER_REGISTRY_REQUIRE_TOKEN_LEDGER=true
TRANSCRIPTION_USER_REGISTRY_CACHE_TTL_SECONDS=30
```

The user table contract is:

```text
pk = USER#<email-or-user-id>
status = active | disabled | revoked | inactive
display_name = optional display label
role = transcription_user | admin
```

The token ledger contract is:

```text
pk = USER#<email-or-user-id>
sk = TOKEN#<jti>
status = active | revoked | disabled | inactive
issued_at = metadata only
expires_at = metadata only
revoked_at = optional revocation timestamp
```

The service still validates JWT signature, issuer, audience, scope, expiry,
not-before time, issued-at skew, and the env `TRANSCRIPTION_REVOKED_TOKEN_IDS`
list before consulting the dynamic registry. The registry then fails closed
when:

- The user is missing from the registry.
- The user exists but is not in an active status.
- Token ledger is required and the token has no `jti`.
- Token ledger is required and the `jti` is missing from the ledger.
- The token ledger entry is revoked, disabled, inactive, or has `revoked_at`.
- DynamoDB lookup fails.

The registry uses a short in-process cache controlled by
`TRANSCRIPTION_USER_REGISTRY_CACHE_TTL_SECONDS`. Use a low TTL such as 30
seconds so disables and revocations propagate quickly while avoiding a DynamoDB
read on every reconnect. `/healthz` includes a metadata-only `user_registry`
object with backend/configuration status and no table contents or token values.

This slice adds the service-side registry and tests. It does not create the
DynamoDB tables, internal admin console, private ALB, Client VPN, Route53
private record, or ECS task IAM policy; those are separate private-admin-console
infrastructure slices.

## Security Controls

- Cubicle endpoints must use `wss://` outside local development.
- Tokens are sent in the `Authorization` header, not query strings.
- Cubicle rejects endpoint URLs containing query parameters.
- The service logs auth mode and user id/email metadata, not token values.
- The service redacts transcript text, raw audio, word timestamps, and speaker
  embeddings from default logs.
- ALB ingress is restricted to CloudFront origin-facing addresses.
- ECS task ingress is restricted to the ALB security group.
- vLLM port `8000` remains private.
- Model provider tokens and signing secrets live in Secrets Manager.
- Raw audio retention is disabled by default.
- If retention is enabled later, use SSE-KMS, lifecycle deletion, and explicit
  privacy review.

## Operational Checks

Check service health:

```bash
curl -fsS https://<cloudfront-host>/healthz
```

Check a signed token connection with `websocat`:

```bash
websocat \
  -H="Authorization: Bearer <signed-user-token>" \
  wss://<cloudfront-host>/v1/transcription
```

A valid start message should receive `session_started` with:

- `auth_mode: signed_user_token`
- `authenticated_user_id: <email-or-subject>`
- `asr_provider: voxtral_self_hosted`
- `language_mode` matching the client mode

Unauthorized sessions should fail the WebSocket handshake with HTTP 1008 policy
violation and no audio should be accepted.
