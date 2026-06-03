# GCP VPC Subnet Architecture Example
# Demonstrates FIRST, BEST, and SPARSE allocation strategies using the cidrblock provider

terraform {
  required_providers {
    cidrblock = {
      source = "finext-networks/cidrblock"
    }
    google = {
      source = "hashicorp/google"
    }
  }
}

provider "cidrblock" {
  prevent_subnet_destruction = false
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# =========================================================================
# STRATEGY 1: "FIRST" (Sequential Packing)
# =========================================================================
resource "cidrblock_pool" "strategy_first" {
  cidr                = "10.0.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-first"
  allocation_strategy = "FIRST"

  allocations = {
    # Evaluated 1st: Lands exactly on 10.0.0.0/20
    gke_pods = {
      prefix_size     = 20
      reserve_sibling = false
    }
    # Evaluated 2nd: Lands on 10.0.16.0/22
    # Sibling Reservation: Locks down 10.0.20.0/22
    gke_nodes = {
      prefix_size     = 22
      reserve_sibling = true
    }
    # Evaluated 3rd: Leaps past nodes + sibling to land on 10.0.24.0/22
    gke_services = {
      prefix_size     = 22
      reserve_sibling = false
    }
    # Evaluated 4th: Fills the next sequential alignment at 10.0.28.0/24
    vm_subnets = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

# =========================================================================
# STRATEGY 2: "BEST" (Fragmentation Minimization / Tightest Fit)
# =========================================================================
resource "cidrblock_pool" "strategy_best" {
  cidr                = "10.20.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-best"
  allocation_strategy = "BEST"

  allocations = {
    # Evaluated 1st: Consumes the front of the pool from 10.20.0.0/18
    large_core_network = {
      prefix_size     = 18
      reserve_sibling = false
    }
    # Evaluated 2nd: Sweeps the open gaps (10.20.64.0/18 vs 10.20.128.0/17).
    # It selects the smaller /18 gap to preserve the massive /17 block.
    # EXACT PLACEMENT: 10.20.64.0/24
    isolated_service_mesh = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

# =========================================================================
# STRATEGY 3: "SPARSE" (Maximum Isolation / Blast Radius Reduction)
# =========================================================================
resource "cidrblock_pool" "strategy_sparse" {
  cidr                = "10.30.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-sparse"
  allocation_strategy = "SPARSE"

  allocations = {
    # Evaluated 1st: Consumes 10.30.0.0/22
    isolated_prod_nodes = {
      prefix_size     = 22
      reserve_sibling = false
    }
    # Evaluated 2nd: Sweeps open gaps and picks the largest continuous chunk (10.30.128.0/17).
    # This maximizes the physical network isolation distance from the prod nodes.
    # EXACT PLACEMENT: 10.30.128.0/24
    isolated_dmz_web = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

# Core GCP VPC Network Linked to the Sequential FIRST Pool
resource "google_compute_network" "vpc" {
  name                    = "gcp-vpc-main"
  auto_create_subnetworks = false
}

# Subnetwork provisioned using the explicitly calculated FIRST coordinates
resource "google_compute_subnetwork" "gke_nodes" {
  name          = "gke-nodes"
  network       = google_compute_network.vpc.id
  ip_cidr_range = cidrblock_pool.strategy_first.allocations["gke_nodes"].allocated_cidr
  region        = var.region
}

