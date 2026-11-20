resource "aws_secretsmanager_secret" "profile" {
  name                    = "kenko/dev/profile"
  description             = "Secreto único profile dev: MONGO_ATLAS_URI (+ futuros). No poner valores reales en tfvars"
  recovery_window_in_days = 0
}

# Placeholder inicial. El valor real se pone con:
# aws secretsmanager put-secret-value --secret-id kenko/dev/profile \
#   --secret-string '{"MONGO_ATLAS_URI":"mongodb+srv://...","MONGO_APP_DATABASE":"kenko_eps"}'
# NUNCA imprimir passwords en logs (mask ://***:***@).
resource "aws_secretsmanager_secret_version" "profile_placeholder" {
  secret_id = aws_secretsmanager_secret.profile.id
  secret_string = jsonencode({
    MONGO_ATLAS_URI    = "REEMPLAZAR-con-put-secret-value"
    MONGO_APP_DATABASE = var.mongo_app_database
  })

  lifecycle {
    ignore_changes = [secret_string]
  }
}
