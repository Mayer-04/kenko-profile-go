terraform {
  backend "s3" {
    bucket       = "kenko-tfstate-137696817246-us-east-1"
    key          = "kenko-profile-dev.tfstate"
    region       = "us-east-1"
    encrypt      = true
    use_lockfile = true
  }
}
