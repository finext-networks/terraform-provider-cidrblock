---
name: Bug Report
about: Create a report to help us improve the allocation engine
title: '[BUG] '
labels: bug
assignees: ''
---

## Description
A clear and concise description of what the unexpected layout behavior is.

## Minimal Reproducible HCL Configuration
```terraform
resource "cidrblock_pool" "error_repro" {
  cidr                = "10.0.0.0/16"
  allocation_strategy = "FIRST" # Or BEST / SPARSE

  allocations = {
    # Paste your exact allocations map block here
  }
}
```

## Expected Output Coordinates
What IP range or behavioral invariant did you expect to see materialize?

## Actual Output / Error Diagnostic Stack Traces
Paste the terminal logs or `terraform apply` panic dumps here.

