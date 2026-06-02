# Design and Implementation Plan: `finext/cidrblock` Terraform Provider

## 1. Architectural Overview
The `finext/cidrblock` provider is a stateless-to-stateful bridge for IP Address Management (IPAM) in Terraform. It operates as a purely logical provider—it makes no external API calls to cloud vendors. Instead, it uses rigorous CIDR mathematics and Terraform's native `.tfstate` file to track, allocate, and recycle subnets from a defined supernet pool.

It is designed to be cloud-agnostic but utilizes standard GCP hierarchical terminology (`organization`, `project`, `network`) for its isolation boundaries.

### 1.1 Core Principles
* **Single-Resource State:** All allocations exist as a map within a single `cidrblock_pool` resource to guarantee atomic updates and prevent cross-resource race conditions.
* **First-Fit Gap Filling:** Freed allocations are mathematically recycled into available gaps.
* **Soft Reservations:** Allocations can flag `reserve_sibling = true` to hold the adjacent binary sibling block for future expansion.
* **Dual-Stack:** Native support for both IPv4 and IPv6 using Go's `net/netip` standard library.

## 2. Schema & Design Details

### 2.1 Input Validation Rules
To ensure state stability and prevent delimiter collisions if the backend concatenates namespaces (e.g., `org:project:network`), strict input validation is enforced on the isolation strings.

* **Allowed Characters:** Alphanumeric, hyphens (`-`), and underscores (`_`). No spaces, colons, or special symbols.
* **Regex:** `^[a-zA-Z0-9-_]+$`
* **Length:** 1 to 64 characters.
* **Framework Implementation:** Handled via `stringvalidator.RegexMatches` in the `terraform-plugin-framework`.

### 2.2 Resource: `cidrblock_pool`
**Attributes:**
* `cidr` (String, Required): The base IPv4/IPv6 supernet.
* `organization` (String, Required): Top-level namespace.
* `project` (String, Required): Mid-level namespace.
* `network` (String, Required): Base-level namespace.
* `allocations` (Map of Objects, Optional): The deterministic state map.
  * `prefix_size` (Int64, Required): The target subnet mask (e.g., `24` or `64`).
  * `reserve_sibling` (Bool, Optional, Default: `false`): Soft-reserves the binary sibling block.
  * `allocated_cidr` (String, Computed): The resulting allocated block.
  * `sibling_cidr` (String, Computed): The reserved block (if `reserve_sibling` is true).

### 2.3 Data Source: `cidrblock_pool`
Provides read-only access to a pool's state, enabling cross-workspace querying.
**Attributes:**
* `id` (String, Required): The pool ID.
* `available_slices` (List of Objects, Computed): Shows all unallocated gaps (start CIDR, max allocatable prefix).
* `metrics` (Object, Computed): Total IPs, Allocated IPs, Reserved IPs.

---

## 3. Implementation Plan (Agent Workflow)

This section dictates the sequential workflow for an AI agent or developer to build this provider.

**General Agent Directives:**
1. **Branching:** Create a new feature branch for each Phase (e.g., `feat/phase-1-scaffolding`).
2. **Commits:** Use Conventional Commits (e.g., `feat:`, `fix:`, `chore:`, `test:`).
3. **Merging:** Simulate a PR merge to `main` at the end of each Phase.
4. **Framework:** Strictly use `hashicorp/terraform-plugin-framework` (avoid the legacy `sdk/v2`).
5. **TDD:** Write Go `*_test.go` files *before* implementing the core logic.

### Phase 1: Repository Scaffolding & Setup
- [ ] **Step 1:** Initialize a new Go module: `go mod init github.com/finext/terraform-provider-cidrblock`.
- [ ] **Step 2:** Pull in the required HashiCorp framework dependencies: `go get github.com/hashicorp/terraform-plugin-framework`.
- [ ] **Step 3:** Setup standard boilerplate: `main.go`, `provider/provider.go`.
- [ ] **Step 4:** Configure `.gitignore`, `Makefile`, and `.golangci.yml` for linting.
- **Commit:** `chore: initialize repository and terraform plugin framework`

### Phase 2: Core CIDR Math Engine (TDD)
- [ ] **Goal:** Build the internal Go package that handles IP logic, completely decoupled from Terraform schemas.
- [ ] **Step 1:** Create `internal/ipam/engine_test.go`. Write tests for first-fit allocation, IPv4/IPv6 detection, overlapping collision detection, and sibling reservation math.
- [ ] **Step 2:** Create `internal/ipam/engine.go` using `net/netip`. Implement the logic to make the tests pass.
- [ ] **Step 3:** Ensure the engine cleanly throws structured Go errors (e.g., `ErrPoolExhausted`, `ErrInvalidPrefix`).
- **Commit:** `feat: implement pure go cidr math engine with tdd`

### Phase 3: Provider Schema Definition
- [ ] **Step 1:** Create `provider/resource_pool.go` and define the `cidrblock_pool` schema.
- [ ] **Step 2:** Implement the `stringvalidator` logic for `organization`, `project`, and `network`.
- [ ] **Step 3:** Define the `allocations` attribute as a `types.Map` of Nested Attributes.
- [ ] **Step 4:** Create `provider/data_source_pool.go` and define the matching query schema.
- **Commit:** `feat: define resource and data source framework schemas`

### Phase 4: Resource CRUD & State Management
- [ ] **Step 1:** Implement the `Create` method in `resource_pool.go`. Read the HCL map, pass it to the `ipam.Engine`, and write the computed `allocated_cidr` values back to state.
- [ ] **Step 2:** Implement the `Update` method. Compare the plan against state. If a map key was removed, ensure the engine frees it. If a new key is added, allocate the next gap.
- [ ] **Step 3:** Implement the `Read` method for drift detection (though drift should be rare for a logical provider).
- [ ] **Step 4:** Implement `Delete` (clears the state).
- **Commit:** `feat: implement resource crud operations and state synchronization`

### Phase 5: Data Source Implementation
- [ ] **Step 1:** Implement the `Read` method in `data_source_pool.go`.
- [ ] **Step 2:** Wire it to read the local state of the requested pool ID and compute the `available_slices` and `metrics`.
- **Commit:** `feat: implement data source read logic`

### Phase 6: Acceptance Testing (Terraform CLI)
- [ ] **Step 1:** Set up `provider/provider_test.go` using HashiCorp's `resource.TestCase`.
- [ ] **Step 2:** Write full lifecycle tests simulating actual Terraform runs:
  - Create a pool with 2 allocations.
  - Add a 3rd allocation (verifying correct gap filling).
  - Remove the 2nd allocation (verifying state cleanup).
  - Toggle `reserve_sibling` on and off.
- **Commit:** `test: add end-to-end framework acceptance tests`

### Phase 7: Documentation & Examples
- [ ] **Step 1:** Add the `hashicorp/terraform-plugin-docs` generator to the `tools.go` and `Makefile`.
- [ ] **Step 2:** Add code-level documentation (`Description` fields in schemas).
- [ ] **Step 3:** Create an `examples/` directory showcasing GCP, AWS, and Azure subnets consuming the `cidrblock` provider.
- [ ] **Step 4:** Run `make generate` to build the `docs/` folder.
- **Commit:** `docs: generate provider registry documentation and examples`

### Phase 8: CI/CD & Release Management
- [ ] **Step 1:** Create `.github/workflows/test.yml` to run `go test` and `golangci-lint` on all PRs.
- [ ] **Step 2:** Create `.github/workflows/release.yml` using `goreleaser` and the standard HashiCorp Terraform Provider release action.
- [ ] **Step 3:** Configure it to trigger on tag pushes (e.g., `v1.0.0`), automatically signing binaries and publishing to the GitHub Release page (ready for the Terraform Registry).
- **Commit:** `chore: setup github actions for test and release`
