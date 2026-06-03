# cidrblock_pool (`Resource`)

Manages an atomic IP address prefix pool allocation layout grid. This resource operates as a stateless calculation engine using continuous boundary mapping to compute and track allocated CIDR blocks without an external database back-end.

## Example Usage

```hcl
resource "cidrblock_pool" "vpc_network" {
  cidr                = "10.0.0.0/20"
  organization        = "corp"
  project             = "web-platform"
  network             = "production"
  allocation_strategy = "FIRST"

  allocations = {
    public_aza = {
      prefix_size     = 24
      reserve_sibling = true # Reserves 10.0.1.0/24 immediately as a buddy partner (Valid: Left-hand aligned)
    }
    private_aza = {
      prefix_size     = 22   # Consumes 10.0.4.0/22
      reserve_sibling = false
    }
    database_aza = {
      prefix_size     = 26   # Consumes 10.0.8.0/26
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

- `reserve_sibling` (Boolean) Toggle to automatically lock the matching continuous binary buddy block sibling. Defaults to `false`. **Note:** Enforces left-hand boundary constraints. Setting this to `true` on right-hand aligned layouts or blocked mutation footprints will trigger a hard allocation error.

#### Computed

- `allocated_cidr` (String) The mathematically isolated, collision-free block computed by the routing engine.
- `sibling_cidr` (String) The paired companion buddy CIDR string reserved by the allocation engine if `reserve_sibling` is enabled.

## Lifecycle & Mutation Invariants

To guarantee production safety across infrastructure updates, the calculation engine enforces strict structural constraints on resource updates:

1. **Base-Address Immutability:** Subnet address assignments are completely anchored upon initial compilation. The engine rejects any modification that forces an active subnet's base gateway IP to change.
2. **Anchored In-Place Expansions:** Altering a prefix size (e.g., `/25` to `/24`) is processed exclusively at the existing base address anchor. If the block is not naturally aligned to the new mask width (such as attempting to grow a right-hand `.128/25` subnet into a `/24`), the engine blocks execution with an alignment boundary error.
3. **Left-Hand Sibling Constraint:** Automated forward sibling pairings are restricted mathematically to lower-half buddy blocks. Activating `reserve_sibling` on an upper-half right-hand block alignment (which can never naturally expand forward) is blocked.
4. **Cascading Upgrade Blockers:** Leaving `reserve_sibling = true` during an in-place prefix size expansion causes the engine to evaluate the availability of the new next-tier companion footprint. If that next block is already claimed by an independent subnet or exits pool boundaries, execution fails. To expand into an existing sibling reservation, `reserve_sibling` must be explicitly toggled to `false`.

## Import

Logical state pool configurations can be imported into the state file by supplying the synthetic composite identifier matching this configuration pattern:

```bash
terraform import cidrblock_pool.vpc_network corporate:billing-platform:prod-east:10.10.0.0/16
```

