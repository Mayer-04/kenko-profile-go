# SG propio de profile (dev): evita que el apply de auth borre la regla 8080
# del SG compartido. Solo dev: 8080 público + egress todo.
resource "aws_security_group" "profile" {
  name        = "kenko-dev-profile-ecs-sg"
  description = "ECS profile dev-only (8080 publico)"
  vpc_id      = var.vpc_id

  tags = {
    Name = "kenko-dev-profile-ecs-sg"
  }
}

resource "aws_vpc_security_group_ingress_rule" "profile_8080_own" {
  security_group_id = aws_security_group.profile.id
  description       = "kenko-dev-profile-api-dev-only"
  ip_protocol       = "tcp"
  from_port         = 8080
  to_port           = 8080
  cidr_ipv4         = "0.0.0.0/0"
}

resource "aws_vpc_security_group_egress_rule" "profile_all" {
  security_group_id = aws_security_group.profile.id
  description       = "egress todo dev"
  ip_protocol       = "-1"
  cidr_ipv4         = "0.0.0.0/0"
}

# Los endpoints de VPC (logs/events) tienen PrivateDnsEnabled=true: todo el
# tráfico a CloudWatch/EventBridge va a IPs privadas del endpoint y necesita
# ingress 443 en el SG del endpoint. Sin esta regla las tareas nuevas mueren
# con ResourceInitializationError (las viejas sobreviven por flujos ya
# establecidos, los SG son stateful). FRÁGIL: el equipo auth gestiona ese SG
# con su TF y puede borrarla; pedirles que añadan nuestro SG en su código.
resource "aws_vpc_security_group_ingress_rule" "endpoints_from_profile" {
  security_group_id            = var.vpc_endpoint_sg_id
  description                  = "HTTPS desde kenko-dev-profile-ecs-sg (logs/events)"
  ip_protocol                  = "tcp"
  from_port                    = 443
  to_port                      = 443
  referenced_security_group_id = aws_security_group.profile.id
}
