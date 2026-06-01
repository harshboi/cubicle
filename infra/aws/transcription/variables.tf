variable "aws_profile" {
  description = "AWS CLI profile to use. The local deployment profile for this workspace is strln."
  type        = string
  default     = "strln"
}

variable "aws_region" {
  description = "AWS region for the transcription service."
  type        = string
  default     = "us-west-2"
}

variable "expected_account_id" {
  description = "Safety guard so this stack is not accidentally applied to the wrong account."
  type        = string
  default     = "562304353751"
}

variable "project_name" {
  description = "Name prefix for AWS resources."
  type        = string
  default     = "cubicle-transcription"
}

variable "image_uri" {
  description = "Pushed ECR image URI for the transcription service."
  type        = string
  default     = ""
}

variable "container_port" {
  description = "Container HTTP/WebSocket port."
  type        = number
  default     = 8080
}

variable "desired_count" {
  description = "Initial ECS desired task count."
  type        = number
  default     = 1
}

variable "task_cpu" {
  description = "Fargate task CPU units. Increase for pyannote CPU fallback testing."
  type        = number
  default     = 4096
}

variable "task_memory" {
  description = "Fargate task memory in MiB. Increase for pyannote CPU fallback testing."
  type        = number
  default     = 8192
}

variable "ecs_launch_type" {
  description = "ECS launch type. Use FARGATE for mock/API-only staging and EC2 for GPU self-hosted model runtime."
  type        = string
  default     = "FARGATE"

  validation {
    condition     = contains(["FARGATE", "EC2"], var.ecs_launch_type)
    error_message = "ecs_launch_type must be FARGATE or EC2."
  }
}

variable "asr_provider" {
  description = "Runtime ASR provider: mock, voxtral_self_hosted, voxtral_realtime, or faster_whisper."
  type        = string
  default     = "mock"
}

variable "diarization_provider" {
  description = "Runtime diarization provider: mock, remote_http, worker_http, pyannote, or disabled."
  type        = string
  default     = "disabled"
}

variable "enable_diarization_worker" {
  description = "Run a private diarization worker ECS service. The public adapter must use remote_http/worker_http to call it."
  type        = bool
  default     = false
}

variable "diarization_worker_image_uri" {
  description = "Pushed ECR image URI for the private diarization worker."
  type        = string
  default     = ""
}

variable "diarization_worker_provider" {
  description = "Provider used inside the private diarization worker."
  type        = string
  default     = "pyannote"
}

variable "diarization_worker_desired_count" {
  description = "Desired private diarization worker ECS task count."
  type        = number
  default     = 0
}

variable "diarization_worker_launch_type" {
  description = "ECS launch type for the private diarization worker. Use EC2 to place pyannote on the dedicated diarization GPU capacity provider."
  type        = string
  default     = "FARGATE"

  validation {
    condition     = contains(["FARGATE", "EC2"], var.diarization_worker_launch_type)
    error_message = "diarization_worker_launch_type must be FARGATE or EC2."
  }
}

variable "diarization_worker_task_cpu" {
  description = "Private diarization worker task CPU units."
  type        = number
  default     = 4096
}

variable "diarization_worker_task_memory" {
  description = "Private diarization worker task memory in MiB."
  type        = number
  default     = 8192
}

variable "diarization_worker_assign_public_ip" {
  description = "Assign a public IP to private worker tasks for outbound model downloads. Security-group ingress still allows only the transcription task."
  type        = bool
  default     = true
}

variable "diarization_worker_namespace_name" {
  description = "Private Cloud Map namespace for the diarization worker. Empty uses project_name.local."
  type        = string
  default     = ""
}

variable "diarization_worker_discovery_name" {
  description = "Private Cloud Map service name for the diarization worker."
  type        = string
  default     = "diarization-worker"
}

variable "diarization_worker_url" {
  description = "Optional explicit private diarization worker URL. Empty uses the Cloud Map service URL when enable_diarization_worker=true."
  type        = string
  default     = ""
}

variable "diarization_worker_timeout_seconds" {
  description = "Public adapter timeout when calling the private diarization worker."
  type        = number
  default     = 90
}

variable "diarization_worker_gpu_count" {
  description = "Number of GPUs reserved by the private diarization worker when diarization_worker_launch_type=EC2."
  type        = number
  default     = 1
}

variable "diarization_worker_pyannote_device" {
  description = "Pyannote device inside the private diarization worker. Empty uses cuda for EC2 workers and pyannote_device otherwise."
  type        = string
  default     = ""
}

variable "enable_diarization_worker_gpu_capacity" {
  description = "Provision a separate ECS EC2 GPU capacity provider used only by private diarization worker tasks."
  type        = bool
  default     = false
}

variable "diarization_worker_gpu_instance_type" {
  description = "GPU instance type for the private diarization worker capacity provider."
  type        = string
  default     = "g5.xlarge"
}

variable "diarization_worker_gpu_min_size" {
  description = "Minimum size for the private diarization worker GPU Auto Scaling group."
  type        = number
  default     = 0
}

variable "diarization_worker_gpu_desired_capacity" {
  description = "Desired size for the private diarization worker GPU Auto Scaling group."
  type        = number
  default     = 0
}

variable "diarization_worker_gpu_max_size" {
  description = "Maximum size for the private diarization worker GPU Auto Scaling group."
  type        = number
  default     = 1
}

variable "diarization_worker_auth_enabled" {
  description = "Require a bearer token between the public adapter and private diarization worker."
  type        = bool
  default     = true
}

variable "enable_text_intelligence_worker" {
  description = "Run a private text-intelligence worker ECS service for transcript translation, summaries, actions, title generation, and transcript Q&A."
  type        = bool
  default     = false
}

variable "text_intelligence_worker_image_uri" {
  description = "Pushed ECR image URI for the text-intelligence FastAPI worker. Defaults to the main transcription service image."
  type        = string
  default     = ""
}

variable "text_intelligence_runtime_image_uri" {
  description = "Container image URI for the vLLM OpenAI-compatible runtime used by the text-intelligence worker."
  type        = string
  default     = "vllm/vllm-openai:v0.21.0-ubuntu2404"
}

variable "text_intelligence_worker_provider" {
  description = "Provider used inside the text-intelligence worker: vllm, openai_compatible, or mock."
  type        = string
  default     = "vllm"

  validation {
    condition     = contains(["vllm", "openai_compatible", "mock"], var.text_intelligence_worker_provider)
    error_message = "text_intelligence_worker_provider must be vllm, openai_compatible, or mock."
  }
}

variable "text_intelligence_model" {
  description = "Instruction model served by vLLM for translation, summaries, actions, title generation, and transcript Q&A."
  type        = string
  default     = "Qwen/Qwen2.5-7B-Instruct"
}

variable "text_intelligence_worker_desired_count" {
  description = "Desired private text-intelligence worker ECS task count. Use 1 only when translation/summarization is enabled."
  type        = number
  default     = 0
}

variable "text_intelligence_worker_launch_type" {
  description = "ECS launch type for the private text-intelligence worker. Use EC2 for vLLM GPU inference."
  type        = string
  default     = "EC2"

  validation {
    condition     = contains(["FARGATE", "EC2"], var.text_intelligence_worker_launch_type)
    error_message = "text_intelligence_worker_launch_type must be FARGATE or EC2."
  }
}

variable "text_intelligence_worker_task_cpu" {
  description = "Private text-intelligence worker task CPU units."
  type        = number
  default     = 4096
}

variable "text_intelligence_worker_task_memory" {
  description = "Private text-intelligence worker task memory in MiB."
  type        = number
  default     = 15360
}

variable "text_intelligence_worker_assign_public_ip" {
  description = "Assign a public IP to Fargate text-intelligence worker tasks. EC2 GPU tasks ignore this and run without a public task IP."
  type        = bool
  default     = true
}

variable "text_intelligence_worker_namespace_name" {
  description = "Private Cloud Map namespace for the text-intelligence worker. Empty uses project_name-text.local."
  type        = string
  default     = ""
}

variable "text_intelligence_worker_discovery_name" {
  description = "Private Cloud Map service name for the text-intelligence worker."
  type        = string
  default     = "text-intelligence-worker"
}

variable "text_intelligence_worker_url" {
  description = "Optional explicit private text-intelligence worker URL. Empty uses the Cloud Map service URL when enable_text_intelligence_worker=true."
  type        = string
  default     = ""
}

variable "text_intelligence_allowed_security_group_ids" {
  description = "Extra security group IDs allowed to call the private text-intelligence worker, for example the VoiceNotes ECS service security group."
  type        = list(string)
  default     = []
}

variable "text_intelligence_worker_auth_enabled" {
  description = "Require a bearer token for calls to the private text-intelligence worker."
  type        = bool
  default     = true
}

variable "text_intelligence_request_timeout_seconds" {
  description = "Worker request timeout for line translation and transcript Q&A calls to vLLM."
  type        = number
  default     = 20
}

variable "text_intelligence_summary_timeout_seconds" {
  description = "Worker request timeout for transcript summary calls to vLLM."
  type        = number
  default     = 60
}

variable "text_intelligence_max_translation_tokens" {
  description = "Maximum generated tokens for a translated realtime transcript line."
  type        = number
  default     = 160
}

variable "text_intelligence_max_summary_tokens" {
  description = "Maximum generated tokens for summary, actions, decisions, open questions, and title JSON."
  type        = number
  default     = 1200
}

variable "text_intelligence_temperature" {
  description = "Sampling temperature for text-intelligence model calls."
  type        = number
  default     = 0
}

variable "text_intelligence_runtime_gpu_count" {
  description = "Number of GPUs reserved by the vLLM text-intelligence runtime sidecar."
  type        = number
  default     = 1
}

variable "text_intelligence_runtime_max_model_len" {
  description = "Maximum vLLM context length for the text-intelligence runtime. 8192 keeps Qwen2.5-7B practical on g5.xlarge for line translation and compact summaries."
  type        = number
  default     = 8192
}

variable "text_intelligence_runtime_gpu_memory_utilization" {
  description = "vLLM GPU memory utilization for the text-intelligence runtime."
  type        = string
  default     = "0.90"
}

variable "enable_text_intelligence_worker_gpu_capacity" {
  description = "Provision dedicated ECS EC2 GPU capacity for the private text-intelligence worker."
  type        = bool
  default     = false
}

variable "reuse_diarization_worker_gpu_capacity_for_text_intelligence" {
  description = "When text intelligence is enabled and diarization is disabled, place the text-intelligence worker on the released diarization worker GPU capacity provider instead of creating a second GPU capacity provider."
  type        = bool
  default     = true
}

variable "text_intelligence_worker_gpu_instance_type" {
  description = "GPU instance type for the private text-intelligence worker capacity provider."
  type        = string
  default     = "g5.xlarge"
}

variable "text_intelligence_worker_gpu_min_size" {
  description = "Minimum size for the private text-intelligence worker GPU Auto Scaling group."
  type        = number
  default     = 0
}

variable "text_intelligence_worker_gpu_desired_capacity" {
  description = "Desired size for the private text-intelligence worker GPU Auto Scaling group."
  type        = number
  default     = 0
}

variable "text_intelligence_worker_gpu_max_size" {
  description = "Maximum size for the private text-intelligence worker GPU Auto Scaling group."
  type        = number
  default     = 1
}

variable "diarization_stop_timeout_seconds" {
  description = "Maximum seconds to wait for stop-time diarization before returning session_stopped."
  type        = number
  default     = 45
}

variable "diarization_warmup_enabled" {
  description = "Pre-load the pyannote diarization pipeline during service startup to avoid first-stop cold-load timeouts."
  type        = bool
  default     = false
}

variable "auth_mode" {
  description = "Authentication mode: shared_token, signed_user_token, or signed_or_shared."
  type        = string
  default     = "shared_token"

  validation {
    condition     = contains(["shared_token", "signed_user_token", "signed_or_shared"], var.auth_mode)
    error_message = "auth_mode must be shared_token, signed_user_token, or signed_or_shared."
  }
}

variable "allowed_users" {
  description = "Comma-separated user ids or emails allowed to use transcription when signed user tokens are enabled. Empty means any valid signed token."
  type        = string
  default     = ""
}

variable "revoked_token_ids" {
  description = "Comma-separated signed-token jti values to reject."
  type        = string
  default     = ""
}

variable "token_issuer" {
  description = "Expected issuer for signed user transcription tokens."
  type        = string
  default     = "cubicle-transcription"
}

variable "token_audience" {
  description = "Expected audience for signed user transcription tokens."
  type        = string
  default     = "cubicle-macos"
}

variable "required_scope" {
  description = "Required scope claim for signed user transcription tokens."
  type        = string
  default     = "transcription:stream"
}

variable "voxtral_model" {
  description = "Voxtral Realtime model name."
  type        = string
  default     = "mistralai/Voxtral-Mini-4B-Realtime-2602"
}

variable "voxtral_model_version" {
  description = "Voxtral Realtime model version metadata."
  type        = string
  default     = "self-hosted-vllm-2602"
}

variable "voxtral_runtime" {
  description = "Self-hosted Voxtral runtime implementation metadata."
  type        = string
  default     = "vllm"
}

variable "voxtral_realtime_url" {
  description = "Internal Realtime WebSocket endpoint for the self-hosted Voxtral runtime."
  type        = string
  default     = "ws://127.0.0.1:8000/v1/realtime"
}

variable "voxtral_final_response_timeout_seconds" {
  description = "Seconds the adapter waits for final vLLM Realtime transcript events after the final audio commit."
  type        = number
  default     = 30
}

variable "voxtral_runtime_image_uri" {
  description = "Pushed ECR image URI for the self-hosted Voxtral vLLM runtime sidecar."
  type        = string
  default     = ""
}

variable "enable_voxtral_runtime" {
  description = "Run a self-hosted Voxtral vLLM runtime container in the ECS task. Requires EC2 GPU launch type."
  type        = bool
  default     = false
}

variable "voxtral_max_model_len" {
  description = "Maximum vLLM context length for the self-hosted Voxtral runtime. The Docker/SSM g5.xlarge profile has been verified at 45000."
  type        = number
  default     = 45000
}

variable "voxtral_max_num_batched_tokens" {
  description = "Maximum batched tokens for the self-hosted Voxtral runtime."
  type        = number
  default     = 4096
}

variable "voxtral_runtime_gpu_count" {
  description = "Number of GPUs reserved by the Voxtral runtime sidecar."
  type        = number
  default     = 1
}

variable "model_cache_dir" {
  description = "Container path for local model weights/cache."
  type        = string
  default     = "/models"
}

variable "models_offline" {
  description = "Set model runtimes to offline mode after model weights are cached."
  type        = bool
  default     = false
}

variable "whisper_model" {
  description = "faster-whisper model name or local model path."
  type        = string
  default     = "h2oai/faster-whisper-large-v3-turbo"
}

variable "whisper_model_version" {
  description = "Whisper/faster-whisper model version metadata."
  type        = string
  default     = "faster-whisper-large-v3-turbo"
}

variable "whisper_device" {
  description = "faster-whisper device."
  type        = string
  default     = "cuda"
}

variable "whisper_compute_type" {
  description = "faster-whisper compute type."
  type        = string
  default     = "float16"
}

variable "require_gpu" {
  description = "Require CUDA GPU visibility for GPU-capable providers."
  type        = bool
  default     = false
}

variable "pyannote_model" {
  description = "pyannote diarization model."
  type        = string
  default     = "pyannote/speaker-diarization-community-1"
}

variable "pyannote_model_version" {
  description = "pyannote diarization model version metadata."
  type        = string
  default     = "pyannote-audio-4.x"
}

variable "pyannote_device" {
  description = "pyannote diarization device. Use cpu for the Fargate adapter path; use cuda only when the service task has GPU resources."
  type        = string
  default     = "cpu"
}

variable "pyannote_min_speakers" {
  description = "Optional minimum speaker count passed to pyannote. Zero means unset."
  type        = number
  default     = 0
}

variable "pyannote_max_speakers" {
  description = "Optional maximum speaker count passed to pyannote. Zero means unset."
  type        = number
  default     = 0
}

variable "enable_mistral_secret" {
  description = "Create and inject the MISTRAL_API_KEY secret."
  type        = bool
  default     = false
}

variable "enable_pyannote_secret" {
  description = "Create and inject the PYANNOTE_AUTH_TOKEN secret."
  type        = bool
  default     = false
}

variable "pyannote_auth_token_secret_arn" {
  description = "Existing Secrets Manager secret ARN to inject as PYANNOTE_AUTH_TOKEN. Use this to reuse the Hugging Face token secret without creating a second secret."
  type        = string
  default     = ""
}

variable "enable_gpu_capacity" {
  description = "Provision ECS EC2 GPU capacity for self-hosted model runtime."
  type        = bool
  default     = false
}

variable "gpu_instance_type" {
  description = "GPU instance type for ECS EC2 self-hosted model runtime."
  type        = string
  default     = "g5.xlarge"
}

variable "gpu_min_size" {
  description = "Minimum GPU ECS Auto Scaling group size."
  type        = number
  default     = 0
}

variable "gpu_desired_capacity" {
  description = "Desired GPU ECS Auto Scaling group size."
  type        = number
  default     = 0
}

variable "gpu_max_size" {
  description = "Maximum GPU ECS Auto Scaling group size."
  type        = number
  default     = 1
}

variable "enable_admin_console" {
  description = "Provision the private transcription admin console infrastructure. Defaults false so no dashboard is exposed by accident."
  type        = bool
  default     = false
}

variable "enable_public_admin_console" {
  description = "Provision a public HTTPS admin endpoint for cubicle.agenticisolation.com with Cognito username/password auth, WAF, and a private ECS backend. Defaults false."
  type        = bool
  default     = false
}

variable "admin_image_uri" {
  description = "Pushed ECR image URI for the private admin console. Defaults to image_uri/service image."
  type        = string
  default     = ""
}

variable "voicenotes_cognito_user_pool_id" {
  description = "VoiceNotes Cognito user pool ID managed by the admin console. Leave empty to disable VoiceNotes user-management actions."
  type        = string
  default     = ""
}

variable "voicenotes_cognito_region" {
  description = "AWS region for the VoiceNotes Cognito user pool. Defaults to aws_region."
  type        = string
  default     = ""
}

variable "voicenotes_admin_lambda_name" {
  description = "Optional Lambda function used by the admin console for VoiceNotes Cognito user management."
  type        = string
  default     = ""
}

variable "voicenotes_admin_lambda_region" {
  description = "AWS region for voicenotes_admin_lambda_name. Defaults to aws_region."
  type        = string
  default     = ""
}

variable "public_admin_domain_name" {
  description = "Public dashboard hostname protected by Cognito username/password auth and WAF."
  type        = string
  default     = "cubicle.agenticisolation.com"
}

variable "public_admin_certificate_arn" {
  description = "Issued ACM certificate ARN in aws_region for the public admin ALB HTTPS listener."
  type        = string
  default     = ""
}

variable "public_admin_request_certificate" {
  description = "Request an ACM DNS-validated regional certificate for public_admin_domain_name and output the GoDaddy validation CNAMEs. Does not deploy the public console."
  type        = bool
  default     = false
}

variable "public_admin_allowed_cidr_blocks" {
  description = "IPv4 CIDR blocks allowed to reach the public admin ALB. Use 0.0.0.0/0 only with Cognito auth and WAF enabled."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "public_admin_allowed_admin_emails" {
  description = "Cognito email addresses allowed through the admin app after username/password login."
  type        = list(string)
  default     = ["iceinmind@yahoo.com"]
}

variable "public_admin_cognito_domain_prefix" {
  description = "Optional globally unique Cognito hosted UI domain prefix. Leave empty to use a deterministic account-scoped prefix."
  type        = string
  default     = ""
}

variable "public_admin_cognito_session_timeout_seconds" {
  description = "ALB Cognito authentication session lifetime for the public admin endpoint."
  type        = number
  default     = 3600
}

variable "public_admin_waf_rate_limit" {
  description = "AWS WAF rate-based limit per 5-minute window for the public admin endpoint."
  type        = number
  default     = 500
}

variable "admin_domain_name" {
  description = "Private dashboard hostname. This must resolve through Route53 private DNS, not public GoDaddy routing."
  type        = string
  default     = "cubicle.agenticisolation.com"
}

variable "admin_certificate_arn" {
  description = "Issued ACM certificate ARN for the private admin ALB HTTPS listener. Request/validate the cert before enabling the console."
  type        = string
  default     = ""
}

variable "admin_request_certificate" {
  description = "Request an ACM DNS-validated certificate for admin_domain_name and output the GoDaddy validation CNAMEs. Does not deploy the console."
  type        = bool
  default     = false
}

variable "admin_create_private_hosted_zone" {
  description = "Create a Route53 private hosted zone for admin_private_zone_name and associate it with the transcription VPC."
  type        = bool
  default     = false
}

variable "admin_private_hosted_zone_id" {
  description = "Existing Route53 private hosted zone id to use for admin_domain_name. Leave empty when admin_create_private_hosted_zone=true."
  type        = string
  default     = ""
}

variable "admin_private_zone_name" {
  description = "Route53 private hosted zone name used for the admin dashboard."
  type        = string
  default     = "agenticisolation.com"
}

variable "admin_allowed_cidr_blocks" {
  description = "IPv4 CIDR blocks allowed to reach the internal admin ALB, normally AWS Client VPN CIDRs. Never use 0.0.0.0/0."
  type        = list(string)
  default     = []
}

variable "admin_allowed_security_group_ids" {
  description = "Optional security group ids allowed to reach the internal admin ALB."
  type        = list(string)
  default     = []
}

variable "admin_desired_count" {
  description = "Desired private admin ECS task count when enable_admin_console=true. Defaults to 0 so secrets and private access can be staged before the task starts."
  type        = number
  default     = 0
}

variable "admin_task_cpu" {
  description = "Private admin console Fargate task CPU units."
  type        = number
  default     = 512
}

variable "admin_task_memory" {
  description = "Private admin console Fargate task memory in MiB."
  type        = number
  default     = 1024
}

variable "admin_log_retention_days" {
  description = "CloudWatch retention for private admin console logs."
  type        = number
  default     = 30
}

variable "admin_session_ttl_seconds" {
  description = "Admin browser session lifetime."
  type        = number
  default     = 900
}

variable "admin_default_user_token_ttl_seconds" {
  description = "Default issued transcription user-token lifetime from the admin console."
  type        = number
  default     = 2592000
}

variable "admin_user_registry_cache_ttl_seconds" {
  description = "Short cache TTL for dynamic user/token registry decisions."
  type        = number
  default     = 30
}

variable "enforce_service_user_registry" {
  description = "Require transcription tokens to match an active DynamoDB registry user and token-ledger entry. Leave false to allow any correctly signed, unexpired token."
  type        = bool
  default     = false
}

variable "enable_admin_client_vpn" {
  description = "Provision an AWS Client VPN endpoint for admin access. Requires certificate ARNs and remains disabled by default."
  type        = bool
  default     = false
}

variable "admin_client_vpn_server_certificate_arn" {
  description = "ACM server certificate ARN for AWS Client VPN."
  type        = string
  default     = ""
}

variable "admin_client_vpn_root_certificate_chain_arn" {
  description = "ACM client root certificate chain ARN for mutual-auth AWS Client VPN."
  type        = string
  default     = ""
}

variable "admin_client_vpn_client_cidr_block" {
  description = "Client IPv4 CIDR for AWS Client VPN admin users."
  type        = string
  default     = "10.73.0.0/22"
}
