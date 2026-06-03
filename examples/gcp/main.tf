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

# Provider configuration with destruction safety disabled for elastic environments
provider "cidrblock" {
  # Safety Guardrail: Disabled (false) to allow GCP workflows to dynamically drop,
  # add, or alter allocation tracking maps during pipeline executions.
  prevent_subnet_destruction = false
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# =========================================================================
# STRATEGY 1: "FIRST" (Sequential Packing)
# =========================================================================
# Mechanics: FFD sorts inputs by size descending. The search pointer walks 
# linearly from the lowest to highest address ranges, claiming the very first 
# naturally aligned free gap that can accommodate the requested mask.
# Use Case: Maximizes sequential packing efficiency from the bottom up.
# =========================================================================
resource "cidrblock_pool" "strategy_first" {
  cidr                = "10.0.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-first"
  allocation_strategy = "FIRST"

  allocations = {
    # Evaluated 1st by FFD (Sized /20). Lands on 10.0.0.0/20.
    gke_pods = {
      prefix_size     = 20
      reserve_sibling = false
    }
    # Evaluated 2nd by FFD (Sized /22). Lands on 10.0.16.0/22.
    # Spawns a sibling reservation block right next to it at 10.0.20.0/22.
    gke_nodes = {
      prefix_size     = 22
      reserve_sibling = true
    }
    # Evaluated 3rd by FFD (Sized /22). Leaps past nodes + sibling to land on 10.0.24.0/22.
    gke_services = {
      prefix_size     = 22
      reserve_sibling = false
    }
    # Evaluated 4th by FFD (Sized /24). Drops into the first open gap at 10.0.28.0/24.
    vm_subnets = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

# =========================================================================
# STRATEGY 2: "BEST" (Fragmentation Minimization / Tightest Fit)
# =========================================================================
# Mechanics: Sweeps all open layout fragments and evaluates their remainder spans.
# It selects the smallest available continuous unallocated slice space that can 
# still tightly fit the request, isolating isolated fragments.
# Use Case: Preserves large contiguous spaces by stuffing new networks into 
# small, pre-existing boundary gaps.
# =========================================================================
resource "cidrblock_pool" "strategy_best" {
  cidr                = "10.20.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-best"
  allocation_strategy = "BEST"

  allocations = {
    # FFD sorts by size descending. When evaluating gaps, "BEST" forces
    # allocations into tight isolated fragments elsewhere in the tree rather 
    # than consuming the widest continuous chunks.
    large_core_network = {
      prefix_size     = 18
      reserve_sibling = false
    }
    isolated_service_mesh = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

# =========================================================================
# STRATEGY 3: "SPARSE" (Maximum Isolation / Blast Radius Reduction)
# =========================================================================
# Mechanics: Scans the layout tree and intentionally claims the largest open 
# contiguous block space chunk available.
# Use Case: Intentionally separates infrastructure blocks (like isolating 
# production clusters from testing setups) to maximize buffering distances.
# =========================================================================
resource "cidrblock_pool" "strategy_sparse" {
  cidr                = "10.30.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-sparse"
  allocation_strategy = "SPARSE"

  allocations = {
    # Allocations are dropped into opposing ends or centers of the widest 
    # open chunks, preventing subnets from sitting back-to-back.
    isolated_prod_nodes = {
      prefix_size     = 22
      reserve_sibling = false
    }
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

# GKE Node Subnetwork provisioning via FIRST Pool Calculation
resource "google_compute_subnetwork" "gke_nodes" {
  name          = "gke-nodes"
  network       = google_compute_network.vpc.id
  ip_cidr_range = cidrblock_pool.strategy_first.allocations["gke_nodes"].allocated_cidr
  region        = var.region
}

# GKE Pod Subnetwork provisioning via FIRST Pool Calculation
resource "google_compute_subnetwork" "gke_pods" {
  name          = "gke-pods"
  network       = google_compute_network.vpc.id
  ip_cidr_range = cidrblock_pool.strategy_first.allocations["gke_pods"].allocated_cidr
  region        = var.region
}

# =========================================================================
# Diagnostic Execution Strategy Outputs
# =========================================================================

output "first_fit_mapping" {
  value       = cidrblock_pool.strategy_first.allocations
  description = "Complete map showing tight sequential bottom-up allocation alignment"
}

output "best_fit_mapping" {
  value       = cidrblock_pool.strategy_best.allocations
  description = "Map showcasing allocation packing into the tightest available gaps"
}

output "sparse_fit_mapping" {
  value       = cidrblock_pool.strategy_sparse.allocations
  description = "Map showcasing high isolation distance between running allocations"
}

