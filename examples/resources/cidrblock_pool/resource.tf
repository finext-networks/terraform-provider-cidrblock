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
  allocation_strategy = "FIRST" # Alphabetical processing sequence applies over keys: backend, database, frontend

  allocations = {
    # 1. Processed First Alphabetically: /24 sizing lands on 10.0.0.0/24. 
    # Valid: Left-hand aligned block, reserving 10.0.1.0/24 cleanly as its buddy partner.
    backend = {
      prefix_size     = 24
      reserve_sibling = true
    }

    # 2. Processed Second: Requires a /22 boundary alignment. 
    # Skips backend + reserved sibling footprint (10.0.0.0 - 10.0.1.255) to land on 10.0.4.0/22.
    database = {
      prefix_size     = 22
      reserve_sibling = false
    }

    # 3. Processed Third: Scans from bottom. Fills first available aligned gap at 10.0.2.0/24.
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

