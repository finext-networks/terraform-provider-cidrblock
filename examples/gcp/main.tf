# GCP VPC Subnet Architecture Example
# Demonstrates FIRST, BEST, and SPARSE layout mechanics using identical subnet inputs

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
# POOL 1: "FIRST" Strategy (Tight Bottom-Up Packing)
# =========================================================================
resource "cidrblock_pool" "pool_first" {
  cidr                = "10.0.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-first"
  allocation_strategy = "FIRST"

  allocations = {
    gke_pods     = { prefix_size = 20, reserve_sibling = false } # Lands on 10.0.0.0/20
    gke_nodes    = { prefix_size = 22, reserve_sibling = true  } # Lands on 10.0.16.0/22 (Sib: .20.0/22)
    gke_services = { prefix_size = 22, reserve_sibling = false } # Lands on 10.0.24.0/22
    vm_subnets   = { prefix_size = 24, reserve_sibling = false } # Lands on 10.0.28.0/24
  }
}

# =========================================================================
# POOL 2: "BEST" Strategy (Tightest Fitting Free Slices)
# =========================================================================
resource "cidrblock_pool" "pool_best" {
  cidr                = "10.1.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-best"
  allocation_strategy = "BEST"

  allocations = {
    gke_pods     = { prefix_size = 20, reserve_sibling = false } # Lands on 10.1.0.0/20
    gke_nodes    = { prefix_size = 22, reserve_sibling = true  } # Lands on 10.1.16.0/22 (Sib: .20.0/22)
    gke_services = { prefix_size = 22, reserve_sibling = false } # Lands on 10.1.24.0/22 (Fills the /22 fragment)
    vm_subnets   = { prefix_size = 24, reserve_sibling = false } # Lands on 10.1.32.0/24
  }
}

# =========================================================================
# POOL 3: "SPARSE" Strategy (Maximum Isolation Distance Scattering)
# =========================================================================
resource "cidrblock_pool" "pool_sparse" {
  cidr                = "10.2.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-sparse"
  allocation_strategy = "SPARSE"

  allocations = {
    gke_pods     = { prefix_size = 20, reserve_sibling = false } # Lands on 10.2.0.0/20
    gke_nodes    = { prefix_size = 22, reserve_sibling = true  } # Lands on 10.2.128.0/22 (Leaps to largest /17 slice)
    gke_services = { prefix_size = 22, reserve_sibling = false } # Lands on 10.2.64.0/22  (Leaps to next largest /18 slice)
    vm_subnets   = { prefix_size = 24, reserve_sibling = false } # Lands on 10.2.192.0/24 (Leaps to upper /18 slice)
  }
}

# =========================================================================
# Core Infrastructure Linkage
# =========================================================================
resource "google_compute_network" "vpc" {
  name                    = "gcp-vpc-main"
  auto_create_subnetworks = false
}

# Example subnet binding to the isolated SPARSE nodes allocation
resource "google_compute_subnetwork" "gke_nodes_sparse" {
  name          = "gke-nodes-isolated"
  network       = google_compute_network.vpc.id
  ip_cidr_range = cidrblock_pool.pool_sparse.allocations["gke_nodes"].allocated_cidr
  region        = var.region
}

# =========================================================================
# Comparative Matrix Outputs
# =========================================================================

output "matrix_first_sequential" {
  value = {
    pods     = cidrblock_pool.pool_first.allocations["gke_pods"].allocated_cidr
    nodes    = cidrblock_pool.pool_first.allocations["gke_nodes"].allocated_cidr
    sibling  = cidrblock_pool.pool_first.allocations["gke_nodes"].sibling_cidr
    services = cidrblock_pool.pool_first.allocations["gke_services"].allocated_cidr
    vms      = cidrblock_pool.pool_first.allocations["vm_subnets"].allocated_cidr
  }
}

output "matrix_best_compact" {
  value = {
    pods     = cidrblock_pool.pool_best.allocations["gke_pods"].allocated_cidr
    nodes    = cidrblock_pool.pool_best.allocations["gke_nodes"].allocated_cidr
    sibling  = cidrblock_pool.pool_best.allocations["gke_nodes"].sibling_cidr
    services = cidrblock_pool.pool_best.allocations["gke_services"].allocated_cidr
    vms      = cidrblock_pool.pool_best.allocations["vm_subnets"].allocated_cidr
  }
}

output "matrix_sparse_isolated" {
  value = {
    pods     = cidrblock_pool.pool_sparse.allocations["gke_pods"].allocated_cidr
    nodes    = cidrblock_pool.pool_sparse.allocations["gke_nodes"].allocated_cidr
    sibling  = cidrblock_pool.pool_sparse.allocations["gke_nodes"].sibling_cidr
    services = cidrblock_pool.pool_sparse.allocations["gke_services"].allocated_cidr
    vms      = cidrblock_pool.pool_sparse.allocations["vm_subnets"].allocated_cidr
  }
}

