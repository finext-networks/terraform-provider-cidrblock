# AWS VPC Subnet Topology Example
# Leverages the cidrblock provider to orchestrate deterministic VPC and subnet topologies

terraform {
  required_providers {
    cidrblock = {
      source = "finext-networks/cidrblock"
    }
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

# Define the IPAM calculation pool for the AWS landing zone
resource "cidrblock_pool" "aws_subnets" {
  cidr                = "10.10.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-primary"
  allocation_strategy = "FIRST" # Keys evaluated alphabetically: app_tier, web_tier

  allocations = {
    # 1. Processed First Alphabetically: Sized at /22, claims 10.10.0.0/22.
    # Valid: Perfectly left-hand aligned relative to the tree radix structure.
    # Spawns a sibling reservation at 10.10.4.0/22 to guarantee uncollided space for AzB apps.
    app_tier = {
      prefix_size     = 22
      reserve_sibling = true
    }

    # 2. Processed Second: /24 sizing scans from the bottom of the pool.
    # Skips app_tier + its sibling reservation space (10.10.0.0 to 10.10.7.255).
    # Securely claims the next aligned block boundary at 10.10.8.0/24.
    web_tier = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

# Base AWS VPC Container Network
resource "aws_vpc" "main" {
  cidr_block           = cidrblock_pool.aws_subnets.cidr
  enable_dns_hostnames = true

  tags = {
    Name = "vpc-primary"
  }
}

# Active Application Subnet (Availability Zone A)
resource "aws_subnet" "app_aza" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrblock_pool.aws_subnets.allocations["app_tier"].allocated_cidr
  availability_zone = "${var.aws_region}a"

  tags = {
    Name = "app-tier-aza"
  }
}

# High-Availability Expansion Subnet (Availability Zone B)
# Consumes the guaranteed uncollided forward sibling block calculated by the engine
resource "aws_subnet" "app_azb" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrblock_pool.aws_subnets.allocations["app_tier"].sibling_cidr
  availability_zone = "${var.aws_region}b"

  tags = {
    Name = "app-tier-azb-ha-expansion"
  }
}

# DMZ Public Web Subnet (Availability Zone A)
resource "aws_subnet" "web_aza" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrblock_pool.aws_subnets.allocations["web_tier"].allocated_cidr
  availability_zone = "${var.aws_region}a"

  tags = {
    Name = "web-tier-aza"
  }
}

output "vpc_id" {
  value = aws_vpc.main.id
}

output "app_aza_cidr" {
  value = aws_subnet.app_aza.cidr_block
}

output "app_azb_reserved_sibling_cidr" {
  value       = aws_subnet.app_azb.cidr_block
  description = "Verified, left-hand aligned buddy sibling used for seamless multi-AZ clustering"
}

