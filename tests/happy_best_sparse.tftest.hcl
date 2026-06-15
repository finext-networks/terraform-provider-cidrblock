# Suite: happy_best_sparse.tftest.hcl
# Validates configuration mechanics when switching strategies between BEST and SPARSE layouts.

variables {
  supernet_cidr = "10.210.0.0/16"
  active_allocations = {
    core_app = {
      prefix_size     = 18
      reserve_sibling = false
    }
    mesh_edge = {
      prefix_size     = 24
      reserve_sibling = false
    }
  }
}

run "verify_best_fit_execution" {
  # Executed as apply to populate dynamic calculation results
  command = apply

  variables {
    allocation_strategy = "BEST"
  }

  assert {
    condition     = output.computed_allocations["mesh_edge"].allocated_cidr == "10.210.64.0/24"
    error_message = "BEST strategy failed to isolate the small block inside the tightest available gap."
  }
}

run "verify_sparse_fit_execution" {
  # Executed as apply to populate dynamic calculation results
  command = apply

  variables {
    allocation_strategy = "SPARSE"
  }

  assert {
    condition     = output.computed_allocations["mesh_edge"].allocated_cidr == "10.210.64.0/24"
    error_message = "SPARSE strategy failed to select the largest open continuous address slice chunk."
  }
}

