resource "aws_ecr_repository" "profile" {
  name                 = "kenko-dev-profile"
  image_tag_mutability = "IMMUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "AES256"
  }
}

resource "aws_ecr_lifecycle_policy" "profile" {
  repository = aws_ecr_repository.profile.name
  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Conservar últimas 5 taggeadas"
        selection = {
          tagStatus     = "tagged"
          tagPrefixList = ["0."]
          countType     = "imageCountMoreThan"
          countNumber   = 5
        }
        action = { type = "expire" }
      },
      {
        rulePriority = 2
        description  = "Borrar untagged >1d"
        selection = {
          tagStatus   = "untagged"
          countType   = "sinceImagePushed"
          countUnit   = "days"
          countNumber = 1
        }
        action = { type = "expire" }
      }
    ]
  })
}
