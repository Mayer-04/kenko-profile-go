variable "aws_region" {
  description = "Región AWS"
  type        = string
  default     = "us-east-1"
}

variable "vpc_id" {
  description = "VPC existente de auth (no crear)"
  type        = string
}

variable "ecs_cluster_name" {
  description = "Cluster ECS existente (no crear)"
  type        = string
}

variable "public_subnet_ids" {
  description = "Subredes públicas existentes para Fargate dev"
  type        = list(string)
}

variable "ecs_security_group_id" {
  description = "SG existente kenko-dev-auth-ecs-sg (legacy, ya no se usa para profile; se conserva por compatibilidad)"
  type        = string
  default     = "sg-033261f5a0531fe94"
}

variable "vpc_endpoint_sg_id" {
  description = "SG de los VPC endpoints (logs/events) de auth. Se añade regla 443 desde nuestro SG. Pedir al equipo auth que lo incluya en su TF."
  type        = string
  default     = "sg-02d7fd1ba3cf7bbed"
}

variable "image_tag" {
  description = "Tag inmutable ECR (ej 0.1.0). NUNCA re-push mismo tag"
  type        = string
}

variable "desired_count" {
  description = "0 primer apply, 1 tras push+migración"
  type        = number
  default     = 0
}

variable "alert_email" {
  description = "Email para SNS (requiere confirmación)"
  type        = string
}

# --- App (defaults = dev actual) ---
variable "sqs_queue_name" {
  type    = string
  default = "kenko-dev-profile-authentication"
}

variable "event_bus_name" {
  type    = string
  default = "kenko-dev-bus"
}

variable "mongo_app_database" {
  type    = string
  default = "kenko_eps"
}

variable "aws_account_id" {
  type    = string
  default = "137696817246"
}

variable "environment" {
  type    = string
  default = "development"
}

variable "port" {
  type    = string
  default = "8080"
}
