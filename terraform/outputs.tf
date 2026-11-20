output "ecr_url" {
  value = aws_ecr_repository.profile.repository_url
}

output "secret_arn" {
  value = aws_secretsmanager_secret.profile.arn
}

output "ecs_services" {
  value = [
    aws_ecs_service.api.name,
    aws_ecs_service.worker.name,
    aws_ecs_service.relay.name
  ]
}

output "alerts_topic_arn" {
  value = aws_sns_topic.alerts.arn
}

output "queue_url" {
  value = data.aws_sqs_queue.profile.url
}

output "event_bus_arn" {
  value = data.aws_cloudwatch_event_bus.main.arn
}
