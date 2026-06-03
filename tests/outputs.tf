output "computed_allocations" {
  value       = cidrblock_pool.integration_pool.allocations
  description = "Exposes calculated block maps directly to HCL test assert blocks"
}

