output "ecr_repository_url" {
  value = aws_ecr_repository.app.repository_url
}

output "alb_dns_name" {
  value = aws_lb.app.dns_name
}

output "domain_cname_target" {
  value = aws_lb.app.dns_name
}

output "cognito_user_pool_id" {
  value = aws_cognito_user_pool.users.id
}

output "cognito_user_pool_client_id" {
  value = aws_cognito_user_pool_client.web.id
}

output "transcript_bucket" {
  value = aws_s3_bucket.transcripts.bucket
}

output "notes_table" {
  value = aws_dynamodb_table.notes.name
}

output "audit_table" {
  value = aws_dynamodb_table.audit.name
}

output "service_security_group_id" {
  value = aws_security_group.service.id
}

output "text_intelligence_enabled" {
  value = var.text_intelligence_enabled
}
