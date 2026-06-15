# ==============================================================================
# Terraform Core & Provider Definitions
# ==============================================================================

terraform {
  required_version = ">= 1.5.0"
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

# Provider orchestration implementing advanced state protections
provider "cidrblock" {
  # Safety Switch: Actively blocks any 'terraform apply' execution that attempts
  # to drop an existing subnet key from the allocations map, protecting live 
  # downstream AWS elastic network interfaces (ENIs).
  prevent_subnet_destruction = true
}

provider "aws" {
  region = var.aws_region
}

# ==============================================================================
# Structural Input Variables
# ==============================================================================

variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "environment" {
  type    = string
  default = "production"
}

# ==============================================================================
# Step 1: The Core IPAM Allocation Pool
# ==============================================================================

resource "cidrblock_pool" "vpc_matrix" {
  cidr                = "10.250.0.0/16"
  organization        = "finext"
  project             = "enterprise-mesh"
  network             = "vpc-primary"
  allocation_strategy = "BEST" # Packs networks efficiently using best-fit binary tracking

  # ADVANCED TRACKING: The underlying engine automatically sorts keys by network size
  # descending (largest blocks first) to completely wipe out dead-space allocation gaps.
  allocations = {
    # PASS 1: Sorted First (Largest block: /20 = 4096 IPs).
    # lands on base 10.250.0.0/20. Spawns its mathematical multi-AZ shadow buddy block
    # cleanly at 10.250.16.0/20. Total space locked: 10.250.0.0 - 10.250.31.255.
    private_compute = {
      prefix_size     = 20
      reserve_sibling = true
    }

    # PASS 2: Sorted Second (Medium block: /24 = 256 IPs). Tied with isolated_db.
    # Wins alphabetical tie-breaker. Aligns on next free block boundary: 10.250.32.0/24.
    # Reserves its companion multi-AZ shadow tier cleanly at 10.250.33.0/24.
    public_dmz = {
      prefix_size     = 24
      reserve_sibling = true
    }

    # PASS 3: Sorted Third (Medium block: /24 = 256 IPs).
    # Evaluated next. Safely skips over the locked public_dmz sibling boundaries
    # to claim 10.250.34.0/24 and its multi-AZ companion at 10.250.35.0/24.
    isolated_db = {
      prefix_size     = 24
      reserve_sibling = true
    }

    # PASS 4: Sorted Fourth (Smallest block: /26 = 64 IPs).
    # No sibling reservation requested. Claims the clean boundary at 10.250.36.0/26.
    edge_vpn_transit = {
      prefix_size     = 26
      reserve_sibling = false
    }
  }
}

# ==============================================================================
# Step 2: Real-Time Telemetry via Hydrated Data Source
# ==============================================================================

# By passing the resource's computed composite ID token, this data source executes
# in-process immediately following the resource change, capturing calculated engine logs.
data "cidrblock_pool" "telemetry" {
  id = cidrblock_pool.vpc_matrix.id
}

# ==============================================================================
# Step 3: Materialize Network Topologies inside AWS
# ==============================================================================

resource "aws_vpc" "cloud_grid" {
  cidr_block           = cidrblock_pool.vpc_matrix.cidr
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    Name = "production-vpc-mesh"
  }
}

# --- Private Compute Tiers (Multi-AZ) ---

resource "aws_subnet" "private_compute_aza" {
  vpc_id            = aws_vpc.cloud_grid.id
  cidr_block        = cidrblock_pool.vpc_matrix.allocations["private_compute"].allocated_cidr
  availability_zone = "${var.aws_region}a"

  tags = { Name = "private-compute-us-east-1a" }
}

resource "aws_subnet" "private_compute_azb" {
  vpc_id            = aws_vpc.cloud_grid.id
  cidr_block        = cidrblock_pool.vpc_matrix.allocations["private_compute"].sibling_cidr
  availability_zone = "${var.aws_region}b"

  tags = { Name = "private-compute-us-east-1b-buddy" }
}

# --- Public DMZ Tiers (Multi-AZ) ---

resource "aws_subnet" "public_dmz_aza" {
  vpc_id            = aws_vpc.cloud_grid.id
  cidr_block        = cidrblock_pool.vpc_matrix.allocations["public_dmz"].allocated_cidr
  availability_zone = "${var.aws_region}a"

  tags = { Name = "public-dmz-us-east-1a" }
}

resource "aws_subnet" "public_dmz_azb" {
  vpc_id            = aws_vpc.cloud_grid.id
  cidr_block        = cidrblock_pool.vpc_matrix.allocations["public_dmz"].sibling_cidr
  availability_zone = "${var.aws_region}b"

  tags = { Name = "public-dmz-us-east-1b-buddy" }
}

# --- Database Storage Tiers (Multi-AZ) ---

resource "aws_subnet" "isolated_db_aza" {
  vpc_id            = aws_vpc.cloud_grid.id
  cidr_block        = cidrblock_pool.vpc_matrix.allocations["isolated_db"].allocated_cidr
  availability_zone = "${var.aws_region}a"

  tags = { Name = "isolated-db-us-east-1a" }
}

resource "aws_subnet" "isolated_db_azb" {
  vpc_id            = aws_vpc.cloud_grid.id
  cidr_block        = cidrblock_pool.vpc_matrix.allocations["isolated_db"].sibling_cidr
  availability_zone = "${var.aws_region}b"

  tags = { Name = "isolated-db-us-east-1b-buddy" }
}

# --- Edge Management Gateway ---

resource "aws_subnet" "transit_gateway" {
  vpc_id            = aws_vpc.cloud_grid.id
  cidr_block        = cidrblock_pool.vpc_matrix.allocations["edge_vpn_transit"].allocated_cidr
  availability_zone = "${var.aws_region}a"

  tags = { Name = "edge-vpn-transit-standalone" }
}

# ==============================================================================
# Step 4: Verification & Capacity Planning Outputs
# ==============================================================================

output "aws_vpc_metadata" {
  description = "The verified baseline networking blocks deployed to Amazon Web Services."
  value = {
    vpc_id     = aws_vpc.cloud_grid.id
    supernet   = aws_vpc.cloud_grid.cidr_block
    routing_id = data.cidrblock_pool.telemetry.id
  }
}

output "ipam_pool_metrics" {
  description = "Live structural matrix capacities compiled out of the engine registry map."
  value = {
    total_ips     = data.cidrblock_pool.telemetry.metrics.total_ips
    allocated_ips = data.cidrblock_pool.telemetry.metrics.allocated_ips
    reserved_ips  = data.cidrblock_pool.telemetry.metrics.reserved_ips
    available_ips = data.cidrblock_pool.telemetry.metrics.available_ips
  }
}

output "unallocated_expansion_slices" {
  description = "Contiguous unallocated chunks discovered inside the matrix. Use these boundaries for future scale operations."
  value       = data.cidrblock_pool.telemetry.available_slices
}

