# AWS VPC Subnet Example
# Uses the cidrblock provider to allocate subnets for AWS VPC

terraform {
  required_providers {
    cidrblock = {
      source  = "finext/cidrblock"
    }
    aws = {
      source  = "hashicorp/aws"
    }
  }
}

provider "aws" {
  region = var.region
}

# CIDR pool for VPC subnets
resource "cidrblock_pool" "vpc_subnets" {
  cidr         = "10.0.0.0/16"
  organization = var.organization
  project      = var.project_id
  network      = "vpc-main"

  allocations = {
    public_a = {
      prefix_size     = 24
      reserve_sibling = false
    }

    public_b = {
      prefix_size     = 24
      reserve_sibling = false
    }

    private_a = {
      prefix_size     = 23
      reserve_sibling = true
    }

    private_b = {
      prefix_size     = 23
      reserve_sibling = true
    }

    database = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

# AWS VPC
resource "aws_vpc" "main" {
  cidr_block           = "10.0.0.0/16"
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name        = "vpc-main"
    Organization = var.organization
    Project     = var.project_id
  }
}

# Public Subnet A
resource "aws_subnet" "public_a" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrblock_pool.vpc_subnets.allocations["public_a"].allocated_cidr
  availability_zone = "${var.region}a"

  tags = {
    Name = "public-a"
  }
}

# Public Subnet B
resource "aws_subnet" "public_b" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrblock_pool.vpc_subnets.allocations["public_b"].allocated_cidr
  availability_zone = "${var.region}b"

  tags = {
    Name = "public-b"
  }
}

# Private Subnet A
resource "aws_subnet" "private_a" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrblock_pool.vpc_subnets.allocations["private_a"].allocated_cidr
  availability_zone = "${var.region}a"

  tags = {
    Name = "private-a"
  }
}

# Private Subnet B
resource "aws_subnet" "private_b" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrblock_pool.vpc_subnets.allocations["private_b"].allocated_cidr
  availability_zone = "${var.region}b"

  tags = {
    Name = "private-b"
  }
}

# Database Subnet
resource "aws_subnet" "database" {
  vpc_id            = aws_vpc.main.id
  cidr_block        = cidrblock_pool.vpc_subnets.allocations["database"].allocated_cidr
  availability_zone = "${var.region}a"

  tags = {
    Name = "database"
  }
}

output "vpc_cidr" {
  value = aws_vpc.main.cidr_block
}

output "subnets" {
  value = {
    public_a  = cidrblock_pool.vpc_subnets.allocations["public_a"].allocated_cidr
    public_b  = cidrblock_pool.vpc_subnets.allocations["public_b"].allocated_cidr
    private_a = cidrblock_pool.vpc_subnets.allocations["private_a"].allocated_cidr
    private_b = cidrblock_pool.vpc_subnets.allocations["private_b"].allocated_cidr
    database  = cidrblock_pool.vpc_subnets.allocations["database"].allocated_cidr
  }
}
