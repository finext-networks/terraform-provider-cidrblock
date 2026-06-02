# cidrblock_pool (`Data Source`)

Reads a CIDR block pool's state, tracking current layout allocations, calculating unallocated contiguous slice boundaries, and emitting exact IP usage capacity metrics statistics out of state parameters.

## Example Usage

```hcl
data "cidrblock_pool" "query" {
  id   = "corp:web-platform:production"
  cidr = "10.0.0.0/20"
}

output "available_slices_report" {
  value = data.cidrblock_pool.query.available_slices
}

output "remaining_ips_count" {
  value = data.cidrblock_pool.query.metrics.available_ips
}
```

## Schema

### Required

- `cidr` (String) The base IPv4/IPv6 supernet super-prefix CIDR required to track unallocated gaps.
- `id` (String) The structural matching compound filter parameter (format: `organization:project:network`).

### Computed

- `allocation_strategy` (String) Layout lookup mode. Always evaluates to `FIRST`.
- `allocations` (Map of Object) Computed layout items map tracking records. (see [below for nested schema](#nestedblock--allocations))
- `available_slices` (List of Object) Ordered collection listing remaining continuous empty gaps. (see [below for nested schema](#nestedblock--available_slices))
- `metrics` (Object) Math utilization storage block tracking capacities. (see [below for nested schema](#nestedblock--metrics))
- `network` (String) Isolated network segment element parsed out from the ID token.
- `organization` (String) Corporate tenancy parent identity parsed out from the ID token.
- `project` (String) Operational workload scope item identifier parsed out from the ID token.

<a id="nestedblock--allocations"></a>
### Nested Schema for `allocations`

#### Computed

- `allocated_cidr` (String) The calculated subnet address.
- `prefix_size` (Number) Bit length size block configuration request.
- `reserve_sibling` (Boolean) Buddy assignment option status.
- `sibling_cidr` (String) Parity companion reservation record string.

<a id="nestedblock--available_slices"></a>
### Nested Schema for `available_slices`

#### Computed

- `max_prefix_size` (Number) The largest power-of-two natural block boundary size that can fit into this specific continuous free gap.
- `start_cidr` (String) The base starting network address block string marking the opening boundary of this empty gap.

<a id="nestedblock--metrics"></a>
### Nested Schema for `metrics`

#### Computed

- `allocated_ips` (Number) Total discrete host counts consumed by your primary allocation blocks.
- `available_ips` (Number) Total free assignable IP host capacity counts remaining before pool exhaustion.
- `reserved_ips` (Number) Total host allocations isolated by active buddy sister sibling reservations.
- `total_ips` (Number) Max mathematical host address capacity block count allowed by the base pool size prefix.
