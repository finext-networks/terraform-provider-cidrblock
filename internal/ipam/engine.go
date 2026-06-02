// Package ipam provides IP Address Management (IPAM) logic for CIDR block allocation.
// It implements first-fit gap filling, sibling reservation, and dual-stack (IPv4/IPv6)
// support using Go's net/netip standard library.
package ipam

import (
	"errors"
	"fmt"
	"net/netip"
)

// Engine errors.
var (
	ErrPoolExhausted     = errors.New("pool exhausted: no available space for allocation")
	ErrInvalidPrefix     = errors.New("invalid prefix size for address family")
	ErrAllocationNotFound = errors.New("allocation not found")
	ErrDuplicateKey      = errors.New("allocation key already exists")
	ErrInvalidPool       = errors.New("invalid pool CIDR")
)

// Allocation represents a single subnet allocation within a pool.
type Allocation struct {
	PrefixSize     int    `json:"prefix_size"`
	ReserveSibling bool   `json:"reserve_sibling"`
	AllocatedCIDR  string `json:"allocated_cidr"`
	SiblingCIDR    string `json:"sibling_cidr,omitempty"`
}

// AvailableSlice represents an unallocated contiguous block in the pool.
type AvailableSlice struct {
	StartCIDR     string `json:"start_cidr"`
	MaxPrefixSize int    `json:"max_prefix_size"`
}

// Metrics represents the current usage statistics of a pool.
type Metrics struct {
	TotalIPs     uint64 `json:"total_ips"`
	AllocatedIPs uint64 `json:"allocated_ips"`
	ReservedIPs  uint64 `json:"reserved_ips"`
	AvailableIPs uint64 `json:"available_ips"`
}

// Engine manages CIDR block allocations within a supernet pool.
type Engine struct {
	poolPrefix  netip.Prefix
	allocations map[string]*Allocation
}

// NewEngine creates a new IPAM engine for the given supernet CIDR.
func NewEngine(poolCIDR string) (*Engine, error) {
	if poolCIDR == "" {
		return nil, ErrInvalidPool
	}

	prefix, err := netip.ParsePrefix(poolCIDR)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPool, err)
	}

	if prefix != prefix.Masked() {
		return nil, fmt.Errorf("%w: %s has host bits set (expected %s)", ErrInvalidPool, poolCIDR, prefix.Masked().String())
	}

	return &Engine{
		poolPrefix:  prefix,
		allocations: make(map[string]*Allocation),
	}, nil
}

// PoolCIDR returns the pool's base CIDR string.
func (e *Engine) PoolCIDR() string {
	return e.poolPrefix.String()
}

// RegisterExistingAllocation force-injects an already allocated CIDR block from the state file.
func (e *Engine) RegisterExistingAllocation(key string, alloc *Allocation) {
	e.allocations[key] = alloc
}

// GetAllocation returns a specific allocation by key.
func (e *Engine) GetAllocation(key string) (Allocation, error) {
	alloc, exists := e.allocations[key]
	if !exists {
		return Allocation{}, ErrAllocationNotFound
	}
	return *alloc, nil
}

// GetState returns a copy of the current allocation state.
func (e *Engine) GetState() map[string]Allocation {
	state := make(map[string]Allocation)
	for k, v := range e.allocations {
		state[k] = *v
	}
	return state
}

// Allocate finds the first available gap and creates a new allocation.
func (e *Engine) Allocate(key string, prefixSize int, reserveSibling bool) (string, error) {
	if _, exists := e.allocations[key]; exists {
		return "", ErrDuplicateKey
	}

	maxPrefix := e.maxPrefix()
	if prefixSize > maxPrefix || prefixSize < e.poolPrefix.Bits() {
		return "", ErrInvalidPrefix
	}

	cidr, err := e.findGap(prefixSize, reserveSibling)
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

// UpdateAllocation updates an existing allocation's properties.
func (e *Engine) UpdateAllocation(key string, prefixSize int, reserveSibling bool) error {
	alloc, exists := e.allocations[key]
	if !exists {
		return ErrAllocationNotFound
	}

	maxPrefix := e.maxPrefix()
	if prefixSize > maxPrefix || prefixSize < e.poolPrefix.Bits() {
		return ErrInvalidPrefix
	}

	if prefixSize != alloc.PrefixSize {
		delete(e.allocations, key)

		cidr, err := e.findGap(prefixSize, reserveSibling)
		if err != nil {
			return err
		}

		alloc = &Allocation{
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
		return nil
	}

	alloc.ReserveSibling = reserveSibling
	if reserveSibling {
		prefix := netip.MustParsePrefix(alloc.AllocatedCIDR)
		sibling := calcSibling(prefix)
		if sibling.IsValid() {
			alloc.SiblingCIDR = sibling.String()
			return nil
		}
	} else {
		alloc.SiblingCIDR = ""
	}

	return nil
}

// Free removes an allocation by key.
func (e *Engine) Free(key string) error {
	if _, exists := e.allocations[key]; !exists {
		return ErrAllocationNotFound
	}
	delete(e.allocations, key)
	return nil
}

// AvailableSlices returns all contiguous unallocated gaps in the pool.
func (e *Engine) AvailableSlices() []AvailableSlice {
	maxBits := e.maxPrefix()
	currentAddr := e.poolPrefix.Masked().Addr()
	poolEndAddr := addBitOffset(currentAddr, maxBits-e.poolPrefix.Bits())
	var slices []AvailableSlice

	for currentAddr.Less(poolEndAddr) {
		var occupied netip.Prefix
		found := false

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

		if found {
			currentAddr = addBitOffset(occupied.Masked().Addr(), maxBits-occupied.Bits())
			continue
		}

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

// Metrics returns pool usage statistics.
func (e *Engine) Metrics() Metrics {
	maxBits := e.maxPrefix()

	var totalIPs uint64
	if shift := maxBits - e.poolPrefix.Bits(); shift >= 64 {
		totalIPs = ^uint64(0)
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

// findGap finds the first contiguous gap large enough for the requested prefix size.
func (e *Engine) findGap(prefixSize int, reserveSibling bool) (netip.Prefix, error) {
	maxBits := e.maxPrefix()
	currentAddr := e.poolPrefix.Masked().Addr()
	poolEndAddr := addBitOffset(currentAddr, maxBits-e.poolPrefix.Bits())

	for {
		candidatePrefix := netip.PrefixFrom(currentAddr, prefixSize).Masked()
		if candidatePrefix.Addr().Less(currentAddr) {
			currentAddr = addBitOffset(candidatePrefix.Addr(), maxBits-prefixSize)
			continue
		} else {
			currentAddr = candidatePrefix.Addr()
		}

		if !currentAddr.Less(poolEndAddr) {
			return netip.Prefix{}, ErrPoolExhausted
		}

		candidate := netip.PrefixFrom(currentAddr, prefixSize)
		if !e.poolPrefix.Contains(candidate.Addr()) {
			return netip.Prefix{}, ErrPoolExhausted
		}

		// Real-World Constraint: If a forward-expanding sibling reservation is requested,
		// the candidate block must align with its future parent boundary (prefixSize - 1).
		// This guarantees the base network address stays stable during expansion, preserving the gateway IP.
		if reserveSibling {
			parentPrefix := netip.PrefixFrom(currentAddr, prefixSize-1).Masked()
			if candidate.Masked().Addr() != parentPrefix.Addr() {
				currentAddr = addBitOffset(candidate.Masked().Addr(), maxBits-prefixSize)
				continue
			}

			// Ensure the forward sibling block fits inside the supernet pool boundary
			sibling := calcSibling(candidate)
			if !e.poolPrefix.Contains(sibling.Addr()) {
				currentAddr = addBitOffset(candidate.Masked().Addr(), maxBits-prefixSize)
				continue
			}
		}

		overlapFound := false
		var overlappingPrefix netip.Prefix
		for _, alloc := range e.allocations {
			if alloc.AllocatedCIDR != "" {
				p, _ := netip.ParsePrefix(alloc.AllocatedCIDR)
				if p.Overlaps(candidate) {
					overlapFound = true
					overlappingPrefix = p
					break
				}
			}
			if alloc.SiblingCIDR != "" {
				p, _ := netip.ParsePrefix(alloc.SiblingCIDR)
				if p.Overlaps(candidate) {
					overlapFound = true
					overlappingPrefix = p
					break
				}
			}
		}

		if reserveSibling && !overlapFound {
			sibling := calcSibling(candidate)
			for _, alloc := range e.allocations {
				if alloc.AllocatedCIDR != "" {
					p, _ := netip.ParsePrefix(alloc.AllocatedCIDR)
					if p.Overlaps(sibling) {
						overlapFound = true
						overlappingPrefix = p
						break
					}
				}
				if alloc.SiblingCIDR != "" {
					p, _ := netip.ParsePrefix(alloc.SiblingCIDR)
					if p.Overlaps(sibling) {
						overlapFound = true
						overlappingPrefix = p
						break
					}
				}
			}
		}

		if overlapFound {
			currentAddr = addBitOffset(overlappingPrefix.Masked().Addr(), maxBits-overlappingPrefix.Bits())
			continue
		}

		return candidate, nil
	}
}

// calcSibling calculates the adjacent forward binary sibling block.
func calcSibling(prefix netip.Prefix) netip.Prefix {
	if prefix.Bits() <= 0 {
		return netip.Prefix{}
	}
	maxBits := 32
	if prefix.Addr().Is6() {
		maxBits = 128
	}

	// Move forward by exactly one block size to reserve the adjacent space
	upperAddr := addBitOffset(prefix.Masked().Addr(), maxBits-prefix.Bits())
	return netip.PrefixFrom(upperAddr, prefix.Bits())
}

func addBitOffset(addr netip.Addr, bitIndex int) netip.Addr {
	if addr.Is4() {
		b := addr.As4()
		carry := uint32(1) << bitIndex
		val := uint32(b[3]) | uint32(b[2])<<8 | uint32(b[1])<<16 | uint32(b[0])<<24
		val += carry
		b[0] = byte(val >> 24)
		b[1] = byte(val >> 16)
		b[2] = byte(val >> 8)
		b[3] = byte(val)
		return netip.AddrFrom4(b)
	}
	b := addr.As16()
	byteIdx := 15 - (bitIndex / 8)
	bitShift := uint(bitIndex % 8)
	var carry uint16 = 1 << bitShift
	for i := byteIdx; i >= 0 && carry > 0; i-- {
		sum := uint16(b[i]) + carry
		b[i] = byte(sum)
		carry = sum >> 8
	}
	return netip.AddrFrom16(b)
}

func (e *Engine) maxPrefix() int {
	if e.poolPrefix.Addr().Is4() {
		return 32
	}
	return 128
}

