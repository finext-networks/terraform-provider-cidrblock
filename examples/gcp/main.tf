# GCP VPC Subnet Example
# Uses the cidrblock provider to allocate subnets for GCP VPC networks

terraform {
  required_providers {
    cidrblock = {
      source  = "finext-networks/cidrblock"
    }
    google = {
      source  = "hashicorp/google"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# CIDR pool for VPC subnets
resource "cidrblock_pool" "vpc_subnets" {
  cidr         = "10.0.0.0/16"
  organization = var.organization
  project      = var.project_id
  network      = "vpc-main"

  allocations = {
    gke_nodes = {
      prefix_size     = 22
      reserve_sibling = true
    }

    gke_pods = {
      prefix_size     = 20
      reserve_sibling = false
    }

    gke_services = {
      prefix_size     = 22
      reserve_sibling = false
    }

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
  value = cidrblock_pool.vpc_subnets.allocations["gke_nodes"].sibling_cidr
  description = "Reserved sibling for future GKE node expansion"
}
