# Terraform Provider CIDR Block (`cidrblock`)

A deterministic, pure-calculation IP Address Management (IPAM) utility designed to handle collision-free subnet allocations directly inside your infrastructure-as-code pipelines. 

Unlike traditional IPAM suites, this provider requires **no external database, stateful API, or standalone IPAM server**. It isolates the core logic as a calculation engine, offloading long-term persistence entirely to the native `terraform.tfstate` ledger. This ensures that your local environment retains a stable, immutable mapping of your layout tree across subsequent pipeline runs.

## Provider Configuration

```hcl
provider "cidrblock" {
  # Safety Guardrail: When true, explicitly blocks any terraform apply configurations 
  # that attempt to delete active subnet keys from an existing pool resource.
  prevent_subnet_destruction = true
}
```

## Features

* **State-Retained Persisted Output Mapping:** Allocation structures are computed dynamically on-the-fly during Terraform execution, using the local state file to lock down address boundaries once committed.
* **First-Fit Decreasing (FFD) Layout Sorting:** Allocations are automatically sorted by network footprint size descending before evaluation. This ensures large networks are placed first, eliminating binary boundary alignment "dead zones" and maximizing pool density. Ties on identical sizing revert to alphabetical order for strict state determinism.
* **Automated Buddy/Sibling Reservations:** Lock down adjacent mirror subnets natively (e.g., reserving a matching `/24` partner for an active `/24` deployment). Sibling placement is mathematically restricted to left-hand (lower-half) aligned blocks to guarantee predictable expansion boundaries.
* **Dual-Stack Capability:** Seamless processing of both IPv4 and IPv6 addressing families down to micro-subnet definitions (e.g., `/128` inside an IPv6 `/48`).
* **Production-Grade Resilience:** Core layout engine hardened against in-place update mutation errors, bit-shift boundaries, memory masking overflows, and algorithmic calculation hangs ($O(M)$ computational scaling where $M$ is active keys).

---

## Requirements

* [Terraform](https://www.terraform.io/downloads.html) >= 1.0
* [Go](https://golang.org/doc/install) >= 1.21 (if building from source)

---

## Usage Example

```hcl
terraform {
  required_providers {
    cidrblock = {
      source = "finext-networks/cidrblock"
    }
  }
}

provider "cidrblock" {}

# Define and allocate subnets within an enterprise VPC container space
resource "cidrblock_pool" "vpc_prod" {
  cidr                = "10.100.0.0/20"
  organization        = "finext"
  project             = "core-banking"
  network             = "prod-east"
  allocation_strategy = "FIRST"

  allocations = {
    # Written in any block order, the provider's FFD scheduler will automatically 
    # process public_aza first (/24), then database_aza (/26) for optimal tree packing.
    database_aza = {
      prefix_size     = 26
      reserve_sibling = false
    }
    public_aza = {
      prefix_size     = 24
      reserve_sibling = true  # Allocates 10.100.0.0/24 and reserves 10.100.1.0/24
    }
  }
}
```

---

## Lifecycle & Mutation Invariants

To safeguard production topologies, the calculation engine enforces strict constraints during subsequent `terraform apply` operations:

* **Base-Address Immutability:** Once a subnet is assigned an address, its base coordinate is anchored. The provider will never automatically shift or float an existing subnet's base IP to fulfill a layout change.
* **Unaligned Upgrades:** Modifying the bit mask length of an active subnet (e.g., growing from `/25` to `/24`) is strictly evaluated as an anchored in-place expansion. If the current base address is mathematically incompatible with the new size's natural alignment boundaries, the update returns a hard allocation error. Moving unaligned blocks requires an explicit delete-and-recreate sequence.
* **Left-Hand Buddy Constraint:** Sibling reservations (`reserve_sibling = true`) are only valid on left-hand (lower-half) aligned blocks of a given tier size. Attempting to assign or toggle a sibling reservation on a right-hand block returns a hard validation error.
* **Sibling Absorption:** To absorb an active sibling reservation block during a size expansion, increase the `prefix_size` and toggle `reserve_sibling = false` simultaneously.

---

## Development & Testing

### Running the Test Suite
The repository includes unit tests, strategy matrix evaluations, and intensive integration acceptance testing hooks using the Terraform Plugin Testing framework:

```bash
# Run unit and algorithmic regression tests
go test ./... -v

# Run full cross-package framework acceptance tests
TF_ACC=1 go test ./... -v -coverprofile=coverage.out -coverpkg=./...
```

### Fuzz Testing
To evaluate core calculation loop integrity against unexpected bit-boundaries or data-type saturation anomalies, run the integrated fuzzer:

```bash
go test ./internal/ipam/... -fuzz=FuzzEngine_Allocation -fuzztime=60s
```

---

## License

This project is licensed under the Mozilla Public License, v. 2.0. See the `LICENSE` file for full details.

