# Terraform Provider CIDR Block (`cidrblock`)

A stateless, pure-calculation IP Address Management (IPAM) Terraform provider designed to handle deterministic subnet allocations directly inside your infrastructure-as-code pipelines. 

Unlike traditional IPAM solutions, this provider requires **no external database, stateful API, or IPAM server**. It models the entire address space as an atomic grid using binary boundary mapping, calculating collision-free allocations dynamically from a defined supernet pool.

## Features

* **100% Stateless Operations:** Subnet allocations are computed deterministically on-the-fly during Terraform plan/apply lifecycles based purely on your configuration mapping.
* **Algorithmic Allocation Strategies:**
  * `FIRST`: Places subnets sequentially in the first available aligned gap (minimized search overhead).
  * `BEST`: Minimizes fragmentation by matching requests to the tightest fitting available continuous space.
  * `SPARSE`: Maximizes isolation distance between subnets by dropping new allocations into the largest available block chunks.
* **Automated Buddy/Sibling Reservations:** Lock down adjacent mirror subnets natively (e.g., reserving a matching `/24` partner for an active `/24` deployment to account for forward-looking high-availability clustering).
* **Dual-Stack Capability:** Seamless processing of both IPv4 and IPv6 addressing families down to micro-subnet definitions (e.g., `/128` inside an IPv6 `/48`).
* **Production-Grade Resilience:** Core layout engine hardened with extensive differential fuzz testing to eliminate bit-shift boundaries, memory masking overflows, and algorithmic calculation hangs ($O(M)$ computational scaling where $M$ is active keys).

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
  allocation_strategy = "BEST"

  allocations = {
    public_aza = {
      prefix_size     = 24
      reserve_sibling = true  # Automatically allocates 10.100.0.0/24 and reserves 10.100.1.0/24
    }
    private_aza = {
      prefix_size     = 22     # Consumes 10.100.4.0/22
      reserve_sibling = false
    }
    database_aza = {
      prefix_size     = 26     # Consumes 10.100.8.0/26
      reserve_sibling = false
    }
  }
}

# Pass calculated allocations down into cloud infrastructure resources smoothly
resource "aws_subnet" "public" {
  vpc_id     = aws_vpc.main.id
  cidr_block = cidrblock_pool.vpc_prod.allocations["public_aza"].allocated_cidr
}

resource "aws_subnet" "private" {
  vpc_id     = aws_vpc.main.id
  cidr_block = cidrblock_pool.vpc_prod.allocations["private_aza"].allocated_cidr
}
```

---

## Querying with Data Sources

You can audit current structural footprints, fetch real-time address capacity metrics, and discover open layout fragments via the companion read-only data source block:

```hcl
data "cidrblock_pool" "audit" {
  id   = "finext:core-banking:prod-east"
  cidr = "10.100.0.0/20"
}

output "remaining_free_ips" {
  value = data.cidrblock_pool.audit.metrics.available_ips
}

output "unallocated_gaps_report" {
  value = data.cidrblock_pool.audit.available_slices
}
```

---

## Development & Testing

### Building From Source
Clone the repository and compile the provider binary locally using the Go toolchain:

```bash
git clone https://github.com/finext-networks/terraform-provider-cidrblock.git
cd terraform-provider-cidrblock
go build -o terraform-provider-cidrblock
```

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
```
