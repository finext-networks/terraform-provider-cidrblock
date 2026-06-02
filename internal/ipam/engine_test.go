// Copyright (c) HashiCorp and Contributors. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package ipam

import (
	"fmt"
	"testing"
)

func TestNewEngine(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		wantErr bool
	}{
		{"valid IPv4", "10.0.0.0/16", false},
		{"valid IPv6", "2001:db8::/32", false},
		{"host IPv4", "10.0.0.1/16", true},
		{"empty CIDR", "", true},
		{"invalid CIDR", "not-a-cidr", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, err := NewEngine(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewEngine(%q) error = %v, wantErr %v", tt.cidr, err, tt.wantErr)
			}
			if err == nil && eng == nil {
				t.Error("NewEngine returned nil engine with no error")
			}
		})
	}
}

func TestEngine_Allocate_FirstFit(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	alloc, err := eng.Allocate("subnet-a", 28, false)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if alloc != "10.0.0.0/28" {
		t.Errorf("First allocation = %s, want 10.0.0.0/28", alloc)
	}

	alloc, err = eng.Allocate("subnet-b", 28, false)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if alloc != "10.0.0.16/28" {
		t.Errorf("Second allocation = %s, want 10.0.0.16/28", alloc)
	}
}

func TestEngine_Allocate_DifferentPrefixSizes(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	alloc, err := eng.Allocate("large", 26, false)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if alloc != "10.0.0.0/26" {
		t.Errorf("Large allocation = %s, want 10.0.0.0/26", alloc)
	}

	alloc, err = eng.Allocate("small", 28, false)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if alloc != "10.0.0.64/28" {
		t.Errorf("Small allocation = %s, want 10.0.0.64/28", alloc)
	}
}

func TestEngine_Allocate_PrefixLargerThanPool(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	alloc1, err := eng.Allocate("half1", 25, false)
	if err != nil {
		t.Fatalf("Allocate half1 failed: %v", err)
	}
	if alloc1 != "10.0.0.0/25" {
		t.Errorf("half1 = %s, want 10.0.0.0/25", alloc1)
	}

	alloc2, err := eng.Allocate("half2", 25, false)
	if err != nil {
		t.Fatalf("Allocate half2 failed: %v", err)
	}
	if alloc2 != "10.0.0.128/25" {
		t.Errorf("half2 = %s, want 10.0.0.128/25", alloc2)
	}

	_, err = eng.Allocate("half3", 25, false)
	if err != ErrPoolExhausted {
		t.Errorf("Third allocation error = %v, want ErrPoolExhausted", err)
	}
}

func TestEngine_Allocate_ExactFit(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	alloc, err := eng.Allocate("full", 24, false)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if alloc != "10.0.0.0/24" {
		t.Errorf("Full allocation = %s, want 10.0.0.0/24", alloc)
	}

	_, err = eng.Allocate("extra", 28, false)
	if err != ErrPoolExhausted {
		t.Errorf("Extra allocation error = %v, want ErrPoolExhausted", err)
	}
}

func TestEngine_FreeAndRefill(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, _ = eng.Allocate("subnet-a", 28, false)
	_, _ = eng.Allocate("subnet-b", 28, false)
	_, _ = eng.Allocate("subnet-c", 28, false)

	err = eng.Free("subnet-b")
	if err != nil {
		t.Fatalf("Free failed: %v", err)
	}

	alloc, err := eng.Allocate("subnet-d", 28, false)
	if err != nil {
		t.Fatalf("Allocate after free failed: %v", err)
	}
	if alloc != "10.0.0.16/28" {
		t.Errorf("Gap fill allocation = %s, want 10.0.0.16/28", alloc)
	}
}

func TestEngine_Free_NonexistentKey(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	err = eng.Free("nonexistent")
	if err != ErrAllocationNotFound {
		t.Errorf("Free nonexistent error = %v, want ErrAllocationNotFound", err)
	}
}

func TestEngine_ReserveSibling(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	// Allocate with forward sibling reservation
	alloc, err := eng.Allocate("subnet-a", 28, true)
	if err != nil {
		t.Fatalf("Allocate with reserve failed: %v", err)
	}
	if alloc != "10.0.0.0/28" {
		t.Errorf("Allocation = %s, want 10.0.0.0/28", alloc)
	}

	// Next allocation skips the forward reserved sibling block (10.0.0.16/28)
	alloc, err = eng.Allocate("subnet-b", 28, false)
	if err != nil {
		t.Fatalf("Second allocate failed: %v", err)
	}
	if alloc != "10.0.0.32/28" {
		t.Errorf("Second allocation = %s, want 10.0.0.32/28", alloc)
	}
}

func TestEngine_SiblingCalculation(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, err = eng.Allocate("a", 28, true)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	state := eng.GetState()
	allocA := state["a"]
	if allocA.AllocatedCIDR != "10.0.0.0/28" {
		t.Errorf("Allocated = %s, want 10.0.0.0/28", allocA.AllocatedCIDR)
	}
	if allocA.SiblingCIDR != "10.0.0.16/28" {
		t.Errorf("Sibling = %s, want 10.0.0.16/28", allocA.SiblingCIDR)
	}
}

func TestEngine_AvailableSlices(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	slices := eng.AvailableSlices()
	if len(slices) != 1 {
		t.Fatalf("Expected 1 available slice, got %d", len(slices))
	}
	if slices[0].StartCIDR != "10.0.0.0/24" {
		t.Errorf("Start = %s, want 10.0.0.0/24", slices[0].StartCIDR)
	}
	if slices[0].MaxPrefixSize != 24 {
		t.Errorf("MaxPrefixSize = %d, want 24", slices[0].MaxPrefixSize)
	}

	_, _ = eng.Allocate("subnet-a", 26, false)
	slices = eng.AvailableSlices()
	if len(slices) != 2 {
		t.Fatalf("Expected 2 aligned remaining slices after alloc, got %d", len(slices))
	}
	if slices[0].StartCIDR != "10.0.0.64/26" {
		t.Errorf("Start after alloc = %s, want 10.0.0.64/26", slices[0].StartCIDR)
	}
}

func TestEngine_AvailableSlices_Gaps(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, _ = eng.Allocate("start", 26, false)
	_, _ = eng.Allocate("middle", 26, false)

	slices := eng.AvailableSlices()
	if len(slices) != 1 {
		t.Fatalf("Expected 1 gap slice, got %d", len(slices))
	}
	if slices[0].StartCIDR != "10.0.0.128/25" {
		t.Errorf("Gap start = %s, want 10.0.0.128/25", slices[0].StartCIDR)
	}
}

func TestEngine_Metrics(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	metrics := eng.Metrics()
	if metrics.TotalIPs != 256 {
		t.Errorf("TotalIPs = %d, want 256", metrics.TotalIPs)
	}

	_, _ = eng.Allocate("a", 28, false)
	_, _ = eng.Allocate("b", 28, true)

	metrics = eng.Metrics()
	if metrics.AllocatedIPs != 32 {
		t.Errorf("AllocatedIPs = %d, want 32", metrics.AllocatedIPs)
	}
	if metrics.ReservedIPs != 16 {
		t.Errorf("ReservedIPs = %d, want 16", metrics.ReservedIPs)
	}
	if metrics.AvailableIPs != 208 {
		t.Errorf("AvailableIPs = %d, want 208", metrics.AvailableIPs)
	}
}

func TestEngine_IPv6(t *testing.T) {
	eng, err := NewEngine("2001:db8::/48")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	alloc, err := eng.Allocate("v6-a", 64, false)
	if err != nil {
		t.Fatalf("IPv6 Allocate failed: %v", err)
	}
	if alloc != "2001:db8::/64" {
		t.Errorf("IPv6 allocation = %s, want 2001:db8::/64", alloc)
	}

	alloc, err = eng.Allocate("v6-b", 64, false)
	if err != nil {
		t.Fatalf("IPv6 Allocate failed: %v", err)
	}
	if alloc != "2001:db8:0:1::/64" {
		t.Errorf("IPv6 second allocation = %s, want 2001:db8:0:1::/64", alloc)
	}
}

func TestEngine_IPv6_Sibling(t *testing.T) {
	eng, err := NewEngine("2001:db8::/48")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, err = eng.Allocate("v6-a", 64, true)
	if err != nil {
		t.Fatalf("IPv6 Allocate with reserve failed: %v", err)
	}

	state := eng.GetState()
	allocA := state["v6-a"]
	if allocA.AllocatedCIDR != "2001:db8::/64" {
		t.Errorf("Allocated = %s, want 2001:db8::/64", allocA.AllocatedCIDR)
	}
	if allocA.SiblingCIDR != "2001:db8:0:1::/64" {
		t.Errorf("Sibling = %s, want 2001:db8:0:1::/64", allocA.SiblingCIDR)
	}
}

func TestEngine_UpdateAllocation(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, _ = eng.Allocate("subnet-a", 28, false)

	err = eng.UpdateAllocation("subnet-a", 28, true)
	if err != nil {
		t.Fatalf("UpdateAllocation failed: %v", err)
	}

	state := eng.GetState()
	allocA := state["subnet-a"]
	if allocA.SiblingCIDR != "10.0.0.16/28" {
		t.Errorf("Updated sibling = %s, want 10.0.0.16/28", allocA.SiblingCIDR)
	}
}

func TestEngine_UpdateAllocation_ChangePrefix(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, _ = eng.Allocate("subnet-a", 28, false)

	err = eng.UpdateAllocation("subnet-a", 27, false)
	if err != nil {
		t.Fatalf("UpdateAllocation prefix change failed: %v", err)
	}

	state := eng.GetState()
	allocA := state["subnet-a"]
	if allocA.AllocatedCIDR != "10.0.0.0/27" {
		t.Errorf("Updated CIDR = %s, want 10.0.0.0/27", allocA.AllocatedCIDR)
	}
}

func TestEngine_GetAllocations(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, _ = eng.Allocate("a", 28, false)
	_, _ = eng.Allocate("b", 28, false)

	state := eng.GetState()
	if len(state) != 2 {
		t.Errorf("State length = %d, want 2", len(state))
	}
}

func TestEngine_PoolCIDR(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	if eng.PoolCIDR() != "10.0.0.0/24" {
		t.Errorf("PoolCIDR = %s, want 10.0.0.0/24", eng.PoolCIDR())
	}
}

func TestEngine_SiblingOverlap(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, _ = eng.Allocate("a", 28, true)
	_, _ = eng.Allocate("b", 28, true)

	state := eng.GetState()
	allocB := state["b"]
	if allocB.AllocatedCIDR != "10.0.0.32/28" {
		t.Errorf("Second alloc = %s, want 10.0.0.32/28", allocB.AllocatedCIDR)
	}
	if allocB.SiblingCIDR != "10.0.0.48/28" {
		t.Errorf("Second sibling = %s, want 10.0.0.48/28", allocB.SiblingCIDR)
	}
}

func TestEngine_FillPool(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	// Forward-only sibling reservations mean a /24 pool can hold 8 pairs of /28 blocks
	for i := 0; i < 8; i++ {
		_, err := eng.Allocate(fmt.Sprintf("subnet-%d", i), 28, true)
		if err != nil {
			t.Fatalf("Allocation %d failed: %v", i, err)
		}
	}

	// The 9th allocation must fail because the pool space is entirely consumed
	_, err = eng.Allocate("extra", 28, false)
	if err != ErrPoolExhausted {
		t.Errorf("Extra allocation error = %v, want ErrPoolExhausted", err)
	}
}

func TestEngine_SiblingWithinPoolBounds(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	// Pre-fill up to the last pair slot index boundary (10.0.0.224/28)
	for i := 0; i < 7; i++ {
		_, _ = eng.Allocate(fmt.Sprintf("s%d", i), 28, true)
	}

	// This secures 10.0.0.224/28 and reserves the last remaining space in the pool (10.0.0.240/28)
	alloc, err := eng.Allocate("last_valid_pair", 28, true)
	if err != nil {
		t.Fatalf("Allocation failed: %v", err)
	}

	if alloc != "10.0.0.224/28" {
		t.Errorf("Allocated slot = %s, want 10.0.0.224/28", alloc)
	}

	// Trying to request a sibling reservation at the final slot (.240/28) must fail,
	// because its forward sibling (.256) falls outside the /24 supernet pool boundary.
	_, err = eng.Allocate("overflow_slot", 28, true)
	if err != ErrPoolExhausted {
		t.Errorf("Expected pool exhaustion error due to sibling overflow, got %v", err)
	}
}

func TestEngine_SiblingCalculationDirect(t *testing.T) {
	tests := []struct {
		name         string
		poolCIDR     string
		preAllocSize int
		prefixSize   int
		wantAlloc    string
		wantSibling  string
	}{
		{"IPv4 Lower Block Sibling", "10.0.0.0/24", 0, 28, "10.0.0.0/28", "10.0.0.16/28"},
		{"IPv4 Next Aligned Sibling", "10.0.0.0/24", 32, 28, "10.0.0.32/28", "10.0.0.48/28"},
		{"IPv6 Aligned Sibling", "2001:db8::/48", 0, 64, "2001:db8::/64", "2001:db8:0:1::/64"},
		{"Wide Subnet Sibling", "10.0.0.0/16", 0, 24, "10.0.0.0/24", "10.0.1.0/24"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, err := NewEngine(tt.poolCIDR)
			if err != nil {
				t.Fatalf("NewEngine failed: %v", err)
			}

			if tt.preAllocSize > 0 {
				_, _ = eng.Allocate("dummy-padding", tt.preAllocSize, false)
			}

			allocStr, err := eng.Allocate("test", tt.prefixSize, true)
			if err != nil {
				t.Fatalf("Allocate failed: %v", err)
			}

			if allocStr != tt.wantAlloc {
				t.Fatalf("Allocated %s, expected %s to test target logic", allocStr, tt.wantAlloc)
			}

			state := eng.GetState()
			alloc := state["test"]
			if alloc.SiblingCIDR != tt.wantSibling {
				t.Errorf("Sibling of %s = %s, want %s", allocStr, alloc.SiblingCIDR, tt.wantSibling)
			}
		})
	}
}

func TestEngine_PrefixValidation(t *testing.T) {
	tests := []struct {
		poolCIDR   string
		prefixSize int
		wantErr    error
	}{
		{"10.0.0.0/24", 28, nil},
		{"10.0.0.0/24", 24, nil},
		{"10.0.0.0/24", 32, nil},
		{"10.0.0.0/24", 25, nil},
		{"10.0.0.0/24", 33, ErrInvalidPrefix},
		{"2001:db8::/48", 64, nil},
		{"2001:db8::/48", 128, nil},
		{"2001:db8::/48", 129, ErrInvalidPrefix},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s/%d", tt.poolCIDR, tt.prefixSize), func(t *testing.T) {
			eng, err := NewEngine(tt.poolCIDR)
			if err != nil {
				t.Fatalf("NewEngine failed: %v", err)
			}

			_, err = eng.Allocate("test", tt.prefixSize, false)
			if err != tt.wantErr {
				t.Errorf("Error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestEngine_DuplicateKey(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, _ = eng.Allocate("subnet-a", 28, false)
	_, err = eng.Allocate("subnet-a", 28, false)
	if err != ErrDuplicateKey {
		t.Errorf("Duplicate key error = %v, want ErrDuplicateKey", err)
	}
}

func TestEngine_IPv4PrefixLimits(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, err = eng.Allocate("host", 32, false)
	if err != nil {
		t.Fatalf("/32 allocation failed: %v", err)
	}
}

func TestEngine_IPv6PrefixLimits(t *testing.T) {
	eng, err := NewEngine("2001:db8::/48")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, err = eng.Allocate("host", 128, false)
	if err != nil {
		t.Fatalf("/128 allocation failed: %v", err)
	}
}

func TestEngine_MixedAllocationSizes(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, _ = eng.Allocate("large", 26, false)  // 10.0.0.0/26
	_, _ = eng.Allocate("medium", 28, false) // 10.0.0.64/28
	_, _ = eng.Allocate("tiny", 30, false)   // 10.0.0.80/30

	state := eng.GetState()
	if state["tiny"].AllocatedCIDR != "10.0.0.80/30" {
		t.Errorf("Tiny alloc = %s, want 10.0.0.80/30", state["tiny"].AllocatedCIDR)
	}

	_ = eng.Free("medium")
	alloc, err := eng.Allocate("new", 27, false)
	if err != nil {
		t.Fatalf("Allocation after free failed: %v", err)
	}
	if alloc != "10.0.0.96/27" {
		t.Errorf("New alloc = %s, want 10.0.0.96/27", alloc)
	}
}

func TestEngine_GetAllocation(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, _ = eng.Allocate("subnet-a", 28, true)

	alloc, err := eng.GetAllocation("subnet-a")
	if err != nil {
		t.Fatalf("GetAllocation failed: %v", err)
	}
	if alloc.AllocatedCIDR != "10.0.0.0/28" {
		t.Errorf("GetAllocation CIDR = %s, want 10.0.0.0/28", alloc.AllocatedCIDR)
	}

	_, err = eng.GetAllocation("nonexistent")
	if err != ErrAllocationNotFound {
		t.Errorf("GetAllocation nonexistent error = %v, want ErrAllocationNotFound", err)
	}
}

func TestEngine_IPv6CanonicalEquivalence(t *testing.T) {
	// Mathematically identical /48 pools using completely different text variations
	poolVariants := []string{
		"2001:db8::/48",
		"2001:0db8:0000:0000:0000:0000:0000:0000/48",
		"2001:DB8::/48",
	}

	for i, poolStr := range poolVariants {
		t.Run(fmt.Sprintf("PoolVariant_%d", i), func(t *testing.T) {
			eng, err := NewEngine(poolStr)
			if err != nil {
				t.Fatalf("Failed to parse pool format variant: %v", err)
			}

			// Allocate using compressed, zero-padded, and alternate notations
			_, err = eng.Allocate("subnet_compressed", 64, true)
			if err != nil {
				t.Fatalf("Failed allocation on compressed format: %v", err)
			}

			// Ensure that regardless of pool format layout inputs, 
			// the stored string key resolves to the exact same canonical netip output string
			state := eng.GetState()
			alloc := state["subnet_compressed"]
			if alloc.AllocatedCIDR != "2001:db8::/64" {
				t.Errorf("Expected canonical compressed output '2001:db8::/64', got '%s'", alloc.AllocatedCIDR)
			}
			if alloc.SiblingCIDR != "2001:db8:0:1::/64" {
				t.Errorf("Expected canonical compressed sibling '2001:db8:0:1::/64', got '%s'", alloc.SiblingCIDR)
			}
		})
	}
}

func TestEngine_PoolStifledBySiblingExhaustion(t *testing.T) {
	// Tight parent pool containing exactly 8 IP addresses
	eng, err := NewEngine("10.0.0.0/29")
	if err != nil {
		t.Fatalf("Failed to initialize engine: %v", err)
	}

	// Requesting a /30 allocation with a forward sibling reservation consumes all 8 IPs
	_, err = eng.Allocate("subnet_alpha", 30, true)
	if err != nil {
		t.Fatalf("Initial allocation failed: %v", err)
	}

	// Attempting to allocate any further space must fail immediately
	_, err = eng.Allocate("subnet_beta", 32, false)
	if err != ErrPoolExhausted {
		t.Errorf("Expected ErrPoolExhausted, got %v", err)
	}
}

func TestEngine_UnalignedReallocationJump(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("Failed to initialize engine: %v", err)
	}

	// Establish a fragmented layout manually using existing state injection
	eng.RegisterExistingAllocation("subnet_a", &Allocation{PrefixSize: 28, AllocatedCIDR: "10.0.0.0/28"})
	eng.RegisterExistingAllocation("subnet_b", &Allocation{PrefixSize: 28, AllocatedCIDR: "10.0.0.16/28"})
	eng.RegisterExistingAllocation("subnet_c", &Allocation{PrefixSize: 28, AllocatedCIDR: "10.0.0.32/28"})

	// Update subnet_b from a /28 to an expanded /27 footprint.
	// It cannot stay at .16 (unaligned for /27) and cannot use .32 (blocked by subnet_c).
	// It must shift forward to the next valid boundary: .64/27
	err = eng.UpdateAllocation("subnet_b", 27, false)
	if err != nil {
		t.Fatalf("Update allocation failed: %v", err)
	}

	state := eng.GetState()
	if state["subnet_b"].AllocatedCIDR != "10.0.0.64/27" {
		t.Errorf("Expected expanded subnet to land at 10.0.0.64/27, got %s", state["subnet_b"].AllocatedCIDR)
	}
}

func TestEngine_GapLeapingAndRefilling(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("Failed to initialize engine: %v", err)
	}

	// 1. Allocate subnet_1 with a sibling reservation
	_, _ = eng.Allocate("subnet_1", 28, true) // Takes .0/28, reserves .16/28

	// 2. Allocate subnet_2 with a sibling reservation (must leap past the .16 reservation)
	_, _ = eng.Allocate("subnet_2", 28, true) // Takes .32/28, reserves .48/28

	state := eng.GetState()
	if state["subnet_2"].AllocatedCIDR != "10.0.0.32/28" {
		t.Errorf("Expected subnet_2 to leap to .32, got %s", state["subnet_2"].AllocatedCIDR)
	}

	// 3. Free up the reservation slot by turning off the sibling flag on subnet_1
	_ = eng.UpdateAllocation("subnet_1", 28, false)

	// 4. Allocate a new subnet_3; it should drop directly into the newly freed gap at .16/28
	allocStr, err := eng.Allocate("subnet_3", 28, false)
	if err != nil {
		t.Fatalf("Failed to allocate subnet_3: %v", err)
	}
	if allocStr != "10.0.0.16/28" {
		t.Errorf("Expected subnet_3 to fill the gap at 10.0.0.16/28, got %s", allocStr)
	}
}

func TestEngine_ExtremePrefixSiblingMath(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("Failed to initialize engine: %v", err)
	}

	// 1. Verify single host (/32) forward sibling alignment logic
	alloc32, err := eng.Allocate("host_route", 32, true)
	if err != nil {
		t.Fatalf("Failed to allocate /32 host route: %v", err)
	}
	if alloc32 != "10.0.0.0/32" {
		t.Errorf("Expected /32 to secure .0, got %s", alloc32)
	}

	state := eng.GetState()
	if state["host_route"].SiblingCIDR != "10.0.0.1/32" {
		t.Errorf("Expected /32 forward sibling to be .1/32, got %s", state["host_route"].SiblingCIDR)
	}

	// 2. Verify point-to-point link (/31) alignment logic.
	// It must skip the reserved .1/32 slot and align on the next parent block boundary (.4)
	alloc31, err := eng.Allocate("p2p_link", 31, true)
	if err != nil {
		t.Fatalf("Failed to allocate /31 point-to-point link: %v", err)
	}
	if alloc31 != "10.0.0.4/31" {
		t.Errorf("Expected /31 link to skip reservation gaps and land on .4/31, got %s", alloc31)
	}
	
	state = eng.GetState()
	if state["p2p_link"].SiblingCIDR != "10.0.0.6/31" {
		t.Errorf("Expected /31 forward sibling to be .6/31, got %s", state["p2p_link"].SiblingCIDR)
	}
}

func TestEngine_IPv6ExtremePrefixSiblingMath(t *testing.T) {
	eng, err := NewEngine("2001:db8::/48")
	if err != nil {
		t.Fatalf("Failed to initialize engine: %v", err)
	}

	// Verify maximum IPv6 host route boundary limits (/128)
	alloc128, err := eng.Allocate("v6_host", 128, true)
	if err != nil {
		t.Fatalf("Failed to allocate IPv6 /128 route: %v", err)
	}
	if alloc128 != "2001:db8::/128" {
		t.Errorf("Expected /128 to claim baseline address, got %s", alloc128)
	}

	state := eng.GetState()
	if state["v6_host"].SiblingCIDR != "2001:db8::1/128" {
		t.Errorf("Expected /128 sibling to increment by 1 bit, got %s", state["v6_host"].SiblingCIDR)
	}
}


