data "aws_caller_identity" "current" {}

data "aws_availability_zones" "available" {
  state = "available"
}

data "aws_ec2_managed_prefix_list" "cloudfront_origin" {
  name = "com.amazonaws.global.cloudfront.origin-facing"
}

locals {
  name      = var.project_name
  azs       = slice(data.aws_availability_zones.available.names, 0, 2)
  image_uri = var.image_uri != "" ? var.image_uri : "${aws_ecr_repository.service.repository_url}:bootstrap"
  diarization_worker_image_uri = (
    var.diarization_worker_image_uri != ""
    ? var.diarization_worker_image_uri
    : "${aws_ecr_repository.diarization_worker.repository_url}:bootstrap"
  )
  text_intelligence_worker_image_uri = (
    var.text_intelligence_worker_image_uri != ""
    ? var.text_intelligence_worker_image_uri
    : local.image_uri
  )
  admin_image_uri                   = var.admin_image_uri != "" ? var.admin_image_uri : local.image_uri
  voxtral_runtime_image_uri         = var.voxtral_runtime_image_uri != "" ? var.voxtral_runtime_image_uri : "${aws_ecr_repository.voxtral_runtime.repository_url}:bootstrap"
  voxtral_model_parts               = split("/", var.voxtral_model)
  voxtral_model_path                = "/models/voxtral/${element(local.voxtral_model_parts, length(local.voxtral_model_parts) - 1)}"
  diarization_provider_normalized   = lower(var.diarization_provider)
  service_uses_remote_diarization   = contains(["remote_http", "worker_http"], local.diarization_provider_normalized)
  diarization_worker_namespace_name = var.diarization_worker_namespace_name != "" ? var.diarization_worker_namespace_name : "${local.name}.local"
  diarization_worker_dns_name       = "${var.diarization_worker_discovery_name}.${local.diarization_worker_namespace_name}"
  effective_diarization_worker_url  = var.diarization_worker_url != "" ? var.diarization_worker_url : (var.enable_diarization_worker ? "http://${local.diarization_worker_dns_name}:${var.container_port}" : "")
  service_needs_worker_auth_token   = var.diarization_worker_auth_enabled && (local.service_uses_remote_diarization || var.diarization_worker_url != "")
  service_diarization_stop_timeout_seconds = (
    local.service_uses_remote_diarization
    ? max(var.diarization_stop_timeout_seconds, var.diarization_worker_timeout_seconds + 5)
    : var.diarization_stop_timeout_seconds
  )
  text_intelligence_worker_provider_normalized = lower(var.text_intelligence_worker_provider)
  text_intelligence_worker_namespace_name      = var.text_intelligence_worker_namespace_name != "" ? var.text_intelligence_worker_namespace_name : "${local.name}-text.local"
  text_intelligence_worker_dns_name            = "${var.text_intelligence_worker_discovery_name}.${local.text_intelligence_worker_namespace_name}"
  effective_text_intelligence_worker_url       = var.text_intelligence_worker_url != "" ? var.text_intelligence_worker_url : (var.enable_text_intelligence_worker ? "http://${local.text_intelligence_worker_dns_name}:${var.container_port}" : "")
  text_intelligence_worker_uses_ec2            = var.text_intelligence_worker_launch_type == "EC2"
  text_intelligence_worker_uses_runtime        = contains(["vllm", "openai_compatible"], local.text_intelligence_worker_provider_normalized)
  text_intelligence_reuses_diarization_worker_gpu_capacity = (
    var.enable_text_intelligence_worker &&
    var.reuse_diarization_worker_gpu_capacity_for_text_intelligence &&
    var.enable_diarization_worker_gpu_capacity &&
    !var.enable_diarization_worker
  )
  diarization_worker_gpu_capacity_enabled = var.enable_diarization_worker_gpu_capacity && (
    var.enable_diarization_worker || local.text_intelligence_reuses_diarization_worker_gpu_capacity
  )
  text_intelligence_worker_gpu_capacity_enabled = (
    var.enable_text_intelligence_worker &&
    var.enable_text_intelligence_worker_gpu_capacity &&
    !local.text_intelligence_reuses_diarization_worker_gpu_capacity
  )
  text_intelligence_worker_gpu_capacity_available = (
    local.text_intelligence_worker_gpu_capacity_enabled || local.text_intelligence_reuses_diarization_worker_gpu_capacity
  )
  diarization_worker_security_group_needed = var.enable_diarization_worker || local.text_intelligence_reuses_diarization_worker_gpu_capacity
  any_ecs_gpu_capacity                     = var.enable_gpu_capacity || local.diarization_worker_gpu_capacity_enabled || local.text_intelligence_worker_gpu_capacity_enabled
  gpu_capacity_provider_names = concat(
    var.enable_gpu_capacity ? [aws_ecs_capacity_provider.gpu[0].name] : [],
    local.diarization_worker_gpu_capacity_enabled ? [aws_ecs_capacity_provider.diarization_worker_gpu[0].name] : [],
    local.text_intelligence_worker_gpu_capacity_enabled ? [aws_ecs_capacity_provider.text_intelligence_worker_gpu[0].name] : []
  )
  default_gpu_capacity_provider_name = (
    var.enable_gpu_capacity
    ? aws_ecs_capacity_provider.gpu[0].name
    : (
      local.diarization_worker_gpu_capacity_enabled
      ? aws_ecs_capacity_provider.diarization_worker_gpu[0].name
      : (
        local.text_intelligence_worker_gpu_capacity_enabled
        ? aws_ecs_capacity_provider.text_intelligence_worker_gpu[0].name
        : ""
      )
    )
  )
  text_intelligence_worker_capacity_provider_name = (
    local.text_intelligence_reuses_diarization_worker_gpu_capacity
    ? aws_ecs_capacity_provider.diarization_worker_gpu[0].name
    : (
      local.text_intelligence_worker_gpu_capacity_enabled
      ? aws_ecs_capacity_provider.text_intelligence_worker_gpu[0].name
      : ""
    )
  )
  diarization_worker_uses_ec2 = var.diarization_worker_launch_type == "EC2"
  effective_diarization_worker_pyannote_device = (
    var.diarization_worker_pyannote_device != ""
    ? var.diarization_worker_pyannote_device
    : (
      local.diarization_worker_uses_ec2 && lower(var.diarization_worker_provider) == "pyannote"
      ? "cuda"
      : var.pyannote_device
    )
  )
  pyannote_auth_token_secret_arn = (
    var.pyannote_auth_token_secret_arn != ""
    ? var.pyannote_auth_token_secret_arn
    : try(aws_secretsmanager_secret.pyannote_auth_token[0].arn, "")
  )
  service_needs_pyannote_secret = local.diarization_provider_normalized == "pyannote" && local.pyannote_auth_token_secret_arn != ""
  admin_allowed_cidr_blocks     = toset(concat(var.admin_allowed_cidr_blocks, var.enable_admin_client_vpn ? [var.admin_client_vpn_client_cidr_block] : []))
  admin_console_enabled         = var.enable_admin_console || var.enable_public_admin_console
  service_user_registry_enabled = local.admin_console_enabled && var.enforce_service_user_registry
  public_admin_cognito_domain_prefix = (
    var.public_admin_cognito_domain_prefix != ""
    ? var.public_admin_cognito_domain_prefix
    : "${local.name}-admin-${data.aws_caller_identity.current.account_id}"
  )
  public_admin_callback_urls      = ["https://${var.public_admin_domain_name}/oauth2/idpresponse"]
  public_admin_logout_uri         = "https://${var.public_admin_domain_name}/signed-out"
  public_admin_logout_urls        = [local.public_admin_logout_uri]
  public_admin_cognito_logout_url = var.enable_public_admin_console ? "https://${aws_cognito_user_pool_domain.admin[0].domain}.auth.${var.aws_region}.amazoncognito.com/logout?client_id=${aws_cognito_user_pool_client.admin[0].id}&logout_uri=${urlencode(local.public_admin_logout_uri)}" : ""

  common_tags = {
    Project     = var.project_name
    Component   = "transcription"
    ManagedBy   = "terraform"
    Application = "Cubicle"
  }

  container_environment = [
    { name = "TRANSCRIPTION_SERVICE_HOST", value = "0.0.0.0" },
    { name = "TRANSCRIPTION_SERVICE_PORT", value = tostring(var.container_port) },
    { name = "TRANSCRIPTION_ASR_PROVIDER", value = var.asr_provider },
    { name = "TRANSCRIPTION_DIARIZATION_PROVIDER", value = var.diarization_provider },
    { name = "TRANSCRIPTION_DIARIZATION_WORKER_URL", value = local.effective_diarization_worker_url },
    { name = "TRANSCRIPTION_DIARIZATION_WORKER_TIMEOUT_SECONDS", value = tostring(var.diarization_worker_timeout_seconds) },
    { name = "TRANSCRIPTION_DIARIZATION_STOP_TIMEOUT_SECONDS", value = tostring(local.service_diarization_stop_timeout_seconds) },
    { name = "TRANSCRIPTION_DIARIZATION_WARMUP_ENABLED", value = tostring(var.diarization_warmup_enabled) },
    { name = "TRANSCRIPTION_AUTH_MODE", value = var.auth_mode },
    { name = "TRANSCRIPTION_ALLOWED_USERS", value = var.allowed_users },
    { name = "TRANSCRIPTION_REVOKED_TOKEN_IDS", value = var.revoked_token_ids },
    { name = "TRANSCRIPTION_TOKEN_ISSUER", value = var.token_issuer },
    { name = "TRANSCRIPTION_TOKEN_AUDIENCE", value = var.token_audience },
    { name = "TRANSCRIPTION_REQUIRED_SCOPE", value = var.required_scope },
    { name = "TRANSCRIPTION_VOXTRAL_MODEL", value = var.voxtral_model },
    { name = "TRANSCRIPTION_VOXTRAL_MODEL_VERSION", value = var.voxtral_model_version },
    { name = "TRANSCRIPTION_VOXTRAL_RUNTIME", value = var.voxtral_runtime },
    { name = "TRANSCRIPTION_VOXTRAL_REALTIME_URL", value = var.voxtral_realtime_url },
    { name = "TRANSCRIPTION_VOXTRAL_FINAL_RESPONSE_TIMEOUT_SECONDS", value = tostring(var.voxtral_final_response_timeout_seconds) },
    { name = "TRANSCRIPTION_MODEL_CACHE_DIR", value = var.model_cache_dir },
    { name = "TRANSCRIPTION_MODELS_OFFLINE", value = tostring(var.models_offline) },
    { name = "HF_HOME", value = "${var.model_cache_dir}/huggingface" },
    { name = "TRANSFORMERS_CACHE", value = "${var.model_cache_dir}/huggingface" },
    { name = "HF_HUB_ENABLE_HF_TRANSFER", value = "1" },
    { name = "TRANSFORMERS_OFFLINE", value = var.models_offline ? "1" : "0" },
    { name = "HF_HUB_OFFLINE", value = var.models_offline ? "1" : "0" },
    { name = "TRANSCRIPTION_WHISPER_MODEL", value = var.whisper_model },
    { name = "TRANSCRIPTION_WHISPER_MODEL_VERSION", value = var.whisper_model_version },
    { name = "TRANSCRIPTION_WHISPER_DEVICE", value = var.whisper_device },
    { name = "TRANSCRIPTION_WHISPER_COMPUTE_TYPE", value = var.whisper_compute_type },
    { name = "TRANSCRIPTION_PYANNOTE_MODEL", value = var.pyannote_model },
    { name = "TRANSCRIPTION_PYANNOTE_MODEL_VERSION", value = var.pyannote_model_version },
    { name = "TRANSCRIPTION_PYANNOTE_DEVICE", value = var.pyannote_device },
    { name = "TRANSCRIPTION_DIARIZATION_MIN_SPEAKERS", value = var.pyannote_min_speakers > 0 ? tostring(var.pyannote_min_speakers) : "" },
    { name = "TRANSCRIPTION_DIARIZATION_MAX_SPEAKERS", value = var.pyannote_max_speakers > 0 ? tostring(var.pyannote_max_speakers) : "" },
    { name = "PYANNOTE_METRICS_ENABLED", value = "0" },
    { name = "TRANSCRIPTION_REQUIRE_GPU", value = tostring(var.require_gpu) },
    { name = "TRANSCRIPTION_RETENTION", value = "disabled" },
    { name = "TRANSCRIPTION_USER_REGISTRY_BACKEND", value = local.service_user_registry_enabled ? "dynamodb" : "env" },
    { name = "TRANSCRIPTION_USER_REGISTRY_TABLE", value = local.service_user_registry_enabled ? try(aws_dynamodb_table.admin_users[0].name, "") : "" },
    { name = "TRANSCRIPTION_TOKEN_LEDGER_TABLE", value = local.service_user_registry_enabled ? try(aws_dynamodb_table.admin_token_ledger[0].name, "") : "" },
    { name = "TRANSCRIPTION_RUNTIME_CONFIG_TABLE", value = local.admin_console_enabled ? try(aws_dynamodb_table.admin_users[0].name, "") : "" },
    { name = "TRANSCRIPTION_ADMIN_AUDIT_TABLE", value = try(aws_dynamodb_table.admin_audit[0].name, "") },
    { name = "TRANSCRIPTION_USER_REGISTRY_REQUIRE_TOKEN_LEDGER", value = local.service_user_registry_enabled ? "true" : "false" },
    { name = "TRANSCRIPTION_USER_REGISTRY_CACHE_TTL_SECONDS", value = tostring(var.admin_user_registry_cache_ttl_seconds) }
  ]

  container_secrets = concat(
    [
      {
        name      = "TRANSCRIPTION_SERVICE_TOKEN"
        valueFrom = aws_secretsmanager_secret.service_token.arn
      },
      {
        name      = "TRANSCRIPTION_TOKEN_SIGNING_SECRET"
        valueFrom = aws_secretsmanager_secret.user_token_signing_key.arn
      }
    ],
    var.enable_mistral_secret ? [
      {
        name      = "MISTRAL_API_KEY"
        valueFrom = aws_secretsmanager_secret.mistral_api_key[0].arn
      }
    ] : [],
    local.service_needs_pyannote_secret ? [
      {
        name      = "PYANNOTE_AUTH_TOKEN"
        valueFrom = local.pyannote_auth_token_secret_arn
      }
    ] : [],
    local.service_needs_worker_auth_token ? [
      {
        name      = "TRANSCRIPTION_DIARIZATION_WORKER_AUTH_TOKEN"
        valueFrom = aws_secretsmanager_secret.diarization_worker_auth_token.arn
      }
    ] : []
  )

  service_container = merge(
    {
      name      = "service"
      image     = local.image_uri
      essential = true
      portMappings = [{
        containerPort = var.container_port
        hostPort      = var.container_port
        protocol      = "tcp"
      }]
      environment = local.container_environment
      secrets     = local.container_secrets
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.service.name
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "service"
        }
      }
    },
    var.enable_voxtral_runtime ? {
      dependsOn = [{
        containerName = "voxtral-runtime"
        condition     = "HEALTHY"
      }]
    } : {}
  )

  voxtral_runtime_container = {
    name      = "voxtral-runtime"
    image     = local.voxtral_runtime_image_uri
    essential = true
    entryPoint = [
      "vllm"
    ]
    command = [
      "serve", local.voxtral_model_path,
      "--served-model-name", var.voxtral_model,
      "--host", "0.0.0.0",
      "--port", "8000",
      "--tokenizer-mode", "mistral",
      "--max-model-len", tostring(var.voxtral_max_model_len),
      "--max-num-batched-tokens", tostring(var.voxtral_max_num_batched_tokens),
      "--gpu-memory-utilization", "0.90",
      "--compilation_config", "{\"cudagraph_mode\":\"PIECEWISE\"}"
    ]
    portMappings = [{
      containerPort = 8000
      hostPort      = 8000
      protocol      = "tcp"
    }]
    environment = [
      { name = "VOXTRAL_MODEL_ID", value = var.voxtral_model },
      { name = "VOXTRAL_MODEL_PATH", value = local.voxtral_model_path },
      { name = "VOXTRAL_TOKENIZER_MODE", value = "mistral" },
      { name = "VOXTRAL_GPU_MEMORY_UTILIZATION", value = "0.90" },
      { name = "VOXTRAL_ENFORCE_EAGER", value = "false" },
      { name = "VOXTRAL_COMPILATION_CONFIG", value = "{\"cudagraph_mode\":\"PIECEWISE\"}" },
      { name = "VOXTRAL_PORT", value = "8000" },
      { name = "VOXTRAL_MAX_MODEL_LEN", value = tostring(var.voxtral_max_model_len) },
      { name = "VOXTRAL_MAX_NUM_BATCHED_TOKENS", value = tostring(var.voxtral_max_num_batched_tokens) },
      { name = "HF_HOME", value = "${var.model_cache_dir}/huggingface" },
      { name = "TRANSFORMERS_CACHE", value = "${var.model_cache_dir}/huggingface" },
      { name = "HF_HUB_ENABLE_HF_TRANSFER", value = "1" },
      { name = "TRANSFORMERS_OFFLINE", value = var.models_offline ? "1" : "0" },
      { name = "HF_HUB_OFFLINE", value = var.models_offline ? "1" : "0" }
    ]
    resourceRequirements = [{
      type  = "GPU"
      value = tostring(var.voxtral_runtime_gpu_count)
    }]
    healthCheck = {
      command     = ["CMD-SHELL", "curl -fsS http://127.0.0.1:8000/health >/dev/null || exit 1"]
      interval    = 30
      timeout     = 5
      retries     = 10
      startPeriod = 120
    }
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.service.name
        awslogs-region        = var.aws_region
        awslogs-stream-prefix = "voxtral-runtime"
      }
    }
  }

  container_definitions = concat(
    [local.service_container],
    var.enable_voxtral_runtime ? [local.voxtral_runtime_container] : []
  )

  admin_container_definitions = [
    {
      name      = "admin"
      image     = local.admin_image_uri
      essential = true
      portMappings = [{
        containerPort = var.container_port
        hostPort      = var.container_port
        protocol      = "tcp"
      }]
      environment = [
        { name = "TRANSCRIPTION_SERVICE_HOST", value = "0.0.0.0" },
        { name = "TRANSCRIPTION_SERVICE_PORT", value = tostring(var.container_port) },
        { name = "TRANSCRIPTION_ADMIN_ENABLED", value = "true" },
        { name = "TRANSCRIPTION_ADMIN_STORE_BACKEND", value = "dynamodb" },
        { name = "TRANSCRIPTION_ADMIN_COOKIE_SECURE", value = "true" },
        { name = "TRANSCRIPTION_ADMIN_EXTERNAL_AUTH_PROVIDER", value = var.enable_public_admin_console ? "cognito_alb" : "private" },
        { name = "TRANSCRIPTION_ADMIN_COGNITO_LOGOUT_URL", value = local.public_admin_cognito_logout_url },
        { name = "TRANSCRIPTION_ADMIN_ALLOWED_EMAILS", value = var.enable_public_admin_console ? join(",", var.public_admin_allowed_admin_emails) : "" },
        { name = "TRANSCRIPTION_ADMIN_REQUIRED_GROUP", value = "" },
        { name = "TRANSCRIPTION_ADMIN_SESSION_TTL_SECONDS", value = tostring(var.admin_session_ttl_seconds) },
        { name = "TRANSCRIPTION_ADMIN_DEFAULT_USER_TOKEN_TTL_SECONDS", value = tostring(var.admin_default_user_token_ttl_seconds) },
        { name = "TRANSCRIPTION_USER_REGISTRY_BACKEND", value = "dynamodb" },
        { name = "TRANSCRIPTION_USER_REGISTRY_TABLE", value = try(aws_dynamodb_table.admin_users[0].name, "") },
        { name = "TRANSCRIPTION_TOKEN_LEDGER_TABLE", value = try(aws_dynamodb_table.admin_token_ledger[0].name, "") },
        { name = "TRANSCRIPTION_ADMIN_AUDIT_TABLE", value = try(aws_dynamodb_table.admin_audit[0].name, "") },
        { name = "TRANSCRIPTION_USER_REGISTRY_REQUIRE_TOKEN_LEDGER", value = "true" },
        { name = "TRANSCRIPTION_USER_REGISTRY_CACHE_TTL_SECONDS", value = tostring(var.admin_user_registry_cache_ttl_seconds) },
        { name = "TRANSCRIPTION_AUTH_MODE", value = "shared_token" },
        { name = "TRANSCRIPTION_TOKEN_ISSUER", value = var.token_issuer },
        { name = "TRANSCRIPTION_TOKEN_AUDIENCE", value = var.token_audience },
        { name = "TRANSCRIPTION_REQUIRED_SCOPE", value = var.required_scope },
        { name = "VOICENOTES_COGNITO_USER_POOL_ID", value = var.voicenotes_cognito_user_pool_id },
        { name = "VOICENOTES_COGNITO_REGION", value = var.voicenotes_cognito_region != "" ? var.voicenotes_cognito_region : var.aws_region },
        { name = "VOICENOTES_ADMIN_LAMBDA_NAME", value = var.voicenotes_admin_lambda_name },
        { name = "VOICENOTES_ADMIN_LAMBDA_REGION", value = var.voicenotes_admin_lambda_region != "" ? var.voicenotes_admin_lambda_region : var.aws_region },
        { name = "TRANSCRIPTION_RETENTION", value = "disabled" }
      ]
      secrets = [
        {
          name      = "TRANSCRIPTION_SERVICE_TOKEN"
          valueFrom = aws_secretsmanager_secret.service_token.arn
        },
        {
          name      = "TRANSCRIPTION_ADMIN_SESSION_SECRET"
          valueFrom = try(aws_secretsmanager_secret.admin_session_secret[0].arn, "")
        },
        {
          name      = "TRANSCRIPTION_TOKEN_SIGNING_SECRET"
          valueFrom = aws_secretsmanager_secret.user_token_signing_key.arn
        }
      ]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = try(aws_cloudwatch_log_group.admin[0].name, "")
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "admin"
        }
      }
    }
  ]

  diarization_worker_container_definitions = [
    merge({
      name      = "diarization-worker"
      image     = local.diarization_worker_image_uri
      essential = true
      portMappings = [{
        containerPort = var.container_port
        hostPort      = var.container_port
        protocol      = "tcp"
      }]
      environment = [
        { name = "TRANSCRIPTION_SERVICE_HOST", value = "0.0.0.0" },
        { name = "TRANSCRIPTION_SERVICE_PORT", value = tostring(var.container_port) },
        { name = "TRANSCRIPTION_SERVICE_ROLE", value = "diarization_worker" },
        { name = "TRANSCRIPTION_WORKER_DIARIZATION_PROVIDER", value = var.diarization_worker_provider },
        { name = "TRANSCRIPTION_DIARIZATION_PROVIDER", value = var.diarization_worker_provider },
        { name = "TRANSCRIPTION_PYANNOTE_MODEL", value = var.pyannote_model },
        { name = "TRANSCRIPTION_PYANNOTE_MODEL_VERSION", value = var.pyannote_model_version },
        { name = "TRANSCRIPTION_PYANNOTE_DEVICE", value = local.effective_diarization_worker_pyannote_device },
        { name = "TRANSCRIPTION_DIARIZATION_MIN_SPEAKERS", value = var.pyannote_min_speakers > 0 ? tostring(var.pyannote_min_speakers) : "" },
        { name = "TRANSCRIPTION_DIARIZATION_MAX_SPEAKERS", value = var.pyannote_max_speakers > 0 ? tostring(var.pyannote_max_speakers) : "" },
        { name = "PYANNOTE_METRICS_ENABLED", value = "0" },
        { name = "TRANSCRIPTION_RETENTION", value = "disabled" },
        { name = "TRANSCRIPTION_MODEL_CACHE_DIR", value = var.model_cache_dir },
        { name = "TRANSCRIPTION_MODELS_OFFLINE", value = tostring(var.models_offline) },
        { name = "HF_HOME", value = "${var.model_cache_dir}/huggingface" },
        { name = "TRANSFORMERS_CACHE", value = "${var.model_cache_dir}/huggingface" },
        { name = "HF_HUB_ENABLE_HF_TRANSFER", value = "1" },
        { name = "TRANSFORMERS_OFFLINE", value = var.models_offline ? "1" : "0" },
        { name = "HF_HUB_OFFLINE", value = var.models_offline ? "1" : "0" }
      ]
      secrets = concat(
        var.diarization_worker_auth_enabled ? [
          {
            name      = "TRANSCRIPTION_DIARIZATION_WORKER_AUTH_TOKEN"
            valueFrom = aws_secretsmanager_secret.diarization_worker_auth_token.arn
          }
        ] : [],
        local.pyannote_auth_token_secret_arn != "" ? [
          {
            name      = "PYANNOTE_AUTH_TOKEN"
            valueFrom = local.pyannote_auth_token_secret_arn
          }
        ] : []
      )
      healthCheck = {
        command = [
          "CMD-SHELL",
          "python -c \"import json, urllib.request; data=json.load(urllib.request.urlopen('http://127.0.0.1:${var.container_port}/healthz', timeout=2)); raise SystemExit(0 if data.get('status') == 'ok' else 1)\""
        ]
        interval    = 30
        timeout     = 5
        retries     = 3
        startPeriod = 30
      }
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.diarization_worker.name
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "worker"
        }
      }
      }, local.diarization_worker_uses_ec2 ? {
      resourceRequirements = [{
        type  = "GPU"
        value = tostring(var.diarization_worker_gpu_count)
      }]
    } : {})
  ]

  text_intelligence_worker_container = merge(
    {
      name      = "text-intelligence-worker"
      image     = local.text_intelligence_worker_image_uri
      essential = true
      portMappings = [{
        containerPort = var.container_port
        hostPort      = var.container_port
        protocol      = "tcp"
      }]
      environment = [
        { name = "TRANSCRIPTION_SERVICE_HOST", value = "0.0.0.0" },
        { name = "TRANSCRIPTION_SERVICE_PORT", value = tostring(var.container_port) },
        { name = "TRANSCRIPTION_SERVICE_ROLE", value = "text_intelligence_worker" },
        { name = "TEXT_INTELLIGENCE_PROVIDER", value = var.text_intelligence_worker_provider },
        { name = "TEXT_INTELLIGENCE_MODEL", value = var.text_intelligence_model },
        { name = "TEXT_INTELLIGENCE_VLLM_BASE_URL", value = "http://127.0.0.1:8000" },
        { name = "TEXT_INTELLIGENCE_REQUEST_TIMEOUT_SECONDS", value = tostring(var.text_intelligence_request_timeout_seconds) },
        { name = "TEXT_INTELLIGENCE_SUMMARY_TIMEOUT_SECONDS", value = tostring(var.text_intelligence_summary_timeout_seconds) },
        { name = "TEXT_INTELLIGENCE_MAX_TRANSLATION_TOKENS", value = tostring(var.text_intelligence_max_translation_tokens) },
        { name = "TEXT_INTELLIGENCE_MAX_SUMMARY_TOKENS", value = tostring(var.text_intelligence_max_summary_tokens) },
        { name = "TEXT_INTELLIGENCE_TEMPERATURE", value = tostring(var.text_intelligence_temperature) },
        { name = "TEXT_INTELLIGENCE_WARMUP_ENABLED", value = "true" },
        { name = "TRANSCRIPTION_RETENTION", value = "disabled" },
        { name = "TRANSCRIPTION_MODEL_CACHE_DIR", value = var.model_cache_dir },
        { name = "TRANSCRIPTION_MODELS_OFFLINE", value = tostring(var.models_offline) },
        { name = "HF_HOME", value = "${var.model_cache_dir}/huggingface" },
        { name = "TRANSFORMERS_CACHE", value = "${var.model_cache_dir}/huggingface" },
        { name = "HF_HUB_ENABLE_HF_TRANSFER", value = "1" },
        { name = "TRANSFORMERS_OFFLINE", value = var.models_offline ? "1" : "0" },
        { name = "HF_HUB_OFFLINE", value = var.models_offline ? "1" : "0" }
      ]
      secrets = var.text_intelligence_worker_auth_enabled ? [
        {
          name      = "TEXT_INTELLIGENCE_WORKER_AUTH_TOKEN"
          valueFrom = aws_secretsmanager_secret.text_intelligence_worker_auth_token.arn
        }
      ] : []
      healthCheck = {
        command = [
          "CMD-SHELL",
          "python -c \"import json, urllib.request; data=json.load(urllib.request.urlopen('http://127.0.0.1:${var.container_port}/healthz', timeout=2)); raise SystemExit(0 if data.get('status') == 'ok' else 1)\""
        ]
        interval    = 30
        timeout     = 5
        retries     = 3
        startPeriod = 30
      }
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.text_intelligence_worker.name
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "worker"
        }
      }
    },
    local.text_intelligence_worker_uses_runtime ? {
      dependsOn = [{
        containerName = "text-intelligence-runtime"
        condition     = "HEALTHY"
      }]
    } : {}
  )

  text_intelligence_runtime_container = {
    name      = "text-intelligence-runtime"
    image     = var.text_intelligence_runtime_image_uri
    essential = true
    entryPoint = [
      "vllm"
    ]
    command = [
      "serve", var.text_intelligence_model,
      "--served-model-name", var.text_intelligence_model,
      "--host", "0.0.0.0",
      "--port", "8000",
      "--max-model-len", tostring(var.text_intelligence_runtime_max_model_len),
      "--gpu-memory-utilization", var.text_intelligence_runtime_gpu_memory_utilization
    ]
    portMappings = [{
      containerPort = 8000
      hostPort      = 8000
      protocol      = "tcp"
    }]
    mountPoints = [{
      sourceVolume  = "text-intelligence-model-cache"
      containerPath = var.model_cache_dir
      readOnly      = false
    }]
    environment = [
      { name = "TEXT_INTELLIGENCE_MODEL", value = var.text_intelligence_model },
      { name = "HF_HOME", value = "${var.model_cache_dir}/huggingface" },
      { name = "TRANSFORMERS_CACHE", value = "${var.model_cache_dir}/huggingface" },
      { name = "HF_HUB_ENABLE_HF_TRANSFER", value = "1" },
      { name = "TRANSFORMERS_OFFLINE", value = var.models_offline ? "1" : "0" },
      { name = "HF_HUB_OFFLINE", value = var.models_offline ? "1" : "0" },
      { name = "VLLM_DISABLE_COMPILE_CACHE", value = "1" },
      { name = "VLLM_CACHE_ROOT", value = "/tmp/vllm" }
    ]
    secrets = local.pyannote_auth_token_secret_arn != "" ? [
      {
        name      = "HF_TOKEN"
        valueFrom = local.pyannote_auth_token_secret_arn
      }
    ] : []
    resourceRequirements = [{
      type  = "GPU"
      value = tostring(var.text_intelligence_runtime_gpu_count)
    }]
    healthCheck = {
      command     = ["CMD-SHELL", "python3 -c \"import urllib.request; urllib.request.urlopen('http://127.0.0.1:8000/health', timeout=2).read()\""]
      interval    = 30
      timeout     = 5
      retries     = 10
      startPeriod = 180
    }
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.text_intelligence_worker.name
        awslogs-region        = var.aws_region
        awslogs-stream-prefix = "runtime"
      }
    }
  }

  text_intelligence_worker_container_definitions = concat(
    [local.text_intelligence_worker_container],
    local.text_intelligence_worker_uses_runtime ? [local.text_intelligence_runtime_container] : []
  )
}

resource "terraform_data" "account_guard" {
  input = data.aws_caller_identity.current.account_id

  lifecycle {
    precondition {
      condition     = data.aws_caller_identity.current.account_id == var.expected_account_id
      error_message = "Refusing to apply transcription stack to account ${data.aws_caller_identity.current.account_id}; expected ${var.expected_account_id}."
    }
  }
}

resource "terraform_data" "admin_console_guard" {
  count = var.enable_admin_console ? 1 : 0
  input = var.admin_domain_name

  lifecycle {
    precondition {
      condition     = var.admin_certificate_arn != ""
      error_message = "enable_admin_console=true requires admin_certificate_arn for an issued ACM certificate. Use admin_request_certificate=true first if you need validation CNAMEs."
    }
    precondition {
      condition     = length(local.admin_allowed_cidr_blocks) + length(var.admin_allowed_security_group_ids) > 0
      error_message = "enable_admin_console=true requires at least one explicit private/VPN admin CIDR or security group."
    }
    precondition {
      condition     = !contains(local.admin_allowed_cidr_blocks, "0.0.0.0/0") && !contains(local.admin_allowed_cidr_blocks, "::/0")
      error_message = "The private admin console must not allow public 0.0.0.0/0 or ::/0 ingress."
    }
    precondition {
      condition     = var.admin_create_private_hosted_zone || var.admin_private_hosted_zone_id != ""
      error_message = "enable_admin_console=true requires admin_create_private_hosted_zone=true or an existing admin_private_hosted_zone_id."
    }
  }
}

resource "terraform_data" "admin_client_vpn_guard" {
  count = var.enable_admin_client_vpn ? 1 : 0
  input = var.admin_client_vpn_client_cidr_block

  lifecycle {
    precondition {
      condition     = var.admin_client_vpn_server_certificate_arn != "" && var.admin_client_vpn_root_certificate_chain_arn != ""
      error_message = "enable_admin_client_vpn=true requires server and client root certificate ARNs."
    }
    precondition {
      condition     = var.enable_admin_console
      error_message = "enable_admin_client_vpn=true requires enable_admin_console=true so the private admin target exists."
    }
  }
}

resource "terraform_data" "worker_gpu_repurpose_guard" {
  count = local.diarization_worker_gpu_capacity_enabled || local.text_intelligence_worker_gpu_capacity_enabled ? 1 : 0
  input = {
    diarization_gpu_worker_enabled             = var.enable_diarization_worker && local.diarization_worker_gpu_capacity_enabled
    text_intelligence_gpu_worker_enabled       = local.text_intelligence_worker_gpu_capacity_available
    text_intelligence_reuses_diarization_gpu   = local.text_intelligence_reuses_diarization_worker_gpu_capacity
    separate_text_intelligence_gpu_asg_enabled = local.text_intelligence_worker_gpu_capacity_enabled
  }

  lifecycle {
    precondition {
      condition     = !(var.enable_diarization_worker && var.enable_text_intelligence_worker && local.diarization_worker_gpu_capacity_enabled)
      error_message = "Do not run diarization and text-intelligence GPU workers together. Disable diarization worker GPU capacity before enabling text-intelligence GPU capacity."
    }
  }
}

resource "terraform_data" "public_admin_console_guard" {
  count = var.enable_public_admin_console ? 1 : 0
  input = var.public_admin_domain_name

  lifecycle {
    precondition {
      condition     = var.public_admin_certificate_arn != ""
      error_message = "enable_public_admin_console=true requires public_admin_certificate_arn for an issued ACM certificate in the stack region. Use public_admin_request_certificate=true first if you need GoDaddy validation CNAMEs."
    }
    precondition {
      condition     = length(var.public_admin_allowed_cidr_blocks) > 0
      error_message = "enable_public_admin_console=true requires at least one explicit HTTPS ingress CIDR. Use 0.0.0.0/0 only with Cognito MFA and WAF."
    }
  }
}

resource "aws_ecr_repository" "service" {
  name                 = "${local.name}-service"
  image_tag_mutability = "MUTABLE"
  force_delete         = false

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "AES256"
  }

  tags = local.common_tags
}

resource "aws_ecr_repository" "voxtral_runtime" {
  name                 = "${local.name}-voxtral-runtime"
  image_tag_mutability = "IMMUTABLE"
  force_delete         = false

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "AES256"
  }

  tags = local.common_tags
}

resource "aws_ecr_repository" "diarization_worker" {
  name                 = "${local.name}-diarization-worker"
  image_tag_mutability = "MUTABLE"
  force_delete         = false

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "AES256"
  }

  tags = merge(local.common_tags, { Component = "transcription-diarization-worker" })
}

resource "aws_cloudwatch_log_group" "service" {
  name              = "/aws/ecs/${local.name}"
  retention_in_days = 14
  tags              = local.common_tags
}

resource "aws_cloudwatch_log_group" "diarization_worker" {
  name              = "/aws/ecs/${local.name}-diarization-worker"
  retention_in_days = 14
  tags              = merge(local.common_tags, { Component = "transcription-diarization-worker" })
}

resource "aws_cloudwatch_log_group" "text_intelligence_worker" {
  name              = "/aws/ecs/${local.name}-text-intelligence-worker"
  retention_in_days = 14
  tags              = merge(local.common_tags, { Component = "transcription-text-intelligence-worker" })
}

resource "aws_secretsmanager_secret" "service_token" {
  name                    = "${local.name}/service-token"
  recovery_window_in_days = 0
  tags                    = local.common_tags
}

resource "aws_secretsmanager_secret" "user_token_signing_key" {
  name                    = "${local.name}/user-token-signing-key"
  recovery_window_in_days = 0
  tags                    = local.common_tags
}

resource "aws_secretsmanager_secret" "mistral_api_key" {
  count                   = var.enable_mistral_secret ? 1 : 0
  name                    = "${local.name}/mistral-api-key"
  recovery_window_in_days = 0
  tags                    = local.common_tags
}

resource "aws_secretsmanager_secret" "pyannote_auth_token" {
  count                   = var.enable_pyannote_secret ? 1 : 0
  name                    = "${local.name}/pyannote-auth-token"
  recovery_window_in_days = 0
  tags                    = local.common_tags
}

resource "aws_secretsmanager_secret" "diarization_worker_auth_token" {
  name                    = "${local.name}/diarization-worker-auth-token"
  recovery_window_in_days = 0
  tags                    = merge(local.common_tags, { Component = "transcription-diarization-worker" })
}

resource "aws_secretsmanager_secret" "text_intelligence_worker_auth_token" {
  name                    = "${local.name}/text-intelligence-worker-auth-token"
  recovery_window_in_days = 0
  tags                    = merge(local.common_tags, { Component = "transcription-text-intelligence-worker" })
}

resource "aws_secretsmanager_secret" "admin_session_secret" {
  count                   = local.admin_console_enabled ? 1 : 0
  name                    = "${local.name}/admin-session-secret"
  recovery_window_in_days = 0
  tags                    = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_acm_certificate" "admin" {
  count             = var.admin_request_certificate ? 1 : 0
  domain_name       = var.admin_domain_name
  validation_method = "DNS"
  tags              = merge(local.common_tags, { Component = "transcription-admin" })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_acm_certificate" "public_admin" {
  count             = var.public_admin_request_certificate ? 1 : 0
  domain_name       = var.public_admin_domain_name
  validation_method = "DNS"
  tags              = merge(local.common_tags, { Component = "transcription-admin-public" })

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_cognito_user_pool" "admin" {
  count = var.enable_public_admin_console ? 1 : 0
  name  = "${local.name}-admin"

  username_attributes      = ["email"]
  auto_verified_attributes = ["email"]
  mfa_configuration        = "OFF"

  admin_create_user_config {
    allow_admin_create_user_only = true
  }

  password_policy {
    minimum_length                   = 14
    require_lowercase                = true
    require_numbers                  = true
    require_symbols                  = true
    require_uppercase                = true
    temporary_password_validity_days = 7
  }

  account_recovery_setting {
    recovery_mechanism {
      name     = "verified_email"
      priority = 1
    }
  }

  tags = merge(local.common_tags, { Component = "transcription-admin-public" })
}

resource "aws_cognito_user_pool_client" "admin" {
  count        = var.enable_public_admin_console ? 1 : 0
  name         = "${local.name}-admin-alb"
  user_pool_id = aws_cognito_user_pool.admin[0].id

  generate_secret                      = true
  allowed_oauth_flows_user_pool_client = true
  allowed_oauth_flows                  = ["code"]
  allowed_oauth_scopes                 = ["openid", "email", "profile"]
  callback_urls                        = local.public_admin_callback_urls
  logout_urls                          = local.public_admin_logout_urls
  supported_identity_providers         = ["COGNITO"]
  prevent_user_existence_errors        = "ENABLED"
  enable_token_revocation              = true

  access_token_validity  = 60
  id_token_validity      = 60
  refresh_token_validity = 1

  token_validity_units {
    access_token  = "minutes"
    id_token      = "minutes"
    refresh_token = "days"
  }
}

resource "aws_cognito_user_pool_domain" "admin" {
  count        = var.enable_public_admin_console ? 1 : 0
  domain       = local.public_admin_cognito_domain_prefix
  user_pool_id = aws_cognito_user_pool.admin[0].id
}

resource "aws_cognito_user_group" "admin" {
  count        = var.enable_public_admin_console ? 1 : 0
  name         = "CubicleTranscriptionAdmins"
  user_pool_id = aws_cognito_user_pool.admin[0].id
  description  = "Users allowed to sign in to the Cubicle transcription admin console"
  precedence   = 1
}

resource "aws_wafv2_web_acl" "admin_public" {
  count = var.enable_public_admin_console ? 1 : 0
  name  = "${local.name}-admin-pub"
  scope = "REGIONAL"

  default_action {
    allow {}
  }

  rule {
    name     = "AWSManagedRulesAmazonIpReputationList"
    priority = 10

    override_action {
      none {}
    }

    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesAmazonIpReputationList"
        vendor_name = "AWS"
      }
    }

    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${local.name}-admin-ip-reputation"
      sampled_requests_enabled   = true
    }
  }

  rule {
    name     = "AWSManagedRulesCommonRuleSet"
    priority = 20

    override_action {
      none {}
    }

    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesCommonRuleSet"
        vendor_name = "AWS"
      }
    }

    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${local.name}-admin-common"
      sampled_requests_enabled   = true
    }
  }

  rule {
    name     = "AWSManagedRulesKnownBadInputsRuleSet"
    priority = 30

    override_action {
      none {}
    }

    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesKnownBadInputsRuleSet"
        vendor_name = "AWS"
      }
    }

    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${local.name}-admin-known-bad-inputs"
      sampled_requests_enabled   = true
    }
  }

  rule {
    name     = "RateLimitAdmin"
    priority = 40

    action {
      block {}
    }

    statement {
      rate_based_statement {
        limit              = var.public_admin_waf_rate_limit
        aggregate_key_type = "IP"
      }
    }

    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${local.name}-admin-rate-limit"
      sampled_requests_enabled   = true
    }
  }

  visibility_config {
    cloudwatch_metrics_enabled = true
    metric_name                = "${local.name}-admin-public"
    sampled_requests_enabled   = true
  }

  tags = merge(local.common_tags, { Component = "transcription-admin-public" })
}

resource "aws_cloudwatch_log_group" "admin" {
  count             = local.admin_console_enabled ? 1 : 0
  name              = "/aws/ecs/${local.name}-admin"
  retention_in_days = var.admin_log_retention_days
  tags              = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_dynamodb_table" "admin_users" {
  count                       = local.admin_console_enabled ? 1 : 0
  name                        = "${local.name}-users"
  billing_mode                = "PAY_PER_REQUEST"
  hash_key                    = "pk"
  deletion_protection_enabled = true

  attribute {
    name = "pk"
    type = "S"
  }

  point_in_time_recovery {
    enabled = true
  }

  server_side_encryption {
    enabled = true
  }

  tags = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_dynamodb_table" "admin_token_ledger" {
  count                       = local.admin_console_enabled ? 1 : 0
  name                        = "${local.name}-token-ledger"
  billing_mode                = "PAY_PER_REQUEST"
  hash_key                    = "pk"
  range_key                   = "sk"
  deletion_protection_enabled = true

  attribute {
    name = "pk"
    type = "S"
  }

  attribute {
    name = "sk"
    type = "S"
  }

  point_in_time_recovery {
    enabled = true
  }

  server_side_encryption {
    enabled = true
  }

  tags = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_dynamodb_table" "admin_audit" {
  count                       = local.admin_console_enabled ? 1 : 0
  name                        = "${local.name}-admin-audit"
  billing_mode                = "PAY_PER_REQUEST"
  hash_key                    = "pk"
  range_key                   = "sk"
  deletion_protection_enabled = true

  attribute {
    name = "pk"
    type = "S"
  }

  attribute {
    name = "sk"
    type = "S"
  }

  point_in_time_recovery {
    enabled = true
  }

  server_side_encryption {
    enabled = true
  }

  tags = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_vpc" "service" {
  cidr_block           = "10.72.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true
  tags                 = merge(local.common_tags, { Name = "${local.name}-vpc" })
}

resource "aws_internet_gateway" "service" {
  vpc_id = aws_vpc.service.id
  tags   = merge(local.common_tags, { Name = "${local.name}-igw" })
}

resource "aws_subnet" "public" {
  for_each = { for index, az in local.azs : az => index }

  vpc_id                  = aws_vpc.service.id
  availability_zone       = each.key
  cidr_block              = cidrsubnet(aws_vpc.service.cidr_block, 8, each.value)
  map_public_ip_on_launch = true
  tags                    = merge(local.common_tags, { Name = "${local.name}-public-${each.key}" })
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.service.id
  tags   = merge(local.common_tags, { Name = "${local.name}-public-rt" })
}

resource "aws_route" "internet" {
  route_table_id         = aws_route_table.public.id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.service.id
}

resource "aws_route_table_association" "public" {
  for_each       = aws_subnet.public
  subnet_id      = each.value.id
  route_table_id = aws_route_table.public.id
}

resource "aws_subnet" "admin_private" {
  for_each = local.admin_console_enabled ? { for index, az in local.azs : az => index } : {}

  vpc_id                  = aws_vpc.service.id
  availability_zone       = each.key
  cidr_block              = cidrsubnet(aws_vpc.service.cidr_block, 8, 100 + each.value)
  map_public_ip_on_launch = false
  tags                    = merge(local.common_tags, { Name = "${local.name}-admin-private-${each.key}", Component = "transcription-admin" })
}

resource "aws_route_table" "admin_private" {
  for_each = local.admin_console_enabled ? { for index, az in local.azs : az => index } : {}

  vpc_id = aws_vpc.service.id
  tags   = merge(local.common_tags, { Name = "${local.name}-admin-private-rt-${each.key}", Component = "transcription-admin" })
}

resource "aws_route_table_association" "admin_private" {
  for_each = aws_subnet.admin_private

  subnet_id      = each.value.id
  route_table_id = aws_route_table.admin_private[each.key].id
}

resource "aws_security_group" "alb" {
  name        = "${local.name}-alb"
  description = "CloudFront and smoke-test ingress to Cubicle transcription ALB"
  vpc_id      = aws_vpc.service.id
  tags        = local.common_tags
}

resource "aws_vpc_security_group_ingress_rule" "alb_http" {
  security_group_id = aws_security_group.alb.id
  ip_protocol       = "tcp"
  from_port         = 80
  to_port           = 80
  prefix_list_id    = data.aws_ec2_managed_prefix_list.cloudfront_origin.id
  description       = "HTTP origin restricted to AWS CloudFront origin-facing addresses"
}

resource "aws_vpc_security_group_egress_rule" "alb_all" {
  security_group_id = aws_security_group.alb.id
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_security_group" "task" {
  name        = "${local.name}-task"
  description = "Only ALB can reach transcription tasks"
  vpc_id      = aws_vpc.service.id
  tags        = local.common_tags
}

resource "aws_security_group" "diarization_worker" {
  count       = local.diarization_worker_security_group_needed ? 1 : 0
  name        = "${local.name}-diarization-worker"
  description = "Only transcription adapter tasks can reach private diarization workers"
  vpc_id      = aws_vpc.service.id
  tags        = merge(local.common_tags, { Component = "transcription-diarization-worker" })
}

resource "aws_security_group" "text_intelligence_worker" {
  count       = var.enable_text_intelligence_worker ? 1 : 0
  name        = "${local.name}-text-intelligence-worker"
  description = "Only approved app tasks can reach private text-intelligence workers"
  vpc_id      = aws_vpc.service.id
  tags        = merge(local.common_tags, { Component = "transcription-text-intelligence-worker" })
}

resource "aws_vpc_security_group_ingress_rule" "task_from_alb" {
  security_group_id            = aws_security_group.task.id
  referenced_security_group_id = aws_security_group.alb.id
  ip_protocol                  = "tcp"
  from_port                    = var.container_port
  to_port                      = var.container_port
}

resource "aws_vpc_security_group_ingress_rule" "diarization_worker_from_task" {
  count                        = var.enable_diarization_worker ? 1 : 0
  security_group_id            = aws_security_group.diarization_worker[0].id
  referenced_security_group_id = aws_security_group.task.id
  ip_protocol                  = "tcp"
  from_port                    = var.container_port
  to_port                      = var.container_port
  description                  = "Allow only app-facing transcription tasks to call the private diarization worker"
}

resource "aws_vpc_security_group_ingress_rule" "text_intelligence_worker_from_task" {
  count                        = var.enable_text_intelligence_worker ? 1 : 0
  security_group_id            = aws_security_group.text_intelligence_worker[0].id
  referenced_security_group_id = aws_security_group.task.id
  ip_protocol                  = "tcp"
  from_port                    = var.container_port
  to_port                      = var.container_port
  description                  = "Allow transcription adapter tasks to call the private text-intelligence worker"
}

resource "aws_vpc_security_group_ingress_rule" "text_intelligence_worker_from_allowed_sg" {
  for_each = var.enable_text_intelligence_worker ? toset(var.text_intelligence_allowed_security_group_ids) : toset([])

  security_group_id            = aws_security_group.text_intelligence_worker[0].id
  referenced_security_group_id = each.value
  ip_protocol                  = "tcp"
  from_port                    = var.container_port
  to_port                      = var.container_port
  description                  = "Allow an approved application security group to call the private text-intelligence worker"
}

resource "aws_vpc_security_group_ingress_rule" "task_to_private_vllm" {
  security_group_id            = aws_security_group.task.id
  referenced_security_group_id = aws_security_group.task.id
  ip_protocol                  = "tcp"
  from_port                    = 8000
  to_port                      = 8000
  description                  = "Allow transcription adapter tasks to reach private vLLM runtime hosts on port 8000"
}

resource "aws_vpc_security_group_egress_rule" "task_all" {
  security_group_id = aws_security_group.task.id
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_vpc_security_group_egress_rule" "diarization_worker_all" {
  count             = local.diarization_worker_security_group_needed ? 1 : 0
  security_group_id = aws_security_group.diarization_worker[0].id
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_vpc_security_group_egress_rule" "text_intelligence_worker_all" {
  count             = var.enable_text_intelligence_worker ? 1 : 0
  security_group_id = aws_security_group.text_intelligence_worker[0].id
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_security_group" "admin_alb" {
  count       = var.enable_admin_console ? 1 : 0
  name        = "${local.name}-admin-alb"
  description = "Private VPN/internal ingress to Cubicle transcription admin ALB"
  vpc_id      = aws_vpc.service.id
  tags        = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_vpc_security_group_ingress_rule" "admin_alb_from_cidr" {
  for_each = var.enable_admin_console ? local.admin_allowed_cidr_blocks : toset([])

  security_group_id = aws_security_group.admin_alb[0].id
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  cidr_ipv4         = each.value
  description       = "Private admin HTTPS ingress from approved VPN/admin CIDR"
}

resource "aws_vpc_security_group_ingress_rule" "admin_alb_from_sg" {
  for_each = var.enable_admin_console ? toset(var.admin_allowed_security_group_ids) : toset([])

  security_group_id            = aws_security_group.admin_alb[0].id
  referenced_security_group_id = each.value
  ip_protocol                  = "tcp"
  from_port                    = 443
  to_port                      = 443
  description                  = "Private admin HTTPS ingress from approved admin security group"
}

resource "aws_vpc_security_group_egress_rule" "admin_alb_all" {
  count             = var.enable_admin_console ? 1 : 0
  security_group_id = aws_security_group.admin_alb[0].id
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_security_group" "admin_public_alb" {
  count       = var.enable_public_admin_console ? 1 : 0
  name        = "${local.name}-admin-public-alb"
  description = "Public HTTPS ingress to Cubicle transcription admin, protected by Cognito MFA and WAF"
  vpc_id      = aws_vpc.service.id
  tags        = merge(local.common_tags, { Component = "transcription-admin-public" })
}

resource "aws_vpc_security_group_ingress_rule" "admin_public_alb_from_cidr" {
  for_each = var.enable_public_admin_console ? toset(var.public_admin_allowed_cidr_blocks) : toset([])

  security_group_id = aws_security_group.admin_public_alb[0].id
  ip_protocol       = "tcp"
  from_port         = 443
  to_port           = 443
  cidr_ipv4         = each.value
  description       = "Public admin HTTPS ingress; Cognito MFA and WAF are enforced before forwarding"
}

resource "aws_vpc_security_group_egress_rule" "admin_public_alb_all" {
  count             = var.enable_public_admin_console ? 1 : 0
  security_group_id = aws_security_group.admin_public_alb[0].id
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_security_group" "admin_task" {
  count       = local.admin_console_enabled ? 1 : 0
  name        = "${local.name}-admin-task"
  description = "Only private admin ALB can reach admin console tasks"
  vpc_id      = aws_vpc.service.id
  tags        = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_vpc_security_group_ingress_rule" "admin_task_from_alb" {
  count                        = var.enable_admin_console ? 1 : 0
  security_group_id            = aws_security_group.admin_task[0].id
  referenced_security_group_id = aws_security_group.admin_alb[0].id
  ip_protocol                  = "tcp"
  from_port                    = var.container_port
  to_port                      = var.container_port
}

resource "aws_vpc_security_group_ingress_rule" "admin_task_from_public_alb" {
  count                        = var.enable_public_admin_console ? 1 : 0
  security_group_id            = aws_security_group.admin_task[0].id
  referenced_security_group_id = aws_security_group.admin_public_alb[0].id
  ip_protocol                  = "tcp"
  from_port                    = var.container_port
  to_port                      = var.container_port
  description                  = "Allow only the Cognito/WAF-protected public admin ALB to reach admin tasks"
}

resource "aws_vpc_security_group_egress_rule" "admin_task_all" {
  count             = local.admin_console_enabled ? 1 : 0
  security_group_id = aws_security_group.admin_task[0].id
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_security_group" "admin_vpc_endpoint" {
  count       = local.admin_console_enabled ? 1 : 0
  name        = "${local.name}-admin-vpce"
  description = "Private admin task access to AWS service VPC endpoints"
  vpc_id      = aws_vpc.service.id
  tags        = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_vpc_security_group_ingress_rule" "admin_vpce_from_task" {
  count                        = local.admin_console_enabled ? 1 : 0
  security_group_id            = aws_security_group.admin_vpc_endpoint[0].id
  referenced_security_group_id = aws_security_group.admin_task[0].id
  ip_protocol                  = "tcp"
  from_port                    = 443
  to_port                      = 443
  description                  = "Allow private admin tasks to reach interface endpoints"
}

resource "aws_vpc_security_group_ingress_rule" "admin_vpce_from_transcription_task" {
  count                        = local.admin_console_enabled ? 1 : 0
  security_group_id            = aws_security_group.admin_vpc_endpoint[0].id
  referenced_security_group_id = aws_security_group.task.id
  ip_protocol                  = "tcp"
  from_port                    = 443
  to_port                      = 443
  description                  = "Allow app-facing transcription tasks to reach shared AWS interface endpoints"
}

resource "aws_vpc_security_group_ingress_rule" "admin_vpce_from_diarization_worker" {
  count                        = local.admin_console_enabled && local.diarization_worker_security_group_needed ? 1 : 0
  security_group_id            = aws_security_group.admin_vpc_endpoint[0].id
  referenced_security_group_id = aws_security_group.diarization_worker[0].id
  ip_protocol                  = "tcp"
  from_port                    = 443
  to_port                      = 443
  description                  = "Allow private diarization workers to reach shared AWS interface endpoints"
}

resource "aws_vpc_security_group_ingress_rule" "admin_vpce_from_text_intelligence_worker" {
  count                        = local.admin_console_enabled && var.enable_text_intelligence_worker ? 1 : 0
  security_group_id            = aws_security_group.admin_vpc_endpoint[0].id
  referenced_security_group_id = aws_security_group.text_intelligence_worker[0].id
  ip_protocol                  = "tcp"
  from_port                    = 443
  to_port                      = 443
  description                  = "Allow private text-intelligence workers to reach shared AWS interface endpoints"
}

resource "aws_vpc_security_group_egress_rule" "admin_vpce_all" {
  count             = local.admin_console_enabled ? 1 : 0
  security_group_id = aws_security_group.admin_vpc_endpoint[0].id
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_vpc_endpoint" "admin_interface" {
  for_each = local.admin_console_enabled ? toset(["ecr.api", "ecr.dkr", "logs", "secretsmanager"]) : toset([])

  vpc_id              = aws_vpc.service.id
  service_name        = "com.amazonaws.${var.aws_region}.${each.value}"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = [for subnet in aws_subnet.admin_private : subnet.id]
  security_group_ids  = [aws_security_group.admin_vpc_endpoint[0].id]
  private_dns_enabled = true
  tags                = merge(local.common_tags, { Name = "${local.name}-admin-${replace(each.value, ".", "-")}", Component = "transcription-admin" })
}

resource "aws_vpc_endpoint" "admin_s3" {
  count             = local.admin_console_enabled ? 1 : 0
  vpc_id            = aws_vpc.service.id
  service_name      = "com.amazonaws.${var.aws_region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [for route_table in aws_route_table.admin_private : route_table.id]
  tags              = merge(local.common_tags, { Name = "${local.name}-admin-s3", Component = "transcription-admin" })
}

resource "aws_vpc_endpoint" "admin_dynamodb" {
  count             = local.admin_console_enabled ? 1 : 0
  vpc_id            = aws_vpc.service.id
  service_name      = "com.amazonaws.${var.aws_region}.dynamodb"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [for route_table in aws_route_table.admin_private : route_table.id]
  tags              = merge(local.common_tags, { Name = "${local.name}-admin-dynamodb", Component = "transcription-admin" })
}

resource "aws_lb" "service" {
  name               = "${local.name}-alb"
  load_balancer_type = "application"
  internal           = false
  security_groups    = [aws_security_group.alb.id]
  subnets            = [for subnet in aws_subnet.public : subnet.id]
  idle_timeout       = 300
  tags               = local.common_tags
}

resource "aws_lb_target_group" "service" {
  name        = "${local.name}-tg"
  port        = var.container_port
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = aws_vpc.service.id

  health_check {
    path                = "/healthz"
    matcher             = "200"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }

  tags = local.common_tags
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.service.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.service.arn
  }

  tags = local.common_tags
}

resource "aws_lb" "admin" {
  count              = var.enable_admin_console ? 1 : 0
  name               = "${local.name}-admin-alb"
  load_balancer_type = "application"
  internal           = true
  security_groups    = [aws_security_group.admin_alb[0].id]
  subnets            = [for subnet in aws_subnet.admin_private : subnet.id]
  idle_timeout       = 300
  tags               = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_lb_target_group" "admin" {
  count       = local.admin_console_enabled ? 1 : 0
  name        = "${local.name}-admin-tg"
  port        = var.container_port
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = aws_vpc.service.id

  health_check {
    path                = "/healthz"
    matcher             = "200"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }

  tags = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_lb_listener" "admin_https" {
  count             = var.enable_admin_console ? 1 : 0
  load_balancer_arn = aws_lb.admin[0].arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.admin_certificate_arn

  default_action {
    type = "fixed-response"

    fixed_response {
      content_type = "text/plain"
      message_body = "not found"
      status_code  = "404"
    }
  }

  tags = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_lb_listener_rule" "admin_path" {
  count        = var.enable_admin_console ? 1 : 0
  listener_arn = aws_lb_listener.admin_https[0].arn
  priority     = 10

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.admin[0].arn
  }

  condition {
    path_pattern {
      values = ["/admin", "/admin/*"]
    }
  }

  tags = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_lb" "admin_public" {
  count                      = var.enable_public_admin_console ? 1 : 0
  name                       = "${local.name}-admin-pub"
  load_balancer_type         = "application"
  internal                   = false
  security_groups            = [aws_security_group.admin_public_alb[0].id]
  subnets                    = [for subnet in aws_subnet.public : subnet.id]
  idle_timeout               = 300
  drop_invalid_header_fields = true
  tags                       = merge(local.common_tags, { Component = "transcription-admin-public" })
}

resource "aws_wafv2_web_acl_association" "admin_public_alb" {
  count        = var.enable_public_admin_console ? 1 : 0
  resource_arn = aws_lb.admin_public[0].arn
  web_acl_arn  = aws_wafv2_web_acl.admin_public[0].arn
}

resource "aws_lb_listener" "admin_public_https" {
  count             = var.enable_public_admin_console ? 1 : 0
  load_balancer_arn = aws_lb.admin_public[0].arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.public_admin_certificate_arn

  default_action {
    type = "fixed-response"

    fixed_response {
      content_type = "text/plain"
      message_body = "not found"
      status_code  = "404"
    }
  }

  tags = merge(local.common_tags, { Component = "transcription-admin-public" })
}

resource "aws_lb_listener_rule" "admin_public_root_redirect" {
  count        = var.enable_public_admin_console ? 1 : 0
  listener_arn = aws_lb_listener.admin_public_https[0].arn
  priority     = 5

  action {
    type = "redirect"

    redirect {
      protocol    = "HTTPS"
      port        = "443"
      host        = "#{host}"
      path        = "/admin"
      query       = "#{query}"
      status_code = "HTTP_302"
    }
  }

  condition {
    path_pattern {
      values = ["/"]
    }
  }

  tags = merge(local.common_tags, { Component = "transcription-admin-public" })

  depends_on = [terraform_data.public_admin_console_guard]
}

resource "aws_lb_listener_rule" "admin_public_signed_out" {
  count        = var.enable_public_admin_console ? 1 : 0
  listener_arn = aws_lb_listener.admin_public_https[0].arn
  priority     = 6

  action {
    type = "fixed-response"

    fixed_response {
      content_type = "text/html"
      message_body = "<!doctype html><html><head><title>Signed out</title></head><body><h1>Signed out</h1><p>You have signed out of Cubicle Transcription Admin.</p><p><a href=\"/admin\">Open admin again</a></p></body></html>"
      status_code  = "200"
    }
  }

  condition {
    path_pattern {
      values = ["/signed-out"]
    }
  }

  tags = merge(local.common_tags, { Component = "transcription-admin-public" })

  depends_on = [terraform_data.public_admin_console_guard]
}

resource "aws_lb_listener_rule" "admin_public_path" {
  count        = var.enable_public_admin_console ? 1 : 0
  listener_arn = aws_lb_listener.admin_public_https[0].arn
  priority     = 10

  action {
    type  = "authenticate-cognito"
    order = 1

    authenticate_cognito {
      user_pool_arn              = aws_cognito_user_pool.admin[0].arn
      user_pool_client_id        = aws_cognito_user_pool_client.admin[0].id
      user_pool_domain           = aws_cognito_user_pool_domain.admin[0].domain
      on_unauthenticated_request = "authenticate"
      scope                      = "openid email profile"
      session_cookie_name        = "CubicleAdminAuth"
      session_timeout            = var.public_admin_cognito_session_timeout_seconds
    }
  }

  action {
    type             = "forward"
    order            = 2
    target_group_arn = aws_lb_target_group.admin[0].arn
  }

  condition {
    path_pattern {
      values = ["/admin", "/admin/*", "/oauth2/*"]
    }
  }

  tags = merge(local.common_tags, { Component = "transcription-admin-public" })

  depends_on = [terraform_data.public_admin_console_guard]
}

resource "aws_ecs_cluster" "service" {
  name = "${local.name}-cluster"
  tags = local.common_tags
}

resource "aws_service_discovery_private_dns_namespace" "diarization_worker" {
  count       = var.enable_diarization_worker ? 1 : 0
  name        = local.diarization_worker_namespace_name
  description = "Private DNS namespace for Cubicle transcription worker services"
  vpc         = aws_vpc.service.id
  tags        = merge(local.common_tags, { Component = "transcription-diarization-worker" })
}

resource "aws_service_discovery_service" "diarization_worker" {
  count = var.enable_diarization_worker ? 1 : 0
  name  = var.diarization_worker_discovery_name

  dns_config {
    namespace_id   = aws_service_discovery_private_dns_namespace.diarization_worker[0].id
    routing_policy = "MULTIVALUE"

    dns_records {
      ttl  = 10
      type = "A"
    }
  }

  tags = merge(local.common_tags, { Component = "transcription-diarization-worker" })
}

resource "aws_service_discovery_private_dns_namespace" "text_intelligence_worker" {
  count       = var.enable_text_intelligence_worker ? 1 : 0
  name        = local.text_intelligence_worker_namespace_name
  description = "Private DNS namespace for Cubicle text-intelligence worker services"
  vpc         = aws_vpc.service.id
  tags        = merge(local.common_tags, { Component = "transcription-text-intelligence-worker" })
}

resource "aws_service_discovery_service" "text_intelligence_worker" {
  count = var.enable_text_intelligence_worker ? 1 : 0
  name  = var.text_intelligence_worker_discovery_name

  dns_config {
    namespace_id   = aws_service_discovery_private_dns_namespace.text_intelligence_worker[0].id
    routing_policy = "MULTIVALUE"

    dns_records {
      ttl  = 10
      type = "A"
    }
  }

  tags = merge(local.common_tags, { Component = "transcription-text-intelligence-worker" })
}

data "aws_ssm_parameter" "ecs_gpu_ami" {
  count = local.any_ecs_gpu_capacity ? 1 : 0
  name  = "/aws/service/ecs/optimized-ami/amazon-linux-2/gpu/recommended/image_id"
}

resource "aws_iam_role" "ecs_gpu_instance" {
  count = local.any_ecs_gpu_capacity ? 1 : 0
  name  = "${local.name}-ecs-gpu-instance"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ec2.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })

  tags = local.common_tags
}

resource "aws_iam_role_policy_attachment" "ecs_gpu_instance_ecs" {
  count      = local.any_ecs_gpu_capacity ? 1 : 0
  role       = aws_iam_role.ecs_gpu_instance[0].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonEC2ContainerServiceforEC2Role"
}

resource "aws_iam_role_policy_attachment" "ecs_gpu_instance_ssm" {
  count      = local.any_ecs_gpu_capacity ? 1 : 0
  role       = aws_iam_role.ecs_gpu_instance[0].name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "ecs_gpu" {
  count = local.any_ecs_gpu_capacity ? 1 : 0
  name  = "${local.name}-ecs-gpu-instance"
  role  = aws_iam_role.ecs_gpu_instance[0].name
  tags  = local.common_tags
}

resource "aws_launch_template" "ecs_gpu" {
  count         = var.enable_gpu_capacity ? 1 : 0
  name_prefix   = "${local.name}-gpu-"
  image_id      = data.aws_ssm_parameter.ecs_gpu_ami[0].value
  instance_type = var.gpu_instance_type

  iam_instance_profile {
    name = aws_iam_instance_profile.ecs_gpu[0].name
  }

  vpc_security_group_ids = [aws_security_group.task.id]
  user_data = base64encode(<<-EOF
#!/bin/bash
echo "ECS_CLUSTER=${aws_ecs_cluster.service.name}" >> /etc/ecs/ecs.config
echo "ECS_ENABLE_GPU_SUPPORT=true" >> /etc/ecs/ecs.config
EOF
  )

  block_device_mappings {
    device_name = "/dev/xvda"

    ebs {
      volume_size           = 200
      volume_type           = "gp3"
      encrypted             = true
      delete_on_termination = true
    }
  }

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 2
  }

  tag_specifications {
    resource_type = "instance"
    tags          = merge(local.common_tags, { Name = "${local.name}-gpu-ecs" })
  }

  tags = local.common_tags
}

resource "aws_autoscaling_group" "ecs_gpu" {
  count                 = var.enable_gpu_capacity ? 1 : 0
  name                  = "${local.name}-gpu"
  vpc_zone_identifier   = [for subnet in aws_subnet.public : subnet.id]
  min_size              = var.gpu_min_size
  desired_capacity      = var.gpu_desired_capacity
  max_size              = var.gpu_max_size
  protect_from_scale_in = true

  launch_template {
    id      = aws_launch_template.ecs_gpu[0].id
    version = "$Latest"
  }

  tag {
    key                 = "Name"
    value               = "${local.name}-gpu-ecs"
    propagate_at_launch = true
  }

  dynamic "tag" {
    for_each = local.common_tags
    content {
      key                 = tag.key
      value               = tag.value
      propagate_at_launch = true
    }
  }
}

resource "aws_ecs_capacity_provider" "gpu" {
  count = var.enable_gpu_capacity ? 1 : 0
  name  = "${local.name}-gpu"

  auto_scaling_group_provider {
    auto_scaling_group_arn         = aws_autoscaling_group.ecs_gpu[0].arn
    managed_termination_protection = "ENABLED"

    managed_scaling {
      status                    = "ENABLED"
      target_capacity           = 100
      minimum_scaling_step_size = 1
      maximum_scaling_step_size = 1
    }
  }

  tags = local.common_tags
}

resource "aws_launch_template" "diarization_worker_gpu" {
  count         = local.diarization_worker_gpu_capacity_enabled ? 1 : 0
  name_prefix   = "${local.name}-diar-gpu-"
  image_id      = data.aws_ssm_parameter.ecs_gpu_ami[0].value
  instance_type = var.diarization_worker_gpu_instance_type

  iam_instance_profile {
    name = aws_iam_instance_profile.ecs_gpu[0].name
  }

  vpc_security_group_ids = [aws_security_group.diarization_worker[0].id]
  user_data = base64encode(<<-EOF
#!/bin/bash
echo "ECS_CLUSTER=${aws_ecs_cluster.service.name}" >> /etc/ecs/ecs.config
echo "ECS_ENABLE_GPU_SUPPORT=true" >> /etc/ecs/ecs.config
EOF
  )

  block_device_mappings {
    device_name = "/dev/xvda"

    ebs {
      volume_size           = 200
      volume_type           = "gp3"
      encrypted             = true
      delete_on_termination = true
    }
  }

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 2
  }

  tag_specifications {
    resource_type = "instance"
    tags          = merge(local.common_tags, { Name = "${local.name}-diarization-gpu-ecs", Component = "transcription-diarization-worker" })
  }

  tags = merge(local.common_tags, { Component = "transcription-diarization-worker" })

  lifecycle {
    precondition {
      condition     = var.enable_diarization_worker || local.text_intelligence_reuses_diarization_worker_gpu_capacity
      error_message = "enable_diarization_worker_gpu_capacity=true requires either enable_diarization_worker=true or text-intelligence reuse of released diarization GPU capacity."
    }
  }
}

resource "aws_autoscaling_group" "diarization_worker_gpu" {
  count                 = local.diarization_worker_gpu_capacity_enabled ? 1 : 0
  name                  = "${local.name}-diarization-gpu"
  vpc_zone_identifier   = [for subnet in aws_subnet.public : subnet.id]
  min_size              = var.diarization_worker_gpu_min_size
  desired_capacity      = var.diarization_worker_gpu_desired_capacity
  max_size              = var.diarization_worker_gpu_max_size
  protect_from_scale_in = true

  launch_template {
    id      = aws_launch_template.diarization_worker_gpu[0].id
    version = "$Latest"
  }

  tag {
    key                 = "Name"
    value               = "${local.name}-diarization-gpu-ecs"
    propagate_at_launch = true
  }

  dynamic "tag" {
    for_each = merge(local.common_tags, { Component = "transcription-diarization-worker" })
    content {
      key                 = tag.key
      value               = tag.value
      propagate_at_launch = true
    }
  }
}

resource "aws_ecs_capacity_provider" "diarization_worker_gpu" {
  count = local.diarization_worker_gpu_capacity_enabled ? 1 : 0
  name  = "${local.name}-diarization-gpu"

  auto_scaling_group_provider {
    auto_scaling_group_arn         = aws_autoscaling_group.diarization_worker_gpu[0].arn
    managed_termination_protection = "ENABLED"

    managed_scaling {
      status                    = "ENABLED"
      target_capacity           = 100
      minimum_scaling_step_size = 1
      maximum_scaling_step_size = 1
    }
  }

  tags = merge(local.common_tags, { Component = "transcription-diarization-worker" })
}

resource "aws_launch_template" "text_intelligence_worker_gpu" {
  count         = local.text_intelligence_worker_gpu_capacity_enabled ? 1 : 0
  name_prefix   = "${local.name}-text-gpu-"
  image_id      = data.aws_ssm_parameter.ecs_gpu_ami[0].value
  instance_type = var.text_intelligence_worker_gpu_instance_type

  iam_instance_profile {
    name = aws_iam_instance_profile.ecs_gpu[0].name
  }

  vpc_security_group_ids = [aws_security_group.text_intelligence_worker[0].id]
  user_data = base64encode(<<-EOF
#!/bin/bash
echo "ECS_CLUSTER=${aws_ecs_cluster.service.name}" >> /etc/ecs/ecs.config
echo "ECS_ENABLE_GPU_SUPPORT=true" >> /etc/ecs/ecs.config
EOF
  )

  block_device_mappings {
    device_name = "/dev/xvda"

    ebs {
      volume_size           = 200
      volume_type           = "gp3"
      encrypted             = true
      delete_on_termination = true
    }
  }

  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"
    http_put_response_hop_limit = 2
  }

  tag_specifications {
    resource_type = "instance"
    tags          = merge(local.common_tags, { Name = "${local.name}-text-intelligence-gpu-ecs", Component = "transcription-text-intelligence-worker" })
  }

  tags = merge(local.common_tags, { Component = "transcription-text-intelligence-worker" })

  lifecycle {
    precondition {
      condition     = var.enable_text_intelligence_worker
      error_message = "enable_text_intelligence_worker_gpu_capacity=true requires enable_text_intelligence_worker=true."
    }
  }
}

resource "aws_autoscaling_group" "text_intelligence_worker_gpu" {
  count                 = local.text_intelligence_worker_gpu_capacity_enabled ? 1 : 0
  name                  = "${local.name}-text-intelligence-gpu"
  vpc_zone_identifier   = [for subnet in aws_subnet.public : subnet.id]
  min_size              = var.text_intelligence_worker_gpu_min_size
  desired_capacity      = var.text_intelligence_worker_gpu_desired_capacity
  max_size              = var.text_intelligence_worker_gpu_max_size
  protect_from_scale_in = true

  launch_template {
    id      = aws_launch_template.text_intelligence_worker_gpu[0].id
    version = "$Latest"
  }

  tag {
    key                 = "Name"
    value               = "${local.name}-text-intelligence-gpu-ecs"
    propagate_at_launch = true
  }

  dynamic "tag" {
    for_each = merge(local.common_tags, { Component = "transcription-text-intelligence-worker" })
    content {
      key                 = tag.key
      value               = tag.value
      propagate_at_launch = true
    }
  }
}

resource "aws_ecs_capacity_provider" "text_intelligence_worker_gpu" {
  count = local.text_intelligence_worker_gpu_capacity_enabled ? 1 : 0
  name  = "${local.name}-text-intelligence-gpu"

  auto_scaling_group_provider {
    auto_scaling_group_arn         = aws_autoscaling_group.text_intelligence_worker_gpu[0].arn
    managed_termination_protection = "ENABLED"

    managed_scaling {
      status                    = "ENABLED"
      target_capacity           = 100
      minimum_scaling_step_size = 1
      maximum_scaling_step_size = 1
    }
  }

  tags = merge(local.common_tags, { Component = "transcription-text-intelligence-worker" })
}

resource "aws_ecs_cluster_capacity_providers" "service" {
  count        = length(local.gpu_capacity_provider_names) > 0 ? 1 : 0
  cluster_name = aws_ecs_cluster.service.name

  capacity_providers = local.gpu_capacity_provider_names

  default_capacity_provider_strategy {
    capacity_provider = local.default_gpu_capacity_provider_name
    weight            = 1
  }
}

resource "aws_iam_role" "task_execution" {
  name = "${local.name}-task-execution"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ecs-tasks.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })

  tags = local.common_tags
}

resource "aws_iam_role_policy_attachment" "task_execution_managed" {
  role       = aws_iam_role.task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "task_execution_secrets" {
  role = aws_iam_role.task_execution.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "secretsmanager:GetSecretValue"
      ]
      Resource = concat(
        [
          aws_secretsmanager_secret.service_token.arn,
          aws_secretsmanager_secret.user_token_signing_key.arn,
          aws_secretsmanager_secret.diarization_worker_auth_token.arn
        ],
        var.enable_mistral_secret ? [aws_secretsmanager_secret.mistral_api_key[0].arn] : [],
        local.pyannote_auth_token_secret_arn != "" ? [local.pyannote_auth_token_secret_arn] : []
      )
    }]
  })
}

resource "aws_iam_role" "diarization_worker_task_execution" {
  count = var.enable_diarization_worker ? 1 : 0
  name  = "${local.name}-diarization-worker-task-execution"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ecs-tasks.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })

  tags = merge(local.common_tags, { Component = "transcription-diarization-worker" })
}

resource "aws_iam_role_policy_attachment" "diarization_worker_task_execution_managed" {
  count      = var.enable_diarization_worker ? 1 : 0
  role       = aws_iam_role.diarization_worker_task_execution[0].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "diarization_worker_task_execution_secrets" {
  count = var.enable_diarization_worker ? 1 : 0
  role  = aws_iam_role.diarization_worker_task_execution[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "secretsmanager:GetSecretValue"
      ]
      Resource = concat(
        [
          aws_secretsmanager_secret.diarization_worker_auth_token.arn
        ],
        local.pyannote_auth_token_secret_arn != "" ? [local.pyannote_auth_token_secret_arn] : []
      )
    }]
  })
}

resource "aws_iam_role" "text_intelligence_worker_task_execution" {
  count = var.enable_text_intelligence_worker ? 1 : 0
  name  = "${local.name}-text-worker-task-exec"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ecs-tasks.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })

  tags = merge(local.common_tags, { Component = "transcription-text-intelligence-worker" })
}

resource "aws_iam_role_policy_attachment" "text_intelligence_worker_task_execution_managed" {
  count      = var.enable_text_intelligence_worker ? 1 : 0
  role       = aws_iam_role.text_intelligence_worker_task_execution[0].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "text_intelligence_worker_task_execution_secrets" {
  count = var.enable_text_intelligence_worker ? 1 : 0
  role  = aws_iam_role.text_intelligence_worker_task_execution[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "secretsmanager:GetSecretValue"
      ]
      Resource = concat(
        [
          aws_secretsmanager_secret.text_intelligence_worker_auth_token.arn
        ],
        local.pyannote_auth_token_secret_arn != "" ? [local.pyannote_auth_token_secret_arn] : []
      )
    }]
  })
}

resource "aws_iam_role" "admin_task_execution" {
  count = local.admin_console_enabled ? 1 : 0
  name  = "${local.name}-admin-task-execution"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ecs-tasks.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })

  tags = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_iam_role_policy_attachment" "admin_task_execution_managed" {
  count      = local.admin_console_enabled ? 1 : 0
  role       = aws_iam_role.admin_task_execution[0].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "admin_task_execution_secrets" {
  count = local.admin_console_enabled ? 1 : 0
  role  = aws_iam_role.admin_task_execution[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "secretsmanager:GetSecretValue"
      ]
      Resource = [
        aws_secretsmanager_secret.service_token.arn,
        aws_secretsmanager_secret.user_token_signing_key.arn,
        aws_secretsmanager_secret.admin_session_secret[0].arn
      ]
    }]
  })
}

resource "aws_iam_role_policy" "task_user_registry_dynamodb" {
  count = local.service_user_registry_enabled ? 1 : 0
  role  = aws_iam_role.task.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "dynamodb:DescribeTable",
          "dynamodb:GetItem"
        ]
        Resource = [
          aws_dynamodb_table.admin_users[0].arn,
          aws_dynamodb_table.admin_token_ledger[0].arn
        ]
      },
      {
        Effect = "Allow"
        Action = [
          "dynamodb:DescribeTable",
          "dynamodb:PutItem"
        ]
        Resource = [
          aws_dynamodb_table.admin_audit[0].arn
        ]
      }
    ]
  })
}

resource "aws_iam_role_policy" "task_runtime_config_dynamodb" {
  count = local.admin_console_enabled ? 1 : 0
  role  = aws_iam_role.task.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "dynamodb:DescribeTable",
        "dynamodb:GetItem"
      ]
      Resource = [
        aws_dynamodb_table.admin_users[0].arn
      ]
    }]
  })
}

resource "aws_iam_role" "admin_task" {
  count = local.admin_console_enabled ? 1 : 0
  name  = "${local.name}-admin-task"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ecs-tasks.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })

  tags = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_iam_role_policy" "admin_task_dynamodb" {
  count = local.admin_console_enabled ? 1 : 0
  role  = aws_iam_role.admin_task[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "dynamodb:BatchGetItem",
        "dynamodb:BatchWriteItem",
        "dynamodb:DeleteItem",
        "dynamodb:DescribeTable",
        "dynamodb:GetItem",
        "dynamodb:PutItem",
        "dynamodb:Query",
        "dynamodb:Scan",
        "dynamodb:UpdateItem"
      ]
      Resource = [
        aws_dynamodb_table.admin_users[0].arn,
        aws_dynamodb_table.admin_token_ledger[0].arn,
        aws_dynamodb_table.admin_audit[0].arn
      ]
    }]
  })
}

resource "aws_iam_role_policy" "admin_task_voicenotes_cognito" {
  count = local.admin_console_enabled && var.voicenotes_cognito_user_pool_id != "" ? 1 : 0
  role  = aws_iam_role.admin_task[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "cognito-idp:AdminCreateUser",
        "cognito-idp:AdminDeleteUser",
        "cognito-idp:AdminDisableUser",
        "cognito-idp:AdminEnableUser",
        "cognito-idp:AdminGetUser",
        "cognito-idp:AdminResetUserPassword",
        "cognito-idp:AdminUserGlobalSignOut",
        "cognito-idp:ListUsers"
      ]
      Resource = "arn:aws:cognito-idp:${var.voicenotes_cognito_region != "" ? var.voicenotes_cognito_region : var.aws_region}:${data.aws_caller_identity.current.account_id}:userpool/${var.voicenotes_cognito_user_pool_id}"
    }]
  })
}

resource "aws_iam_role_policy" "admin_task_voicenotes_lambda" {
  count = local.admin_console_enabled && var.voicenotes_admin_lambda_name != "" ? 1 : 0
  role  = aws_iam_role.admin_task[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["lambda:InvokeFunction"]
      Resource = "arn:aws:lambda:${var.voicenotes_admin_lambda_region != "" ? var.voicenotes_admin_lambda_region : var.aws_region}:${data.aws_caller_identity.current.account_id}:function:${var.voicenotes_admin_lambda_name}"
    }]
  })
}

resource "aws_iam_role" "task" {
  name = "${local.name}-task"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ecs-tasks.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })

  tags = local.common_tags
}

resource "aws_iam_role" "diarization_worker_task" {
  count = var.enable_diarization_worker ? 1 : 0
  name  = "${local.name}-diarization-worker-task"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ecs-tasks.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })

  tags = merge(local.common_tags, { Component = "transcription-diarization-worker" })
}

resource "aws_iam_role" "text_intelligence_worker_task" {
  count = var.enable_text_intelligence_worker ? 1 : 0
  name  = "${local.name}-text-worker-task"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Principal = {
        Service = "ecs-tasks.amazonaws.com"
      }
      Action = "sts:AssumeRole"
    }]
  })

  tags = merge(local.common_tags, { Component = "transcription-text-intelligence-worker" })
}

resource "aws_ecs_task_definition" "admin" {
  count                    = local.admin_console_enabled ? 1 : 0
  family                   = "${local.name}-admin"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = tostring(var.admin_task_cpu)
  memory                   = tostring(var.admin_task_memory)
  execution_role_arn       = aws_iam_role.admin_task_execution[0].arn
  task_role_arn            = aws_iam_role.admin_task[0].arn

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "X86_64"
  }

  container_definitions = jsonencode(local.admin_container_definitions)

  tags = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_ecs_task_definition" "service" {
  family                   = "${local.name}-service"
  requires_compatibilities = [var.ecs_launch_type]
  network_mode             = "awsvpc"
  cpu                      = tostring(var.task_cpu)
  memory                   = tostring(var.task_memory)
  execution_role_arn       = aws_iam_role.task_execution.arn
  task_role_arn            = aws_iam_role.task.arn

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "X86_64"
  }

  container_definitions = jsonencode(local.container_definitions)

  lifecycle {
    precondition {
      condition     = !var.enable_voxtral_runtime || var.ecs_launch_type == "EC2"
      error_message = "enable_voxtral_runtime requires ecs_launch_type=EC2 because Fargate does not support GPU inference."
    }
    precondition {
      condition     = var.ecs_launch_type != "EC2" || var.enable_gpu_capacity
      error_message = "ecs_launch_type=EC2 requires enable_gpu_capacity=true so ECS has GPU container instances."
    }
  }

  tags = local.common_tags
}

resource "aws_ecs_task_definition" "diarization_worker" {
  count                    = var.enable_diarization_worker ? 1 : 0
  family                   = "${local.name}-diarization-worker"
  requires_compatibilities = [var.diarization_worker_launch_type]
  network_mode             = "awsvpc"
  cpu                      = tostring(var.diarization_worker_task_cpu)
  memory                   = tostring(var.diarization_worker_task_memory)
  execution_role_arn       = aws_iam_role.diarization_worker_task_execution[0].arn
  task_role_arn            = aws_iam_role.diarization_worker_task[0].arn

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "X86_64"
  }

  container_definitions = jsonencode(local.diarization_worker_container_definitions)

  lifecycle {
    precondition {
      condition     = lower(var.diarization_worker_provider) != "pyannote" || local.pyannote_auth_token_secret_arn != ""
      error_message = "enable_diarization_worker=true with pyannote requires PYANNOTE_AUTH_TOKEN, PYANNOTE_AUTH_TOKEN_SECRET_ARN, or PYANNOTE_AUTH_TOKEN_SECRET_NAME."
    }
    precondition {
      condition     = !local.diarization_worker_uses_ec2 || local.diarization_worker_gpu_capacity_enabled
      error_message = "diarization_worker_launch_type=EC2 requires enable_diarization_worker_gpu_capacity=true so the private worker has dedicated GPU capacity."
    }
    precondition {
      condition     = !local.diarization_worker_uses_ec2 || var.diarization_worker_gpu_count > 0
      error_message = "diarization_worker_launch_type=EC2 requires diarization_worker_gpu_count greater than zero."
    }
  }

  tags = merge(local.common_tags, { Component = "transcription-diarization-worker" })
}

resource "aws_ecs_task_definition" "text_intelligence_worker" {
  count                    = var.enable_text_intelligence_worker ? 1 : 0
  family                   = "${local.name}-text-intelligence-worker"
  requires_compatibilities = [var.text_intelligence_worker_launch_type]
  network_mode             = "awsvpc"
  cpu                      = tostring(var.text_intelligence_worker_task_cpu)
  memory                   = tostring(var.text_intelligence_worker_task_memory)
  execution_role_arn       = aws_iam_role.text_intelligence_worker_task_execution[0].arn
  task_role_arn            = aws_iam_role.text_intelligence_worker_task[0].arn

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "X86_64"
  }

  container_definitions = jsonencode(local.text_intelligence_worker_container_definitions)

  dynamic "volume" {
    for_each = local.text_intelligence_worker_uses_runtime && local.text_intelligence_worker_uses_ec2 ? [1] : []
    content {
      name      = "text-intelligence-model-cache"
      host_path = "/opt/cubicle/model-cache"
    }
  }

  lifecycle {
    precondition {
      condition     = !local.text_intelligence_worker_uses_runtime || var.text_intelligence_worker_launch_type == "EC2"
      error_message = "text_intelligence_worker_provider=vllm/openai_compatible requires text_intelligence_worker_launch_type=EC2 for GPU inference."
    }
    precondition {
      condition     = !local.text_intelligence_worker_uses_ec2 || local.text_intelligence_worker_gpu_capacity_available
      error_message = "text_intelligence_worker_launch_type=EC2 requires either released diarization GPU capacity reuse or enable_text_intelligence_worker_gpu_capacity=true."
    }
    precondition {
      condition     = !local.text_intelligence_worker_uses_runtime || var.text_intelligence_runtime_gpu_count > 0
      error_message = "text_intelligence_worker_provider=vllm/openai_compatible requires text_intelligence_runtime_gpu_count greater than zero."
    }
  }

  tags = merge(local.common_tags, { Component = "transcription-text-intelligence-worker" })
}

resource "aws_ecs_service" "service" {
  name                          = "${local.name}-service"
  cluster                       = aws_ecs_cluster.service.id
  task_definition               = aws_ecs_task_definition.service.arn
  desired_count                 = var.desired_count
  launch_type                   = var.ecs_launch_type == "FARGATE" ? "FARGATE" : null
  force_new_deployment          = true
  availability_zone_rebalancing = var.ecs_launch_type == "EC2" && var.enable_gpu_capacity ? "DISABLED" : "ENABLED"

  dynamic "capacity_provider_strategy" {
    for_each = var.ecs_launch_type == "EC2" && var.enable_gpu_capacity ? [1] : []
    content {
      capacity_provider = aws_ecs_capacity_provider.gpu[0].name
      weight            = 1
    }
  }

  deployment_minimum_healthy_percent = var.ecs_launch_type == "EC2" && var.enable_gpu_capacity ? 0 : 100
  deployment_maximum_percent         = var.ecs_launch_type == "EC2" && var.enable_gpu_capacity ? 100 : 200

  network_configuration {
    subnets          = [for subnet in aws_subnet.public : subnet.id]
    security_groups  = [aws_security_group.task.id]
    assign_public_ip = var.ecs_launch_type == "FARGATE" ? true : false
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.service.arn
    container_name   = "service"
    container_port   = var.container_port
  }

  depends_on = [
    aws_lb_listener.http,
    aws_ecs_cluster_capacity_providers.service,
  ]
  tags = local.common_tags
}

resource "aws_ecs_service" "diarization_worker" {
  count                         = var.enable_diarization_worker ? 1 : 0
  name                          = "${local.name}-diarization-worker"
  cluster                       = aws_ecs_cluster.service.id
  task_definition               = aws_ecs_task_definition.diarization_worker[0].arn
  desired_count                 = var.diarization_worker_desired_count
  launch_type                   = local.diarization_worker_uses_ec2 ? null : "FARGATE"
  force_new_deployment          = true
  availability_zone_rebalancing = local.diarization_worker_uses_ec2 ? "DISABLED" : "ENABLED"

  dynamic "capacity_provider_strategy" {
    for_each = local.diarization_worker_uses_ec2 && local.diarization_worker_gpu_capacity_enabled ? [1] : []
    content {
      capacity_provider = aws_ecs_capacity_provider.diarization_worker_gpu[0].name
      weight            = 1
    }
  }

  deployment_minimum_healthy_percent = local.diarization_worker_uses_ec2 ? 0 : 100
  deployment_maximum_percent         = local.diarization_worker_uses_ec2 ? 100 : 200

  network_configuration {
    subnets          = [for subnet in aws_subnet.public : subnet.id]
    security_groups  = [aws_security_group.diarization_worker[0].id]
    assign_public_ip = local.diarization_worker_uses_ec2 ? false : var.diarization_worker_assign_public_ip
  }

  service_registries {
    registry_arn = aws_service_discovery_service.diarization_worker[0].arn
  }

  depends_on = [
    aws_service_discovery_service.diarization_worker,
    aws_ecs_cluster_capacity_providers.service,
  ]

  tags = merge(local.common_tags, { Component = "transcription-diarization-worker" })
}

resource "aws_ecs_service" "text_intelligence_worker" {
  count                         = var.enable_text_intelligence_worker ? 1 : 0
  name                          = "${local.name}-text-intelligence-worker"
  cluster                       = aws_ecs_cluster.service.id
  task_definition               = aws_ecs_task_definition.text_intelligence_worker[0].arn
  desired_count                 = var.text_intelligence_worker_desired_count
  launch_type                   = local.text_intelligence_worker_uses_ec2 ? null : "FARGATE"
  force_new_deployment          = true
  availability_zone_rebalancing = local.text_intelligence_worker_uses_ec2 ? "DISABLED" : "ENABLED"

  dynamic "capacity_provider_strategy" {
    for_each = local.text_intelligence_worker_uses_ec2 && local.text_intelligence_worker_gpu_capacity_available ? [1] : []
    content {
      capacity_provider = local.text_intelligence_worker_capacity_provider_name
      weight            = 1
    }
  }

  deployment_minimum_healthy_percent = local.text_intelligence_worker_uses_ec2 ? 0 : 100
  deployment_maximum_percent         = local.text_intelligence_worker_uses_ec2 ? 100 : 200

  network_configuration {
    subnets          = [for subnet in aws_subnet.public : subnet.id]
    security_groups  = [aws_security_group.text_intelligence_worker[0].id]
    assign_public_ip = local.text_intelligence_worker_uses_ec2 ? false : var.text_intelligence_worker_assign_public_ip
  }

  service_registries {
    registry_arn = aws_service_discovery_service.text_intelligence_worker[0].arn
  }

  depends_on = [
    aws_service_discovery_service.text_intelligence_worker,
    aws_ecs_cluster_capacity_providers.service,
    terraform_data.worker_gpu_repurpose_guard,
  ]

  tags = merge(local.common_tags, { Component = "transcription-text-intelligence-worker" })
}

resource "aws_ecs_service" "admin" {
  count                         = local.admin_console_enabled ? 1 : 0
  name                          = "${local.name}-admin"
  cluster                       = aws_ecs_cluster.service.id
  task_definition               = aws_ecs_task_definition.admin[0].arn
  desired_count                 = var.admin_desired_count
  launch_type                   = "FARGATE"
  force_new_deployment          = true
  availability_zone_rebalancing = "ENABLED"

  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200

  network_configuration {
    subnets          = [for subnet in aws_subnet.admin_private : subnet.id]
    security_groups  = [aws_security_group.admin_task[0].id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.admin[0].arn
    container_name   = "admin"
    container_port   = var.container_port
  }

  depends_on = [
    aws_lb_listener_rule.admin_path,
    aws_lb_listener_rule.admin_public_root_redirect,
    aws_lb_listener_rule.admin_public_path,
    aws_vpc_endpoint.admin_interface,
    aws_vpc_endpoint.admin_s3,
    aws_vpc_endpoint.admin_dynamodb
  ]

  tags = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_route53_zone" "admin_private" {
  count = var.enable_admin_console && var.admin_create_private_hosted_zone ? 1 : 0
  name  = var.admin_private_zone_name

  vpc {
    vpc_id = aws_vpc.service.id
  }

  tags = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_route53_record" "admin_private" {
  count   = var.enable_admin_console ? 1 : 0
  zone_id = var.admin_private_hosted_zone_id != "" ? var.admin_private_hosted_zone_id : aws_route53_zone.admin_private[0].zone_id
  name    = var.admin_domain_name
  type    = "A"

  alias {
    name                   = aws_lb.admin[0].dns_name
    zone_id                = aws_lb.admin[0].zone_id
    evaluate_target_health = true
  }
}

resource "aws_security_group" "admin_client_vpn" {
  count       = var.enable_admin_client_vpn ? 1 : 0
  name        = "${local.name}-admin-client-vpn"
  description = "AWS Client VPN network associations for private admin console access"
  vpc_id      = aws_vpc.service.id
  tags        = merge(local.common_tags, { Component = "transcription-admin" })
}

resource "aws_vpc_security_group_egress_rule" "admin_client_vpn_all" {
  count             = var.enable_admin_client_vpn ? 1 : 0
  security_group_id = aws_security_group.admin_client_vpn[0].id
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_ec2_client_vpn_endpoint" "admin" {
  count                  = var.enable_admin_client_vpn ? 1 : 0
  description            = "${local.name} private admin console VPN"
  server_certificate_arn = var.admin_client_vpn_server_certificate_arn
  client_cidr_block      = var.admin_client_vpn_client_cidr_block
  split_tunnel           = true
  transport_protocol     = "udp"
  vpc_id                 = aws_vpc.service.id
  security_group_ids     = [aws_security_group.admin_client_vpn[0].id]

  authentication_options {
    type                       = "certificate-authentication"
    root_certificate_chain_arn = var.admin_client_vpn_root_certificate_chain_arn
  }

  connection_log_options {
    enabled = false
  }

  tags = merge(local.common_tags, { Component = "transcription-admin" })

  depends_on = [terraform_data.admin_client_vpn_guard]
}

resource "aws_ec2_client_vpn_network_association" "admin" {
  for_each = var.enable_admin_client_vpn ? aws_subnet.admin_private : {}

  client_vpn_endpoint_id = aws_ec2_client_vpn_endpoint.admin[0].id
  subnet_id              = each.value.id
}

resource "aws_ec2_client_vpn_authorization_rule" "admin_vpc" {
  count                  = var.enable_admin_client_vpn ? 1 : 0
  client_vpn_endpoint_id = aws_ec2_client_vpn_endpoint.admin[0].id
  target_network_cidr    = aws_vpc.service.cidr_block
  authorize_all_groups   = true
  description            = "Allow authenticated admin VPN clients to reach the private transcription VPC"
}

resource "aws_cloudfront_distribution" "service" {
  enabled         = true
  comment         = "${local.name} secure WebSocket edge"
  is_ipv6_enabled = true
  price_class     = "PriceClass_100"
  http_version    = "http2"

  origin {
    domain_name = aws_lb.service.dns_name
    origin_id   = "alb-origin"

    custom_origin_config {
      http_port                = 80
      https_port               = 443
      origin_protocol_policy   = "http-only"
      origin_ssl_protocols     = ["TLSv1.2"]
      origin_read_timeout      = 60
      origin_keepalive_timeout = 60
    }
  }

  default_cache_behavior {
    target_origin_id       = "alb-origin"
    viewer_protocol_policy = "redirect-to-https"
    allowed_methods        = ["DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"]
    cached_methods         = ["GET", "HEAD"]
    compress               = false
    min_ttl                = 0
    default_ttl            = 0
    max_ttl                = 0

    forwarded_values {
      query_string = false
      headers = [
        "Authorization",
        "Origin",
        "Sec-WebSocket-Key",
        "Sec-WebSocket-Version",
        "Sec-WebSocket-Protocol",
        "Sec-WebSocket-Extensions"
      ]

      cookies {
        forward = "none"
      }
    }
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  viewer_certificate {
    cloudfront_default_certificate = true
  }

  tags = local.common_tags
}
