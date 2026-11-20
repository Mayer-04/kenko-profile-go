resource "aws_sns_topic" "alerts" {
  name = "kenko-dev-profile-alerts"
}

resource "aws_sns_topic_subscription" "alerts_email" {
  topic_arn = aws_sns_topic.alerts.arn
  protocol  = "email"
  endpoint  = var.alert_email
}

# Alarma: mensajes visibles en DLQ >=1 en 5min
resource "aws_cloudwatch_metric_alarm" "dlq_visible" {
  alarm_name          = "kenko-dev-profile-dlq-visible"
  comparison_operator = "GreaterThanOrEqualToThreshold"
  evaluation_periods  = 1
  metric_name         = "ApproximateNumberOfMessagesVisible"
  namespace           = "AWS/SQS"
  period              = 300
  statistic           = "Maximum"
  threshold           = 1
  alarm_description   = "Hay mensajes en DLQ profile"
  alarm_actions       = [aws_sns_topic.alerts.arn]
  ok_actions          = [aws_sns_topic.alerts.arn]
  dimensions = {
    QueueName = "${var.sqs_queue_name}-dlq"
  }
}

# Alarma: edad del mensaje más viejo >300s (2 periodos 5min)
resource "aws_cloudwatch_metric_alarm" "queue_age" {
  alarm_name          = "kenko-dev-profile-queue-age"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  metric_name         = "ApproximateAgeOfOldestMessage"
  namespace           = "AWS/SQS"
  period              = 300
  statistic           = "Maximum"
  threshold           = 300
  alarm_description   = "Worker profile no drena la cola"
  alarm_actions       = [aws_sns_topic.alerts.arn]
  ok_actions          = [aws_sns_topic.alerts.arn]
  dimensions = {
    QueueName = var.sqs_queue_name
  }
}
