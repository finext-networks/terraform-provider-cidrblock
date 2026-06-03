terraform {
  required_providers {
    cidrblock = {
      source = "finext-networks/cidrblock"
    }
  }
}

provider "cidrblock" {
  # Anchored to true to catch security/destruction edge cases in sad paths
  prevent_subnet_destruction = true
}

resource "cidrblock_pool" "integration_pool" {
  cidr                = var.supernet_cidr
  organization        = "integration-testing-org"
  project             = "e2e-harness-project"
  network             = "vpc-integration-boundary"
  allocation_strategy = var.allocation_strategy
  allocations         = var.active_allocations
}

