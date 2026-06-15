# ==============================================================================
# Core Provider & Variables Initialization
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
  # Safety Guardrail: Protects active production network interfaces 
  prevent_subnet_destruction = true
}

provider "google" {
  project = var.project_id
  region  = var.region_primary
}

variable "project_id" {
  type    = string
  default = "finext-prod-mesh"
}

variable "region_primary" {
  type    = string
  default = "us-central1"
}

variable "region_backup" {
  type    = string
  default = "us-east1"
}

# ==============================================================================
# Step 1: The Global IPAM Allocation Matrix
# ==============================================================================

resource "cidrblock_pool" "gcp_matrix" {
  cidr                = "10.120.0.0/16"
  organization        = "finext"
  project             = "gke-production-mesh"
  network             = "vpc-primary"
  allocation_strategy = "BEST" # Packs ranges cleanly to optimize binary space

  # ADVANCED GCP PACKING: Employs First-Fit Decreasing (FFD) size-sorting.
  # This allows us to declare specialized GKE Pod and Service structures 
  # alongside standard compute baselines without fragmentation overlap.
  allocations = {
    # PASS 1: Sorted First (Largest block: /20 = 4,096 IPs).
    # Lands on base 10.120.0.0/20. Dedicated to the dense GKE Pod network.
    gke_pods_tier = {
      prefix_size     = 20
      reserve_sibling = false
    }

    # PASS 2: Sorted Second (Medium block: /22 = 1,024 IPs).
    # Claims 10.120.16.0/22. Dedicated to GKE Cluster Services.
    gke_services_tier = {
      prefix_size     = 22
      reserve_sibling = false
    }

    # PASS 3: Sorted Third (Standard block: /24 = 256 IPs). Tied with internal_compute.
    # Wins alphabetical tie-breaker. Claims 10.120.20.0/24. Mapped to GKE Cluster Nodes.
    gke_nodes_tier = {
      prefix_size     = 24
      reserve_sibling = false
    }

    # PASS 4: Sorted Fourth (Standard block: /24 = 256 IPs).
    # Claims 10.120.21.0/24. Dedicated to core regional internal VMs.
    # SIBLING EXPANSION: Setting reserve_sibling = true dynamically allocates an
    # identical companion block at 10.120.22.0/24. We map the base to our primary region
    # and the sibling to our backup region for a flawless Multi-Region DR setup.
    internal_compute = {
      prefix_size     = 24
      reserve_sibling = true
    }
  }
}

# ==============================================================================
# Step 2: Real-Time Telemetry via Hydrated Data Source
# ==============================================================================

# Accesses the shared memory registry via the 4-part composite identity token.
# Yields instant capacity telemetry directly to the plan output window.
data "cidrblock_pool" "telemetry" {
  id = cidrblock_pool.gcp_matrix.id
}

# ==============================================================================
# Step 3: Materialize Network Topologies inside Google Cloud
# ==============================================================================

resource "google_compute_network" "hub_vpc" {
  name                    = "vpc-production-mesh"
  auto_create_subnetworks = false
}

# The Primary Enterprise Subnetwork
# Combines GKE Nodes, Pods, and Services into a unified, non-overlapping regional asset
resource "google_compute_subnetwork" "primary_subnet" {
  name          = "sb-us-central1-production"
  network       = google_compute_network.hub_vpc.id
  region        = var.region_primary
  
  # Primary range mapped to GKE worker nodes
  ip_cidr_range = cidrblock_pool.gcp_matrix.allocations["gke_nodes_tier"].allocated_cidr

  # Secondary ranges mapped to GKE internal execution abstractions
  secondary_ip_range {
    range_name    = "gke-pods-secondary"
    ip_cidr_range = cidrblock_pool.gcp_matrix.allocations["gke_pods_tier"].allocated_cidr
  }

  secondary_ip_range {
    range_name    = "gke-services-secondary"
    ip_cidr_range = cidrblock_pool.gcp_matrix.allocations["gke_services_tier"].allocated_cidr
  }

  secondary_ip_range {
    range_name    = "internal-compute-primary"
    ip_cidr_range = cidrblock_pool.gcp_matrix.allocations["internal_compute"].allocated_cidr
  }
}

# The Disaster Recovery Subnetwork (Cross-Region Standby)
# Safely consumes the guaranteed unique sibling block calculated by the provider
resource "google_compute_subnetwork" "dr_backup_subnet" {
  name          = "sb-us-east1-disaster-recovery"
  network       = google_compute_network.hub_vpc.id
  region        = var.region_backup
  
  # Mapped cleanly to the calculated shadow companion block
  ip_cidr_range = cidrblock_pool.gcp_matrix.allocations["internal_compute"].sibling_cidr
}

# ==============================================================================
# Step 4: Verification & Capacity Analytics Outputs
# ==============================================================================

output "gcp_vpc_metadata" {
  description = "The baseline infrastructure network configurations deployed to Google Cloud."
  value = {
    vpc_id            = google_compute_network.hub_vpc.id
    primary_subnet    = google_compute_subnetwork.primary_subnet.id
    dr_backup_subnet  = google_compute_subnetwork.dr_backup_subnet.id
    ipam_routing_id   = data.cidrblock_pool.telemetry.id
  }
}

output "ipam_pool_metrics" {
  description = "Live utilization stats compiled from the global IPAM engine."
  value = {
    total_ips     = data.cidrblock_pool.telemetry.metrics.total_ips
    allocated_ips = data.cidrblock_pool.telemetry.metrics.allocated_ips
    reserved_ips  = data.cidrblock_pool.telemetry.metrics.reserved_ips
    available_ips = data.cidrblock_pool.telemetry.metrics.available_ips
  }
}

output "unallocated_expansion_slices" {
  description = "Contiguous unallocated chunks discovered inside the matrix. Use these for future node-pool scaling."
  value       = data.cidrblock_pool.telemetry.available_slices
}

