// Copyright (c) HashiCorp and Contributors. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

// Package ipam provides IP Address Management (IPAM) logic for CIDR block allocation.
// It implements first-fit gap filling, sibling reservation, and dual-stack (IPv4/IPv6)
// support using Go's net/netip standard library.
package ipam

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
)

// Strategy represents the layout positioning choice criteria used when
// selecting an unallocated space block from multiple available candidate gaps.
type Strategy string

const (
	// StrategyFirst allocates the very first aligned address block that can fit
	// the requested prefix size, scanning from the lowest to highest address ranges.
	StrategyFirst Strategy = "FIRST"

	// StrategyBest selects the smallest available continuous unallocated slice
	// that can still accommodate the request, minimizing fragment size degradation.
	StrategyBest Strategy = "BEST"

	// StrategySparse selects the largest available continuous unallocated slice,
	// maximizing isolation distance between independent subnet groups.
	StrategySparse Strategy = "SPARSE"
)

// Engine errors defining explicit boundary validation failures.
var (
	ErrPoolExhausted      = errors.New("pool exhausted: no available space for allocation")
	ErrInvalidPrefix      = errors.New("invalid prefix size for address family")
	ErrAllocationNotFound = errors.New("allocation not found")
	ErrDuplicateKey       = errors.New("allocation key already exists")
	ErrInvalidPool        = errors.New("invalid pool CIDR")
)

// Allocation models a single locked subnet space block tracked by the engine.
type Allocation struct {
	PrefixSize     int    `json:"prefix_size"`
	ReserveSibling bool   `json:"reserve_sibling"`
	AllocatedCIDR  string `json:"allocated_cidr"`
	SiblingCIDR    string `json:"sibling_cidr,omitempty"`
}

// AvailableSlice details an empty, contiguous span of addresses inside the pool.
type AvailableSlice struct {
	StartCIDR     string `json:"start_cidr"`
	MaxPrefixSize int    `json:"max_prefix_size"`
}

// Metrics tracks mathematical utilization snapshots of the managed address pool.
type Metrics struct {
	TotalIPs     uint64 `json:"total_ips"`
	AllocatedIPs uint64 `json:"allocated_ips"`
	ReservedIPs  uint64 `json:"reserved_ips"`
	AvailableIPs uint64 `json:"available_ips"`
}

// Engine encapsulates stateful logical calculations over a single base CIDR supernet.
type Engine struct {
	poolPrefix  netip.Prefix
	allocations map[string]*Allocation
}

// NewEngine instantiates a validation-guarded IPAM calculator.
func NewEngine(poolCIDR string) (*Engine, error) {
	if poolCIDR == "" {
		return nil, ErrInvalidPool
	}

	prefix, err := netip.ParsePrefix(poolCIDR)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPool, err)
	}

	// Host bit validation prevents parsing broad ambiguous ranges (e.g., 10.0.0.5/24)
	if prefix != prefix.Masked() {
		return nil, fmt.Errorf("%w: %s has host bits set (expected %s)", ErrInvalidPool, poolCIDR, prefix.Masked().String())
	}

	return &Engine{
		poolPrefix:  prefix,
		allocations: make(map[string]*Allocation),
	}, nil
}

// PoolCIDR returns the clean structural base supernet string.
func (e *Engine) PoolCIDR() string {
	return e.poolPrefix.String()
}

// RegisterExistingAllocation hydrates state directly from the provider's TF memory.
func (e *Engine) RegisterExistingAllocation(key string, alloc *Allocation) {
	e.allocations[key] = alloc
}

// GetAllocation reads tracking components securely for an existing key.
func (e *Engine) GetAllocation(key string) (Allocation, error) {
	alloc, exists := e.allocations[key]
	if !exists {
		return Allocation{}, ErrAllocationNotFound
	}
	return *alloc, nil
}

// GetState dumps a isolated, deep-copied copy of the inner runtime map.
func (e *Engine) GetState() map[string]Allocation {
	state := make(map[string]Allocation)
	for k, v := range e.allocations {
		state[k] = *v
	}
	return state
}

// Allocate executes searching and claims a block matching targeted layout constraints.
func (e *Engine) Allocate(key string, prefixSize int, reserveSibling bool, strategy Strategy) (string, error) {
	if _, exists := e.allocations[key]; exists {
		return "", ErrDuplicateKey
	}

	maxPrefix := e.maxPrefix()
	if prefixSize > maxPrefix || prefixSize < e.poolPrefix.Bits() {
		return "", ErrInvalidPrefix
	}

	// Fuzz Guard: Prevents bit length underflows (-1) inside root supernet evaluations
	if reserveSibling && prefixSize <= 0 {
		return "", ErrInvalidPrefix
	}

	cidr, err := e.findGap(prefixSize, reserveSibling, strategy)
	if err != nil {
		return "", err
	}

	alloc := &Allocation{
		PrefixSize:     prefixSize,
		ReserveSibling: reserveSibling,
		AllocatedCIDR:  cidr.String(),
	}

	if reserveSibling {
		sibling := calcSibling(cidr)
		if sibling.IsValid() {
			alloc.SiblingCIDR = sibling.String()
		}
	}

	e.allocations[key] = alloc
	return cidr.String(), nil
}

// UpdateAllocation updates an existing allocation's properties, enforcing strict base-address
// immutability and validating that sibling requests are restricted exclusively to left-hand aligned blocks.
func (e *Engine) UpdateAllocation(key string, prefixSize int, reserveSibling bool, strategy Strategy) error {
	alloc, exists := e.allocations[key]
	if !exists {
		return ErrAllocationNotFound
	}

	maxPrefix := e.maxPrefix()
	if prefixSize > maxPrefix || prefixSize < e.poolPrefix.Bits() {
		return ErrInvalidPrefix
	}

	if reserveSibling && prefixSize <= 0 {
		return ErrInvalidPrefix
	}

	// Handle size mutations strictly within the current anchored base coordinate
	if prefixSize != alloc.PrefixSize {
		oldPrefix := netip.MustParsePrefix(alloc.AllocatedCIDR)
		proposedPrefix := netip.PrefixFrom(oldPrefix.Addr(), prefixSize).Masked()

		// INVALIDATION 1: Enforce base-address immutability
		if proposedPrefix.Addr() != oldPrefix.Addr() {
			return fmt.Errorf("increasing size to /%d breaks binary alignment boundaries at the current address %s (requires shifting base to %s). Subnets cannot be automatically relocated; delete and recreate the allocation to move it",
				prefixSize, oldPrefix.Addr(), proposedPrefix.Addr())
		}

		// // INVALIDATION 2: Verify the expanded boundary doesn't run outside the master pool
		// if !e.poolPrefix.Contains(proposedPrefix.Addr()) || proposedPrefix.Bits() < e.poolPrefix.Bits() {
		// 	return fmt.Errorf("expanded prefix %s extends outside pool boundaries", proposedPrefix)
		// }

		// INVALIDATION 3: Verify the expanded footprint doesn't collide with any OTHER subnets
		for k, v := range e.allocations {
			if k == key {
				continue // Skip self
			}
			if v.AllocatedCIDR != "" {
				p, _ := netip.ParsePrefix(v.AllocatedCIDR)
				if p.Overlaps(proposedPrefix) {
					return fmt.Errorf("expanded prefix %s overlaps with existing allocation %q", proposedPrefix, k)
				}
			}
			if v.SiblingCIDR != "" {
				p, _ := netip.ParsePrefix(v.SiblingCIDR)
				if p.Overlaps(proposedPrefix) {
					return fmt.Errorf("expanded prefix %s overlaps with reserved sibling of %q", proposedPrefix, k)
				}
			}
		}

		var proposedSibling netip.Prefix
		if reserveSibling {
			// INVALIDATION 4: Enforce left-hand alignment for buddy expansion compatibility
			parentPrefix := netip.PrefixFrom(proposedPrefix.Addr(), prefixSize-1).Masked()
			if proposedPrefix.Addr() != parentPrefix.Addr() {
				return fmt.Errorf("allocation %s is a right-hand (upper-half) block at this size tier and cannot claim a forward sibling reservation", proposedPrefix)
			}

			// Calculate the new next-block sibling location
			proposedSibling = netip.PrefixFrom(addBitOffset(proposedPrefix.Addr(), maxPrefix-prefixSize), prefixSize)

			// INVALIDATION 5: Verify the companion block falls inside pool boundaries
			if !proposedSibling.IsValid() || !e.poolPrefix.Contains(proposedSibling.Addr()) {
				return fmt.Errorf("the requested companion block falls outside the master pool boundaries. Toggle reserve_sibling to false")
			}

			// INVALIDATION 6: Verify the next continuous block isn't already claimed
			for k, v := range e.allocations {
				if k == key {
					continue
				}
				if v.AllocatedCIDR != "" {
					p, _ := netip.ParsePrefix(v.AllocatedCIDR)
					if p.Overlaps(proposedSibling) {
						return fmt.Errorf("the next continuous block (%s) is already occupied by allocation %q. Toggle reserve_sibling to false to expand in-place",
							proposedSibling, k)
					}
				}
				if v.SiblingCIDR != "" {
					p, _ := netip.ParsePrefix(v.SiblingCIDR)
					if p.Overlaps(proposedSibling) {
						return fmt.Errorf("the next continuous block (%s) is already reserved as a sibling by allocation %q. Toggle reserve_sibling to false to expand in-place",
							proposedSibling, k)
					}
				}
			}
		}

		// COMMIT: Expansion is safe, clean, and perfectly anchored in-place
		alloc.PrefixSize = prefixSize
		alloc.ReserveSibling = reserveSibling
		alloc.AllocatedCIDR = proposedPrefix.String()
		if reserveSibling {
			alloc.SiblingCIDR = proposedSibling.String()
		} else {
			alloc.SiblingCIDR = ""
		}
		return nil
	}

	// Handle non-size mutations (toggling sibling reservations on a completely static prefix size)
	if reserveSibling != alloc.ReserveSibling {
		if reserveSibling {
			prefix := netip.MustParsePrefix(alloc.AllocatedCIDR)

			// INVALIDATION: Enforce left-hand alignment on static sibling toggles
			parentPrefix := netip.PrefixFrom(prefix.Addr(), prefix.Bits()-1).Masked()
			if prefix.Addr() != parentPrefix.Addr() {
				return fmt.Errorf("allocation %s is a right-hand block and cannot claim a forward buddy pair", prefix)
			}

			sibling := netip.PrefixFrom(addBitOffset(prefix.Addr(), maxPrefix-prefix.Bits()), prefix.Bits())
			if !sibling.IsValid() || !e.poolPrefix.Contains(sibling.Addr()) {
				return fmt.Errorf("companion block is out of pool boundaries")
			}

			for k, v := range e.allocations {
				if k == key {
					continue
				}
				if v.AllocatedCIDR != "" {
					p, _ := netip.ParsePrefix(v.AllocatedCIDR)
					if p.Overlaps(sibling) {
						return fmt.Errorf("target block %s is already occupied by %q", sibling, k)
					}
				}
				if v.SiblingCIDR != "" {
					p, _ := netip.ParsePrefix(v.SiblingCIDR)
					if p.Overlaps(sibling) {
						return fmt.Errorf("target block %s is already reserved by %q", sibling, k)
					}
				}
			}
			alloc.SiblingCIDR = sibling.String()
		} else {
			alloc.SiblingCIDR = ""
		}
		alloc.ReserveSibling = reserveSibling
	}

	return nil
}

// Free cleans tracking records, releasing the target namespace key.
func (e *Engine) Free(key string) error {
	if _, exists := e.allocations[key]; !exists {
		return ErrAllocationNotFound
	}
	delete(e.allocations, key)
	return nil
}

// AvailableSlices runs an optimized $O(M)$ step iteration through active maps
// to extract discrete spans of remaining contiguous address gaps.
func (e *Engine) AvailableSlices() []AvailableSlice {
	maxBits := e.maxPrefix()
	currentAddr := e.poolPrefix.Masked().Addr()
	poolEndAddr := addBitOffset(currentAddr, maxBits-e.poolPrefix.Bits())
	var slices []AvailableSlice

	if !poolEndAddr.IsValid() {
		return []AvailableSlice{{StartCIDR: e.poolPrefix.String(), MaxPrefixSize: e.poolPrefix.Bits()}}
	}

	for currentAddr.IsValid() && currentAddr.Less(poolEndAddr) {
		var occupied netip.Prefix
		found := false

		// Identify whether the pointer sits inside a primary block or sibling reservation
		for _, alloc := range e.allocations {
			if alloc.AllocatedCIDR != "" {
				p, _ := netip.ParsePrefix(alloc.AllocatedCIDR)
				if p.Contains(currentAddr) {
					occupied = p
					found = true
					break
				}
			}
			if alloc.SiblingCIDR != "" {
				p, _ := netip.ParsePrefix(alloc.SiblingCIDR)
				if p.Contains(currentAddr) {
					occupied = p
					found = true
					break
				}
			}
		}

		// If occupied, leap clean past the boundary block without executing step cycles
		if found {
			currentAddr = addBitOffset(occupied.Masked().Addr(), maxBits-occupied.Bits())
			continue
		}

		// Discover the largest naturally aligned block size that fits the open gap
		bestBits := maxBits
		for bits := e.poolPrefix.Bits(); bits <= maxBits; bits++ {
			p := netip.PrefixFrom(currentAddr, bits)
			if p.Masked().Addr() != currentAddr || !e.poolPrefix.Contains(p.Addr()) {
				continue
			}

			collision := false
			for _, alloc := range e.allocations {
				if alloc.AllocatedCIDR != "" {
					a, _ := netip.ParsePrefix(alloc.AllocatedCIDR)
					if a.Overlaps(p) {
						collision = true
						break
					}
				}
				if alloc.SiblingCIDR != "" {
					a, _ := netip.ParsePrefix(alloc.SiblingCIDR)
					if a.Overlaps(p) {
						collision = true
						break
					}
				}
			}

			if !collision {
				bestBits = bits
				break
			}
		}

		p := netip.PrefixFrom(currentAddr, bestBits)
		slices = append(slices, AvailableSlice{
			StartCIDR:     p.String(),
			MaxPrefixSize: bestBits,
		})
		currentAddr = addBitOffset(currentAddr, maxBits-bestBits)
	}
	return slices
}

// Metrics generates math sums calculating complete capacity snapshots.
func (e *Engine) Metrics() Metrics {
	maxBits := e.maxPrefix()

	var totalIPs uint64
	if shift := maxBits - e.poolPrefix.Bits(); shift >= 64 {
		totalIPs = ^uint64(0) // Safe bit saturation max handling for wide v6 spaces (/48, etc.)
	} else {
		totalIPs = 1 << shift
	}

	var allocatedIPs, reservedIPs uint64
	for _, alloc := range e.allocations {
		if alloc.AllocatedCIDR != "" {
			p, _ := netip.ParsePrefix(alloc.AllocatedCIDR)
			if shift := maxBits - p.Bits(); shift < 64 {
				allocatedIPs += 1 << shift
			}
		}
		if alloc.SiblingCIDR != "" {
			p, _ := netip.ParsePrefix(alloc.SiblingCIDR)
			if shift := maxBits - p.Bits(); shift < 64 {
				reservedIPs += 1 << shift
			}
		}
	}

	availableIPs := totalIPs - allocatedIPs - reservedIPs
	if allocatedIPs+reservedIPs > totalIPs {
		availableIPs = 0
	}

	return Metrics{
		TotalIPs:     totalIPs,
		AllocatedIPs: allocatedIPs,
		ReservedIPs:  reservedIPs,
		AvailableIPs: availableIPs,
	}
}

// findGap navigates slice arrays dynamically to isolate targets.
// Complexity is tightly bounded at $O(M)$ where $M$ is active keys, avoiding brute force hangs.
func (e *Engine) findGap(prefixSize int, reserveSibling bool, strategy Strategy) (netip.Prefix, error) {
	maxBits := e.maxPrefix()
	slices := e.AvailableSlices()

	type candidate struct {
		prefix    netip.Prefix
		sliceBits int
	}
	var candidates []candidate

	for _, s := range slices {
		slicePrefix, _ := netip.ParsePrefix(s.StartCIDR)
		currentAddr := slicePrefix.Masked().Addr()

		var sliceEndAddr netip.Addr
		if s.MaxPrefixSize > e.poolPrefix.Bits() {
			sliceEndAddr = addBitOffset(slicePrefix.Masked().Addr(), maxBits-s.MaxPrefixSize)
		}

		for currentAddr.IsValid() {
			if !e.poolPrefix.Contains(currentAddr) {
				break
			}
			if sliceEndAddr.IsValid() && !currentAddr.Less(sliceEndAddr) {
				break
			}

			candidatePrefix := netip.PrefixFrom(currentAddr, prefixSize).Masked()
			if candidatePrefix.Addr().Less(currentAddr) {
				currentAddr = addBitOffset(candidatePrefix.Addr(), maxBits-prefixSize)
				continue
			}

			candidateBlock := netip.PrefixFrom(currentAddr, prefixSize)
			if !e.poolPrefix.Contains(candidateBlock.Addr()) {
				break
			}

			// Verify structural alignment limits for paired subnets
			if reserveSibling {
				parentPrefix := netip.PrefixFrom(currentAddr, prefixSize-1).Masked()
				if candidateBlock.Masked().Addr() != parentPrefix.Addr() {
					currentAddr = addBitOffset(candidateBlock.Masked().Addr(), maxBits-prefixSize)
					continue
				}

				sibling := calcSibling(candidateBlock)
				if !sibling.IsValid() || !e.poolPrefix.Contains(sibling.Addr()) {
					currentAddr = addBitOffset(candidateBlock.Masked().Addr(), maxBits-prefixSize)
					continue
				}
			}

			// Boundary Check: Explicit collision verification prevents cross-slice boundary overflow leakage
			overlapFound := false
			var conflictingPrefix netip.Prefix // Track the actual blocking obstacle

			for _, alloc := range e.allocations {
				if alloc.AllocatedCIDR != "" {
					p, _ := netip.ParsePrefix(alloc.AllocatedCIDR)
					if p.Overlaps(candidateBlock) {
						overlapFound = true
						conflictingPrefix = p
						break
					}
				}
				if alloc.SiblingCIDR != "" {
					p, _ := netip.ParsePrefix(alloc.SiblingCIDR)
					if p.Overlaps(candidateBlock) {
						overlapFound = true
						conflictingPrefix = p
						break
					}
				}
			}

			if overlapFound {
				// LEAP GUARDRAIL: Leap clean past the conflicting block footprint
				// to completely eliminate address-by-address linear scanning hangs
				currentAddr = addBitOffset(conflictingPrefix.Masked().Addr(), maxBits-conflictingPrefix.Bits())
				continue
			}

			// Separated Sibling validation path
			if reserveSibling {
				sibling := calcSibling(candidateBlock)
				for _, alloc := range e.allocations {
					if alloc.AllocatedCIDR != "" {
						p, _ := netip.ParsePrefix(alloc.AllocatedCIDR)
						if p.Overlaps(sibling) {
							overlapFound = true
							break
						}
					}
					if alloc.SiblingCIDR != "" {
						p, _ := netip.ParsePrefix(alloc.SiblingCIDR)
						if p.Overlaps(sibling) {
							overlapFound = true
							break
						}
					}
				}
			}

			if overlapFound {
				// Sibling collision only: safely advance by a single aligned block step
				currentAddr = addBitOffset(candidateBlock.Masked().Addr(), maxBits-prefixSize)
				continue
			}

			candidates = append(candidates, candidate{
				prefix:    candidateBlock,
				sliceBits: s.MaxPrefixSize,
			})

			// FIRST strategy exits instantly on match to maximize deployment speed
			if strategy == StrategyFirst || strategy == "" {
				return candidateBlock, nil
			}
			break // Strategy sorting loops evaluate only the first aligned candidate per slice gap
		}
	}

	if len(candidates) == 0 {
		return netip.Prefix{}, ErrPoolExhausted
	}

	// Order matches to fulfill structural positioning definitions
	switch strategy {
	case StrategyBest:
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].sliceBits == candidates[j].sliceBits {
				return candidates[i].prefix.Addr().Less(candidates[j].prefix.Addr())
			}
			return candidates[i].sliceBits > candidates[j].sliceBits
		})
	case StrategySparse:
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].sliceBits == candidates[j].sliceBits {
				return candidates[i].prefix.Addr().Less(candidates[j].prefix.Addr())
			}
			return candidates[i].sliceBits < candidates[j].sliceBits
		})
	default:
		sort.SliceStable(candidates, func(i, j int) bool {
			return candidates[i].prefix.Addr().Less(candidates[j].prefix.Addr())
		})
	}

	return candidates[0].prefix, nil
}

// calcSibling evaluates the high-half counterpart of a lower-half binary pair.
func calcSibling(prefix netip.Prefix) netip.Prefix {
	if prefix.Bits() <= 0 {
		return netip.Prefix{}
	}
	maxBits := 32
	if prefix.Addr().Is6() {
		maxBits = 128
	}
	upperAddr := addBitOffset(prefix.Masked().Addr(), maxBits-prefix.Bits())
	if !upperAddr.IsValid() {
		return netip.Prefix{}
	}
	return netip.PrefixFrom(upperAddr, prefix.Bits())
}

// addBitOffset maps raw additions over byte arrays.
// Fixed Fuzz Bug: Shift widths exceeding or matching boundaries are caught to avoid masking loops.
func addBitOffset(addr netip.Addr, bitIndex int) netip.Addr {
	if !addr.IsValid() {
		return netip.Addr{}
	}
	if addr.Is4() {
		if bitIndex >= 32 {
			return netip.Addr{}
		}
		b := addr.As4()
		carry := uint32(1) << bitIndex

		// Upgrade calculations to uint64 to catch boundary wrap-around triggers safely
		val := uint64(b[3]) | uint64(b[2])<<8 | uint64(b[1])<<16 | uint64(b[0])<<24
		val += uint64(carry)

		if val > 0xFFFFFFFF {
			return netip.Addr{} // Safely intercepts exact-edge overflows
		}

		b[0] = byte(val >> 24)
		b[1] = byte(val >> 16)
		b[2] = byte(val >> 8)
		b[3] = byte(val)
		return netip.AddrFrom4(b)
	}

	if bitIndex >= 128 {
		return netip.Addr{}
	}
	b := addr.As16()
	byteIdx := 15 - (bitIndex / 8)
	bitShift := uint(bitIndex % 8)
	var carry uint16 = 1 << bitShift

	// Sequentially ripple addition carrying operations down across the array block
	for i := byteIdx; i >= 0 && carry > 0; i-- {
		sum := uint16(b[i]) + carry
		b[i] = byte(sum)
		carry = sum >> 8
	}
	if carry > 0 {
		return netip.Addr{}
	}
	return netip.AddrFrom16(b)
}

func (e *Engine) maxPrefix() int {
	if e.poolPrefix.Addr().Is4() {
		return 32
	}
	return 128
}
