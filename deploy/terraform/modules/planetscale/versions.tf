terraform {
  required_version = ">= 1.10"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.0"
    }
    planetscale = {
      source  = "planetscale/planetscale"
      version = "~> 1.3"
    }
  }
}
