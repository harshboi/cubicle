# Cubicle Transcription Admin Console

This document describes the management console for Cubicle transcription users.
The console administers who can use the transcription service and issues
short-lived signed user tokens for the Cubicle macOS app.

The current recommended deployment is public DNS with mandatory managed
authentication:

```text
https://cubicle.agenticisolation.com/admin
  -> public HTTPS ALB
  -> AWS WAF
  -> ALB authenticate-cognito
  -> Cognito user pool with MFA required
  -> private ECS admin task
  -> DynamoDB + Secrets Manager
```

This is intentionally not an unauthenticated public web app. The admin ECS task
is private, has no public IP, and only receives requests after the public ALB's
Cognito action succeeds. The admin application trusts the ALB/Cognito identity
headers, requires the `CubicleTranscriptionAdmins` Cognito group, then issues
only an internal CSRF/session cookie for form safety. There is no second
credential prompt after Cognito.

## Current Build Status

Implemented in this repository:

- Service-side admin router scaffold in `transcription_service/admin.py`.
- Admin user/token store in `transcription_service/admin_store.py`.
- In-memory store for local tests and DynamoDB store for AWS.
- Admin login through Cognito username, password, and MFA at the ALB layer.
- Application-side validation of ALB/Cognito identity headers and the
  `CubicleTranscriptionAdmins` group.
- HttpOnly, Secure, SameSite=Lax admin session cookie. Lax is intentional
  because Cognito hosted login returns to the console through a top-level
  cross-site redirect; CSRF tokens still protect all mutation forms.
- CSRF checks on cookie-authenticated mutation routes.
- Add/reactivate user.
- Disable user.
- Lookup user usage by email.
- Issue short-lived signed transcription token.
- Record issued token metadata without storing the token value.
- Revoke token metadata.
- Record metadata-only transcription usage sessions in the audit table.
- Separate Audio Tuning admin section for the Voxtral input-normalization
  controls: target RMS, RMS floor, max server gain, and peak ceiling.
- Runtime audio tuning is stored as an admin config row in DynamoDB and read by
  the app-facing transcription service when a new WebSocket session starts.
- Admin API and store unit tests.
- Disabled-by-default Terraform for private admin resources shared by both
  private and public modes: DynamoDB user/token/audit tables, Secrets Manager
  admin secrets, private admin subnets, VPC endpoints, private ECS service, and
  least-privilege IAM.
- Disabled-by-default Terraform for the public protected mode: public HTTPS
  admin ALB, Cognito user pool/client/domain/group, MFA-required sign-in, AWS
  WAF association, and GoDaddy CNAME outputs for
  `cubicle.agenticisolation.com`.
- The 2026-05-23 audio-tuning implementation is deployed to the app-facing
  transcription service and the private admin service.

Deployed in AWS account `562304353751` as of 2026-05-18:

- The first ACM certificate
  `arn:aws:acm:us-west-2:562304353751:certificate/cf0e2e52-ac1a-4521-aae4-10773f259af9`
  failed with `CAA_ERROR` because GoDaddy initially allowed only Let's Encrypt
  issuance for the domain.
- After adding a GoDaddy CAA record that permits `amazon.com`, Terraform
  replaced the failed certificate with
  `arn:aws:acm:us-west-2:562304353751:certificate/4e87bf99-c142-4033-868e-703db7d60c61`.
  ACM now reports the replacement certificate as `ISSUED`.
- The public protected admin stack has been applied with Cognito MFA, AWS WAF,
  public HTTPS admin ALB, private admin ECS service, private VPC endpoints,
  encrypted/PITR DynamoDB tables, and Secrets Manager secret containers.
- The admin service is running with desired/running/pending `1/1/0` in ECS and
  the admin target group has a healthy private target on port `8080`.
- The admin image is
  `562304353751.dkr.ecr.us-west-2.amazonaws.com/cubicle-transcription-service:admin-cookie-lax-20260518213614`
  with ECR digest
  `sha256:060669a54b6b18f36c199636566a79149675feb8a0c610930bb0f979f15510e3`.
- The old `cubicle-transcription/admin-token` Secrets Manager secret was
  removed. The admin ECS task now receives only the service token, admin
  session secret, and user-token signing key.
- The extra app-level login page was removed. Admins sign in with Cognito
  username/password/MFA only; the admin service accepts the authenticated
  ALB/Cognito identity and group claim.
- The app-facing transcription service now checks the DynamoDB user registry
  and token ledger before accepting signed user tokens, so disabling a user or
  revoking a token blocks new transcription sessions for that user/token.
- The app-facing transcription service writes metadata-only usage records into
  the admin audit table at session stop. The admin console can query usage by
  user email without storing raw audio or transcript text.
- The shared AWS interface endpoints for ECR, CloudWatch Logs, and Secrets
  Manager now allow both the private admin task security group and the
  app-facing transcription task security group on TCP 443. This fixes Fargate
  startup after private DNS is enabled for those endpoints without opening the
  endpoints broadly.
- The issued-token page now gives explicit Cubicle Keychain save steps, shows
  the short Token ID separately from the long bearer token, and includes a
  direct "Revoke this token" action wired to that exact Token ID.
- Exposed token `4c29bee6-1620-4ac8-8208-5740596a0673` for
  `neelamsingh@gmail.com` was revoked in the DynamoDB token ledger after it
  appeared in a screenshot.
- The first Cognito admin user invitation was requested for
  `prabhat7@cisco.com` and that user was added to the
  `CubicleTranscriptionAdmins` group.
- The admin session secret value is stored in Secrets Manager and is used only
  to sign the admin console's internal CSRF/session cookie. It is not an admin
  login credential.
- Unauthenticated requests to the admin ALB return `302` to Cognito instead of
  exposing admin content.
- Runtime GoDaddy CNAME is now published:
  `cubicle -> cubicle-transcription-admin-pub-1065473193.us-west-2.elb.amazonaws.com`.
  `https://cubicle.agenticisolation.com/` returns an ALB `302` redirect to
  `https://cubicle.agenticisolation.com:443/admin`.
  `https://cubicle.agenticisolation.com/admin` returns `302` to Cognito with
  the final hostname as the callback URL.
- Admin login is now Cognito-only: complete the Cognito password/MFA flow and
  the admin console should open directly.
- The stale `/admin/login` browser page is no longer a destination in public
  Cognito mode. Requests to `/admin/login` are redirected back to `/admin`, so
  the ALB/Cognito authentication rule remains the only admin login surface.
- The internal admin session cookie now uses SameSite=Lax to avoid a Cognito
  hosted-login redirect loop. If ALB/Cognito identity headers are missing or the
  user is not in `CubicleTranscriptionAdmins`, the app returns a 403 page
  instead of redirecting to itself.
- The admin console includes `/admin/audio-tuning`, a separate page from the
  user-management menu. It shows the recommended values beside each field:
  target RMS `20%`, RMS floor `0.8%`, max server gain `24x`, and peak ceiling
  `92%`.
- Audio tuning is live in AWS on image
  `562304353751.dkr.ecr.us-west-2.amazonaws.com/cubicle-transcription-service:direct-aws-202605230036-admin-audio-tuning`
  with digest
  `sha256:c65bdcb385d44890864659ed8dba363f4330d941edd890f332c07eff788998dc`.
  ECS stabilized on transcription task definition
  `cubicle-transcription-service:47` and admin task definition
  `cubicle-transcription-admin:42`.

Do not enable the admin router on the existing public transcription CloudFront
service. It is meant to run as a separate admin service, not as a route on the
audio transcription ingress.

## Terraform Controls

The public Cognito-protected infrastructure is controlled by these variables in
`infra/transcription`:

```text
enable_public_admin_console=false
public_admin_domain_name=cubicle.agenticisolation.com
public_admin_certificate_arn=
public_admin_request_certificate=false
public_admin_allowed_cidr_blocks=["0.0.0.0/0"]
public_admin_cognito_domain_prefix=
public_admin_cognito_session_timeout_seconds=3600
public_admin_waf_rate_limit=500
admin_desired_count=0
```

The stricter private/VPN infrastructure remains available through:

```text
enable_admin_console=false
admin_request_certificate=false
admin_domain_name=cubicle.agenticisolation.com
admin_create_private_hosted_zone=false
admin_private_zone_name=agenticisolation.com
admin_allowed_cidr_blocks=[]
admin_allowed_security_group_ids=[]
enable_admin_client_vpn=false
```

For this round, use the public Cognito/WAF path unless the user explicitly asks
to return to VPN-only access.

## Security Model

There are layered gates:

1. HTTPS only: the public admin ALB listens on 443 with an issued ACM
   certificate for `cubicle.agenticisolation.com`.
2. AWS WAF: the ALB is associated with AWS managed rule groups and a rate-limit
   rule.
3. Cognito MFA: ALB `authenticate-cognito` requires a Cognito session before
   any `/admin` request is forwarded.
4. Admin-created users only: Cognito self-registration is disabled.
5. Private backend: the admin ECS task runs in private subnets with no public
   IP and accepts traffic only from the admin ALB security group.
6. Application session: the admin app validates ALB/Cognito headers, requires
   the `CubicleTranscriptionAdmins` group, then uses an internal SameSite
   SameSite=Lax cookie and CSRF token for forms. There is no second login after
   Cognito.
7. Token service separation: admin login does not grant transcription access by
   itself. It only manages registry rows and issues signed user tokens that
   Cubicle stores in Keychain.

The first public release is therefore internet-reachable but not anonymous. A
future identity-broker slice can add Okta/Cisco/Cognito federation and automatic
client-token refresh, but the management console itself already uses
username/password/MFA without a separate app password.

## Audio Tuning Settings

The audio tuning page controls the server-side normalization that prepares
microphone PCM before it is sent to the Voxtral runtime. These settings are for
capturing quiet but real speech, such as low-volume TV audio, without amplifying
near-silence into model input.

Recommended production values:

```text
Target RMS:      20%
RMS floor:       0.8%
Max server gain: 24x
Peak ceiling:    92%
```

Field behavior:

- Target RMS is the output loudness the server tries to reach for speech-like
  audio. Recommended: `20%`.
- RMS floor prevents near-silence from being multiplied. Recommended: `0.8%`.
- Max server gain caps how aggressively quiet speech can be lifted.
  Recommended: `24x`.
- Peak ceiling prevents clipping after gain is applied. Recommended: `92%`.

Saving this form does not require an ECS service restart. The admin service
writes one DynamoDB config row, `CONFIG#transcription_audio_tuning`, in the
admin/user registry table. The app-facing transcription service has read-only
access to that config row and checks it when each new transcription WebSocket
session starts. Already-open sessions keep the settings they started with; stop
and start the Cubicle live transcription session to pick up a new value.

If the runtime config lookup fails, the transcription service falls back to the
startup/default values rather than blocking transcription.

## Runtime Topology

Admin path:

```text
Admin browser
  -> GoDaddy public CNAME cubicle.agenticisolation.com
  -> public admin ALB
  -> AWS WAF
  -> Cognito hosted auth and MFA
  -> private admin ECS service
  -> DynamoDB user registry + token ledger
  -> Secrets Manager signing/admin/session secrets
```

App-facing transcription path remains separate:

```text
Cubicle.app
  -> wss://dcabsri6ekziv.cloudfront.net/v1/transcription
  -> CloudFront-restricted ALB
  -> ECS transcription adapter
  -> DynamoDB audio tuning config lookup at new session start
  -> private EC2 vLLM Voxtral runtime
```

## GoDaddy DNS Instructions

Your registrar can stay GoDaddy for `agenticisolation.com`.

Add the ACM validation record first. Terraform prints the exact record:

```bash
terraform -chdir=infra/transcription output admin_public_certificate_validation_records
```

In GoDaddy DNS Management, add the validation CNAME that looks like:

```text
_random.cubicle.agenticisolation.com CNAME _random.acm-validations.aws
```

This record validates certificate ownership and does not route the dashboard.

Current ACM validation record in GoDaddy:

```text
Type:  CNAME
Name:  _cd15ff0c92e13ccbc561c4ef88a470c5.cubicle
Value: _4aa889b42ca6fd1930e4de3a6a8ccc8d.jkddzztszm.acm-validations.aws
TTL:   default or 600 seconds
```

GoDaddy may also accept the fully qualified name:

```text
_cd15ff0c92e13ccbc561c4ef88a470c5.cubicle.agenticisolation.com
```

This validation record is now accepted by ACM for certificate
`arn:aws:acm:us-west-2:562304353751:certificate/4e87bf99-c142-4033-868e-703db7d60c61`.

After the public admin stack exists, Terraform prints the runtime CNAME:

```bash
terraform -chdir=infra/transcription output admin_public_godaddy_cname
```

In GoDaddy DNS Management for `agenticisolation.com`, add:

```text
Type:  CNAME
Name:  cubicle
Value: cubicle-transcription-admin-pub-1065473193.us-west-2.elb.amazonaws.com
TTL:   default or 600 seconds
```

Do not point `cubicle.agenticisolation.com` to an EC2 public IP. Do not reuse
the transcription CloudFront distribution for the admin dashboard.

The bare host is intentionally not a dashboard route. Terraform adds an ALB
listener rule that redirects only `/` to `/admin`; `/admin` remains protected
by Cognito MFA, the required admin group, and the app's session/CSRF controls,
while unrelated paths continue to return `404`.

## Staged AWS Enablement

Run the rollout in stages so the public DNS name never fronts an unprotected
service.

1. Request a DNS-validated ACM certificate without enabling the dashboard:

   ```bash
   terraform -chdir=infra/transcription apply \
     -var 'public_admin_request_certificate=true' \
     -var 'enable_public_admin_console=false'

   terraform -chdir=infra/transcription output admin_public_certificate_validation_records
   ```

2. Add only the ACM validation CNAME in GoDaddy. Wait until ACM reports the
   certificate as `ISSUED`.

3. Stage the public protected resources with the admin service stopped:

   ```bash
   terraform -chdir=infra/transcription apply \
     -var 'enable_public_admin_console=true' \
     -var 'public_admin_certificate_arn=arn:aws:acm:us-west-2:562304353751:certificate/...' \
     -var 'public_admin_domain_name=cubicle.agenticisolation.com' \
     -var 'public_admin_allowed_cidr_blocks=["0.0.0.0/0"]' \
     -var 'admin_desired_count=0'
   ```

4. Populate the admin session secret through Secrets Manager, not Terraform
   variables:

   ```bash
   openssl rand -base64 48 > /tmp/cubicle-admin-session-secret

   aws secretsmanager put-secret-value \
     --profile strln \
     --region us-west-2 \
     --secret-id cubicle-transcription/admin-session-secret \
     --secret-string file:///tmp/cubicle-admin-session-secret

   rm -f /tmp/cubicle-admin-session-secret
   ```

5. Create the first Cognito admin user:

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

6. Start one admin task:

   ```bash
   terraform -chdir=infra/transcription apply \
     -var 'enable_public_admin_console=true' \
     -var 'public_admin_certificate_arn=arn:aws:acm:us-west-2:562304353751:certificate/...' \
     -var 'public_admin_domain_name=cubicle.agenticisolation.com' \
     -var 'public_admin_allowed_cidr_blocks=["0.0.0.0/0"]' \
     -var 'admin_desired_count=1'
   ```

7. Add the GoDaddy runtime CNAME from `admin_public_godaddy_cname`.

8. Verify that `https://cubicle.agenticisolation.com/admin` redirects to
   Cognito, requires MFA, then opens the admin console directly.

## Admin Service Environment

Use a separate ECS service/task for the admin console with these settings:

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
TRANSCRIPTION_ADMIN_SESSION_TTL_SECONDS=900
TRANSCRIPTION_ADMIN_DEFAULT_USER_TOKEN_TTL_SECONDS=86400
TRANSCRIPTION_TOKEN_ISSUER=cubicle-transcription
TRANSCRIPTION_TOKEN_AUDIENCE=cubicle-macos
TRANSCRIPTION_REQUIRED_SCOPE=transcription:stream
```

Use Secrets Manager for:

- `cubicle-transcription/admin-session-secret`
- `cubicle-transcription/user-token-signing-key`

The admin service needs IAM permissions for:

- `secretsmanager:GetSecretValue` on those exact secrets.
- DynamoDB read/write on the user registry table.
- DynamoDB read/write on the token ledger table.
- DynamoDB write on the admin audit table.
- CloudWatch logs for its own log group.

The app-facing transcription service needs DynamoDB `GetItem` and
`DescribeTable` only on the admin/user registry table so it can read the audio
tuning config row for new sessions. It must not have write access to admin
configuration.

## Admin Routes

```text
GET  /admin/login
POST /admin/logout
GET  /admin
GET  /admin/audio-tuning
POST /admin/audio-tuning
GET  /admin/usage?email={email}
GET  /admin/health
GET  /admin/users
POST /admin/users
POST /admin/users/{email}/disable
POST /admin/users/{email}/tokens
POST /admin/users/{email}/tokens/{token_id}/revoke
```

The preferred revoke path is the exact-token route shown on the issued-token
page: `/admin/users/{email}/tokens/{token_id}/revoke`. The generic revoke form
requires the short UUID Token ID; pasting the long bearer token into that field
is incorrect.

Browser form usage gets an HttpOnly session cookie and uses CSRF tokens for
mutations after Cognito authentication succeeds.

## Data Rules

- The admin console never stores plaintext transcription tokens.
- DynamoDB stores token IDs, status, scope, issue time, expiry time, revocation
  time, and revocation reason.
- DynamoDB audit records store usage metadata only: user email, token ID,
  session ID, language mode, diarization flag, audio bytes, audio milliseconds,
  and timestamps.
- CloudWatch logs must not include bearer tokens, user token values, audio,
  transcript text, or model output.
- Disabling a user causes the transcription service dynamic registry check to
  fail closed after the short cache TTL.
- Revoking a token causes the token-ledger check to reject that token ID.
- Audio tuning changes are stored as numeric percentages/gain caps only. They
  do not store audio, transcript text, or model output.

## Local Smoke

After installing service requirements:

```bash
PYTHONPATH=aws/transcription-service \
TRANSCRIPTION_ADMIN_ENABLED=true \
TRANSCRIPTION_ADMIN_EXTERNAL_AUTH_PROVIDER=cognito_alb \
TRANSCRIPTION_ADMIN_REQUIRED_GROUP=CubicleTranscriptionAdmins \
TRANSCRIPTION_ADMIN_SESSION_SECRET=example-session-secret \
TRANSCRIPTION_TOKEN_SIGNING_SECRET=example-user-signing-key \
TRANSCRIPTION_ADMIN_COOKIE_SECURE=false \
python3 -m transcription_service.main
```

Open:

```text
http://127.0.0.1:8080/admin/login
```

In public Cognito mode, `/admin/login` redirects back to `/admin`; it is kept
only as a compatibility route and should not render a credential form or an
instruction page. Route tests inject synthetic ALB identity headers to exercise
the admin console. Production must use the Cognito/WAF path above or the
stricter private/VPN mode.

## Production Acceptance Checks

- `https://cubicle.agenticisolation.com/admin` redirects unauthenticated users
  to Cognito.
- `https://cubicle.agenticisolation.com/` redirects to `/admin`; unrelated
  paths still return `404`.
- `https://cubicle.agenticisolation.com/admin/login` also returns to the
  Cognito-protected `/admin` flow and does not render a second login page.
- Cognito MFA is required before any request reaches the admin ECS task.
- The admin task runs in private subnets and has no public IP.
- AWS WAF is attached to the public admin ALB.
- Admin login works over HTTPS with Cognito username/password/MFA only.
- The admin page does not prompt for any extra admin credential after Cognito.
- Add user creates `USER#<email>` in DynamoDB.
- Usage lookup by email shows session and token metadata without transcript or
  audio content.
- Issue token returns the signed token once and writes only metadata to the
  token ledger.
- Disable user blocks new transcription sessions for that user.
- Revoke token blocks that token ID.
- CloudWatch logs contain metadata only.
