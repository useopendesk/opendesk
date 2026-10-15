# Credentials come from the environment, never from files in the repo:
#   AWS:        the usual AWS_PROFILE / SSO session
#   Cloudflare: CLOUDFLARE_API_TOKEN (Zone DNS, Zone Settings, Email Routing,
#               Pages, Account read; on the help zone also SSL and Certificates
#               edit for the custom hostname fallback origin)
#   PlanetScale: PLANETSCALE_SERVICE_TOKEN_ID and PLANETSCALE_SERVICE_TOKEN
#               (a service token with access to the database's roles)

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project     = var.name
      Environment = local.env
      ManagedBy   = "terraform"
    }
  }
}

provider "cloudflare" {}

provider "planetscale" {}
