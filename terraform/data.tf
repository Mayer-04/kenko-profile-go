data "aws_vpc" "main" {
  id = var.vpc_id
}

data "aws_ecs_cluster" "main" {
  cluster_name = var.ecs_cluster_name
}

data "aws_security_group" "ecs" {
  id = var.ecs_security_group_id
}

data "aws_sqs_queue" "profile" {
  name = var.sqs_queue_name
}

data "aws_sqs_queue" "profile_dlq" {
  name = "${var.sqs_queue_name}-dlq"
}

data "aws_cloudwatch_event_bus" "main" {
  name = var.event_bus_name
}

data "aws_caller_identity" "current" {}
