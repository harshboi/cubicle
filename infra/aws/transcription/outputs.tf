output "account_id" {
  value = data.aws_caller_identity.current.account_id
}

output "region" {
  value = var.aws_region
}

output "repository_url" {
  value = aws_ecr_repository.service.repository_url
}

output "voxtral_runtime_repository_url" {
  value = aws_ecr_repository.voxtral_runtime.repository_url
}

output "diarization_worker_repository_url" {
  value = aws_ecr_repository.diarization_worker.repository_url
}

output "text_intelligence_worker_image_uri_default" {
  value = local.text_intelligence_worker_image_uri
}

output "service_token_secret_arn" {
  value     = aws_secretsmanager_secret.service_token.arn
  sensitive = true
}

output "user_token_signing_key_secret_arn" {
  value     = aws_secretsmanager_secret.user_token_signing_key.arn
  sensitive = true
}

output "mistral_api_key_secret_arn" {
  value     = var.enable_mistral_secret ? aws_secretsmanager_secret.mistral_api_key[0].arn : null
  sensitive = true
}

output "pyannote_auth_token_secret_arn" {
  value     = var.enable_pyannote_secret ? aws_secretsmanager_secret.pyannote_auth_token[0].arn : null
  sensitive = true
}

output "alb_dns_name" {
  value = aws_lb.service.dns_name
}

output "cloudfront_domain_name" {
  value = aws_cloudfront_distribution.service.domain_name
}

output "websocket_endpoint" {
  value = "wss://${aws_cloudfront_distribution.service.domain_name}/v1/transcription"
}

output "health_endpoint" {
  value = "https://${aws_cloudfront_distribution.service.domain_name}/healthz"
}

output "vpc_id" {
  value = aws_vpc.service.id
}

output "public_subnet_ids" {
  value = [for subnet in aws_subnet.public : subnet.id]
}

output "service_subnet_ids" {
  value = [for subnet in aws_subnet.public : subnet.id]
}

output "task_security_group_id" {
  value = aws_security_group.task.id
}

output "shared_vpce_security_group_id" {
  value = local.admin_console_enabled ? aws_security_group.admin_vpc_endpoint[0].id : null
}

output "diarization_worker_private_url" {
  value = var.enable_diarization_worker ? local.effective_diarization_worker_url : null
}

output "diarization_worker_service_name" {
  value = var.enable_diarization_worker ? aws_ecs_service.diarization_worker[0].name : null
}

output "diarization_worker_launch_type" {
  value = var.enable_diarization_worker ? var.diarization_worker_launch_type : null
}

output "diarization_worker_gpu_autoscaling_group_name" {
  value = local.diarization_worker_gpu_capacity_enabled ? aws_autoscaling_group.diarization_worker_gpu[0].name : null
}

output "text_intelligence_worker_private_url" {
  value = var.enable_text_intelligence_worker ? local.effective_text_intelligence_worker_url : null
}

output "text_intelligence_worker_auth_token_secret_arn" {
  value     = aws_secretsmanager_secret.text_intelligence_worker_auth_token.arn
  sensitive = true
}

output "text_intelligence_worker_security_group_id" {
  value = var.enable_text_intelligence_worker ? aws_security_group.text_intelligence_worker[0].id : null
}

output "text_intelligence_worker_service_name" {
  value = var.enable_text_intelligence_worker ? aws_ecs_service.text_intelligence_worker[0].name : null
}

output "text_intelligence_worker_launch_type" {
  value = var.enable_text_intelligence_worker ? var.text_intelligence_worker_launch_type : null
}

output "text_intelligence_reuses_diarization_worker_gpu_capacity" {
  value = local.text_intelligence_reuses_diarization_worker_gpu_capacity
}

output "text_intelligence_worker_gpu_autoscaling_group_name" {
  value = (
    local.text_intelligence_reuses_diarization_worker_gpu_capacity
    ? aws_autoscaling_group.diarization_worker_gpu[0].name
    : (
      local.text_intelligence_worker_gpu_capacity_enabled
      ? aws_autoscaling_group.text_intelligence_worker_gpu[0].name
      : null
    )
  )
}

output "admin_console_enabled" {
  value = local.admin_console_enabled
}

output "admin_console_private_url" {
  value = var.enable_admin_console ? "https://${var.admin_domain_name}/admin" : null
}

output "admin_console_public_url" {
  value = var.enable_public_admin_console ? "https://${var.public_admin_domain_name}/admin" : null
}

output "admin_internal_alb_dns_name" {
  value = var.enable_admin_console ? aws_lb.admin[0].dns_name : null
}

output "admin_public_alb_dns_name" {
  value = var.enable_public_admin_console ? aws_lb.admin_public[0].dns_name : null
}

output "admin_public_godaddy_cname" {
  value = var.enable_public_admin_console ? {
    name  = replace(var.public_admin_domain_name, ".agenticisolation.com", "")
    type  = "CNAME"
    value = aws_lb.admin_public[0].dns_name
  } : null
}

output "admin_private_hosted_zone_id" {
  value = var.enable_admin_console ? (var.admin_private_hosted_zone_id != "" ? var.admin_private_hosted_zone_id : aws_route53_zone.admin_private[0].zone_id) : null
}

output "admin_users_table_name" {
  value = local.admin_console_enabled ? aws_dynamodb_table.admin_users[0].name : null
}

output "admin_token_ledger_table_name" {
  value = local.admin_console_enabled ? aws_dynamodb_table.admin_token_ledger[0].name : null
}

output "admin_audit_table_name" {
  value = local.admin_console_enabled ? aws_dynamodb_table.admin_audit[0].name : null
}

output "admin_session_secret_arn" {
  value     = local.admin_console_enabled ? aws_secretsmanager_secret.admin_session_secret[0].arn : null
  sensitive = true
}

output "admin_certificate_validation_records" {
  value = var.admin_request_certificate ? [
    for option in aws_acm_certificate.admin[0].domain_validation_options : {
      name  = option.resource_record_name
      type  = option.resource_record_type
      value = option.resource_record_value
    }
  ] : []
}

output "admin_public_certificate_validation_records" {
  value = var.public_admin_request_certificate ? [
    for option in aws_acm_certificate.public_admin[0].domain_validation_options : {
      name  = option.resource_record_name
      type  = option.resource_record_type
      value = option.resource_record_value
    }
  ] : []
}

output "admin_public_requested_certificate_arn" {
  value = var.public_admin_request_certificate ? aws_acm_certificate.public_admin[0].arn : null
}

output "admin_public_cognito_user_pool_id" {
  value = var.enable_public_admin_console ? aws_cognito_user_pool.admin[0].id : null
}

output "admin_public_cognito_user_pool_client_id" {
  value = var.enable_public_admin_console ? aws_cognito_user_pool_client.admin[0].id : null
}

output "admin_public_cognito_domain" {
  value = var.enable_public_admin_console ? aws_cognito_user_pool_domain.admin[0].domain : null
}

output "admin_client_vpn_endpoint_id" {
  value = var.enable_admin_client_vpn ? aws_ec2_client_vpn_endpoint.admin[0].id : null
}
