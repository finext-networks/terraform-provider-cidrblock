# cidrblock_pool (`Resource`)

Manages an atomic IP address prefix pool allocation layout grid. This resource operates as a stateless calculation engine using continuous boundary mapping to compute and track allocated CIDR blocks without an external database back-end.

## Example Usage

```hcl
resource "cidrblock_pool" "vpc_network" {
  cidr                = "10.0.0.0/20"
  organization        = "corp"
  project             = "web-platform"
  network             = "production"
  allocation_strategy = "BEST"

  allocations = {
    public_aza = {
      prefix_size     = 24
      reserve_sibling = true # Reserves 10.0.1.0/24 immediately as a buddy partner
    }
    private_aza = {
      prefix_size     = 22
      reserve_sibling = false
    }
    database_aza = {
      prefix_size     = 26
      reserve_sibling = false
    }
  }
}

# Consuming generated address space blocks downstream
output "public_subnet_cidr" {
  value = cidrblock_pool.vpc_network.allocations["public_aza"].allocated_cidr
}
```

## Schema

### Required

- `cidr` (String) The base IPv4/IPv6 supernet container prefix block (e.g., `10.0.0.0/16`, `2001:db8::/48`). Host bits must be unset.
- `network` (String) Part of the structural namespace boundaries. Allowed characters: alphanumeric, hyphens, and underscores.
- `organization` (String) Part of the structural namespace boundaries. Allowed characters: alphanumeric, hyphens, and underscores.
- `project` (String) Part of the structural namespace boundaries. Allowed characters: alphanumeric, hyphens, and underscores.

### Optional

- `allocation_strategy` (String) Algorithmic layout search placement logic strategy choice:
  - `FIRST`: Claims the lowest naturally aligned free gap (Default).
  - `BEST`: Claims the smallest possible free slice space fitting the query, isolating fragments.
  - `SPARSE`: Claims the largest open contiguous block space chunk, maximizing network isolation.
- `allocations` (Map of Object) Nested map tracking independent requested subnets. (see [below for nested schema](#nestedblock--allocations))

### Computed

- `id` (String) The synthetic unique structural tracking provider identifier (format: `organization:project:network:cidr`).

<a id="nestedblock--allocations"></a>
### Nested Schema for `allocations`

#### Required

- `prefix_size` (Number) Intended block length mask bits requested for this specific key name (e.g., `24` inside an IPv4 pool or `64` inside an IPv6 space).

#### Optional

- `reserve_sibling` (Boolean) Toggle to automatically lock the matching continuous binary buddy block sibling. Defaults to `false`.

#### Computed

- `allocated_cidr` (String) The mathematically isolated, collision-free block computed by the routing engine.
- `sibling_cidr` (String) The paired companion buddy CIDR string reserved by the allocation engine if `reserve_sibling` is enabled.

## Import

Logical state pool configurations can be imported into the state file by supplying the synthetic composite identifier matching this configuration pattern:

```bash
terraform import cidrblock_pool.vpc_network corporate:billing-platform:prod-east:10.10.0.0/16
```
