# CIDR pool for VPC subnets managing complex alignment constraints deterministically
resource "cidrblock_pool" "vpc_subnets" {
  cidr                = "10.0.0.0/16"
  organization        = var.organization 
  project             = var.project_id 
  network             = "vpc-main" 
  allocation_strategy = "FIRST" 

  allocations = {
    # PASS 1: Evaluated First (Largest block width). 
    # Captures the absolute baseline of the pool.
    # Consumes: 10.0.0.0 to 10.0.15.255
    gke_pods = {
      prefix_size     = 20
      reserve_sibling = false
    }

    # PASS 2: Evaluated Second (Sized /22, ties broken alphabetically).
    # Next available naturally aligned /22 boundary after the pod block is 10.0.16.0/22.
    # Consumes: 10.0.16.0 to 10.0.19.255
    # Sibling Reservation: Locks down the next buddy block at 10.0.20.0/22.
    gke_nodes = {
      prefix_size     = 22 
      reserve_sibling = true 
    }

    # PASS 3: Evaluated Third (Sized /22, ties broken alphabetically).
    # Scans past the active nodes block and its locked sibling range (.16.0 to .23.255).
    # Claims the next available naturally aligned multiple-of-4 gap at 10.0.24.0/22.
    # Consumes: 10.0.24.0 to 10.0.27.255
    gke_services = {
      prefix_size     = 22 
      reserve_sibling = false 
    }

    # PASS 4: Evaluated Fourth (Smallest block width).
    # Drops into the first available free slot sitting cleanly after the services block.
    # Consumes: 10.0.28.0 to 10.0.28.255
    vm_subnets = {
      prefix_size     = 24 
      reserve_sibling = false 
    }
  }
}

# ==========================================
# Pool Allocation Outputs
# ==========================================

output "gke_pods_cidr" {
  value       = cidrblock_pool.vpc_subnets.allocations["gke_pods"].allocated_cidr
  description = "Calculated CIDR block for GKE Pods (Evaluated 1st by FFD)"
}

output "gke_nodes_cidr" {
  value       = cidrblock_pool.vpc_subnets.allocations["gke_nodes"].allocated_cidr
  description = "Calculated CIDR block for GKE Nodes (Evaluated 2nd by FFD)"
}

output "gke_nodes_sibling_cidr" {
  value       = cidrblock_pool.vpc_subnets.allocations["gke_nodes"].sibling_cidr
  description = "Reserved companion buddy block for future GKE Node expansion"
}

output "gke_services_cidr" {
  value       = cidrblock_pool.vpc_subnets.allocations["gke_services"].allocated_cidr
  description = "Calculated CIDR block for GKE Services (Evaluated 3rd by FFD)"
}

output "vm_subnets_cidr" {
  value       = cidrblock_pool.vpc_subnets.allocations["vm_subnets"].allocated_cidr
  description = "Calculated CIDR block for standard VMs (Evaluated 4th by FFD)"
}
