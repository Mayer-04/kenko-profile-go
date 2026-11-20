resource "aws_cloudwatch_log_group" "api" {
  name              = "/ecs/kenko-dev-profile-api"
  retention_in_days = 14
}

resource "aws_cloudwatch_log_group" "worker" {
  name              = "/ecs/kenko-dev-profile-worker"
  retention_in_days = 14
}

resource "aws_cloudwatch_log_group" "relay" {
  name              = "/ecs/kenko-dev-profile-relay"
  retention_in_days = 14
}
