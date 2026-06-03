# GCP VPC Subnet Example
# Uses the cidrblock provider to allocate subnets for GCP VPC networks

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

provider "google" {
  project = var.project_id
  region  = var.region
}

# CIDR pool for VPC subnets managing complex alignment constraints deterministically
resource "cidrblock_pool" "vpc_subnets" {
  cidr                = "10.0.0.0/16"
  organization        = var.organization
  project             = var.project_id
  network             = "vpc-main"
  allocation_strategy = "FIRST" # Keys evaluated alphabetically: gke_nodes, gke_pods, gke_services, vm_subnets

  allocations = {
    # 1. Processed First: Sized at /22, lands on 10.0.0.0/22.
    # Valid: Perfectly left-hand aligned relative to its parent bit structure, reserving 10.0.4.0/22.
    gke_nodes = {
      prefix_size     = 22
      reserve_sibling = true
    }

    # 2. Processed Second: /20 sizing requires a 16-base boundary alignment block multiplier.
    # Skips active node footprint + sibling space to securely claim 10.0.16.0/20.
    gke_pods = {
      prefix_size     = 20
      reserve_sibling = false
    }

    # 3. Processed Third: Scans from bottom. Finds the unallocated aligned gap starting at 10.0.8.0/22.
    gke_services = {
      prefix_size     = 22
      reserve_sibling = false
    }

    # 4. Processed Fourth: Scans from bottom. Placed into the first available slot at 10.0.12.0/24.
    vm_subnets = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

# GCP VPC Network
resource "google_compute_network" "vpc" {
  name                    = "vpc-main"
  auto_create_subnetworks = false
}

# GKE Node Subnet
resource "google_compute_subnetwork" "gke_nodes" {
  name          = "gke-nodes"
  network       = google_compute_network.vpc.id
  ip_cidr_range = cidrblock_pool.vpc_subnets.allocations["gke_nodes"].allocated_cidr
  region        = var.region
}

# GKE Pod Subnet
resource "google_compute_subnetwork" "gke_pods" {
  name          = "gke-pods"
  network       = google_compute_network.vpc.id
  ip_cidr_range = cidrblock_pool.vpc_subnets.allocations["gke_pods"].allocated_cidr
  region        = var.region
}

# GKE Service Subnet
resource "google_compute_subnetwork" "gke_services" {
  name          = "gke-services"
  network       = google_compute_network.vpc.id
  ip_cidr_range = cidrblock_pool.vpc_subnets.allocations["gke_services"].allocated_cidr
  region        = var.region
}

# VM Subnet
resource "google_compute_subnetwork" "vm_subnets" {
  name          = "vm-subnets"
  network       = google_compute_network.vpc.id
  ip_cidr_range = cidrblock_pool.vpc_subnets.allocations["vm_subnets"].allocated_cidr
  region        = var.region
}

output "gke_nodes_cidr" {
  value = cidrblock_pool.vpc_subnets.allocations["gke_nodes"].allocated_cidr
}

output "gke_nodes_sibling_cidr" {
  value       = cidrblock_pool.vpc_subnets.allocations["gke_nodes"].sibling_cidr
  description = "Reserved sibling for future horizontal GKE node expansion in-place"
}

