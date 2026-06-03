# Suite: happy_first.tftest.hcl
# Validates sequential bottom-up packing metrics via the FIRST strategy.

variables {
  supernet_cidr       = "10.200.0.0/16"
  allocation_strategy = "FIRST"
  active_allocations  = {
    gke_pods = {
      prefix_size     = 20
      reserve_sibling = false
    }
    gke_nodes = {
      prefix_size     = 22
      reserve_sibling = true
    }
  }
}

run "validate_sequential_first_packing" {
  # Aligned to apply so computed allocations are fully materialized in state
  command = apply

  assert {
    condition     = output.computed_allocations["gke_pods"].allocated_cidr == "10.200.0.0/20"
    error_message = "FIRST strategy failed to anchor the largest priority subnet to the baseline coordinate."
  }

  assert {
    condition     = output.computed_allocations["gke_nodes"].allocated_cidr == "10.200.16.0/22"
    error_message = "FIRST strategy calculated incorrect start address for second-tier layout block."
  }

  assert {
    condition     = output.computed_allocations["gke_nodes"].sibling_cidr == "10.200.20.0/22"
    error_message = "Forward companion sibling reservation coordinate computation mismatched tree bounds."
  }
}

