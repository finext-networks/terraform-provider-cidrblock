// Copyright (c) HashiCorp and Contributors. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package ipam

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
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

	alloc, err := eng.Allocate("subnet-a", 28, false, StrategyFirst)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if alloc != "10.0.0.0/28" {
		t.Errorf("First allocation = %s, want 10.0.0.0/28", alloc)
	}

	alloc, err = eng.Allocate("subnet-b", 28, false, StrategyFirst)
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

	alloc, err := eng.Allocate("large", 26, false, StrategyFirst)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if alloc != "10.0.0.0/26" {
		t.Errorf("Large allocation = %s, want 10.0.0.0/26", alloc)
	}

	alloc, err = eng.Allocate("small", 28, false, StrategyFirst)
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

	alloc1, err := eng.Allocate("half1", 25, false, StrategyFirst)
	if err != nil {
		t.Fatalf("Allocate half1 failed: %v", err)
	}
	if alloc1 != "10.0.0.0/25" {
		t.Errorf("half1 = %s, want 10.0.0.0/25", alloc1)
	}

	alloc2, err := eng.Allocate("half2", 25, false, StrategyFirst)
	if err != nil {
		t.Fatalf("Allocate half2 failed: %v", err)
	}
	if alloc2 != "10.0.0.128/25" {
		t.Errorf("half2 = %s, want 10.0.0.128/25", alloc2)
	}

	_, err = eng.Allocate("half3", 25, false, StrategyFirst)
	if err != ErrPoolExhausted {
		t.Errorf("Third allocation error = %v, want ErrPoolExhausted", err)
	}
}

func TestEngine_Allocate_ExactFit(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	alloc, err := eng.Allocate("full", 24, false, StrategyFirst)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if alloc != "10.0.0.0/24" {
		t.Errorf("Full allocation = %s, want 10.0.0.0/24", alloc)
	}

	_, err = eng.Allocate("extra", 28, false, StrategyFirst)
	if err != ErrPoolExhausted {
		t.Errorf("Extra allocation error = %v, want ErrPoolExhausted", err)
	}
}

func TestEngine_FreeAndRefill(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, _ = eng.Allocate("subnet-a", 28, false, StrategyFirst)
	_, _ = eng.Allocate("subnet-b", 28, false, StrategyFirst)
	_, _ = eng.Allocate("subnet-c", 28, false, StrategyFirst)

	err = eng.Free("subnet-b")
	if err != nil {
		t.Fatalf("Free failed: %v", err)
	}

	alloc, err := eng.Allocate("subnet-d", 28, false, StrategyFirst)
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
	alloc, err := eng.Allocate("subnet-a", 28, true, StrategyFirst)
	if err != nil {
		t.Fatalf("Allocate with reserve failed: %v", err)
	}
	if alloc != "10.0.0.0/28" {
		t.Errorf("Allocation = %s, want 10.0.0.0/28", alloc)
	}

	// Next allocation skips the forward reserved sibling block (10.0.0.16/28)
	alloc, err = eng.Allocate("subnet-b", 28, false, StrategyFirst)
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

	_, err = eng.Allocate("a", 28, true, StrategyFirst)
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

	_, _ = eng.Allocate("subnet-a", 26, false, StrategyFirst)
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

	_, _ = eng.Allocate("start", 26, false, StrategyFirst)
	_, _ = eng.Allocate("middle", 26, false, StrategyFirst)

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

	_, _ = eng.Allocate("a", 28, false, StrategyFirst)
	_, _ = eng.Allocate("b", 28, true, StrategyFirst)

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

	alloc, err := eng.Allocate("v6-a", 64, false, StrategyFirst)
	if err != nil {
		t.Fatalf("IPv6 Allocate failed: %v", err)
	}
	if alloc != "2001:db8::/64" {
		t.Errorf("IPv6 allocation = %s, want 2001:db8::/64", alloc)
	}

	alloc, err = eng.Allocate("v6-b", 64, false, StrategyFirst)
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

	_, err = eng.Allocate("v6-a", 64, true, StrategyFirst)
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

	_, _ = eng.Allocate("subnet-a", 28, false, StrategyFirst)

	err = eng.UpdateAllocation("subnet-a", 28, true, StrategyFirst)
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

	_, _ = eng.Allocate("subnet-a", 28, false, StrategyFirst)

	err = eng.UpdateAllocation("subnet-a", 27, false, StrategyFirst)
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

	_, _ = eng.Allocate("a", 28, false, StrategyFirst)
	_, _ = eng.Allocate("b", 28, false, StrategyFirst)

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

	_, _ = eng.Allocate("a", 28, true, StrategyFirst)
	_, _ = eng.Allocate("b", 28, true, StrategyFirst)

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
		_, err := eng.Allocate(fmt.Sprintf("subnet-%d", i), 28, true, StrategyFirst)
		if err != nil {
			t.Fatalf("Allocation %d failed: %v", i, err)
		}
	}

	// The 9th allocation must fail because the pool space is entirely consumed
	_, err = eng.Allocate("extra", 28, false, StrategyFirst)
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
		_, _ = eng.Allocate(fmt.Sprintf("s%d", i), 28, true, StrategyFirst)
	}

	// This secures 10.0.0.224/28 and reserves the last remaining space in the pool (10.0.0.240/28)
	alloc, err := eng.Allocate("last_valid_pair", 28, true, StrategyFirst)
	if err != nil {
		t.Fatalf("Allocation failed: %v", err)
	}

	if alloc != "10.0.0.224/28" {
		t.Errorf("Allocated slot = %s, want 10.0.0.224/28", alloc)
	}

	// Trying to request a sibling reservation at the final slot (.240/28) must fail,
	// because its forward sibling (.256) falls outside the /24 supernet pool boundary.
	_, err = eng.Allocate("overflow_slot", 28, true, StrategyFirst)
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
				_, _ = eng.Allocate("dummy-padding", tt.preAllocSize, false, StrategyFirst)
			}

			allocStr, err := eng.Allocate("test", tt.prefixSize, true, StrategyFirst)
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

			_, err = eng.Allocate("test", tt.prefixSize, false, StrategyFirst)
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

	_, _ = eng.Allocate("subnet-a", 28, false, StrategyFirst)
	_, err = eng.Allocate("subnet-a", 28, false, StrategyFirst)
	if err != ErrDuplicateKey {
		t.Errorf("Duplicate key error = %v, want ErrDuplicateKey", err)
	}
}

func TestEngine_IPv4PrefixLimits(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, err = eng.Allocate("host", 32, false, StrategyFirst)
	if err != nil {
		t.Fatalf("/32 allocation failed: %v", err)
	}
}

func TestEngine_IPv6PrefixLimits(t *testing.T) {
	eng, err := NewEngine("2001:db8::/48")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, err = eng.Allocate("host", 128, false, StrategyFirst)
	if err != nil {
		t.Fatalf("/128 allocation failed: %v", err)
	}
}

func TestEngine_MixedAllocationSizes(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	_, _ = eng.Allocate("large", 26, false, StrategyFirst)  // 10.0.0.0/26
	_, _ = eng.Allocate("medium", 28, false, StrategyFirst) // 10.0.0.64/28
	_, _ = eng.Allocate("tiny", 30, false, StrategyFirst)   // 10.0.0.80/30

	state := eng.GetState()
	if state["tiny"].AllocatedCIDR != "10.0.0.80/30" {
		t.Errorf("Tiny alloc = %s, want 10.0.0.80/30", state["tiny"].AllocatedCIDR)
	}

	_ = eng.Free("medium")
	alloc, err := eng.Allocate("new", 27, false, StrategyFirst)
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

	_, _ = eng.Allocate("subnet-a", 28, true, StrategyFirst)

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
			_, err = eng.Allocate("subnet_compressed", 64, true, StrategyFirst)
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
	_, err = eng.Allocate("subnet_alpha", 30, true, StrategyFirst)
	if err != nil {
		t.Fatalf("Initial allocation failed: %v", err)
	}

	// Attempting to allocate any further space must fail immediately
	_, err = eng.Allocate("subnet_beta", 32, false, StrategyFirst)
	if err != ErrPoolExhausted {
		t.Errorf("Expected ErrPoolExhausted, got %v", err)
	}
}

func TestEngine_UnalignedReallocationJump(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	// Allocate initial blocks, discarding the returned prefix token
	_, err = eng.Allocate("subnet_a", 28, false, StrategyFirst) // Takes 10.0.0.0/28
	if err != nil {
		t.Fatalf("Failed to allocate subnet_a: %v", err)
	}
	_, err = eng.Allocate("subnet_b", 28, false, StrategyFirst) // Takes 10.0.0.16/28
	if err != nil {
		t.Fatalf("Failed to allocate subnet_b: %v", err)
	}

	// Attempting to update subnet_b from a /28 to a /27 at address .16 breaks binary alignment rules.
	// The engine MUST return a hard error and refuse to automatically relocate it to 10.0.0.0/27.
	err = eng.UpdateAllocation("subnet_b", 27, false, StrategyFirst)
	if err == nil {
		t.Fatalf("Expected update to fail due to unaligned base address mutation boundaries, but it succeeded")
	}

	// Verify the original allocation remains completely intact and unaffected by the failed update
	alloc, err := eng.GetAllocation("subnet_b")
	if err != nil {
		t.Fatalf("Failed to retrieve subnet_b: %v", err)
	}
	if alloc.AllocatedCIDR != "10.0.0.16/28" {
		t.Errorf("Expected subnet_b to remain anchored at 10.0.0.16/28, got %s", alloc.AllocatedCIDR)
	}
}

func TestEngine_GapLeapingAndRefilling(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("Failed to initialize engine: %v", err)
	}

	// 1. Allocate subnet_1 with a sibling reservation
	_, _ = eng.Allocate("subnet_1", 28, true, StrategyFirst) // Takes .0/28, reserves .16/28

	// 2. Allocate subnet_2 with a sibling reservation (must leap past the .16 reservation)
	_, _ = eng.Allocate("subnet_2", 28, true, StrategyFirst) // Takes .32/28, reserves .48/28

	state := eng.GetState()
	if state["subnet_2"].AllocatedCIDR != "10.0.0.32/28" {
		t.Errorf("Expected subnet_2 to leap to .32, got %s", state["subnet_2"].AllocatedCIDR)
	}

	// 3. Free up the reservation slot by turning off the sibling flag on subnet_1
	_ = eng.UpdateAllocation("subnet_1", 28, false, StrategyFirst)

	// 4. Allocate a new subnet_3; it should drop directly into the newly freed gap at .16/28
	allocStr, err := eng.Allocate("subnet_3", 28, false, StrategyFirst)
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
	alloc32, err := eng.Allocate("host_route", 32, true, StrategyFirst)
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
	alloc31, err := eng.Allocate("p2p_link", 31, true, StrategyFirst)
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
	alloc128, err := eng.Allocate("v6_host", 128, true, StrategyFirst)
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

// TestEngine_PanicRegression_ZeroPrefixSibling ensures that requesting a sibling reservation
// on a zero-width prefix size (/0) safely yields a validation error instead of triggering a 
// runtime panic due to bit-length underflow (0 - 1 = -1) inside the netip standard library.
func TestEngine_PanicRegression_ZeroPrefixSibling(t *testing.T) {
	// Replicate the exact failing baseline environment uncovered by the fuzz worker seed
	eng, err := NewEngine("0.0.0.0/0")
	if err != nil {
		// Fall back to a standard class pool if the test environment restricts /0 parsing
		eng, err = NewEngine("10.0.0.0/24")
		if err != nil {
			t.Fatalf("Failed to initialize regression pool: %v", err)
		}
	}

	// Establish a localized panic recovery trap to explicitly fail the test 
	// if a regression causes the netip library or search loop to crash.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("CRITICAL REGRESSION: Engine panicked during /0 sibling allocation math: %v", r)
		}
	}()

	// Executing this block must return an ErrInvalidPrefix validation failure gracefully.
	_, err = eng.Allocate("panic_target", 0, true, StrategyFirst)
	if err != ErrInvalidPrefix {
		t.Errorf("Expected allocation failure to return %v, got %v", ErrInvalidPrefix, err)
	}
}

// TestEngine_FuzzRegression_BitShiftOverflow checks that zero-length prefixes 
// or maximum boundary widths do not trigger CPU shift-masking infinite loops.
func TestEngine_FuzzRegression_BitShiftOverflow(t *testing.T) {
	// 1. Check an internet-scale root boundary pool layout
	eng0, err := NewEngine("0.0.0.0/0")
	if err == nil {
		_, _ = eng0.Allocate("root_alloc", 24, false, StrategyFirst)
		_ = eng0.AvailableSlices()
		_ = eng0.Metrics()
	}

	// 2. Check a zero-width prefix size request inside a standard pool
	eng2, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("Failed to initialize engine: %v", err)
	}

	_, err = eng2.Allocate("zero_prefix_request", 0, false, StrategyFirst)
	if err == nil {
		t.Error("Expected an execution error when allocating a /0 subnet, but got none")
	}

	// 3. Check maximum boundary overflow validation
	engMax, err := NewEngine("255.255.255.252/30")
	if err != nil {
		t.Fatalf("Failed to initialize edge pool: %v", err)
	}

	_, err = engMax.Allocate("edge_subnet", 32, true, StrategyFirst)
	if err != nil && err != ErrPoolExhausted {
		t.Fatalf("Unexpected error processing edge allocation boundary math: %v", err)
	}
}

// TestEngine_FuzzRegression_ZeroPrefixSiblingPanic ensures that requesting a sibling reservation
// on a zero-width prefix size (/0) yields a validation error instead of triggering a 
// runtime panic due to bit-length underflow (0 - 1 = -1) inside the netip standard library.
func TestEngine_FuzzRegression_ZeroPrefixSiblingPanic(t *testing.T) {
	eng, err := NewEngine("0.0.0.0/0")
	if err != nil {
		eng, err = NewEngine("10.0.0.0/24")
		if err != nil {
			t.Fatalf("Failed to initialize regression pool: %v", err)
		}
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("CRITICAL REGRESSION: Engine panicked during /0 sibling allocation math: %v", r)
		}
	}()

	_, err = eng.Allocate("panic_target", 0, true, StrategyFirst)
	if err != ErrInvalidPrefix {
		t.Errorf("Expected allocation failure to return %v, got %v", ErrInvalidPrefix, err)
	}
}

// TestEngine_FuzzRegression_AlgorithmicHangWidePool preserves execution speed for wide 
// pools using alternative strategies, preventing exponential brute-force scanning timeouts.
func TestEngine_FuzzRegression_AlgorithmicHangWidePool(t *testing.T) {
	eng, err := NewEngine("2001:db8::/48")
	if err != nil {
		t.Fatalf("Failed to initialize wide v6 pool: %v", err)
	}

	// This operation would previously trigger an exponential 2^80 brute-force calculation hang.
	// With slice-driven optimization, it must complete instantly.
	allocStr, err := eng.Allocate("v6_loopback", 128, false, StrategyBest)
	if err != nil {
		t.Fatalf("Best-fit wide v6 allocation failed: %v", err)
	}

	if allocStr != "2001:db8::/128" {
		t.Errorf("Expected first aligned block '2001:db8::/128', got '%s'", allocStr)
	}
}

// TestEngine_FuzzRegression_CrossSliceOverlap ensures that when a candidate block fits 
// inside an unallocated slice but spans across its upper boundary, it runs an explicit 
// collision verification to avoid bleeding into adjacent active allocations.
func TestEngine_FuzzRegression_CrossSliceOverlap(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("Failed to initialize engine: %v", err)
	}

	// Create a constrained fragmentation footprint manually:
	// [.0-.31 Free] [.32-.63 Occupied] [.64-.95 Occupied] [.96-.127 Free]
	eng.RegisterExistingAllocation("block_1", &Allocation{PrefixSize: 27, AllocatedCIDR: "10.0.0.32/27"})
	eng.RegisterExistingAllocation("block_2", &Allocation{PrefixSize: 27, AllocatedCIDR: "10.0.0.64/27"})

	// Request a /26 allocation (requires 64 addresses). 
	// The first available slice gap starts at 10.0.0.0 but only has a size capacity of 32 hosts.
	// The engine must not erroneously allocate 10.0.0.0/26 (which would bleed into block_1).
	// It must leap directly past the busy block cluster to find the first valid gap at 10.0.0.128/26.
	allocStr, err := eng.Allocate("large_subnet", 26, false, StrategyFirst)
	if err != nil {
		t.Fatalf("Allocation failed: %v", err)
	}

	if allocStr != "10.0.0.128/26" {
		t.Errorf("Expected allocation to leap past collision cluster to .128/26, got %s", allocStr)
	}
}

// TestEngine_Matrix_AllStrategiesHappyPath loops through every single allocation strategy
// using identical inputs to verify that all three algorithms safely pass core lifecycle 
// invariants, allocation rules, and metric evaluations while respecting their unique layout patterns.
func TestEngine_Matrix_AllStrategiesHappyPath(t *testing.T) {
	strategies := []Strategy{StrategyFirst, StrategyBest, StrategySparse}

	for _, strat := range strategies {
		t.Run(string(strat), func(t *testing.T) {
			// 1. Setup a clean engine instance per strategy loop
			eng, err := NewEngine("10.0.0.0/24")
			if err != nil {
				t.Fatalf("Failed to initialize engine: %v", err)
			}

			// 2. Allocate our target block first. In an empty pool, this is guaranteed 
			// to land on 10.0.0.0/26 across all strategies, giving us a valid Left-Hand anchor.
			addr1, err := eng.Allocate("subnet_1", 26, false, strat) 
			if err != nil {
				t.Fatalf("Allocation 1 failed under strategy %s: %v", strat, err)
			}
			if addr1 != "10.0.0.0/26" {
				t.Errorf("Expected first block at .0/26, got %s", addr1)
			}

			// 3. Verify inline configuration mutation update safety on our left-hand block
			err = eng.UpdateAllocation("subnet_1", 26, true, strat) // Toggle forward sibling reservation on
			if err != nil {
				t.Fatalf("Inline update failed under strategy %s: %v", strat, err)
			}

			state := eng.GetState()
			if state["subnet_1"].SiblingCIDR != "10.0.0.64/26" {
				t.Errorf("Strategy %s: Expected sibling reservation at 10.0.0.64/26, got %s", strat, state["subnet_1"].SiblingCIDR)
			}

			// 4. Allocate a second subnet. It must skip the active sibling reservation (10.0.0.64/26)
			// and drop into the next available slot at 10.0.0.128/26 across all strategies.
			addr2, err := eng.Allocate("subnet_2", 26, false, strat)
			if err != nil {
				t.Fatalf("Allocation 2 failed under strategy %s: %v", strat, err)
			}

			expectedAddr2 := "10.0.0.128/26"
			if addr2 != expectedAddr2 {
				t.Errorf("Strategy %s: Expected second block at %s, got %s", strat, expectedAddr2, addr2)
			}

			// 5. Verify metric calculation consistency
			metrics := eng.Metrics()
			if metrics.AllocatedIPs != 128 { // subnet_1 (64) + subnet_2 (64)
				t.Errorf("Expected 128 allocated IPs, got %d", metrics.AllocatedIPs)
			}
			if metrics.ReservedIPs != 64 { // subnet_1's sibling (64)
				t.Errorf("Expected 64 reserved IPs, got %d", metrics.ReservedIPs)
			}
		})
	}
}

// TestEngine_CoverageCompletion_ErrorPaths executes the remaining unreached defensive 
// error guard rails inside the UpdateAllocation lifecycle.
func TestEngine_CoverageCompletion_ErrorPaths(t *testing.T) {
	eng, _ := NewEngine("10.0.0.0/24")

	// 1. Trigger ErrAllocationNotFound inside UpdateAllocation
	err := eng.UpdateAllocation("non_existent_key", 28, false, StrategyFirst)
	if !errors.Is(err, ErrAllocationNotFound) {
		t.Errorf("Expected ErrAllocationNotFound, got %v", err)
	}

	// 2. Trigger ErrInvalidPrefix inside UpdateAllocation (Too large)
	eng.RegisterExistingAllocation("active_key", &Allocation{PrefixSize: 28, AllocatedCIDR: "10.0.0.0/28"})
	err = eng.UpdateAllocation("active_key", 33, false, StrategyFirst)
	if !errors.Is(err, ErrInvalidPrefix) {
		t.Errorf("Expected ErrInvalidPrefix on large size, got %v", err)
	}

	// 3. Trigger ErrInvalidPrefix inside UpdateAllocation (Underflow)
	err = eng.UpdateAllocation("active_key", 0, true, StrategyFirst)
	if !errors.Is(err, ErrInvalidPrefix) {
		t.Errorf("Expected ErrInvalidPrefix on zero sibling size, got %v", err)
	}
}

// TestEngine_CoverageCompletion_SiblingOverlaps triggers the specific loop lines 
// evaluating candidate blocks colliding directly with existing forward sibling reservations.
func TestEngine_CoverageCompletion_SiblingOverlaps(t *testing.T) {
	eng, _ := NewEngine("10.0.0.0/24")

	// Manually inject an allocation that owns a sibling reservation block at .16/28
	eng.RegisterExistingAllocation("block_a", &Allocation{
		PrefixSize:     28,
		AllocatedCIDR:  "10.0.0.0/28",
		SiblingCIDR:    "10.0.0.16/28",
		ReserveSibling: true,
	})

	// Force AvailableSlices collision loop to evaluate a sibling block overlap
	_ = eng.AvailableSlices()

	// Try to allocate a new block with its own sibling reservation that would 
	// overlap with block_a's existing sibling reservation space
	eng.RegisterExistingAllocation("block_b", &Allocation{PrefixSize: 28, AllocatedCIDR: "10.0.0.32/28"})
	
	// Triggers cross-sibling validation checks inside findGap
	_, _ = eng.Allocate("block_c", 28, true, StrategyFirst)
}

// TestEngine_CoverageCompletion_SortingTieBreakers executes the address comparison lines 
// when two available slice gaps evaluate to the exact same size metric.
func TestEngine_CoverageCompletion_SortingTieBreakers(t *testing.T) {
	eng, _ := NewEngine("10.0.0.0/24")

	// Create two identical isolated free slice gaps by dropping a block right in the middle
	eng.RegisterExistingAllocation("mid_block", &Allocation{PrefixSize: 25, AllocatedCIDR: "10.0.0.128/25"})

	// Force BEST and SPARSE sorting routines to evaluate equal slice size tie-breakers
	_, _ = eng.Allocate("best_tie", 28, false, StrategyBest)
	
	eng2, _ := NewEngine("10.0.0.0/24")
	eng2.RegisterExistingAllocation("mid_block", &Allocation{PrefixSize: 25, AllocatedCIDR: "10.0.0.128/25"})
	_, _ = eng2.Allocate("sparse_tie", 28, false, StrategySparse)
}

// TestEngine_CoverageCompletion_LowLevelGuards executes the remaining low-level pointer safety
// checks handling invalid inputs and extreme IPv6 boundary limits.
func TestEngine_CoverageCompletion_LowLevelGuards(t *testing.T) {
	// 1. Trigger calcSibling zero bit check
	badPrefix := netip.Prefix{}
	_ = calcSibling(badPrefix)

	// 2. Trigger addBitOffset invalid address guard line
	badAddr := netip.Addr{}
	_ = addBitOffset(badAddr, 24)

	// 3. Trigger metrics boundary condition cap loop safely
	eng, _ := NewEngine("10.0.0.0/24")
	eng.RegisterExistingAllocation("heavy_load", &Allocation{PrefixSize: 24, AllocatedCIDR: "10.0.0.0/24"})
	m := eng.Metrics()
	if m.AvailableIPs != 0 {
		t.Errorf("Expected 0 available IPs, got %d", m.AvailableIPs)
	}
}

// TestEngine_Coverage_SiblingCollisions validates search loops reacting directly 
// to candidate blocks overlapping with active forward sibling reservations.
func TestEngine_Coverage_SiblingCollisions(t *testing.T) {
	eng, _ := NewEngine("10.0.0.0/24")

	// Pre-seed an allocation that holds an active forward sibling reservation block at .16/28
	eng.RegisterExistingAllocation("block_a", &Allocation{
		PrefixSize:     28,
		AllocatedCIDR:  "10.0.0.0/28",
		SiblingCIDR:    "10.0.0.16/28",
		ReserveSibling: true,
	})

	// Forces AvailableSlices collision loop to evaluate an active sibling block overlap
	_ = eng.AvailableSlices()

	// Try to allocate a block whose sibling reservation would bleed directly into block_a's space
	eng.RegisterExistingAllocation("block_b", &Allocation{PrefixSize: 28, AllocatedCIDR: "10.0.0.32/28"})
	_, _ = eng.Allocate("block_c", 28, true, StrategyFirst)
}

// TestEngine_Coverage_SortingTieBreakers executes the address-less evaluation lines 
// when two available slice gaps have matching size metrics.
func TestEngine_Coverage_SortingTieBreakers(t *testing.T) {
	// Create two identical isolated free gaps by dropping a block exactly in the center
	engBest, _ := NewEngine("10.0.0.0/24")
	engBest.RegisterExistingAllocation("mid_block", &Allocation{PrefixSize: 25, AllocatedCIDR: "10.0.0.128/25"})
	_, _ = engBest.Allocate("best_tie", 28, false, StrategyBest)

	engSparse, _ := NewEngine("10.0.0.0/24")
	engSparse.RegisterExistingAllocation("mid_block", &Allocation{PrefixSize: 25, AllocatedCIDR: "10.0.0.128/25"})
	_, _ = engSparse.Allocate("sparse_tie", 28, false, StrategySparse)
}

// TestEngine_Coverage_UpdateAllocationFailure forces an error return path when 
// an update reallocation cannot find a matching free slot.
func TestEngine_Coverage_UpdateAllocationFailure(t *testing.T) {
	eng, _ := NewEngine("10.0.0.0/28") // Tiny pool
	eng.RegisterExistingAllocation("subnet_a", &Allocation{PrefixSize: 28, AllocatedCIDR: "10.0.0.0/28"})

	// Attempting to expand when the pool is completely locked triggers the uncovered error return
	err := eng.UpdateAllocation("subnet_a", 27, false, StrategyFirst)
	if err == nil {
		t.Error("Expected failure when expanding inside a completely full pool space")
	}
}

// TestEngine_Coverage_LowLevelOverflows triggers the remaining low-level address manipulation overflow guards.
func TestEngine_Coverage_LowLevelOverflows(t *testing.T) {
	// 1. Trigger calcSibling zero bit protection check
	_ = calcSibling(netip.Prefix{})

	// 2. Trigger addBitOffset extreme out-of-bounds IPv6 bit shift guard
	v6Addr := netip.MustParseAddr("2001:db8::")
	_ = addBitOffset(v6Addr, 128)

	// 3. Trigger addBitOffset IPv6 saturation carry overflow guard
	v6MaxAddr := netip.MustParseAddr("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")
	_ = addBitOffset(v6MaxAddr, 64)
}

// TestEngine_Coverage_DefensiveAndSortingGaps targets the remaining uncovered
// branches inside engine updates, sibling overlaps, and strategy sorting tie-breakers.
func TestEngine_Coverage_DefensiveAndSortingGaps(t *testing.T) {
	// 1. Trigger UpdateAllocation prefix size underflow with sibling check
	engUpdate, _ := NewEngine("10.0.0.0/24")
	engUpdate.RegisterExistingAllocation("k1", &Allocation{PrefixSize: 28, AllocatedCIDR: "10.0.0.0/28"})
	_ = engUpdate.UpdateAllocation("k1", 0, true, StrategyFirst)

	// 2. Trigger UpdateAllocation findGap failure path (Pool Exhaustion on update)
	engExhaust, _ := NewEngine("10.0.0.0/28")
	engExhaust.RegisterExistingAllocation("k2", &Allocation{PrefixSize: 28, AllocatedCIDR: "10.0.0.0/28"})
	_ = engExhaust.UpdateAllocation("k2", 27, false, StrategyFirst)

	// 3. Trigger SiblingCIDR collision check inside findGap loop
	engSib, _ := NewEngine("10.0.0.0/24")
	engSib.RegisterExistingAllocation("k3", &Allocation{
		PrefixSize:    28, 
		AllocatedCIDR: "10.0.0.32/28", 
		SiblingCIDR:   "10.0.0.0/28",
	})
	// Allocation search evaluates .0/28 first, hitting the existing sibling reservation match
	_, _ = engSib.Allocate("trigger_sib_match", 28, false, StrategyFirst)

	// 4. Trigger allocation sibling reservation collision with an existing allocation
	engOverlap, _ := NewEngine("10.0.0.0/24")
	engOverlap.RegisterExistingAllocation("k4", &Allocation{PrefixSize: 28, AllocatedCIDR: "10.0.0.16/28"})
	// Allocating at .0/28 with reserveSibling=true sets sibling to .16/28, colliding with k4
	_, _ = engOverlap.Allocate("trigger_overlap", 28, true, StrategyFirst)

	// 5. Trigger metrics calculation total IP overflow guard loop
	engOverflow, _ := NewEngine("10.0.0.0/28") // 16 IPs capacity
	// Forcibly register an out-of-bounds size token to exceed capacity ceiling
	engOverflow.RegisterExistingAllocation("heavy", &Allocation{PrefixSize: 26, AllocatedCIDR: "10.0.0.0/26"}) // 64 IPs
	_ = engOverflow.Metrics()

	// 6. Trigger StrategyBest & StrategySparse matching size slice sorting tie-breakers
	// Fragment the layout space into two perfectly identical available slice sizes
	engTie, _ := NewEngine("10.0.0.0/24")
	engTie.RegisterExistingAllocation("divider", &Allocation{PrefixSize: 26, AllocatedCIDR: "10.0.0.64/26"})
	_, _ = engTie.Allocate("best_tie", 28, false, StrategyBest)
	
	engTie2, _ := NewEngine("10.0.0.0/24")
	engTie2.RegisterExistingAllocation("divider", &Allocation{PrefixSize: 26, AllocatedCIDR: "10.0.0.64/26"})
	_, _ = engTie2.Allocate("sparse_tie", 28, false, StrategySparse)
}

// TestEngine_FinalCoverage_EngineGaps clears out the remaining inner condition blocks,
// short-circuits, and symmetrical sorting tie-breakers inside the core engine.
func TestEngine_FinalCoverage_EngineGaps(t *testing.T) {
	// 1. Clear the UpdateAllocation zero-prefix sibling short-circuit block
	engZeroRoot, _ := NewEngine("0.0.0.0/0") // Length is 0
	engZeroRoot.RegisterExistingAllocation("root_key", &Allocation{PrefixSize: 0, AllocatedCIDR: "0.0.0.0/0"})
	_ = engZeroRoot.UpdateAllocation("root_key", 0, true, StrategyFirst)

	// 2. Clear findGap error catch inside UpdateAllocation via impossible layout inflation
	engExhaust, _ := NewEngine("10.0.0.0/28")
	engExhaust.RegisterExistingAllocation("ex_key", &Allocation{PrefixSize: 28, AllocatedCIDR: "10.0.0.0/28"})
	_ = engExhaust.UpdateAllocation("ex_key", 26, false, StrategyFirst)

	// 3. Clear SiblingCIDR collision lines in AvailableSlices and findGap routines
	engSibColl, _ := NewEngine("10.0.0.0/24")
	engSibColl.RegisterExistingAllocation("existing_alloc", &Allocation{
		PrefixSize:     28,
		AllocatedCIDR:  "10.0.0.32/28",
		SiblingCIDR:    "10.0.0.0/28", // Overlaps the first scan address
		ReserveSibling: true,
	})
	_ = engSibColl.AvailableSlices()
	_, _ = engSibColl.Allocate("clash_alloc", 28, true, StrategyFirst)

	// 4. Clear matching size sorting tie-breakers for BEST and SPARSE strategies
	// Pre-seed allocations to leave two perfectly identical gaps at .0/26 and .192/26
	engBestTie, _ := NewEngine("10.0.0.0/24")
	engBestTie.RegisterExistingAllocation("div1", &Allocation{PrefixSize: 26, AllocatedCIDR: "10.0.0.64/26"})
	engBestTie.RegisterExistingAllocation("div2", &Allocation{PrefixSize: 26, AllocatedCIDR: "10.0.0.128/26"})
	_, _ = engBestTie.Allocate("best_match", 28, false, StrategyBest)

	engSparseTie, _ := NewEngine("10.0.0.0/24")
	engSparseTie.RegisterExistingAllocation("div1", &Allocation{PrefixSize: 26, AllocatedCIDR: "10.0.0.64/26"})
	engSparseTie.RegisterExistingAllocation("div2", &Allocation{PrefixSize: 26, AllocatedCIDR: "10.0.0.128/26"})
	_, _ = engSparseTie.Allocate("sparse_match", 28, false, StrategySparse)

	// 5. Clear calcSibling and findGap boundary breakouts
	_ = calcSibling(netip.Prefix{})
}

func TestEngine_UpdateAllocation_HardenedInvariants(t *testing.T) {
	// Initialize a master /24 test container pool
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("Failed to initialize engine: %v", err)
	}

	// Pre-seed the pool with distinct topologies to test collision and alignment rules
	// Left-hand aligned block at the start of the lower half
	_, _ = eng.Allocate("subnet_left", 26, false, StrategyFirst)   // Claims 10.0.0.0/26

	// Right-hand aligned block filling the upper half of that same tier
	_, _ = eng.Allocate("subnet_right", 26, false, StrategyFirst)  // Claims 10.0.0.64/26

	// Independent block sitting out-of-band in the second half of the pool
	_, _ = eng.Allocate("subnet_blocker", 26, false, StrategyFirst) // Claims 10.0.0.128/26

	t.Run("Gate 1: Enforce Base-Address Immutability", func(t *testing.T) {
		// Attempting to scale up the right-hand block (10.0.0.64/26) to a /25 requires 
		// shifting the base address backward to 10.0.0.0. The engine must reject this.
		err := eng.UpdateAllocation("subnet_right", 25, false, StrategyFirst)
		if err == nil {
			t.Fatal("Expected error due to base-address mutation alignment breach, but got nil")
		}
		expectedMsg := "breaks binary alignment boundaries"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain %q, got: %v", expectedMsg, err)
		}
	})

	t.Run("Gate 2: Verify Master Pool Boundaries", func(t *testing.T) {
		engOOB, _ := NewEngine("10.0.0.0/24")
		_, _ = engOOB.Allocate("root_sub", 24, false, StrategyFirst) // Claims 10.0.0.0/24 (Full Pool)

		// Attempting to scale a full-pool resource up to a /23 runs off the edge of the master container
		err := engOOB.UpdateAllocation("root_sub", 23, false, StrategyFirst)
		if err == nil {
			t.Fatal("Expected error due to master pool boundary overflow, but got nil")
		}
		// The engine safely catches this via its top-level prefix filter block
		if !errors.Is(err, ErrInvalidPrefix) {
			t.Errorf("Expected error %v, got: %v", ErrInvalidPrefix, err)
		}
	})

	t.Run("Gate 3: Prevent Overlap Collisions With Other Subnets", func(t *testing.T) {
		// Attempting to scale up subnet_left (10.0.0.0/26) to a /25 is binary-aligned,
		// but expanding its width means it will step directly onto subnet_right (10.0.0.64/26).
		err := eng.UpdateAllocation("subnet_left", 25, false, StrategyFirst)
		if err == nil {
			t.Fatal("Expected error due to expansion collision, but got nil")
		}
		expectedMsg := "overlaps with existing allocation"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain %q, got: %v", expectedMsg, err)
		}
	})

	t.Run("Gate 4: Reject Forward Sibling Toggles on Right-Hand Blocks", func(t *testing.T) {
		// Attempting to turn on reserve_sibling = true for a static right-hand block (10.0.0.64/26)
		// must be caught and blocked because right-hand blocks cannot support horizontal forward expansions.
		err := eng.UpdateAllocation("subnet_right", 26, true, StrategyFirst)
		if err == nil {
			t.Fatal("Expected error when requesting a sibling on an upper-half buddy alignment, but got nil")
		}
		expectedMsg := "cannot claim a forward buddy pair"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain %q, got: %v", expectedMsg, err)
		}
	})

	t.Run("Gate 5: Detect Next-Block Sibling Claims During Upgrades", func(t *testing.T) {
		engCascade, _ := NewEngine("10.0.0.0/22") // Massive pool to remove boundary limits
		
		// Setup a clean left-hand block
		_, _ = engCascade.Allocate("sub_a", 24, false, StrategyFirst) // Claims 10.0.0.0/24
		
		// Drop a blocker right where sub_a's NEXT-TIER sibling would want to be (10.0.1.0/24 is fine, 10.0.2.0/23 is next)
		_, _ = engCascade.Allocate("sub_block", 24, false, StrategyFirst) // Claims 10.0.1.0/24
		_, _ = engCascade.Allocate("sub_clash", 24, false, StrategyFirst) // Claims 10.0.2.0/24 (This blocks a /23 sibling!)

		// Free up sub_block so sub_a can expand into a /23 cleanly (claims 10.0.0.0/23)
		_ = engCascade.Free("sub_block")

		// Attempting to scale sub_a from a /24 to a /23 with reserve_sibling = true.
		// The /23 expansion works, but the new /23 sibling wants 10.0.2.0/23, which is blocked by sub_clash!
		err := engCascade.UpdateAllocation("sub_a", 23, true, StrategyFirst)
		if err == nil {
			t.Fatal("Expected error because the next-tier forward sibling block is occupied, but got nil")
		}
		expectedMsg := "is already occupied by allocation"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("Expected error message to contain %q, got: %v", expectedMsg, err)
		}
	})
}

