# Cubicle AWS Handoff And Fresh Account Setup

This document is the operator handoff for gifting this Cubicle system to
someone who will run it in their own AWS account from a laptop.

The transfer model is source-code plus repeatable provisioning. Do not transfer
this machine's AWS credentials, Terraform state, local app data, service
tokens, signing keys, Hugging Face token, Cognito users, DynamoDB rows, S3
objects, or old ECS task-definition snapshots. The recipient should create
fresh AWS resources in their own account.

## What Is In Scope

- Cubicle macOS source and build scripts.
- Cubicle live transcription service source.
- AWS Terraform for transcription ingress, CloudFront, ALB, ECS, ECR,
  Secrets Manager, DynamoDB admin tables, Cognito/WAF admin console, EC2 GPU
  capacity, and the private vLLM runtime path.
- VoiceNotes source and AWS Terraform for its Cognito/ALB/ECS/DynamoDB/S3/KMS
  deployment.
- Fresh-account scripts that run from a laptop and create a new stack in a new
  AWS account.

## Current AWS Shape

The production path this repo is designed to recreate is:

```text
Cubicle.app
  -> wss://<cloudfront-host>/v1/transcription
  -> CloudFront TLS
  -> ALB restricted to CloudFront origin-facing AWS prefix list
  -> ECS Fargate transcription adapter
  -> private EC2 GPU vLLM runtime on port 8000
  -> Voxtral Mini Realtime
```

Optional speaker labeling uses a separate private ECS diarization worker and a
separate EC2 GPU capacity provider. VoiceNotes is a separate web app that calls
the transcription endpoint as its upstream service.

The expensive part is EC2 GPU compute. A single `g5.xlarge` is about one
full-time GPU host; enabling both Voxtral and the diarization worker means two
full-time `g5.xlarge` hosts.

## Source Backup

Create a transfer archive:

```bash
Scripts/backup-source.sh
```

The archive is written to:

```text
backups/source-handoff/<timestamp>/cubicle-source-<timestamp>.tar.gz
```

It excludes local build products, previous backups, result dumps, Terraform
state, Terraform plans, `terraform.tfvars`, virtualenvs, pycache, screenshots,
local VoiceNotes data, secret-like env files, and account-bound ECS task
definition snapshots.

Verify before sending:

```bash
cd backups/source-handoff/<timestamp>
shasum -a 256 -c cubicle-source-<timestamp>.tar.gz.sha256
```

## Recipient Prerequisites

The recipient laptop needs:

- AWS CLI v2 authenticated to the recipient AWS account.
- Terraform.
- Docker with buildx.
- Python 3.
- OpenSSL.
- AWS Session Manager plugin for EC2/vLLM debugging.
- A Hugging Face token that has accepted the gated model terms for
  `mistralai/Voxtral-Mini-4B-Realtime-2602` and
  `pyannote/speaker-diarization-community-1` if diarization is enabled.
- EC2 On-Demand G/VT quota high enough for `g5.xlarge` in `us-west-2`
  or the chosen region.
- Optional DNS control and an issued ACM certificate for VoiceNotes and any
  public admin host.

Use a short project name because AWS ALB names have length limits:

```text
cubicle-transcript-rp
```

## Fresh Account Preflight

From the restored repo root:

```bash
AWS_PROFILE_NAME=<recipient-profile> \
EXPECTED_ACCOUNT_ID=<recipient-account-id> \
AWS_REGION_NAME=us-west-2 \
PROJECT_NAME=cubicle-transcript-rp \
TRANSCRIPTION_ALLOWED_USERS=user@example.com \
HF_TOKEN_FILE=/path/to/huggingface-token \
Scripts/setup-fresh-aws-system.sh preflight
```

The preflight validates:

- AWS identity matches `EXPECTED_ACCOUNT_ID`.
- Required local tools exist.
- Terraform validates.
- Docker/buildx/ECR auth works.
- `g5.xlarge` is offered and quota appears sufficient.
- No existing AWS resources collide with `PROJECT_NAME`.

It does not create resources.

## Fresh Transcription Stack

Provision the direct Cubicle transcription path:

```bash
AWS_PROFILE_NAME=<recipient-profile> \
EXPECTED_ACCOUNT_ID=<recipient-account-id> \
AWS_REGION_NAME=us-west-2 \
PROJECT_NAME=cubicle-transcript-rp \
TRANSCRIPTION_ALLOWED_USERS=user@example.com \
HF_TOKEN_FILE=/path/to/huggingface-token \
Scripts/setup-fresh-aws-system.sh transcription
```

The wrapper calls `infra/transcription/provision-transcription-replica.sh`.
That helper:

1. Creates a dedicated Terraform workspace for the project.
2. Creates or updates the Hugging Face token secret.
3. Creates ECR repositories, service secrets, VPC, public subnets, ALB,
   CloudFront, ECS service, and EC2 GPU Auto Scaling group.
4. Builds and pushes the transcription service image.
5. Starts one GPU EC2 instance.
6. Grants that instance least-privilege access to the Hugging Face token.
7. Uses SSM Run Command to start Dockerized `vllm/vllm-openai` with
   `mistralai/Voxtral-Mini-4B-Realtime-2602` on port `8000`.
8. Redeploys the app-facing adapter so it reaches the private vLLM runtime.
9. Prints the client WSS URL and health URL.

Save the printed client WSS URL into Cubicle Settings. Store only short-lived
user tokens in the app Keychain, not signing secrets.

## Optional VoiceNotes

VoiceNotes needs a real HTTPS domain and an ACM certificate in the same AWS
region as the ALB.

To request a DNS-validated certificate:

```bash
AWS_PROFILE_NAME=<recipient-profile> \
EXPECTED_ACCOUNT_ID=<recipient-account-id> \
AWS_REGION_NAME=us-west-2 \
PROJECT_NAME=cubicle-transcript-rp \
VOICENOTES_DOMAIN_NAME=notes.example.com \
REQUEST_VOICENOTES_CERTIFICATE=true \
Scripts/setup-fresh-aws-system.sh voicenotes
```

Add the printed DNS validation record, wait for ACM to show `ISSUED`, then
deploy:

```bash
AWS_PROFILE_NAME=<recipient-profile> \
EXPECTED_ACCOUNT_ID=<recipient-account-id> \
AWS_REGION_NAME=us-west-2 \
PROJECT_NAME=cubicle-transcript-rp \
VOICENOTES_DOMAIN_NAME=notes.example.com \
VOICENOTES_CERTIFICATE_ARN=arn:aws:acm:us-west-2:<account>:certificate/<id> \
VOICENOTES_ADMIN_EMAIL=user@example.com \
Scripts/setup-fresh-aws-system.sh voicenotes
```

The VoiceNotes wrapper:

1. Reads the freshly-created transcription outputs for VPC, subnets,
   WebSocket endpoint, and service-token secret.
2. Creates the VoiceNotes ECR repository.
3. Builds and pushes the ARM64 VoiceNotes image.
4. Applies the VoiceNotes Terraform stack.
5. Optionally creates an initial Cognito user.
6. Prints the required DNS CNAME:

```text
notes.example.com -> <voicenotes-alb-dns-name>
```

## Full Wrapper

Run backup plus transcription, and optionally VoiceNotes:

```bash
AWS_PROFILE_NAME=<recipient-profile> \
EXPECTED_ACCOUNT_ID=<recipient-account-id> \
AWS_REGION_NAME=us-west-2 \
PROJECT_NAME=cubicle-transcript-rp \
TRANSCRIPTION_ALLOWED_USERS=user@example.com \
HF_TOKEN_FILE=/path/to/huggingface-token \
DEPLOY_VOICENOTES=true \
VOICENOTES_DOMAIN_NAME=notes.example.com \
VOICENOTES_CERTIFICATE_ARN=arn:aws:acm:us-west-2:<account>:certificate/<id> \
VOICENOTES_ADMIN_EMAIL=user@example.com \
Scripts/setup-fresh-aws-system.sh full
```

If `DEPLOY_VOICENOTES` is not `true`, `full` backs up the source and provisions
only the transcription stack.

## Important Secret Rules

- Never commit or transfer `.env`, `terraform.tfvars`, Terraform state, service
  tokens, signing keys, or Hugging Face tokens.
- Use `HF_TOKEN_FILE` so model-download credentials are read from a local file.
- The transcription setup creates `PROJECT_NAME/service-token` and
  `PROJECT_NAME/user-token-signing-key` in Secrets Manager.
- Cubicle receives only short-lived signed user tokens.
- VoiceNotes receives the upstream transcription service token through Secrets
  Manager, not through a local config file.

## Validation After Provisioning

Check transcription:

```bash
terraform -chdir=infra/transcription output -raw health_endpoint
curl -fsS "$(terraform -chdir=infra/transcription output -raw health_endpoint)"
```

Check the GPU host through SSM:

```bash
AWS_PROFILE_NAME=<recipient-profile> \
EXPECTED_ACCOUNT_ID=<recipient-account-id> \
AWS_REGION_NAME=us-west-2 \
ASG_NAME=cubicle-transcript-rp-gpu \
HF_SECRET_NAME=cubicle-transcript-rp/huggingface-token \
infra/transcription/rebuild-voxtral-vllm-ec2.sh status
```

For detailed runtime checks use:

```bash
infra/transcription/rebuild-voxtral-vllm-ec2.sh status
```

For VoiceNotes:

```bash
curl -fsS https://notes.example.com/healthz
```

Then sign in through the Cognito Hosted UI with the invited user and complete
the temporary-password challenge.

## What Not To Reuse From This Machine

- `infra/transcription/terraform.tfstate`
- `voicenotes/infra/aws/terraform.tfstate`
- `terraform.tfvars`
- `backups/transcription-known-good-*`
- local result dumps
- old ECR image tags as a source of truth
- existing account IDs, certificate ARNs, Cognito pool IDs, or CloudFront IDs

Those files are useful for historical debugging only. A recipient account must
create new Terraform state and new AWS resources.

## Rollback And Teardown

The fresh setup uses dedicated Terraform workspaces. To avoid damaging the
original account, always keep `EXPECTED_ACCOUNT_ID` set.

For a recipient account teardown, select the same workspaces and run Terraform
destroy in reverse order:

```bash
terraform -chdir=voicenotes/infra/aws workspace select <voice-notes-workspace>
terraform -chdir=voicenotes/infra/aws destroy

terraform -chdir=infra/transcription workspace select <transcription-workspace>
terraform -chdir=infra/transcription destroy
```

Delete retained Secrets Manager secrets after their recovery windows only when
the recipient no longer needs the deployment.
