# ==============================================================================
# Algorithmic Strategy Comparison Simulation
# Demonstrates how FIRST, BEST, and SPARSE handle fragmented day-2 address spaces.
# ==============================================================================

terraform {
  required_version = ">= 1.5.0"
  required_providers {
    cidrblock = {
      source = "finext-networks/cidrblock"
    }
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
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

# ==============================================================================
# Variables Block
# ==============================================================================

variable "project_id" {
  type    = string
  default = "finext-lab-environment"
}

variable "region" {
  type    = string
  default = "us-west1"
}

variable "organization" {
  type    = string
  default = "finext"
}

# Core VPC Network where our strategy experiment will materialize
resource "google_compute_network" "strategy_vpc" {
  name                    = "vpc-strategy-sandbox"
  auto_create_subnetworks = false
}

# ==============================================================================
# POOL 1: "FIRST" Strategy (Sequential Packing / Greedy Front-Edge Fitting)
# ==============================================================================
# Mechanics: Scans linearly from the bottom address up. It instantly claims the 
# very first empty block that can fit the mask, ignoring fragmentation noise.
# ==============================================================================

resource "cidrblock_pool" "pool_first" {
  cidr                = "10.80.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-first-demo"
  allocation_strategy = "FIRST"

  allocations = {
    # --- ACTIVE RETAINED RUNTIME WORKLOADS ---
    gke_pods   = { prefix_size = 18, reserve_sibling = false } # Anchored at 10.80.64.0/18
    gke_nodes  = { prefix_size = 19, reserve_sibling = false } # Anchored at 10.80.160.0/19
    vm_subnets = { prefix_size = 19, reserve_sibling = false } # Anchored at 10.80.192.0/19

    # --- THE NEW DAY-2 FRAGMENTATION REQUEST ---
    # SIMULATION HISTORY: 
    # 1. A legacy /18 block was deleted from the bottom (10.80.0.0/18).
    # 2. A legacy /22 block was deleted from the center (10.80.128.0/22).
    #
    # RUNTIME EVALUATION: 
    # FIRST starts scanning at .0.0. It encounters the large empty /18 gap. 
    # Because our new /24 request fits, it claims the front edge immediately.
    #
    # RESULT: Lands on 10.80.0.0/24 (Carves up and fragments the pristine /18 block).
    new_analytics_tier = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

data "cidrblock_pool" "telemetry_first" {
  id = cidrblock_pool.pool_first.id
}

# ==============================================================================
# POOL 2: "BEST" Strategy (Fragmentation Shielding / Tightest Fit Match)
# ==============================================================================
# Mechanics: Sweeps all available open gaps across the matrix. It purposefully 
# chooses the smallest continuous chunk that can accommodate the range, saving 
# your large open expansion regions from getting broken up.
# ==============================================================================

resource "cidrblock_pool" "pool_best" {
  cidr                = "10.80.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-best-demo"
  allocation_strategy = "BEST"

  allocations = {
    # --- ACTIVE RETAINED RUNTIME WORKLOADS ---
    gke_pods   = { prefix_size = 18, reserve_sibling = false } # Anchored at 10.80.64.0/18
    gke_nodes  = { prefix_size = 19, reserve_sibling = false } # Anchored at 10.80.160.0/19
    vm_subnets = { prefix_size = 19, reserve_sibling = false } # Anchored at 10.80.192.0/19

    # --- THE NEW DAY-2 FRAGMENTATION REQUEST ---
    # RUNTIME EVALUATION: 
    # BEST evaluates both holes: Gap A (/18 -> 16,384 IPs) vs Gap B (/22 -> 1,024 IPs).
    # It identifies that the /22 gap is a significantly tighter fit for our /24 (256 IPs).
    # It intentionally bypasses the lower addresses to stuff the subnet into the pocket.
    #
    # RESULT: Lands on 10.80.128.0/24 (Preserves the massive 10.80.0.0/18 area entirely).
    new_analytics_tier = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

data "cidrblock_pool" "telemetry_best" {
  id = cidrblock_pool.pool_best.id
}

# ==============================================================================
# POOL 3: "SPARSE" Strategy (Maximum Isolation Distance / Blast Radius Control)
# ==============================================================================
# Mechanics: Sweeps the layout map and claims the absolute largest continuous 
# chunk of free address space available, maximizing distance between workloads.
# ==============================================================================

resource "cidrblock_pool" "pool_sparse" {
  cidr                = "10.80.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-sparse-demo"
  allocation_strategy = "SPARSE"

  allocations = {
    # --- ACTIVE RETAINED RUNTIME WORKLOADS ---
    gke_pods   = { prefix_size = 18, reserve_sibling = false } # Anchored at 10.80.64.0/18
    gke_nodes  = { prefix_size = 19, reserve_sibling = false } # Anchored at 10.80.160.0/19
    vm_subnets = { prefix_size = 19, reserve_sibling = false } # Anchored at 10.80.192.0/19

    # --- THE NEW DAY-2 FRAGMENTATION REQUEST ---
    # RUNTIME EVALUATION: 
    # SPARSE compares the open fragments: the /18 gap (16,384 IPs), the /22 gap (1,024 IPs),
    # and a trailing unallocated /19 gap at the top of the pool (10.80.224.0/19 -> 8,192 IPs).
    # It picks the absolute largest block. The largest is the /18 gap at the bottom.
    #
    # RESULT: Lands on 10.80.0.0/24 (Isolates the network step inside the largest zone).
    new_analytics_tier = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

data "cidrblock_pool" "telemetry_sparse" {
  id = cidrblock_pool.pool_sparse.id
}

# ==============================================================================
# Materialize Real GCP Subnetworks to Empiricalize Results
# ==============================================================================

resource "google_compute_subnetwork" "fit_first" {
  name          = "sb-analytics-strategy-first"
  network       = google_compute_network.strategy_vpc.id
  region        = var.region
  ip_cidr_range = cidrblock_pool.pool_first.allocations["new_analytics_tier"].allocated_cidr
}

resource "google_compute_subnetwork" "fit_best" {
  name          = "sb-analytics-strategy-best"
  network       = google_compute_network.strategy_vpc.id
  region        = var.region
  ip_cidr_range = cidrblock_pool.pool_best.allocations["new_analytics_tier"].allocated_cidr
}

resource "google_compute_subnetwork" "fit_sparse" {
  name          = "sb-analytics-strategy-sparse"
  network       = google_compute_network.strategy_vpc.id
  region        = var.region
  ip_cidr_range = cidrblock_pool.pool_sparse.allocations["new_analytics_tier"].allocated_cidr
}

# ==============================================================================
# Side-By-Side Comparison Verification Matrix
# ==============================================================================

output "empirical_strategy_matrix" {
  description = "Side-by-side verification of packing behaviors on an identical fragmented map."
  value = {
    first_fit_range        = google_compute_subnetwork.fit_first.ip_cidr_range
    best_fit_range         = google_compute_subnetwork.fit_best.ip_cidr_range
    sparse_fit_range       = google_compute_subnetwork.fit_sparse.ip_cidr_range
    first_residual_slices  = data.cidrblock_pool.telemetry_first.available_slices
    best_residual_slices   = data.cidrblock_pool.telemetry_best.available_slices
    sparse_residual_slices = data.cidrblock_pool.telemetry_sparse.available_slices
  }
}

