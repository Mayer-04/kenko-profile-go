# Rol de ejecución: pull ECR + logs + lectura secreto (común a los 3 servicios)
resource "aws_iam_role" "execution" {
  name = "kenko-dev-profile-execution"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })
}

resource "aws_iam_role_policy_attachment" "execution_base" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_iam_policy" "execution_secrets" {
  name = "kenko-dev-profile-execution-secrets"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "secretsmanager:GetSecretValue",
        "secretsmanager:DescribeSecret"
      ]
      Resource = aws_secretsmanager_secret.profile.arn
    }]
  })
}

resource "aws_iam_role_policy_attachment" "execution_secrets_attach" {
  role       = aws_iam_role.execution.name
  policy_arn = aws_iam_policy.execution_secrets.arn
}

# --- Task roles mínimo privilegio ---
resource "aws_iam_role" "api_task" {
  name = "kenko-dev-profile-api-task"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })
}

# api: Mongo + S3 (PDFs de certificates/history/authz) + EventBridge PutEvents.
resource "aws_iam_policy" "api_docs" {
  name = "kenko-dev-profile-api-docs"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:PutObject",
          "s3:GetObject"
        ]
        Resource = "arn:aws:s3:::${var.documents_bucket_name}/*"
      },
      {
        Effect   = "Allow"
        Action   = ["events:PutEvents"]
        Resource = data.aws_cloudwatch_event_bus.main.arn
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "api_docs_attach" {
  role       = aws_iam_role.api_task.name
  policy_arn = aws_iam_policy.api_docs.arn
}

resource "aws_iam_role" "worker_task" {
  name = "kenko-dev-profile-worker-task"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })
}

resource "aws_iam_policy" "worker_sqs" {
  name = "kenko-dev-profile-worker-sqs"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "sqs:ReceiveMessage",
        "sqs:DeleteMessage",
        "sqs:GetQueueAttributes",
        "sqs:ChangeMessageVisibility"
      ]
      Resource = [
        data.aws_sqs_queue.profile.arn,
        data.aws_sqs_queue.profile_dlq.arn
      ]
    }]
  })
}

resource "aws_iam_role_policy_attachment" "worker_sqs_attach" {
  role       = aws_iam_role.worker_task.name
  policy_arn = aws_iam_policy.worker_sqs.arn
}

resource "aws_iam_role" "relay_task" {
  name = "kenko-dev-profile-relay-task"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action    = "sts:AssumeRole"
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
    }]
  })
}

resource "aws_iam_policy" "relay_eb" {
  name = "kenko-dev-profile-relay-eb"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect   = "Allow"
      Action   = ["events:PutEvents"]
      Resource = data.aws_cloudwatch_event_bus.main.arn
    }]
  })
}

resource "aws_iam_role_policy_attachment" "relay_eb_attach" {
  role       = aws_iam_role.relay_task.name
  policy_arn = aws_iam_policy.relay_eb.arn
}
