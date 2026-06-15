terraform {
  required_providers {
    cidrblock = {
      source = "finext-networks/cidrblock"
    }
  }
}

# Approach A: Using the Composite ID (Inter-Resource Binding)
# Use this pattern when querying a pool managed within the same configuration 
# or state footprint by passing the resource's computed identifier string.
resource "cidrblock_pool" "core_network" {
  cidr         = "10.0.0.0/16"
  organization = "finext"
  project      = "production"
  network      = "backbone"
}

data "cidrblock_pool" "by_id" {
  id = cidrblock_pool.core_network.id
}

output "unallocated_address_slices" {
  value = data.cidrblock_pool.by_id.available_slices
}

output "pool_metrics" {
  value = data.cidrblock_pool.by_id.metrics
}

# Approach B: Using Discrete Coordinates (Remote Workspaces Lookup)
# Use this pattern to look up a pool dynamically across independent team 
# workspaces or remote state architectures where a direct HCL resource 
# reference is unavailable. All four attributes must be non-empty.
data "cidrblock_pool" "by_filters" {
  organization = "finext"
  project      = "production"
  network      = "backbone"
  cidr         = "10.0.0.0/16"
}

output "allocated_subnets" {
  value = data.cidrblock_pool.by_filters.allocations
}

