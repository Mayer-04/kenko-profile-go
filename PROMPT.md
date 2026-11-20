# PROMPT AGENTE — Replicar despliegue Kenko (dev) en otro microservicio

> Pega esto a un agente con acceso a AWS + repo del microservicio. Está basado en el despliegue real de `notifications` (2026-11-19, dev 100%).
> No inventes IDs: usa los de §2. Si algo difiere (Mongo vs Postgres, API vs worker), sigue §3.

## 1. OBJETIVO

Desplegar el microservicio de `APP_REPO` en AWS dev, reutilizando la infra de `auth`, con el mismo patrón de `notifications`:
`ECR (IMMUTABLE) → Secrets Manager → ECS Fargate en cluster existente → RDS compartido o Mongo según motor → SQS/EventBridge solo si aplica → CloudWatch Logs+Alarmas+SNS → CI mínimo`.
Termina con servicio `1/1 RUNNING` + prueba e2e + `terraform apply` limpio + `pytest` verde.

## 2. DATOS FIJOS — NO CREAR, REUSAR (us-east-1, cuenta 137696817246)

```
Cuenta: 137696817246 / Región: us-east-1 / Caller: jd_rod
VPC: vpc-014ec1d1e62527b66 (10.0.0.0/16)
Subredes públicas: subnet-0043c71cab6cc9422 (1a, 10.0.1.0/24, kenko-dev-auth-public-a), subnet-053d5bc1211afe9aa (1b, 10.0.2.0/24, kenko-dev-auth-public-b)
Subredes privadas: subnet-0d29248a08c9b7b81 (1a, 10.0.11.0/24), subnet-0a3b0b112fb2b327f (1b, 10.0.12.0/24)
SGs: sg-033261f5a0531fe94 (kenko-dev-auth-ecs-sg, 3000 público + egress todo), sg-0017734fa0ffdc093 (kenko-dev-auth-rds-sg, 5432 solo desde sg-033... y sg-0fdc...), sg-0fdc495fe9a1e6d11 (kenko-dev-auth-ec2-sg)
Cluster ECS: kenko-dev-auth-cluster (servicio existente kenko-dev-auth-api)
RDS Postgres compartido: kenko-dev-auth-db (postgres 18.6, db.t4g.micro, 20GB, endpoint kenko-dev-auth-db.cezucwi0oldg.us-east-1.rds.amazonaws.com:5432, DBName kenko_auth, MasterUser kenko_auth, VPC privada, Encrypted true, PubliclyAccessible false, sin NAT)
Admin DB en SSM: /kenko/dev/rds_postgres_url, /kenko/dev/db_password, /kenko/dev/jwt_secret (con WithDecryption)
Bus: kenko-dev-bus (arn:aws:events:us-east-1:137696817246:event-bus/kenko-dev-bus)
S3 PDFs: kenko-dev-documents-files (existe) / Backend TF: s3://kenko-tfstate-137696817246-us-east-1 (encrypt + use_lockfile)
SES sandbox: ProductionAccessEnabled=false, 200/24h, 1/seg. Verificados: kenko.salud.eps@gmail.com (SES_SENDER dev), jdanielrr98@gmail.com, andres.20.chaves@gmail.com, jupa.roqui7@hotmail.com
SNS alertas notifications: arn:aws:sns:us-east-1:137696817246:kenko-dev-notifications-alerts (Active)
EC2 bastión SSM: i-0803e33ee6b61438d (Amazon Linux 2023, Docker 25, Online, 10.0.1.198 / 52.203.252.87, SG sg-0fdc..., rol kenko-dev-auth-ec2 solo ssm:Get + events:PutEvents)
```

## 3. DECIDE TIPO EN 1 MINUTO

1. Inspecciona `APP_REPO`: `Dockerfile`, `pyproject/package.json`, `src/config`, `terraform/*.tf`, `README`.
2. Motor DB:
   - Si `SQLAlchemy/psycopg2` o `pg`/`DATABASE_URL` Postgres → **reusa RDS**: crea `DATABASE <svc>` + `USER <svc>_user` vía SSM en EC2 (abajo), secreto `kenko/dev/<svc>`. NUNCA otro RDS en dev.
   - Si `mongoose/mongodb` o `MONGO_URL` → **NO uses RDS**. Usa Atlas M0 gratis o Mongo en EC2. Pide al usuario la URI y guárdala en secreto `kenko/dev/<svc>`.
3. Runtime:
   - API HTTP (`/health`, puerto 3000, Hono/Express/FastAPI) → ECS Fargate + **ALB o puerto público dev** (auth hoy usa EC2 `:3000` + SG abierto; en ECS usa mismo SG `sg-033...` + public IP en dev, ALB en prod).
   - Worker SQS (loop, sin puerto, como notifications) → ECS Fargate **sin ALB**, `assignPublicIp=ENABLED`, subredes públicas, health por `ApproximateAgeOfOldestMessage` + DLQ.
   - Relay Lambda (como `kenko-dev-auth-relay`, `arm64`, `512MB`, `PORT=8080`) → ECR + Lambda + IAM mínimo, sin VPC.

## 4. CONVENCIONES OBLIGATORIAS (copia notifications)

```
ECR: kenko-dev-<svc> (IMMUTABLE, scan on push, lifecycle 5 taggeadas, untagged >1d). Tag nuevo por build (:0.1.x). NUNCA re-push mismo tag.
Logs: /ecs/kenko-dev-<svc> retención 14d
Secreto: kenko/dev/<svc> JSON único {"DATABASE_URL|MONGO_URL":"...","JWT_SECRET":"...","...":"..."} + lifecycle ignore_changes
IAM: execution (ECR pull + logs + secrets Get) y task (SQS/S3/SES/logs/secrets según necesite, mínimo privilegio, sin AWS_ACCESS_KEY)
ECS: family kenko-dev-<svc>, FARGATE, ARM64, 256/512 dev, execution+task roles, stopTimeout 30 (worker) / healthCheck /health (API), deploymentCircuitBreaker+rollback
TF backend: mismo bucket, key kenko-<svc>-dev.tfstate. Prod: -reconfigure -backend-config="key=kenko-<svc>-prod.tfstate"
TF vars: vpc_id, ecs_cluster_name=kenko-dev-auth-cluster, public_subnet_ids=[subnet-0043...,subnet-053d...], ecs_security_group_id=sg-033261f5a0531fe94, image_tag, desired_count (0 primer apply, 1 tras push+migración), alert_email=kenko.salud.eps@gmail.com
Alarmas: DLQ Visible>=1/5min + Age>300s/2x5min por cola (for_each), SNS topic kenko-dev-<svc>-alerts + sub email (pedir confirmación)
CI: .github/workflows/ci.yml (pytest/npm test + lint + terraform fmt -check + init -backend=false + validate). Plan/apply manual como auth.
```

## 5. PASOS (en orden, no saltar)

1. **Fase 0 local:** completa `Dockerfile` (python:3.13-slim multistage, nonroot, STOPSIGNAL SIGTERM o `node:20-slim` según stack), `.dockerignore` (excluye .env, *.tfstate, tests), `docker-compose.yml` (app + `postgres:18.6` si Postgres en `5433`, o `mongo:7` si Mongo). Unifica envs (`SQS_QUEUE_URLS` plural, `RECEIVE_*=1/20` si worker). Deja `pytest` verde.
2. **Backend TF:** crea `backend.tf` (S3 dev key), `data.tf` (data VPC/cluster/RDS por ID, no crear), `ecr.tf`, `logs.tf`, `secrets.tf` (placeholder + ignore_changes), `iam.tf`, `ecs.tf` (o `lambda.tf`), `alerts.tf`, variables/outputs (`ecr_url`, `secret_arn`, `ecs_service`, `alerts_topic_arn`). `terraform fmt + init -migrate-state -force-copy + validate`.
3. **DB:** si Postgres, vía SSM en `i-0803...` (tiene acceso RDS, Docker, sin psql): lee admin de SSM, genera `PASS=$(openssl rand -base64 24|tr -d '/+='|cut -c1-24)`, `docker run postgres:18.6 psql` crea `DATABASE <svc>` + `USER <svc>_user` + grants schema public, `put-secret-value` al secreto. Verifica `SELECT 1` como nuevo user. Si Mongo, pide URI y haz `put-secret-value`. NUNCA imprimas passwords en logs (mask `://***:***@`).
4. **Imagen:** `aws ecr get-login-password | docker login`, `docker buildx build --platform linux/arm64 -t <ecr>:0.1.x --push .`. Si Dockerfile cambió, sube tag nuevo (IMMUTABLE).
5. **Migración (solo Postgres):** incluye `alembic.ini+migrations/` en la imagen. `aws ecs run-task --cluster kenko-dev-auth-cluster --task-definition <svc> --launch-type FARGATE --network-configuration awsvpcConfiguration={subnets=[...public...],securityGroups=[sg-033...],assignPublicIp=ENABLED} --overrides '{"containerOverrides":[{"name":"<container>","command":["alembic","upgrade","head"]}]}'`. Espera `STOPPED exit 0`, verifica vía SSM `SELECT version_num FROM alembic_version` + `\dt` owner `<svc>_user`.
6. **Prende:** `desired_count=0→1` en `envs/dev.tfvars` + `terraform apply`. Espera `running 1/1`, `aws logs tail /ecs/kenko-dev-<svc> --since 10m`.
7. **E2E:** API: `GET /health 200`, `POST /auth/register` con email verificado → `resultado=sent` + fila `sent` en `<svc>` DB + correo (spam). Worker: `put-events` al bus → cola → `sent`. Solo a verificados (sandbox).
8. **Cierre:** `terraform output`, `pytest`, `ruff`, actualiza `DEPLOYMENT_<svc>.md` con IDs reales + justificación sandbox (`$0.02/día cap`, `$0.10/1000 + $0.12/GB`, transaccional vs masivo).

## 6. VALIDACIONES ANTES DE DECIR “LISTO”

- `terraform validate` OK, `plan`: solo +esperados, `apply` sin errores
- `aws ecs describe-services --cluster kenko-dev-auth-cluster --services kenko-dev-<svc>` → `running==desired==1`
- `aws logs tail` muestra `worker iniciado` o `listening :3000`, sin `NoCredentials` (revisa Task Role)
- DB: tablas owner `<svc>_user`, `alembic_version` correcto
- SQS: `ApproximateNumberOfMessages` baja, DLQ `0`
- SNS suscripción `Active` (no `PendingConfirmation`)
- `.env` NUNCA con secretos reales, `*.tfstate` NUNCA en git, `AWS_ACCESS_KEY` NUNCA en código

## 7. PROHIBIDO

- Crear VPC/cluster/RDS nuevos en dev (reusar §2). Prod sí con `prod.tfvars`.
- Re-push tag inmutable, `desired=1` sin imagen, `latest` móvil.
- Poner `DATABASE_URL` real en `tfvars`/imagen/env plano (solo Secrets + ignore_changes).
- Correr SQL/Mongo admin desde laptop (RDS privado no resuelve DNS fuera VPC; usa SSM en EC2 o `ecs run-task`).
- Pedir `Production Access` SES en dev (quédate sandbox 200/día; prod con cuota baja + Budget Alert).
