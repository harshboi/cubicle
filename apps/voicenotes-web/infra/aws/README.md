# VoiceNotes AWS Stack

This Terraform is intentionally separate from the existing Cubicle
transcription Terraform. It creates only the VoiceNotes web application layer
and reuses the existing transcription WebSocket as an upstream service.

It creates:

- VoiceNotes ECR repository.
- Cognito user pool and app client.
- HTTPS ALB for `voicenotes.agenticisolation.com`.
- ECS Fargate service for the web app.
- DynamoDB notes and audit tables.
- Private S3 transcript bucket.
- KMS key for transcript objects.
- Secrets Manager session secret reference.

It does not create or modify:

- Cubicle macOS app resources.
- Existing Cubicle transcription ECS service.
- Existing Voxtral/vLLM runtime.
- Existing Cubicle admin console.
- `dcabsri6ekziv.cloudfront.net`.

## Prerequisites

1. Build and push the VoiceNotes image.
2. Create or identify an ACM certificate for
   `voicenotes.agenticisolation.com` in `us-west-2`.
3. Provide VPC, public subnet, and private subnet ids.
4. Provide a Secrets Manager secret ARN containing the upstream transcription
   bearer token.
5. After apply, create the GoDaddy CNAME:

```text
voicenotes -> <alb_dns_name>
```

## Example

```bash
terraform init
terraform plan \
  -var='certificate_arn=arn:aws:acm:us-west-2:562304353751:certificate/...' \
  -var='vpc_id=vpc-...' \
  -var='public_subnet_ids=["subnet-a","subnet-b"]' \
  -var='private_subnet_ids=["subnet-c","subnet-d"]' \
  -var='container_image=562304353751.dkr.ecr.us-west-2.amazonaws.com/voicenotes:latest' \
  -var='upstream_transcription_token_secret_arn=arn:aws:secretsmanager:us-west-2:562304353751:secret:...'
```

