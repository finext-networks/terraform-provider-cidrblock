terraform {
  required_providers {
    cidrblock = {
      source = "finext-networks/cidrblock"
    }
  }
}

# Basic IPv4 pool showing predictable sibling allocation layout tree map boundaries
resource "cidrblock_pool" "ipv4_pool" {
  cidr                = "10.0.0.0/16"
  organization        = "my-org"
  project             = "my-project"
  network             = "vpc-main"
  allocation_strategy = "FIRST"

  # The provider automatically processes allocations using First-Fit Decreasing (FFD) order. 
  # Subnets are sorted by largest block footprint first (smallest prefix bit length), 
  # falling back to alphabetical sorting only as a deterministic tie-breaker.
  allocations = {
    # 1. Processed First: Has the largest network footprint (/22 = 1024 IPs).
    # Evaluated first due to size, landing squarely on the base boundary: 10.0.0.0/22.
    database = {
      prefix_size     = 22
      reserve_sibling = false
    }

    # 2. Processed Second: Tied on footprint size with frontend (/24 = 256 IPs).
    # Wins the alphabetical tie-breaker. Skips database allocation to land on 10.0.4.0/24.
    # Reserves adjacent shadow partner block 10.0.5.0/24 cleanly as its buddy partner.
    backend = {
      prefix_size     = 24
      reserve_sibling = true
    }

    # 3. Processed Third: Tied on footprint size with backend (/24 = 256 IPs).
    # Evaluated last due to alphabetical naming. Skips database, backend, and the 
    # backend reserved sibling footprint to find the next aligned gap at 10.0.6.0/24.
    frontend = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

output "frontend_cidr" {
  value = cidrblock_pool.ipv4_pool.allocations["frontend"].allocated_cidr
}

output "backend_cidr" {
  value = cidrblock_pool.ipv4_pool.allocations["backend"].allocated_cidr
}

output "backend_sibling" {
  value = cidrblock_pool.ipv4_pool.allocations["backend"].sibling_cidr
}

output "database_cidr" {
  value = cidrblock_pool.ipv4_pool.allocations["database"].allocated_cidr
}

