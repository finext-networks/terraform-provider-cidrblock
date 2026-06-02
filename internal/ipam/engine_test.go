package ipam

import (
	"fmt"
	"net/netip"
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

	// First allocation should start at the beginning
	alloc, err := eng.Allocate("subnet-a", 28, false)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if alloc != "10.0.0.0/28" {
		t.Errorf("First allocation = %s, want 10.0.0.0/28", alloc)
	}

	// Second allocation should follow immediately
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

	// Allocate a /26 first (64 IPs)
	alloc, err := eng.Allocate("large", 26, false)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if alloc != "10.0.0.0/26" {
		t.Errorf("Large allocation = %s, want 10.0.0.0/26", alloc)
	}

	// Allocate a /28 should fit in the remaining space
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

	// Try to allocate /25 from /24 pool - should work (2 allocations possible)
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

	// Third /25 should fail
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

	// Allocate the entire pool
	alloc, err := eng.Allocate("full", 24, false)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if alloc != "10.0.0.0/24" {
		t.Errorf("Full allocation = %s, want 10.0.0.0/24", alloc)
	}

	// Pool should be exhausted
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

	// Fill with three /28s
	eng.Allocate("subnet-a", 28, false) // 10.0.0.0/28
	eng.Allocate("subnet-b", 28, false) // 10.0.0.16/28
	eng.Allocate("subnet-c", 28, false) // 10.0.0.32/28

	// Free middle one
	err = eng.Free("subnet-b")
	if err != nil {
		t.Fatalf("Free failed: %v", err)
	}

	// Next allocation should fill the gap
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

	// Allocate with sibling reservation
	alloc, err := eng.Allocate("subnet-a", 28, true)
	if err != nil {
		t.Fatalf("Allocate with reserve failed: %v", err)
	}
	if alloc != "10.0.0.0/28" {
		t.Errorf("Allocation = %s, want 10.0.0.0/28", alloc)
	}

	// Next allocation should skip the sibling block
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

	// Allocate first /28 with sibling
	_, err = eng.Allocate("a", 28, true)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}

	// Get allocations to verify sibling is reserved
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

	// Before any allocation
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

	// After one allocation
	eng.Allocate("subnet-a", 26, false)
	slices = eng.AvailableSlices()
	if len(slices) != 1 {
		t.Fatalf("Expected 1 available slice after alloc, got %d", len(slices))
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

	// Allocate at beginning and end
	eng.Allocate("start", 26, false) // 10.0.0.0/26
	eng.Allocate("middle", 26, false) // 10.0.0.64/26

	slices := eng.AvailableSlices()
	if len(slices) != 1 {
		t.Fatalf("Expected 1 gap, got %d", len(slices))
	}
	if slices[0].StartCIDR != "10.0.0.128/26" {
		t.Errorf("Gap start = %s, want 10.0.0.128/26", slices[0].StartCIDR)
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

	eng.Allocate("a", 28, false) // 16 IPs
	eng.Allocate("b", 28, true) // 16 IPs + 16 reserved

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

	// First /64 allocation
	alloc, err := eng.Allocate("v6-a", 64, false)
	if err != nil {
		t.Fatalf("IPv6 Allocate failed: %v", err)
	}
	if alloc != "2001:db8::/64" {
		t.Errorf("IPv6 allocation = %s, want 2001:db8::/64", alloc)
	}

	// Second /64 allocation
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

	// Allocate without sibling
	eng.Allocate("subnet-a", 28, false)

	state := eng.GetState()
	allocA := state["subnet-a"]
	if allocA.SiblingCIDR != "" {
		t.Errorf("Initial sibling = %s, want empty", allocA.SiblingCIDR)
	}

	// Update to reserve sibling
	err = eng.UpdateAllocation("subnet-a", 28, true)
	if err != nil {
		t.Fatalf("UpdateAllocation failed: %v", err)
	}

	state = eng.GetState()
	allocA = state["subnet-a"]
	if allocA.SiblingCIDR != "10.0.0.16/28" {
		t.Errorf("Updated sibling = %s, want 10.0.0.16/28", allocA.SiblingCIDR)
	}
}

func TestEngine_UpdateAllocation_ChangePrefix(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	// Allocate /28
	eng.Allocate("subnet-a", 28, false)

	// Update to /27 (should reallocate if current block can't fit)
	// Since 10.0.0.0/28 is already allocated, changing to /27 should work if it stays within bounds
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

	// Empty state
	state := eng.GetState()
	if len(state) != 0 {
		t.Errorf("Empty state length = %d, want 0", len(state))
	}

	eng.Allocate("a", 28, false)
	eng.Allocate("b", 28, false)

	state = eng.GetState()
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

	// Allocate first /28 with sibling (reserves 10.0.0.0/28 + 10.0.0.16/28)
	eng.Allocate("a", 28, true)

	// Allocate another /28 with sibling (should get 10.0.0.32/28 + reserve 10.0.0.48/28)
	eng.Allocate("b", 28, true)

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

	// Fill with /28s (256/16 = 16 allocations)
	for i := 0; i < 16; i++ {
		_, err := eng.Allocate(fmt.Sprintf("subnet-%d", i), 28, false)
		if err != nil {
			t.Fatalf("Allocation %d failed: %v", i, err)
		}
	}

	// Pool should be full
	_, err = eng.Allocate("extra", 28, false)
	if err != ErrPoolExhausted {
		t.Errorf("Extra allocation error = %v, want ErrPoolExhausted", err)
	}

	// Free one and verify we can allocate again
	err = eng.Free("subnet-8")
	if err != nil {
		t.Fatalf("Free failed: %v", err)
	}

	alloc, err := eng.Allocate("refill", 28, false)
	if err != nil {
		t.Fatalf("Refill allocation failed: %v", err)
	}
	if alloc != "10.0.0.128/28" {
		t.Errorf("Refill = %s, want 10.0.0.128/28", alloc)
	}
}

func TestEngine_SiblingCalculation_Network(t *testing.T) {
	// Test sibling calculation for different prefix sizes
	tests := []struct {
		cidr       string
		prefixSize int
		wantSibling string
	}{
		{"10.0.0.0/24", 28, "10.0.0.16/28"},
		{"10.0.0.0/24", 24, "10.0.1.0/24"},
		{"10.0.0.0/16", 20, "10.0.64.0/20"},
	}

	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			eng, err := NewEngine(tt.cidr)
			if err != nil {
				t.Fatalf("NewEngine(%q) failed: %v", tt.cidr, err)
			}

			_, err = eng.Allocate("test", tt.prefixSize, true)
			if err != nil {
				t.Fatalf("Allocate failed: %v", err)
			}

			state := eng.GetState()
			alloc := state["test"]
			if alloc.SiblingCIDR != tt.wantSibling {
				t.Errorf("Sibling = %s, want %s", alloc.SiblingCIDR, tt.wantSibling)
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
		{"10.0.0.0/24", 0, nil},
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

	eng.Allocate("subnet-a", 28, false)
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

	// Test IPv4 max prefix
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

	// Test IPv6 max prefix
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

	// Allocate /26 (64 IPs)
	eng.Allocate("large", 26, false)  // 10.0.0.0/26
	eng.Allocate("medium", 28, false) // 10.0.0.64/28
	eng.Allocate("tiny", 30, false)   // 10.0.0.80/30

	state := eng.GetState()
	if state["tiny"].AllocatedCIDR != "10.0.0.80/30" {
		t.Errorf("Tiny alloc = %s, want 10.0.0.80/30", state["tiny"].AllocatedCIDR)
	}

	// Free medium and allocate a /27 (32 IPs) - should fit in the freed space
	eng.Free("medium")
	alloc, err := eng.Allocate("new", 27, false)
	if err != nil {
		t.Fatalf("Allocation after free failed: %v", err)
	}
	if alloc != "10.0.0.64/27" {
		t.Errorf("New alloc = %s, want 10.0.0.64/27", alloc)
	}
}

func TestEngine_GetAllocation(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	eng.Allocate("subnet-a", 28, true)

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

func TestEngine_SiblingWithinPoolBounds(t *testing.T) {
	eng, err := NewEngine("10.0.0.0/24")
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	// Allocate the last possible /28 with sibling
	// 10.0.0.0/24 has 16 /28 slots: 0-15
	// Last /28 is 10.0.0.240/28 (slot 15)
	// Its sibling would be 10.0.1.0/24 which is outside the pool
	// So we should still allow allocation but the sibling should be the next /28

	// Fill up to the second-to-last slot
	for i := 0; i < 15; i++ {
		eng.Allocate(fmt.Sprintf("s%d", i), 28, false)
	}

	// Last slot with sibling
	alloc, err := eng.Allocate("last", 28, true)
	if err != nil {
		t.Fatalf("Last allocation failed: %v", err)
	}
	if alloc != "10.0.0.240/28" {
		t.Errorf("Last alloc = %s, want 10.0.0.240/28", alloc)
	}

	state := eng.GetState()
	last := state["last"]
	if last.SiblingCIDR != "10.0.1.0/24" {
		t.Errorf("Last sibling = %s, want 10.0.1.0/24", last.SiblingCIDR)
	}
}

func TestEngine_SiblingCalculationDirect(t *testing.T) {
	tests := []struct {
		cidr       string
		wantSibling string
	}{
		{"10.0.0.0/28", "10.0.0.16/28"},
		{"10.0.0.16/28", "10.0.0.32/28"},
		{"2001:db8:0:1::/64", "2001:db8:0:2::/64"},
		{"10.0.0.0/24", "10.0.1.0/24"},
	}

	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			eng, err := NewEngine("0.0.0.0/0")
			if err != nil {
				t.Fatalf("NewEngine failed: %v", err)
			}

			_, err = eng.Allocate("test", netip.MustParsePrefix(tt.cidr).Bits(), true)
			if err != nil {
				t.Fatalf("Allocate failed: %v", err)
			}

			state := eng.GetState()
			alloc := state["test"]
			if alloc.SiblingCIDR != tt.wantSibling {
				t.Errorf("Sibling of %s = %s, want %s", tt.cidr, alloc.SiblingCIDR, tt.wantSibling)
			}
		})
	}
}
