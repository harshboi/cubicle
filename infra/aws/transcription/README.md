# Cubicle Transcription AWS Stack

This Terraform stack deploys the Cubicle transcription service into AWS account `562304353751` with the `strln` profile.

The stack creates:

- ECR repository with scan-on-push for the service image.
- ECR repository with scan-on-push and immutable tags for the self-hosted Voxtral vLLM runtime image.
- Secrets Manager secret for the Cubicle service bearer token.
- Secrets Manager secret for the per-user token signing key.
- Optional Secrets Manager secrets for `MISTRAL_API_KEY` and `PYANNOTE_AUTH_TOKEN`.
- Dedicated VPC, public subnets, Internet Gateway, and security groups.
- ECS Fargate service behind an Application Load Balancer for mock/API-only staging.
- Optional ECS EC2 GPU capacity provider and Voxtral runtime sidecar for self-hosted inference.
- CloudFront distribution that provides a trusted `wss://...cloudfront.net/v1/transcription` endpoint without requiring DNS changes.
- Optional admin console infrastructure for token-based user administration. It is disabled by default. The recommended public mode uses a dedicated HTTPS ALB protected by Cognito MFA and AWS WAF, with the admin ECS task still running privately. A stricter private/VPN mode is also parameterized for later use.
- CloudWatch log group with 14-day retention.

The service still keeps raw audio retention disabled by default and logs only metadata.

## Deploy

Use the helper so the right AWS profile/account is enforced:

```bash
./infra/transcription/deploy.sh
```

The helper refuses to run if shell-level AWS credential variables such as
`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, or `AWS_SESSION_TOKEN` are set.
This prevents temporary credentials for another account from shadowing the
`strln` profile. Before deploying, verify the intended account with:

```bash
env -u AWS_ACCESS_KEY_ID \
  -u AWS_SECRET_ACCESS_KEY \
  -u AWS_SESSION_TOKEN \
  -u AWS_SECURITY_TOKEN \
  AWS_PROFILE=strln \
  aws sts get-caller-identity
```

The returned account must be `562304353751`.

The helper builds `linux/amd64` images by default so deployments from Apple Silicon hosts run on the explicit ECS Fargate `X86_64` task platform. Override with `DOCKER_PLATFORM=...` only if the Terraform runtime platform changes too.

Ingress is CloudFront-first. Terraform restricts the ALB security group to the
AWS-managed CloudFront origin-facing prefix list, so clients should use the
CloudFront HTTPS/WSS endpoint rather than the ALB DNS name.

To deploy the app-facing adapter directly to the currently verified private EC2
vLLM runtime, use:

```bash
TRANSCRIPTION_ALLOWED_USERS=prabhat7@cisco.com \
AWS_PROFILE_NAME=strln \
EXPECTED_ACCOUNT_ID=562304353751 \
./infra/transcription/deploy-direct-aws-adapter.sh
```

This deploys a Fargate transcription adapter behind CloudFront/ALB with
`TRANSCRIPTION_AUTH_MODE=signed_user_token` and
`TRANSCRIPTION_ASR_PROVIDER=voxtral_self_hosted`. The wrapper discovers the
private IP for `i-02b84c39f9912a77a`, sets the service-side vLLM endpoint to
`ws://<private-ip>:8000/v1/realtime`, and relies on the Terraform security
group rule that allows port `8000` only between members of the transcription
task security group. Client laptops do not need SSM, tmux, or the local
adapter in this topology. The wrapper preserves the existing GPU capacity
resources because the verified EC2 runtime is still attached to that
infrastructure, but it keeps the app-facing adapter on Fargate and does not
start a new GPU sidecar task.

To deploy the self-hosted model runtime in this AWS account, use:

```bash
TRANSCRIPTION_AUTH_MODE=signed_user_token \
TRANSCRIPTION_ALLOWED_USERS=prabhat7@cisco.com \
SELF_HOSTED_MODELS=true \
PRELOAD_MODEL_WEIGHTS=true \
HF_TOKEN_FILE=/tmp/cubicle-hf-token \
./infra/transcription/deploy.sh
```

This builds/pushes both the service image and Voxtral runtime image, switches the service to `TRANSCRIPTION_ASR_PROVIDER=voxtral_self_hosted`, enables signed per-user tokens, and provisions ECS EC2 GPU capacity. Set `GPU_DESIRED_CAPACITY=0` to stage the infrastructure and repositories without running a GPU instance.

To publish the Voxtral model-weight image into private ECR without changing the
live ECS service or starting GPU capacity, use:

```bash
HF_TOKEN_FILE=/tmp/cubicle-hf-token \
./infra/transcription/publish-model-images.sh
```

For large model downloads, prefer the AWS-hosted CodeBuild publisher so model
weights are downloaded inside account `562304353751` instead of through local
Docker/QEMU:

```bash
SECRET_NAME="cubicle-transcription/huggingface-token"
aws secretsmanager create-secret \
  --profile strln \
  --region us-west-2 \
  --name "$SECRET_NAME" \
  --secret-string file:///tmp/cubicle-hf-token

IMAGE_TAG=models-$(date +%Y%m%d%H%M) \
HF_SECRET_NAME="$SECRET_NAME" \
./infra/transcription/start-model-publish-codebuild.sh
```

The CodeBuild publisher creates/updates a constrained service role and a
no-source, privileged CPU builder project. The builder reads the Hugging Face
token from Secrets Manager into a temporary file, passes it to Docker BuildKit
as a file secret, pushes only the Voxtral runtime image to private ECR, removes
the temporary token file, and does not update ECS service state or GPU desired
capacity.

`publish-model-images.sh` prefers `HF_TOKEN_FILE`/`HUGGINGFACE_TOKEN_FILE`, so
the token is not placed in Docker process arguments or environment. If only
`HF_TOKEN`/`HUGGINGFACE_TOKEN` is provided, the helper copies it into a
temporary `0600` BuildKit secret file, unsets the token before invoking Docker,
removes the file on exit, and logs out of private ECR by default. If a corporate
TLS CA is required for Hugging Face downloads, pass it with
`EXTRA_CA_CERT_FILE=/path/to/ca-bundle.pem`; the CA bundle is also mounted as a
BuildKit secret.

This helper creates/verifies only the ECR repositories, builds the service image
if explicitly requested, builds the Voxtral vLLM runtime image with Voxtral
weights, pushes the requested images to ECR, and then stops. It does not update
the ECS service, task definition, Auto Scaling group, or GPU desired capacity.
Like the deploy helper, it refuses to run if ambient AWS credential variables
could shadow the `strln` profile.

The default is intentionally vLLM-only. Set `PUSH_SERVICE_IMAGE=true` only when
you want to build the heavier fallback/diarization service image; that path
installs pyannote/Torch dependencies and should usually be split into a
dedicated diarization worker image.

For real speaker labeling, keep the public adapter on the lightweight image and
run pyannote as a private worker image built from
`aws/transcription-service/Dockerfile.diarization-worker`. The adapter-side
provider should be `TRANSCRIPTION_DIARIZATION_PROVIDER=remote_http` with
`TRANSCRIPTION_DIARIZATION_WORKER_URL` pointed at the private worker. Do not
switch the public adapter back to `TRANSCRIPTION_DIARIZATION_PROVIDER=pyannote`;
that was the failure mode that made ALB health and WebSocket startup unstable.

If you provide model secrets as environment variables, the deployment script stores them in Secrets Manager and enables the matching runtime provider:

```bash
PYANNOTE_AUTH_TOKEN=... SELF_HOSTED_MODELS=true ./infra/transcription/deploy.sh
```

`MISTRAL_API_KEY` is only used by the legacy hosted provider when `SELF_HOSTED_MODELS` is not enabled. Without self-hosted mode or model secrets, the stack deploys the secure endpoint with mock ASR and mock diarization so the WebSocket, auth, health, and app integration can still be tested end to end.

Current staging endpoint:

```text
wss://dcabsri6ekziv.cloudfront.net/v1/transcription
```

## Client Settings

After apply, use:

```bash
terraform -chdir=infra/transcription output -raw websocket_endpoint
```

For production, store a short-lived signed user token in Cubicle Settings. The
token is saved to Keychain and sent as an `Authorization: Bearer ...` WebSocket
header, not as a query string or settings-file value.

Mint a user token from the signing key:

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

Leave `TRANSCRIPTION_ALLOWED_USERS` empty to accept any correctly signed,
unexpired token with the expected issuer, audience, and scope. Set
`enforce_service_user_registry=true` only when the WebSocket service should also
require an active DynamoDB user row and non-revoked token-ledger entry. To
revoke one issued token before it expires, add its `jti` to
`TRANSCRIPTION_REVOKED_TOKEN_IDS` and redeploy.

## GPU Note

Fargate is still used for the low-cost mock staging path. The self-hosted Voxtral runtime requires ECS EC2 GPU capacity or EKS GPU nodes; Fargate is not valid for GPU inference. The Terraform EC2 path uses the ECS GPU-optimized AMI, reserves one GPU for the vLLM Voxtral sidecar, and keeps customer audio inference inside the AWS account.

Pyannote diarization should use the private worker path for production-length
sessions. Set `ENABLE_DIARIZATION_WORKER=true`,
`DIARIZATION_WORKER_LAUNCH_TYPE=EC2`, and
`ENABLE_DIARIZATION_WORKER_GPU_CAPACITY=true` to place the worker on a separate
GPU Auto Scaling group/capacity provider from the app-facing adapter and the
Voxtral runtime. Keep `DIARIZATION_PROVIDER=remote_http` on the adapter and
`DIARIZATION_WORKER_PYANNOTE_DEVICE=cuda` on the worker. The Terraform config
raises the adapter's effective stop-time wait to at least the worker timeout
plus a small buffer for remote-worker deployments, so the legacy 45-second
adapter cap no longer preempts the worker's own timeout.

## Current Verified vLLM Runtime

The currently verified runtime is a manual Docker container on EC2 instance
`i-02b84c39f9912a77a` using SSM administrative access only. In the direct AWS
path, the ECS transcription service reaches this runtime through the private
VPC address on port `8000`; client laptops do not use SSM. The SSM port-forward
command remains useful only for debugging or for the old local adapter fallback:

```bash
aws ssm start-session \
  --profile strln \
  --region us-west-2 \
  --target i-02b84c39f9912a77a \
  --document-name AWS-StartPortForwardingSession \
  --parameters '{"portNumber":["8000"],"localPortNumber":["8000"]}'
```

Use `aws/transcription-service/env.vllm-forwarded.example` for the service-side
adapter environment. `localhost:8000` is the forwarded vLLM server; Cubicle
itself should point at the transcription service WebSocket endpoint, not the
raw vLLM `/v1/realtime` route.

## Admin Console

The admin console is for managing transcription users and issuing short-lived
user tokens. It is disabled by default:

```text
enable_admin_console=false
enable_public_admin_console=false
public_admin_request_certificate=false
admin_desired_count=0
```

The current recommended launch mode for `cubicle.agenticisolation.com` is
public DNS plus managed authentication, not an unauthenticated public dashboard:

```text
Browser
  -> https://cubicle.agenticisolation.com/admin
  -> public HTTPS ALB
  -> AWS WAF
  -> ALB authenticate-cognito action
  -> Cognito user pool with MFA required
  -> private admin ECS task
  -> DynamoDB + Secrets Manager
```

The admin ECS task still has no public IP. The public ALB can only forward
matching `/admin`, `/admin/*`, and `/oauth2/*` traffic after Cognito
authentication succeeds. The admin application validates the ALB/Cognito
identity headers and requires the `CubicleTranscriptionAdmins` Cognito group.
There is no second credential prompt after Cognito.

When `enable_public_admin_console=true`, Terraform provisions:

- DynamoDB user registry table, token-ledger table, and admin-audit table with
  point-in-time recovery, server-side encryption, and deletion protection.
- Secrets Manager secret for the admin session cookie/CSRF signing key. The
  existing user-token signing secret is reused for issued transcription tokens.
- Private admin subnets and VPC endpoints for ECR, CloudWatch Logs, Secrets
  Manager, S3, and DynamoDB so the admin task does not need public internet
  reachability.
- Dedicated public HTTPS ALB for the admin console.
- AWS WAF on the public admin ALB, including reputation, common-rule,
  known-bad-input, and rate-limit controls.
- Cognito user pool with admin-created users only and software-token MFA
  required.
- Cognito app client/domain configured for the ALB
  `authenticate-cognito` listener action.
- Private ECS Fargate admin service running the admin router only, separate from
  the app-facing transcription WebSocket service.

Terraform guards refuse to enable the public console unless an issued ACM
certificate ARN is supplied and at least one HTTPS ingress CIDR is configured.
Using `0.0.0.0/0` is acceptable only for this public Cognito/WAF mode because
the dashboard route is still authenticated by Cognito MFA before the request
reaches the admin service.

The stricter private/VPN mode remains available through `enable_admin_console`
without `enable_public_admin_console`, but it is not the active GoDaddy plan for
this round.

### GoDaddy DNS

Keep GoDaddy as the registrar/public DNS authority for `agenticisolation.com`.
For the Cognito-protected public console, GoDaddy needs two kinds of DNS
records:

1. The ACM validation CNAME for `cubicle.agenticisolation.com`:

   ```text
   _random.cubicle.agenticisolation.com CNAME _random.acm-validations.aws
   ```

2. After the public admin stack is created, the runtime CNAME:

   ```text
   cubicle CNAME cubicle-transcription-admin-pub-1065473193.us-west-2.elb.amazonaws.com
   ```

Get the exact runtime value from Terraform:

```bash
terraform -chdir=infra/transcription output admin_public_godaddy_cname
```

In GoDaddy DNS Management for `agenticisolation.com`, add a CNAME with:

- Type: `CNAME`
- Name: `cubicle`
- Value: `cubicle-transcription-admin-pub-1065473193.us-west-2.elb.amazonaws.com`
- TTL: default or 600 seconds

Do not point this hostname at EC2 directly and do not use the app-facing
CloudFront transcription distribution for the admin console.

Current public admin AWS state as of 2026-05-18:

- The original ACM certificate
  `arn:aws:acm:us-west-2:562304353751:certificate/cf0e2e52-ac1a-4521-aae4-10773f259af9`
  failed with `CAA_ERROR`.
- GoDaddy now has a CAA record allowing `amazon.com`; replacement certificate
  `arn:aws:acm:us-west-2:562304353751:certificate/4e87bf99-c142-4033-868e-703db7d60c61`
  is `ISSUED`.
- Terraform applied the public Cognito/WAF/admin ALB stack and then set
  `admin_desired_count=1`.
- ECS service `cubicle-transcription-admin` is stable with
  desired/running/pending `1/1/0`.
- The admin target group has a healthy private target on port `8080`.
- The admin image is
  `562304353751.dkr.ecr.us-west-2.amazonaws.com/cubicle-transcription-service:admin-cookie-lax-20260518213614`
  with ECR digest
  `sha256:060669a54b6b18f36c199636566a79149675feb8a0c610930bb0f979f15510e3`.
- The issued-token page now includes Keychain save instructions and a direct
  revoke button for the exact token ID shown on that page.
- Token `4c29bee6-1620-4ac8-8208-5740596a0673` for
  `neelamsingh@gmail.com` is revoked in the token ledger because it was exposed
  in a screenshot.
- `prabhat7@cisco.com` has been invited into the Cognito user pool and added to
  the `CubicleTranscriptionAdmins` group.
- The admin session secret value was generated, stored in Secrets Manager, and
  removed from local `/tmp` files without printing it. Admin sign-in uses
  Cognito username/password/MFA, not a second app password.
- The old `cubicle-transcription/admin-token` Secrets Manager secret has been
  removed from Terraform state and AWS.
- The app-facing transcription task can now reach the shared AWS interface
  endpoints for ECR, CloudWatch Logs, and Secrets Manager through a scoped TCP
  443 security-group rule. This prevents Fargate startup failures after private
  DNS is enabled for the admin VPC endpoints.
- A direct unauthenticated request to the admin ALB returns a Cognito `302`.
- `dig cubicle.agenticisolation.com` now resolves through the GoDaddy runtime
  CNAME to `cubicle-transcription-admin-pub-1065473193.us-west-2.elb.amazonaws.com`.
- `https://cubicle.agenticisolation.com/` now returns an ALB `302` redirect to
  `https://cubicle.agenticisolation.com:443/admin`; this redirect does not
  bypass authentication.
- `https://cubicle.agenticisolation.com/admin` returns a Cognito `302` with
  `redirect_uri=https://cubicle.agenticisolation.com/oauth2/idpresponse`.
- `https://cubicle.agenticisolation.com/admin/login` is not a second login UI.
  Unauthenticated requests are still redirected to Cognito by the ALB, and
  authenticated requests are redirected by the admin app back to `/admin`.
- The admin app uses an HttpOnly, Secure, SameSite=Lax internal session cookie
  after Cognito. Lax is required for Cognito's hosted-login top-level redirect;
  mutation forms still require CSRF tokens.
- Cognito currently reports `prabhat7@cisco.com` as enabled and
  `FORCE_CHANGE_PASSWORD`, so first-password setup and MFA enrollment are the
  remaining manual login steps.

### Staged Public Enablement

Request the ACM certificate first, then copy the validation CNAME into GoDaddy:

```bash
terraform -chdir=infra/transcription apply \
  -var 'public_admin_request_certificate=true' \
  -var 'enable_public_admin_console=false'

terraform -chdir=infra/transcription output admin_public_certificate_validation_records
```

Current certificate request:

```text
Original ARN:    arn:aws:acm:us-west-2:562304353751:certificate/cf0e2e52-ac1a-4521-aae4-10773f259af9
Original status: FAILED with CAA_ERROR
Replacement ARN: arn:aws:acm:us-west-2:562304353751:certificate/4e87bf99-c142-4033-868e-703db7d60c61
Current status:  ISSUED
```

Current GoDaddy ACM validation CNAME to add:

```text
Type:  CNAME
Name:  _cd15ff0c92e13ccbc561c4ef88a470c5.cubicle
Value: _4aa889b42ca6fd1930e4de3a6a8ccc8d.jkddzztszm.acm-validations.aws
TTL:   default or 600 seconds
```

GoDaddy may also accept the fully qualified host name:
`_cd15ff0c92e13ccbc561c4ef88a470c5.cubicle.agenticisolation.com`.

After ACM reports the certificate as `ISSUED`, stage the public Cognito/WAF
infrastructure with the task still stopped:

```bash
terraform -chdir=infra/transcription apply \
  -var 'enable_public_admin_console=true' \
  -var 'public_admin_certificate_arn=arn:aws:acm:us-west-2:562304353751:certificate/...' \
  -var 'public_admin_domain_name=cubicle.agenticisolation.com' \
  -var 'public_admin_allowed_cidr_blocks=["0.0.0.0/0"]' \
  -var 'admin_desired_count=0'
```

Populate the admin session secret through Secrets Manager. Do not pass this value in
Terraform variables because that would put secret material in Terraform state:

```bash
openssl rand -base64 48 > /tmp/cubicle-admin-session-secret

aws secretsmanager put-secret-value \
  --profile strln \
  --region us-west-2 \
  --secret-id cubicle-transcription/admin-session-secret \
  --secret-string file:///tmp/cubicle-admin-session-secret

rm -f /tmp/cubicle-admin-session-secret
```

Create the first Cognito admin user. The user pool is admin-create-only and MFA
is required on first login:

```bash
USER_POOL_ID="$(terraform -chdir=infra/transcription output -raw admin_public_cognito_user_pool_id)"

aws cognito-idp admin-create-user \
  --profile strln \
  --region us-west-2 \
  --user-pool-id "$USER_POOL_ID" \
  --username prabhat7@cisco.com \
  --user-attributes Name=email,Value=prabhat7@cisco.com Name=email_verified,Value=true

aws cognito-idp admin-add-user-to-group \
  --profile strln \
  --region us-west-2 \
  --user-pool-id "$USER_POOL_ID" \
  --username prabhat7@cisco.com \
  --group-name CubicleTranscriptionAdmins
```

Start one admin task only after the Cognito user and admin session secret are ready:

```bash
terraform -chdir=infra/transcription apply \
  -var 'enable_public_admin_console=true' \
  -var 'public_admin_certificate_arn=arn:aws:acm:us-west-2:562304353751:certificate/...' \
  -var 'public_admin_domain_name=cubicle.agenticisolation.com' \
  -var 'public_admin_allowed_cidr_blocks=["0.0.0.0/0"]' \
  -var 'admin_desired_count=1'
```

Then add the GoDaddy runtime CNAME:

```bash
terraform -chdir=infra/transcription output admin_public_godaddy_cname
```

The current output is:

```text
cubicle CNAME cubicle-transcription-admin-pub-1065473193.us-west-2.elb.amazonaws.com
```

Acceptance checks:

- `https://cubicle.agenticisolation.com/admin` redirects unauthenticated users
  to Cognito.
- `https://cubicle.agenticisolation.com/` redirects to `/admin`; unrelated
  paths still return the listener default `404`.
- Cognito requires MFA before the ALB forwards to the admin ECS task.
- The admin task has no public IP.
- AWS WAF is associated with the public admin ALB.
- The admin app does not ask for any extra admin credential after Cognito.
- The stale `/admin/login` compatibility route redirects back to `/admin` in
  public Cognito mode and does not render an explanatory or credential page.
- If the app receives a request without valid ALB/Cognito admin identity
  headers, it returns `403` instead of redirecting to itself.
- Usage lookup by email shows metadata-only session counts, audio duration,
  byte totals, last session time, and issued/revoked/expired token counts.
- Issued-token pages show the token once, explain how to save it into Cubicle
  Keychain, and include a direct revoke action for accidental exposure.
- Revoking a token updates the DynamoDB token ledger; the app-facing
  transcription service checks that ledger before accepting signed user tokens.
- Admin logs must not contain token plaintext, raw audio, transcript text, or
  model output.
