terraform {
  required_providers {
    cidrblock = {
      source = "finext-networks/cidrblock"
    }
  }
}

# Basic IPv4 pool
resource "cidrblock_pool" "ipv4_pool" {
  cidr         = "10.0.0.0/16"
  organization = "my-org"
  project      = "my-project"
  network      = "vpc-main"

  allocations = {
    frontend = {
      prefix_size     = 24
      reserve_sibling = false
    }

    backend = {
      prefix_size     = 24
      reserve_sibling = true
    }

    database = {
      prefix_size     = 22
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
