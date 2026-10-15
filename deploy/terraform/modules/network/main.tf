# Network: one VPC with public subnets only, and no NAT Gateway.
#
# Why public subnets with security groups instead of private subnets + NAT:
# every task needs general internet egress anyway (Google and Microsoft
# OAuth, Gmail and Graph mailbox APIs, customer webhooks, PostHog), so VPC
# interface endpoints (about $7/month each per AZ) would not remove the
# internet path, and a NAT Gateway costs about $33/month per AZ plus
# $0.045/GB processed. Tasks get a public IP for egress only: their security
# groups admit nothing from the internet, only the ALB and intra-cluster
# ports. S3 traffic goes through the free gateway endpoint below so
# attachment bytes never leave the AWS network.

data "aws_availability_zones" "this" {
  state = "available"
}

locals {
  azs = slice(data.aws_availability_zones.this.names, 0, var.az_count)
}

resource "aws_vpc" "this" {
  cidr_block           = var.cidr
  enable_dns_support   = true
  enable_dns_hostnames = true # Cloud Map private DNS needs both

  tags = { Name = var.name }
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id
  tags   = { Name = var.name }
}

resource "aws_subnet" "public" {
  count = var.az_count

  vpc_id                  = aws_vpc.this.id
  availability_zone       = local.azs[count.index]
  cidr_block              = cidrsubnet(var.cidr, 4, count.index)
  map_public_ip_on_launch = false # tasks opt in with assign_public_ip

  tags = { Name = "${var.name}-public-${local.azs[count.index]}" }
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id
  tags   = { Name = "${var.name}-public" }
}

resource "aws_route" "internet" {
  route_table_id         = aws_route_table.public.id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.this.id
}

resource "aws_route_table_association" "public" {
  count = var.az_count

  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

# Gateway endpoints are free and keep S3 traffic on the AWS backbone.
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = aws_vpc.this.id
  service_name      = "com.amazonaws.${var.region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = [aws_route_table.public.id]

  tags = { Name = "${var.name}-s3" }
}

# The default security group is locked down so nothing lands in it by accident.
resource "aws_default_security_group" "this" {
  vpc_id = aws_vpc.this.id
}
