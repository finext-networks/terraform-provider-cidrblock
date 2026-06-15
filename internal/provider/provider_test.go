// Copyright (c) Finext Networks. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function is invoked for every Terraform
// CLI command executed to create a localized provider server speaking Protocol 6.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"cidrblock": providerserver.NewProtocol6WithError(New("test")()),
}

// TestAccPoolResource_Basic tests basic pool creation with allocations.
func TestAccPoolResource_Basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
						subnet_a = {
								prefix_size     = 28
								reserve_sibling = false
						}
						subnet_b = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "id", "test-org:test-project:test-network:10.0.0.0/24"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "cidr", "10.0.0.0/24"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "organization", "test-org"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "project", "test-project"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "network", "test-network"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_a.allocated_cidr", "10.0.0.0/28"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_b.allocated_cidr", "10.0.0.16/28"),
				),
			},
			// ImportState testing
			{
				ResourceName:            "cidrblock_pool.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"allocations", "allocation_strategy"}, // Fix: Ignore un-inferable strategy parameter on logical imports
			},
		},
	})
}

// TestAccPoolResource_AddAllocation tests adding a new allocation.
func TestAccPoolResource_AddAllocation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with 2 allocations
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
						subnet_a = {
								prefix_size     = 28
								reserve_sibling = false
						}
						subnet_b = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_a.allocated_cidr", "10.0.0.0/28"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_b.allocated_cidr", "10.0.0.16/28"),
				),
			},
			// Add a 3rd allocation
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
						subnet_a = {
								prefix_size     = 28
								reserve_sibling = false
						}
						subnet_b = {
								prefix_size     = 28
								reserve_sibling = false
						}
						subnet_c = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_c.allocated_cidr", "10.0.0.32/28"),
				),
			},
		},
	})
}

// TestAccPoolResource_RemoveAllocation tests removing an allocation.
func TestAccPoolResource_RemoveAllocation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with 3 allocations
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
						subnet_a = {
								prefix_size     = 28
								reserve_sibling = false
						}
						subnet_b = {
								prefix_size     = 28
								reserve_sibling = false
						}
						subnet_c = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_a.allocated_cidr", "10.0.0.0/28"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_b.allocated_cidr", "10.0.0.16/28"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_c.allocated_cidr", "10.0.0.32/28"),
				),
			},
			// Remove the 2nd allocation
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
						subnet_a = {
								prefix_size     = 28
								reserve_sibling = false
						}
						subnet_c = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_a.allocated_cidr", "10.0.0.0/28"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_c.allocated_cidr", "10.0.0.32/28"),
				),
			},
		},
	})
}

// TestAccPoolResource_ReserveSibling tests toggling reserve_sibling.
func TestAccPoolResource_ReserveSibling(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with reserve_sibling = false
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
						subnet_a = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_a.allocated_cidr", "10.0.0.0/28"),
					// Assert the sibling_cidr attribute is completely null/absent from state mapping
					resource.TestCheckNoResourceAttr("cidrblock_pool.test", "allocations.subnet_a.sibling_cidr"),
				),
			},
			// Toggle reserve_sibling to true
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
						subnet_a = {
								prefix_size     = 28
								reserve_sibling = true
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_a.allocated_cidr", "10.0.0.0/28"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_a.sibling_cidr", "10.0.0.16/28"),
				),
			},
			// Toggle reserve_sibling back to false
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
						subnet_a = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_a.allocated_cidr", "10.0.0.0/28"),
					// Assert the sibling_cidr attribute transitions back to clean null absence
					resource.TestCheckNoResourceAttr("cidrblock_pool.test", "allocations.subnet_a.sibling_cidr"),
				),
			},
		},
	})
}

// TestAccPoolResource_GapFilling tests that freed allocations are refilled.
func TestAccPoolResource_GapFilling(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create with 3 allocations
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
						subnet_a = {
								prefix_size     = 28
								reserve_sibling = false
						}
						subnet_b = {
								prefix_size     = 28
								reserve_sibling = false
						}
						subnet_c = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_a.allocated_cidr", "10.0.0.0/28"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_b.allocated_cidr", "10.0.0.16/28"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_c.allocated_cidr", "10.0.0.32/28"),
				),
			},
			// Remove middle allocation
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
						subnet_a = {
								prefix_size     = 28
								reserve_sibling = false
						}
						subnet_c = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_a.allocated_cidr", "10.0.0.0/28"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_c.allocated_cidr", "10.0.0.32/28"),
				),
			},
			// Add new allocation - should fill the gap
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
						subnet_a = {
								prefix_size     = 28
								reserve_sibling = false
						}
						subnet_d = {
								prefix_size     = 28
								reserve_sibling = false
						}
						subnet_c = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_a.allocated_cidr", "10.0.0.0/28"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_d.allocated_cidr", "10.0.0.16/28"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_c.allocated_cidr", "10.0.0.32/28"),
				),
			},
		},
	})
}

// TestAccPoolResource_IPv6 tests IPv6 allocation.
func TestAccPoolResource_IPv6(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPoolResourceConfig("2001:db8::/48", `
						v6_subnet_a = {
								prefix_size     = 64
								reserve_sibling = false
						}
						v6_subnet_b = {
								prefix_size     = 64
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "cidr", "2001:db8::/48"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.v6_subnet_a.allocated_cidr", "2001:db8::/64"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.v6_subnet_b.allocated_cidr", "2001:db8:0:1::/64"),
				),
			},
		},
	})
}

// TestAccDataSource_Basic tests the data source.
func TestAccDataSource_Basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create a pool resource first
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
						subnet_a = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`),
			},
			// Read the pool via data source
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
						subnet_a = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`) + `
						data "cidrblock_pool" "test" {
								id = "test-org:test-project:test-network:10.0.0.0/24"
						}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.cidrblock_pool.test", "id", "test-org:test-project:test-network:10.0.0.0/24"),
					resource.TestCheckResourceAttr("data.cidrblock_pool.test", "organization", "test-org"),
					resource.TestCheckResourceAttr("data.cidrblock_pool.test", "project", "test-project"),
					resource.TestCheckResourceAttr("data.cidrblock_pool.test", "network", "test-network"),
					resource.TestCheckResourceAttr("data.cidrblock_pool.test", "cidr", "10.0.0.0/24"),
					resource.TestCheckResourceAttr("data.cidrblock_pool.test", "metrics.total_ips", "256"),
				),
			},
		},
	})
}

// testAccPoolResourceConfig generates a test config for the pool resource.
func testAccPoolResourceConfig(cidr string, allocations string) string {
	return `
provider "cidrblock" {}

resource "cidrblock_pool" "test" {
  cidr         = "` + cidr + `"
  organization = "test-org"
  project      = "test-project"
  network      = "test-network"

  allocations = {
` + allocations + `
  }
}
`
}

// TestAccPoolResource_AlgorithmicBestFit verifies that the BEST strategy
// isolates small fragments instead of breaking up larger free blocks.
func TestAccPoolResource_AlgorithmicBestFit(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPoolResourceConfigWithStrategy("10.0.0.0/24", "BEST", `
						subnet_large_block = {
								prefix_size     = 26
								reserve_sibling = false
						}
						subnet_small_fragment = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_large_block.allocated_cidr", "10.0.0.0/26"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_small_fragment.allocated_cidr", "10.0.0.64/28"),
				),
			},
		},
	})
}

// TestAccPoolResource_StrategyInPlaceMutation validates that changing the strategy flag
// on an active resource functions cleanly without shifting existing allocations.
func TestAccPoolResource_StrategyInPlaceMutation(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: Establish baseline allocations with FIRST strategy
			{
				Config: testAccPoolResourceConfigWithStrategy("10.0.0.0/24", "FIRST", `
						subnet_a = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_a.allocated_cidr", "10.0.0.0/28"),
				),
			},
			// Step 2: Swap the strategy to SPARSE in place.
			// Stateful hydration must preserve subnet_a at .0/28 without moving it.
			{
				Config: testAccPoolResourceConfigWithStrategy("10.0.0.0/24", "SPARSE", `
						subnet_a = {
								prefix_size     = 28
								reserve_sibling = false
						}
						subnet_b = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocation_strategy", "SPARSE"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_a.allocated_cidr", "10.0.0.0/28"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_b.allocated_cidr", "10.0.0.128/28"),
				),
			},
		},
	})
}

func testAccPoolResourceConfigWithStrategy(cidr string, strategy string, allocations string) string {
	return `
provider "cidrblock" {}

resource "cidrblock_pool" "test" {
  cidr                = "` + cidr + `"
  organization        = "test-org"
  project             = "test-project"
  network             = "test-network"
  allocation_strategy = "` + strategy + `"

  allocations = {
` + allocations + `
  }
}
`
}

// TestAccPoolResource_InvalidNamespace Regex rejection error paths validation.
func TestAccPoolResource_InvalidNamespace(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPoolResourceConfigWithStrategy("10.0.0.0/24", "FIRST", `
						invalid_namespace_subnet = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`) + `
						// Overwrite with an invalid configuration character (colon)
						resource "cidrblock_pool" "invalid" {
						  cidr         = "10.0.0.0/24"
						  organization = "corporate:finance" // Fails regex validation
						  project      = "valid-proj"
						  network      = "valid-net"
						}
				`,
				ExpectError: regexp.MustCompile("Invalid Namespace Boundary"),
			},
		},
	})
}

// TestAccPoolResource_CreateWithSibling verifies saving SiblingCIDR strings right inside the Create lifecycle step.
func TestAccPoolResource_CreateWithSibling(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
						subnet_with_immediate_sibling = {
								prefix_size     = 28
								reserve_sibling = true // Checked during initial create
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_with_immediate_sibling.allocated_cidr", "10.0.0.0/28"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_with_immediate_sibling.sibling_cidr", "10.0.0.16/28"),
				),
			},
		},
	})
}

// TestAccDataSource_MalformedID verifies the error parsing return paths inside the data source block.
func TestAccDataSource_MalformedID(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
						provider "cidrblock" {}
						data "cidrblock_pool" "malformed" {
								id   = "broken-string-missing-colons" // Triggers error parsing logic
								cidr = "10.0.0.0/24"
						}
				`,
				ExpectError: regexp.MustCompile("Invalid Pool ID"),
			},
		},
	})
}

// TestAccPoolResource_FrameworkFaults executes schema validation errors, engine failures,
// and deployment allocation crashes to cover provider diagnostic return paths.
func TestAccPoolResource_FrameworkFaults(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// 1. Force engine initialization failure path via malformed base CIDR
			{
				Config: `
						resource "cidrblock_pool" "fault_cidr" {
								cidr         = "invalid-cidr-string"
								organization = "ops"
								project      = "infra"
								network      = "core"
						}
				`,
				ExpectError: regexp.MustCompile("Invalid Base Pool Prefix"),
			},
			// 2. Force allocation execution crash via impossible layout sizing requests
			{
				Config: `
						resource "cidrblock_pool" "fault_alloc" {
								cidr         = "10.0.0.0/28"
								organization = "ops"
								project      = "infra"
								network      = "core"
								allocations = {
										oversized = {
												prefix_size = 24 // Too large for a /28 pool container
										}
								}
						}
				`,
				ExpectError: regexp.MustCompile("Allocation Failed"),
			},
		},
	})
}

// TestAccPoolResource_UpdateWithNewSibling Key insertion branch testing.
func TestAccPoolResource_UpdateWithNewSibling(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Initialize standard base block layout
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
						initial_subnet = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`),
			},
			// Inject a completely new map element containing an active sibling reservation on update
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
						initial_subnet = {
								prefix_size     = 28
								reserve_sibling = false
						}
						brand_new_subnet = {
								prefix_size     = 28
								reserve_sibling = true
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.brand_new_subnet.allocated_cidr", "10.0.0.32/28"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.brand_new_subnet.sibling_cidr", "10.0.0.48/28"), // Fix: Assert the mathematically correct paired boundary
				),
			},
		},
	})
}

// TestAccDataSource_FaultPaths exercises parsing engine dropouts inside the data source block.
func TestAccDataSource_FaultPaths(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
						provider "cidrblock" {}
						data "cidrblock_pool" "faulty" {
								id   = "org:project:net:10.0.0.0/24"
								cidr = "broken-cidr-format"
						}
				`,
				ExpectError: regexp.MustCompile("Engine Creation Failed"),
			},
		},
	})
}

// TestAccPoolResource_FrameworkFallbacks exercises empty maps, missing configurations,
// and runtime update error bubbles to validate provider diagnostic pipelines.
func TestAccPoolResource_FrameworkFallbacks(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// 1. Create a pool completely empty of allocations to clear the Create fallback blocks
			{
				Config: `
						resource "cidrblock_pool" "empty_test" {
								cidr         = "10.0.0.0/24"
								organization = "minimal"
								project      = "clean-slate"
								network      = "dev"
						}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.empty_test", "allocation_strategy", "FIRST"),
				),
			},
			// 2. Add an impossible allocation dynamically during update to trigger the modification error paths
			{
				Config: `
						resource "cidrblock_pool" "empty_test" {
								cidr         = "10.0.0.0/24"
								organization = "minimal"
								project      = "clean-slate"
								network      = "dev"
								allocations = {
										impossible_subnet = {
												prefix_size = 16 // Invalid size for a /24 container pool
										}
								}
						}
				`,
				ExpectError: regexp.MustCompile("Pool Allocation Failed"),
			},
		},
	})
}

// TestAccDataSource_EmptyInputs checks diagnostic short-circuits inside the data source block.
func TestAccDataSource_EmptyInputs(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
						provider "cidrblock" {}
						data "cidrblock_pool" "empty_fault" {
								id   = "" // Empty inputs trigger short circuit return blocks
								cidr = ""
						}
				`,
				ExpectError: regexp.MustCompile("Missing Parameters"), // Fix: Synchronize with our internal data source error diagnostic title
			},
		},
	})
}

// TestAccPoolResource_FirstFitDecreasingLifecycle validates that the provider re-orders incoming requests
// by network size descending to optimize packing efficiency, and checks null map update safety paths.
func TestAccPoolResource_FirstFitDecreasingLifecycle(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: Create a pool with anti-optimal alphabetical names.
			// FFD sorting intercepts this and evaluates 'a_large_tier' first, preventing fragmentation dead-zones.
			{
				Config: testAccPoolResourceConfig("10.80.0.0/24", `
						z_small_tier = {
								prefix_size     = 28
								reserve_sibling = false
						}
						a_large_tier = {
								prefix_size     = 26
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.a_large_tier.allocated_cidr", "10.80.0.0/26"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.z_small_tier.allocated_cidr", "10.80.0.64/28"),
				),
			},
			// Step 2: Update the pool by adding a third tier.
			// Base-Address Immutability guarantees 'z_small_tier' stays anchored at .64/28.
			// The new /27 medium tier aligns cleanly onto the next available boundaries at .96/27.
			{
				Config: testAccPoolResourceConfig("10.80.0.0/24", `
						z_small_tier = {
								prefix_size     = 28
								reserve_sibling = false
						}
						a_large_tier = {
								prefix_size     = 26
								reserve_sibling = false
						}
						m_medium_tier = {
								prefix_size     = 27
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.a_large_tier.allocated_cidr", "10.80.0.0/26"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.m_medium_tier.allocated_cidr", "10.80.0.96/27"), // Aligned past anchored small block
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.z_small_tier.allocated_cidr", "10.80.0.64/28"),  // Stays locked in place
				),
			},
			// Step 3: Wipe all allocations entirely to clear the MapNull update diagnostic coverage blocks.
			{
				Config: testAccPoolResourceConfig("10.80.0.0/24", ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocation_strategy", "FIRST"),
				),
			},
			// Step 4: Update the resource by completely omitting allocation_strategy and allocations parameters.
			// Because allocation_strategy is Computed, omitting it triggers our provider fallback value ("FIRST").
			{
				Config: `
						provider "cidrblock" {}
						resource "cidrblock_pool" "test" {
						  cidr         = "10.80.0.0/24"
						  organization = "test-org"
						  project      = "test-project"
						  network      = "test-network"
						}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "cidr", "10.80.0.0/24"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocation_strategy", "FIRST"), // Asserts default compute fallback
				),
			},
		},
	})
}

// TestAccPoolResource_SafetyGuardrailDestruction blocks resource degradation plans
func TestAccPoolResource_SafetyGuardrailDestruction(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: Initialize with safety flag enabled and a test subnet
			{
				Config: `
						provider "cidrblock" {
								prevent_subnet_destruction = true
						}
						resource "cidrblock_pool" "secure" {
								cidr         = "10.90.0.0/24"
								organization = "sec"
								project      = "ops"
								network      = "main"
								allocations = {
										important_service = {
												prefix_size     = 26
												reserve_sibling = false
										}
								}
						}
				`,
				Check: resource.TestCheckResourceAttr("cidrblock_pool.secure", "allocations.important_service.allocated_cidr", "10.90.0.0/26"),
			},
			// Step 2: Attempt to wipe the subnet key out of the block. The provider must fail the run.
			{
				Config: `
						provider "cidrblock" {
								prevent_subnet_destruction = true
						}
						resource "cidrblock_pool" "secure" {
								cidr         = "10.90.0.0/24"
								organization = "sec"
								project      = "ops"
								network      = "main"
								allocations  = {}
						}
				`,
				ExpectError: regexp.MustCompile("Subnet Destruction Blocked"),
			},
		},
	})
}

// TestAccPoolResource_UpdateValidationFailure forces a structural error during the update
// phase to ensure the framework gracefully catches and exposes calculation boundary errors.
func TestAccPoolResource_UpdateValidationFailure(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: Initialize a stable layout where alphabetical sorting forces
			// the target update block onto an unaligned offset address (.16/28).
			{
				Config: testAccPoolResourceConfig("10.75.0.0/24", `
						a_anchor_subnet = {
								prefix_size     = 28
								reserve_sibling = false
						}
						z_unaligned_subnet = {
								prefix_size     = 28
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.a_anchor_subnet.allocated_cidr", "10.75.0.0/28"),
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.z_unaligned_subnet.allocated_cidr", "10.75.0.16/28"),
				),
			},
			// Step 2: Attempt an unaligned size alteration on z_unaligned_subnet (.16/28 to /27).
			// This breaks binary tree boundaries and forces an UpdateAllocation alignment error pass.
			{
				Config: testAccPoolResourceConfig("10.75.0.0/24", `
						a_anchor_subnet = {
								prefix_size     = 28
								reserve_sibling = false
						}
						z_unaligned_subnet = {
								prefix_size     = 27
								reserve_sibling = false
						}
				`),
				// Fix: Shortened to a wrap-proof substring because the Terraform CLI text-wraps
				// long diagnostic details, breaking multi-word checks that happen to cross line breaks.
				ExpectError: regexp.MustCompile("breaks binary alignment"),
			},
		},
	})
}

// TestAccPoolDataSource_HydratedLifecycle verifies that the read-only data source
// successfully pulls from the backend state grid and yields fully accurate allocation maps,
// tracking strategies, available structural slices, and matching calculation metrics.
func TestAccPoolDataSource_HydratedLifecycle(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Step 1: Create a live pool with a complex mix of sibling reservations
			// and anti-optimal sizing alignments under the BEST strategy.
			{
				Config: testAccPoolResourceConfigWithStrategy("192.168.0.0/16", "BEST", `
						management = {
								prefix_size     = 24
								reserve_sibling = false
						}
						appliances = {
								prefix_size     = 23
								reserve_sibling = true
						}
						pods = {
								prefix_size     = 20
								reserve_sibling = true
						}
						junk = {
								prefix_size     = 18
								reserve_sibling = false
						}
						wan = {
								prefix_size     = 24
								reserve_sibling = false
						}
				`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "id", "test-org:test-project:test-network:192.168.0.0/16"),
				),
			},
			// Step 2: Query the pool state via the data source using the computed resource attributes.
			// This explicitly checks that the data source engine hydrates and calculates active metrics
			// rather than returning a blank, unallocated /16 block.
			{
				Config: testAccPoolResourceConfigWithStrategy("192.168.0.0/16", "BEST", `
						management = {
								prefix_size     = 24
								reserve_sibling = false
						}
						appliances = {
								prefix_size     = 23
								reserve_sibling = true
						}
						pods = {
								prefix_size     = 20
								reserve_sibling = true
						}
						junk = {
								prefix_size     = 18
								reserve_sibling = false
						}
						wan = {
								prefix_size     = 24
								reserve_sibling = false
						}
				`) + `
						data "cidrblock_pool" "hydrated_check" {
								id   = cidrblock_pool.test.id
								cidr = cidrblock_pool.test.cidr
						}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Assert basic system identity parameters are cleanly parsed
					resource.TestCheckResourceAttr("data.cidrblock_pool.hydrated_check", "id", "test-org:test-project:test-network:192.168.0.0/16"),
					resource.TestCheckResourceAttr("data.cidrblock_pool.hydrated_check", "organization", "test-org"),
					resource.TestCheckResourceAttr("data.cidrblock_pool.hydrated_check", "project", "test-project"),
					resource.TestCheckResourceAttr("data.cidrblock_pool.hydrated_check", "network", "test-network"),
					resource.TestCheckResourceAttr("data.cidrblock_pool.hydrated_check", "allocation_strategy", "BEST"),

					// Assert calculations metrics match our exact mathematical targets:
					// Total pool size (/16) = 65536 IPs
					resource.TestCheckResourceAttr("data.cidrblock_pool.hydrated_check", "metrics.total_ips", "65536"),
					// Allocated base entries: /24(256) + /23(512) + /20(4096) + /18(16384) + /24(256) = 21504 IPs
					resource.TestCheckResourceAttr("data.cidrblock_pool.hydrated_check", "metrics.allocated_ips", "21504"),
					// Active shadow buddy blocks: /23(512) appliances companion + /20(4096) pods companion = 4608 IPs
					resource.TestCheckResourceAttr("data.cidrblock_pool.hydrated_check", "metrics.reserved_ips", "4608"),
					// Total remaining capacity: 65536 - (21504 + 4608) = 39424 IPs
					resource.TestCheckResourceAttr("data.cidrblock_pool.hydrated_check", "metrics.available_ips", "39424"),

					// Assert key allocations are exposed with accurate network values inside the map schema
					resource.TestCheckResourceAttrSet("data.cidrblock_pool.hydrated_check", "allocations.management.allocated_cidr"),
					resource.TestCheckResourceAttrSet("data.cidrblock_pool.hydrated_check", "allocations.pods.sibling_cidr"),
				),
			},
		},
	})
}

// TestAccPoolDataSource_ValidationErrors handles data source sad paths.
// It verifies that missing input states and corrupted structural lookups
// throw predictable, clean errors to user terminals rather than execution panics.
func TestAccPoolDataSource_ValidationErrors(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// 1. Force the short-circuit breakout by omitting the required ID token fields entirely
			{
				Config: `
						provider "cidrblock" {}
						data "cidrblock_pool" "empty_fault" {
								id   = "" 
								cidr = "10.0.0.0/24"
						}
				`,
				ExpectError: regexp.MustCompile("Missing Parameters"),
			},
			// 2. Force a parsing crash by feeding an invalid, split-proof identifier block
			{
				Config: `
						provider "cidrblock" {}
						data "cidrblock_pool" "malformed_id" {
								id   = "broken_string_no_colons"
								cidr = "10.0.0.0/24"
						}
				`,
				ExpectError: regexp.MustCompile("Invalid Pool ID"),
			},
		},
	})
}
