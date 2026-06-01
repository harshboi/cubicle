data "aws_caller_identity" "current" {}

locals {
  name                  = var.project_name
  cognito_domain_prefix = var.cognito_domain_prefix != "" ? var.cognito_domain_prefix : "${local.name}-${data.aws_caller_identity.current.account_id}"
  callback_url          = "https://${var.domain_name}/auth/callback"
  logout_url            = "https://${var.domain_name}/login"
  execution_secret_arns = concat(
    [
      aws_secretsmanager_secret.session_secret.arn,
      var.upstream_transcription_token_secret_arn
    ],
    var.upstream_transcription_signing_secret_arn != "" ? [var.upstream_transcription_signing_secret_arn] : [],
    var.text_intelligence_token_secret_arn != "" ? [var.text_intelligence_token_secret_arn] : []
  )
  app_secrets = concat(
    [
      {
        name      = "VOICENOTES_SESSION_SECRET"
        valueFrom = aws_secretsmanager_secret.session_secret.arn
      },
      {
        name      = "VOICENOTES_UPSTREAM_TRANSCRIPTION_TOKEN"
        valueFrom = var.upstream_transcription_token_secret_arn
      }
    ],
    var.upstream_transcription_signing_secret_arn != "" ? [
      {
        name      = "VOICENOTES_UPSTREAM_TRANSCRIPTION_SIGNING_SECRET"
        valueFrom = var.upstream_transcription_signing_secret_arn
      }
    ] : [],
    var.text_intelligence_token_secret_arn != "" ? [
      {
        name      = "VOICENOTES_TEXT_INTELLIGENCE_TOKEN"
        valueFrom = var.text_intelligence_token_secret_arn
      }
    ] : []
  )
  common_tags = {
    Project     = local.name
    Application = "VoiceNotes"
    ManagedBy   = "terraform"
  }
}

resource "aws_ecr_repository" "app" {
  name                 = local.name
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "AES256"
  }

  tags = local.common_tags
}

resource "aws_kms_key" "transcripts" {
  description             = "VoiceNotes transcript encryption key"
  deletion_window_in_days = 30
  enable_key_rotation     = true
  tags                    = local.common_tags
}

resource "aws_kms_alias" "transcripts" {
  name          = "alias/${local.name}-transcripts"
  target_key_id = aws_kms_key.transcripts.key_id
}

resource "aws_s3_bucket" "transcripts" {
  bucket = "${local.name}-transcripts-${data.aws_caller_identity.current.account_id}"
  tags   = local.common_tags
}

resource "aws_s3_bucket_public_access_block" "transcripts" {
  bucket                  = aws_s3_bucket.transcripts.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "transcripts" {
  bucket = aws_s3_bucket.transcripts.id

  rule {
    apply_server_side_encryption_by_default {
      kms_master_key_id = aws_kms_key.transcripts.arn
      sse_algorithm     = "aws:kms"
    }
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_versioning" "transcripts" {
  bucket = aws_s3_bucket.transcripts.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_dynamodb_table" "notes" {
  name         = "${local.name}-notes"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "pk"
  range_key    = "sk"

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

  tags = local.common_tags
}

resource "aws_dynamodb_table" "audit" {
  name         = "${local.name}-audit"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "pk"
  range_key    = "sk"

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

  tags = local.common_tags
}

resource "aws_secretsmanager_secret" "session_secret" {
  name                    = "${local.name}/session-secret"
  recovery_window_in_days = 7
  tags                    = local.common_tags
}

resource "random_password" "session_secret" {
  length  = 48
  special = true
}

resource "aws_secretsmanager_secret_version" "session_secret" {
  secret_id     = aws_secretsmanager_secret.session_secret.id
  secret_string = random_password.session_secret.result
}

resource "aws_cognito_user_pool" "users" {
  name                     = "${local.name}-users"
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

  tags = local.common_tags
}

resource "aws_cognito_user_pool_client" "web" {
  name                                 = "${local.name}-web"
  user_pool_id                         = aws_cognito_user_pool.users.id
  generate_secret                      = false
  allowed_oauth_flows_user_pool_client = true
  allowed_oauth_flows                  = ["code"]
  allowed_oauth_scopes                 = ["openid", "email", "profile"]
  callback_urls                        = [local.callback_url]
  logout_urls                          = [local.logout_url]
  supported_identity_providers         = ["COGNITO"]
  prevent_user_existence_errors        = "ENABLED"
}

resource "aws_cognito_user_pool_domain" "web" {
  domain       = local.cognito_domain_prefix
  user_pool_id = aws_cognito_user_pool.users.id
}

resource "aws_cloudwatch_log_group" "app" {
  name              = "/aws/ecs/${local.name}"
  retention_in_days = 14
  tags              = local.common_tags
}

resource "aws_iam_role" "execution" {
  name = "${local.name}-execution"

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

resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_role_policy" "execution_secrets" {
  name = "${local.name}-execution-secrets"
  role = aws_iam_role.execution.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["secretsmanager:GetSecretValue"]
      Resource = local.execution_secret_arns
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

resource "aws_iam_role_policy" "task" {
  name = "${local.name}-task"
  role = aws_iam_role.task.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "dynamodb:GetItem",
          "dynamodb:PutItem",
          "dynamodb:Query",
          "dynamodb:UpdateItem"
        ]
        Resource = [
          aws_dynamodb_table.notes.arn,
          aws_dynamodb_table.audit.arn
        ]
      },
      {
        Effect = "Allow"
        Action = [
          "s3:GetObject",
          "s3:PutObject",
          "s3:DeleteObject"
        ]
        Resource = "${aws_s3_bucket.transcripts.arn}/*"
      },
      {
        Effect = "Allow"
        Action = [
          "kms:Decrypt",
          "kms:Encrypt",
          "kms:GenerateDataKey"
        ]
        Resource = aws_kms_key.transcripts.arn
      },
      {
        Effect   = "Allow"
        Action   = ["cognito-idp:ListUsers"]
        Resource = aws_cognito_user_pool.users.arn
      }
    ]
  })
}

resource "aws_security_group" "alb" {
  name        = "${local.name}-alb"
  description = "VoiceNotes public ALB"
  vpc_id      = var.vpc_id

  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = local.common_tags
}

resource "aws_security_group" "service" {
  name        = "${local.name}-service"
  description = "VoiceNotes ECS service"
  vpc_id      = var.vpc_id

  ingress {
    from_port       = var.container_port
    to_port         = var.container_port
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = local.common_tags
}

resource "aws_vpc_security_group_ingress_rule" "shared_vpce_from_service" {
  count = var.shared_vpce_security_group_id != "" ? 1 : 0

  security_group_id            = var.shared_vpce_security_group_id
  referenced_security_group_id = aws_security_group.service.id
  ip_protocol                  = "tcp"
  from_port                    = 443
  to_port                      = 443
  description                  = "Allow VoiceNotes tasks to reach shared AWS interface endpoints"

  tags = local.common_tags
}

resource "aws_lb" "app" {
  name               = local.name
  load_balancer_type = "application"
  internal           = false
  security_groups    = [aws_security_group.alb.id]
  subnets            = var.public_subnet_ids
  tags               = local.common_tags
}

resource "aws_lb_target_group" "app" {
  name        = local.name
  port        = var.container_port
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = var.vpc_id

  health_check {
    path                = "/healthz"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    timeout             = 5
    interval            = 30
    matcher             = "200"
  }

  tags = local.common_tags
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.app.arn
  port              = 443
  protocol          = "HTTPS"
  certificate_arn   = var.certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app.arn
  }
}

resource "aws_ecs_cluster" "app" {
  name = local.name
  tags = local.common_tags
}

resource "aws_ecs_task_definition" "app" {
  family                   = local.name
  network_mode             = "awsvpc"
  requires_compatibilities = ["FARGATE"]
  cpu                      = "512"
  memory                   = "1024"
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task.arn

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "ARM64"
  }

  container_definitions = jsonencode([
    {
      name      = "app"
      image     = var.container_image
      essential = true
      portMappings = [{
        containerPort = var.container_port
        protocol      = "tcp"
      }]
      environment = [
        { name = "VOICENOTES_AUTH_MODE", value = "oidc" },
        { name = "VOICENOTES_SECURE_COOKIES", value = "true" },
        { name = "VOICENOTES_STORAGE_BACKEND", value = "aws" },
        { name = "VOICENOTES_NOTES_TABLE", value = aws_dynamodb_table.notes.name },
        { name = "VOICENOTES_AUDIT_TABLE", value = aws_dynamodb_table.audit.name },
        { name = "VOICENOTES_TRANSCRIPT_BUCKET", value = aws_s3_bucket.transcripts.bucket },
        { name = "VOICENOTES_TRANSCRIPT_KMS_KEY_ID", value = aws_kms_key.transcripts.arn },
        { name = "VOICENOTES_MOCK_TRANSCRIPTION", value = "false" },
        { name = "VOICENOTES_UPSTREAM_TRANSCRIPTION_URL", value = var.upstream_transcription_url },
        { name = "VOICENOTES_TEXT_INTELLIGENCE_ENABLED", value = tostring(var.text_intelligence_enabled) },
        { name = "VOICENOTES_TEXT_INTELLIGENCE_URL", value = var.text_intelligence_url },
        { name = "VOICENOTES_TEXT_INTELLIGENCE_MODEL", value = var.text_intelligence_model },
        { name = "VOICENOTES_TEXT_INTELLIGENCE_CONTEXT_LINES", value = tostring(var.text_intelligence_context_lines) },
        { name = "VOICENOTES_TEXT_INTELLIGENCE_REQUEST_TIMEOUT_SECONDS", value = tostring(var.text_intelligence_request_timeout_seconds) },
        { name = "VOICENOTES_TEXT_INTELLIGENCE_FLUSH_TIMEOUT_SECONDS", value = tostring(var.text_intelligence_flush_timeout_seconds) },
        { name = "VOICENOTES_TEXT_INTELLIGENCE_SUMMARY_ENABLED", value = tostring(var.text_intelligence_summary_enabled) },
        { name = "VOICENOTES_TEXT_INTELLIGENCE_SUMMARY_TIMEOUT_SECONDS", value = tostring(var.text_intelligence_summary_timeout_seconds) },
        { name = "VOICENOTES_MONTHLY_MINUTE_QUOTA", value = tostring(var.monthly_minute_quota) },
        { name = "VOICENOTES_MAX_RECORDING_SECONDS", value = tostring(var.max_recording_seconds) },
        { name = "VOICENOTES_OIDC_ISSUER", value = "https://cognito-idp.${var.aws_region}.amazonaws.com/${aws_cognito_user_pool.users.id}" },
        { name = "VOICENOTES_OIDC_CLIENT_ID", value = aws_cognito_user_pool_client.web.id },
        { name = "VOICENOTES_OIDC_REDIRECT_URI", value = local.callback_url },
        { name = "VOICENOTES_OIDC_AUTHORIZATION_ENDPOINT", value = "https://${aws_cognito_user_pool_domain.web.domain}.auth.${var.aws_region}.amazoncognito.com/oauth2/authorize" },
        { name = "VOICENOTES_OIDC_TOKEN_ENDPOINT", value = "https://${aws_cognito_user_pool_domain.web.domain}.auth.${var.aws_region}.amazoncognito.com/oauth2/token" },
        { name = "VOICENOTES_OIDC_JWKS_URI", value = "https://cognito-idp.${var.aws_region}.amazonaws.com/${aws_cognito_user_pool.users.id}/.well-known/jwks.json" },
        { name = "VOICENOTES_OIDC_LOGOUT_URL", value = "https://${aws_cognito_user_pool_domain.web.domain}.auth.${var.aws_region}.amazoncognito.com/logout" },
        { name = "VOICENOTES_OIDC_USER_POOL_ID", value = aws_cognito_user_pool.users.id },
        { name = "VOICENOTES_OIDC_SESSION_VALIDATION_ENABLED", value = "false" },
        { name = "VOICENOTES_OIDC_SESSION_VALIDATION_TTL_SECONDS", value = "5" },
        { name = "VOICENOTES_OIDC_SESSION_VALIDATION_REQUEST_TIMEOUT_SECONDS", value = "3" },
        { name = "VOICENOTES_OIDC_RECORDING_ACCESS_CHECK_SECONDS", value = "5" }
      ]
      secrets = local.app_secrets
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.app.name
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "app"
        }
      }
    }
  ])

  tags = local.common_tags
}

resource "aws_ecs_service" "app" {
  name            = local.name
  cluster         = aws_ecs_cluster.app.id
  task_definition = aws_ecs_task_definition.app.arn
  desired_count   = var.desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = var.service_subnet_ids
    security_groups  = [aws_security_group.service.id]
    assign_public_ip = var.service_assign_public_ip
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.app.arn
    container_name   = "app"
    container_port   = var.container_port
  }

  depends_on = [aws_lb_listener.https]
  tags       = local.common_tags
}
