// Copyright (c) HashiCorp and Contributors. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// testAccProtoV5ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV5ProviderFactories = map[string]func() (tftypes.ProviderServer, error){
	"cidrblock": providerserver.NewProtocol5WithError(New("test")()),
}

func testAccProtoV5ProviderFactoriesWithVersion(version string) map[string]func() (tftypes.ProviderServer, error) {
	return map[string]func() (tftypes.ProviderServer, error){
		"cidrblock": providerserver.NewProtocol5WithError(New(version)()),
	}
}

// TestAccPoolResource_Basic tests basic pool creation with allocations.
func TestAccPoolResource_Basic(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
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
					resource.TestCheckResourceAttr("cidrblock_pool.test", "id", "test-org:test-project:test-network"),
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
				ImportStateVerifyIgnore: []string{"allocations"},
			},
		},
	})
}

// TestAccPoolResource_AddAllocation tests adding a new allocation.
func TestAccPoolResource_AddAllocation(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
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
	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
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
	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
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
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_a.sibling_cidr", ""),
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
					resource.TestCheckResourceAttr("cidrblock_pool.test", "allocations.subnet_a.sibling_cidr", ""),
				),
			},
		},
	})
}

// TestAccPoolResource_GapFilling tests that freed allocations are refilled.
func TestAccPoolResource_GapFilling(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
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
	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
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

// TestAccPoolResource_Validation tests namespace validation.
func TestAccPoolResource_Validation(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
					subnet_a = {
						prefix_size     = 28
						reserve_sibling = false
					}
				`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "id", "test-org:test-project:test-network"),
				),
			},
		},
	})
}

// TestAccDataSource_Basic tests the data source.
func TestAccDataSource_Basic(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
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
						id   = "test-org:test-project:test-network"
						cidr = "10.0.0.0/24"
					}
				`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.cidrblock_pool.test", "id", "test-org:test-project:test-network"),
					resource.TestCheckResourceAttr("data.cidrblock_pool.test", "organization", "test-org"),
					resource.TestCheckResourceAttr("data.cidrblock_pool.test", "project", "test-project"),
					resource.TestCheckResourceAttr("data.cidrblock_pool.test", "network", "test-network"),
					resource.TestCheckResourceAttr("data.cidrblock_pool.test", "metrics.total_ips", "256"),
				),
			},
		},
	})
}

// TestAccPoolResource_Delete tests pool deletion.
func TestAccPoolResource_Delete(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV5ProviderFactories: testAccProtoV5ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPoolResourceConfig("10.0.0.0/24", `
					subnet_a = {
						prefix_size     = 28
						reserve_sibling = false
					}
				`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("cidrblock_pool.test", "id", "test-org:test-project:test-network"),
				),
			},
			{
				Config: "",
				Check:  resource.ComposeTestCheckFunc(),
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

// unused import guards
var _ = context.Background
