mock_provider "aws" {}
mock_provider "planetscale" {
  mock_resource "planetscale_postgres_branch_role" {
    defaults = {
      username                        = "opendesk-app.abc123"
      password                        = "p@ss/word+="
      private_access_host_url         = "us-east.private-pg.psdb.cloud"
      private_connection_service_name = "com.amazonaws.vpce.us-east-1.vpce-svc-123"
      database_name                   = "postgres"
    }
  }
}
variables {
  name                      = "opendesk"
  organization              = "opendesk"
  database                  = "opendesk"
  vpc_id                    = "vpc-1"
  subnet_ids                = ["s1", "s2", "s3"]
  client_security_group_ids = { api = "sg-a", worker = "sg-w" }
}
run "dsn" {
  command = apply
  assert {
    condition     = output.database_url == "postgresql://opendesk-app.abc123:p%40ss%2Fword%2B%3D@us-east.private-pg.psdb.cloud:6432/postgres?sslmode=verify-full"
    error_message = "pooled: ${nonsensitive(output.database_url)}"
  }
  assert {
    condition     = endswith(output.database_url_direct, ":5432/postgres?sslmode=verify-full")
    error_message = "direct"
  }
  assert {
    condition     = length(aws_vpc_endpoint.this.subnet_ids) == 2
    error_message = "subnets"
  }
  assert {
    condition     = length(aws_vpc_security_group_ingress_rule.clients) == 4
    error_message = "rules"
  }
}
