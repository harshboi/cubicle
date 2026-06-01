variable "aws_region" {
  type        = string
  default     = "us-west-2"
  description = "AWS region for VoiceNotes."
}

variable "aws_profile" {
  type        = string
  default     = "strln"
  description = "Local AWS CLI profile used by Terraform."
}

variable "project_name" {
  type        = string
  default     = "voicenotes"
  description = "Resource name prefix."
}

variable "domain_name" {
  type        = string
  default     = "voicenotes.agenticisolation.com"
  description = "Public VoiceNotes hostname."
}

variable "certificate_arn" {
  type        = string
  description = "Issued ACM certificate ARN for domain_name in this region."
}

variable "vpc_id" {
  type        = string
  description = "VPC where the VoiceNotes service runs."
}

variable "public_subnet_ids" {
  type        = list(string)
  description = "Public subnets for the HTTPS ALB."
}

variable "service_subnet_ids" {
  type        = list(string)
  description = "Subnets for the ECS service tasks."
}

variable "service_assign_public_ip" {
  type        = bool
  default     = true
  description = "Assign public IPs to service tasks for outbound Cognito and transcription access when NAT is unavailable."
}

variable "shared_vpce_security_group_id" {
  type        = string
  default     = ""
  description = "Optional shared interface VPC endpoint security group to allow VoiceNotes task access to ECR, logs, and Secrets Manager."
}

variable "container_image" {
  type        = string
  description = "ECR image URI for the VoiceNotes container."
}

variable "container_port" {
  type        = number
  default     = 8080
  description = "VoiceNotes container port."
}

variable "desired_count" {
  type        = number
  default     = 1
  description = "Number of VoiceNotes tasks."
}

variable "upstream_transcription_url" {
  type        = string
  default     = "wss://dcabsri6ekziv.cloudfront.net/v1/transcription"
  description = "Existing Cubicle transcription WebSocket endpoint."
}

variable "upstream_transcription_token_secret_arn" {
  type        = string
  description = "Secrets Manager ARN containing the upstream transcription bearer token."
}

variable "upstream_transcription_signing_secret_arn" {
  type        = string
  default     = ""
  description = "Optional Secrets Manager ARN containing the upstream transcription signed-token HMAC secret. When set, VoiceNotes mints per-user upstream tokens instead of using the shared bearer token."
}

variable "text_intelligence_enabled" {
  type        = bool
  default     = false
  description = "Enable realtime transcript translation and transcript summary generation through the private text-intelligence worker."
}

variable "text_intelligence_url" {
  type        = string
  default     = ""
  description = "Private text-intelligence worker URL, normally the transcription stack text_intelligence_worker_private_url output."
}

variable "text_intelligence_token_secret_arn" {
  type        = string
  default     = ""
  description = "Secrets Manager ARN containing the text-intelligence worker bearer token."
}

variable "text_intelligence_model" {
  type        = string
  default     = "Qwen/Qwen2.5-7B-Instruct"
  description = "Text-intelligence model name used for UI metadata and worker requests."
}

variable "text_intelligence_context_lines" {
  type        = number
  default     = 2
  description = "Number of previous finalized transcript lines sent as context for each realtime translation."
}

variable "text_intelligence_request_timeout_seconds" {
  type        = number
  default     = 12
  description = "VoiceNotes timeout for realtime line translation worker calls."
}

variable "text_intelligence_flush_timeout_seconds" {
  type        = number
  default     = 20
  description = "Maximum seconds to wait for pending line translations before recording finalization."
}

variable "text_intelligence_summary_enabled" {
  type        = bool
  default     = false
  description = "Generate transcript summaries, action items, decisions, open questions, and titles after recording stop."
}

variable "text_intelligence_summary_timeout_seconds" {
  type        = number
  default     = 45
  description = "VoiceNotes timeout for transcript summary worker calls."
}

variable "monthly_minute_quota" {
  type        = number
  default     = 300
  description = "Default monthly recording minute quota per user."
}

variable "max_recording_seconds" {
  type        = number
  default     = 7200
  description = "Maximum single recording duration."
}

variable "cognito_domain_prefix" {
  type        = string
  default     = ""
  description = "Optional Cognito hosted UI domain prefix. Defaults to project/account scoped value."
}
