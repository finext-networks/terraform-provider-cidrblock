# GCP VPC Subnet Architecture - Comprehensive Algorithmic Strategy Comparison
# Simulates a Day 2 fragmented pool state to contrast FIRST, BEST, and SPARSE behaviors.

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
  # Disabled (false) to allow GCP workflows to dynamically alter allocation tracking maps
  prevent_subnet_destruction = false
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# Core GCP VPC Network where the calculated subnets will be provisioned
resource "google_compute_network" "vpc" {
  name                    = "gcp-vpc-production"
  auto_create_subnetworks = false
}

# =========================================================================
# POOL 1: "FIRST" Strategy (Sequential Packing / Lowest Fit)
# =========================================================================
# Algorithm: Scans linearly from the lowest to highest address ranges. It claims 
# the very first aligned gap that can accommodate the request, ignoring the size 
# of the surrounding empty space.
# =========================================================================
resource "cidrblock_pool" "pool_first" {
  cidr                = "10.50.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-first-topology"
  allocation_strategy = "FIRST"

  allocations = {
    # --- ACTIVE RETAINED WORKLOADS ---
    # FFD Pass 1 (/18): Anchored at 10.50.64.0/18  (Consumes: 10.50.64.0 - 10.50.127.255)
    gke_pods = { prefix_size = 18, reserve_sibling = false }
    
    # FFD Pass 2 (/19): Anchored at 10.50.160.0/19 (Consumes: 10.50.160.0 - 10.50.191.255)
    gke_nodes = { prefix_size = 19, reserve_sibling = false }
    
    # FFD Pass 3 (/19): Anchored at 10.50.192.0/19 (Consumes: 10.50.192.0 - 10.50.223.255)
    vm_subnets = { prefix_size = 19, reserve_sibling = false }

    # --- THE NEW DAY 2 REQUEST ---
    # History: 'legacy_frontend' (/18) was deleted from the bottom of the pool (10.50.0.0/18).
    # History: 'legacy_dmz' (/22) was deleted from the middle of the pool (10.50.128.0/22).
    # Evaluation: FIRST starts scanning from 10.50.0.0. It immediately encounters the 
    # large empty /18 gap. Because a /24 fits, it claims the front edge of it instantly.
    # 
    # EXACT PLACEMENT: 10.50.0.0/24 (Fragments the pristine /18 block)
    bi_analytics = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

# =========================================================================
# POOL 2: "BEST" Strategy (Fragmentation Minimization / Tightest Fit)
# =========================================================================
# Algorithm: Evaluates all available open gaps across the entire pool. It selects 
# the smallest continuous slice that can fit the request, intentionally preserving 
# large open blocks for heavy-duty workloads.
# =========================================================================
resource "cidrblock_pool" "pool_best" {
  cidr                = "10.50.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-best-topology"
  allocation_strategy = "BEST"

  allocations = {
    # --- ACTIVE RETAINED WORKLOADS ---
    gke_pods   = { prefix_size = 18, reserve_sibling = false } # Anchored at 10.50.64.0/18
    gke_nodes  = { prefix_size = 19, reserve_sibling = false } # Anchored at 10.50.160.0/19
    vm_subnets = { prefix_size = 19, reserve_sibling = false } # Anchored at 10.50.192.0/19

    # --- THE NEW DAY 2 REQUEST ---
    # History: Same deletions occur, leaving a /18 gap at the bottom and a /22 gap at .128.0.
    # Evaluation: BEST evaluates both open gaps: Gap 1 (/18 -> 16,384 IPs) vs Gap 2 (/22 -> 1,024 IPs).
    # It recognizes that the smaller /22 gap is a much tighter fit for a /24 request (256 IPs).
    # It intentionally bypasses the lower addresses to stuff the subnet into the small fragment.
    #
    # EXACT PLACEMENT: 10.50.128.0/24 (Shields the massive 10.50.0.0/18 space from getting carved up)
    bi_analytics = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

# =========================================================================
# POOL 3: "SPARSE" Strategy (Maximum Isolation / Blast Radius Reduction)
# =========================================================================
# Algorithm: Sweeps all open layout fragments and intentionally claims the 
# absolute largest contiguous block space chunk available, maximizing physical 
# network distance between your subnets.
# =========================================================================
resource "cidrblock_pool" "pool_sparse" {
  cidr                = "10.50.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-sparse-topology"
  allocation_strategy = "SPARSE"

  allocations = {
    # --- ACTIVE RETAINED WORKLOADS ---
    gke_pods   = { prefix_size = 18, reserve_sibling = false } # Anchored at 10.50.64.0/18
    gke_nodes  = { prefix_size = 19, reserve_sibling = false } # Anchored at 10.50.160.0/19
    vm_subnets = { prefix_size = 19, reserve_sibling = false } # Anchored at 10.50.192.0/19

    # --- THE NEW DAY 2 REQUEST ---
    # History: Same deletions occur, leaving a /18 gap at the bottom, a /22 gap at .128.0,
    # and a trailing /19 gap at the absolute top of the pool (10.50.224.0/19 -> 8,192 IPs).
    # Evaluation: SPARSE compares all open gaps: the /18 gap (16,384 IPs), the /22 gap (1,024 IPs),
    # and the trailing /19 gap (8,192 IPs). It picks the absolute largest contiguous chunk.
    # The largest open block is the /18 gap at the bottom.
    #
    # EXACT PLACEMENT: 10.50.0.0/24 (If tied with an equal top-end chunk, it defaults to the lowest address)
    bi_analytics = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

# =========================================================================
# REAL GOOGLE CLOUD SUBNETWORK PROVISIONING
# =========================================================================

# Subnet 1: Provisioned using the FIRST strategy output (Lands on 10.50.0.0/24)
resource "google_compute_subnetwork" "analytics_subnet_first" {
  name          = "analytics-first-sequential"
  network       = google_compute_network.vpc.id
  ip_cidr_range = cidrblock_pool.pool_first.allocations["bi_analytics"].allocated_cidr
  region        = var.region
}

# Subnet 2: Provisioned using the BEST strategy output (Lands on 10.50.128.0/24)
resource "google_compute_subnetwork" "analytics_subnet_best" {
  name          = "analytics-best-compact"
  network       = google_compute_network.vpc.id
  ip_cidr_range = cidrblock_pool.pool_best.allocations["bi_analytics"].allocated_cidr
  region        = var.region
}

# Subnet 3: Provisioned using the SPARSE strategy output (Lands on 10.50.0.0/24)
resource "google_compute_subnetwork" "analytics_subnet_sparse" {
  name          = "analytics-sparse-isolated"
  network       = google_compute_network.vpc.id
  ip_cidr_range = cidrblock_pool.pool_sparse.allocations["bi_analytics"].allocated_cidr
  region        = var.region
}

# =========================================================================
# COMPARATIVE MATRIX OUTPUTS
# =========================================================================

output "allocation_strategy_matrix" {
  value = {
    first_fit_range  = google_compute_subnetwork.analytics_subnet_first.ip_cidr_range
    best_fit_range   = google_compute_subnetwork.analytics_subnet_best.ip_cidr_range
    sparse_fit_range = google_compute_subnetwork.analytics_subnet_sparse.ip_cidr_range
  }
  description = "Side-by-side empirical proof of different structural layouts on identical fragmented pools"
}

