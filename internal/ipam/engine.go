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

	// Validate that the prefix is aligned (no host bits set)
	if !prefix.IsSingleBit() {
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

// Allocate finds the first available gap and creates a new allocation.
func (e *Engine) Allocate(key string, prefixSize int, reserveSibling bool) (string, error) {
	// Check for duplicate key
	if _, exists := e.allocations[key]; exists {
		return "", ErrDuplicateKey
	}

	// Validate prefix size
	maxPrefix := e.maxPrefix()
	if prefixSize > maxPrefix || prefixSize < 0 {
		return "", ErrInvalidPrefix
	}

	// Find available gap
	cidr, err := e.findGap(prefixSize)
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
	if prefixSize > maxPrefix || prefixSize < 0 {
		return ErrInvalidPrefix
	}

	// If prefix size changed, reallocate
	if prefixSize != alloc.PrefixSize {
		delete(e.allocations, key)

		cidr, err := e.findGap(prefixSize)
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

	// Same prefix, just update sibling reservation
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

// GetState returns a copy of the current allocation state.
func (e *Engine) GetState() map[string]Allocation {
	state := make(map[string]Allocation)
	for k, v := range e.allocations {
		state[k] = *v
	}
	return state
}

// GetAllocation returns a specific allocation by key.
func (e *Engine) GetAllocation(key string) (Allocation, error) {
	alloc, exists := e.allocations[key]
	if !exists {
		return Allocation{}, ErrAllocationNotFound
	}
	return *alloc, nil
}

// AvailableSlices returns all contiguous unallocated gaps in the pool.
func (e *Engine) AvailableSlices() []AvailableSlice {
	// Collect all occupied CIDRs
	var occupied []netip.Prefix
	for _, alloc := range e.allocations {
		occupied = append(occupied, netip.MustParsePrefix(alloc.AllocatedCIDR))
		if alloc.SiblingCIDR != "" {
			occupied = append(occupied, netip.MustParsePrefix(alloc.SiblingCIDR))
		}
	}

	if len(occupied) == 0 {
		return []AvailableSlice{{
			StartCIDR:     e.poolPrefix.String(),
			MaxPrefixSize: e.poolPrefix.Bits(),
		}}
	}

	// Sort by start address
	sort.Slice(occupied, func(i, j int) bool {
		return occupied[i].From().Less(occupied[j].From())
	})

	// Find gaps
	var slices []AvailableSlice
	currentStart := e.poolPrefix.From()

	for _, block := range occupied {
		start := block.From()
		bits := block.Bits()

		// Skip if this block overlaps with what we're tracking
		if currentStart.Cmp(start) >= 0 {
			continue
		}

		// Calculate gap prefix
		gapPrefix := calcGapSize(currentStart, start, e.poolPrefix.Addr().BitLen())

		if gapPrefix >= bits {
			slices = append(slices, AvailableSlice{
				StartCIDR:     currentStart.Prefix(currentStart.BitLen()-gapPrefix).String(),
				MaxPrefixSize: gapPrefix,
			})
		}

		// Move current past this block
		nextAddr := block.NextAddr()
		if nextAddr.Cmp(currentStart) > 0 {
			currentStart = nextAddr
		}
	}

	return slices
}

// Metrics returns pool usage statistics.
func (e *Engine) Metrics() Metrics {
	m := Metrics{
		TotalIPs: ipsInPrefix(e.poolPrefix),
	}

	for _, alloc := range e.allocations {
		prefix := netip.MustParsePrefix(alloc.AllocatedCIDR)
		m.AllocatedIPs += ipsInPrefix(prefix)

		if alloc.SiblingCIDR != "" {
			sibling := netip.MustParsePrefix(alloc.SiblingCIDR)
			m.ReservedIPs += ipsInPrefix(sibling)
		}
	}

	m.AvailableIPs = m.TotalIPs - m.AllocatedIPs - m.ReservedIPs
	return m
}

// findGap finds the first contiguous gap large enough for the requested prefix size.
func (e *Engine) findGap(prefixSize int) (netip.Prefix, error) {
	// Collect all occupied ranges
	var occupied []netip.Prefix
	for _, alloc := range e.allocations {
		occupied = append(occupied, netip.MustParsePrefix(alloc.AllocatedCIDR))
		if alloc.SiblingCIDR != "" {
			occupied = append(occupied, netip.MustParsePrefix(alloc.SiblingCIDR))
		}
	}

	if len(occupied) == 0 {
		return e.poolPrefix.WithPrefixBits(prefixSize), nil
	}

	// Sort by start address
	sort.Slice(occupied, func(i, j int) bool {
		return occupied[i].From().Less(occupied[j].From())
	})

	bitsLen := e.poolPrefix.Addr().BitLen()

	// Check gap before first block
	first := occupied[0]
	if e.poolPrefix.From().Cmp(first.From()) < 0 {
		// There's space before the first block
		gapEnd := first.From()
		gapPrefix := calcGapSize(e.poolPrefix.From(), gapEnd, bitsLen)
		if gapPrefix >= prefixSize {
			return e.poolPrefix.From().Prefix(prefixSize), nil
		}
	}

	// Check gaps between blocks
	for i := 0; i < len(occupied)-1; i++ {
		current := occupied[i]
		next := occupied[i+1]

		// Find the end of current block
		blockSize := 1 << (bitsLen - current.Bits())
		nextStart := addAddr(current.From(), blockSize)

		if nextStart.Cmp(next.From()) < 0 {
			// There's a gap
			gapEnd := next.From()
			gapPrefix := calcGapSize(nextStart, gapEnd, bitsLen)
			if gapPrefix >= prefixSize {
				return nextStart.Prefix(prefixSize), nil
			}
		}
	}

	// Check space after last block
	last := occupied[len(occupied)-1]
	lastSize := 1 << (bitsLen - last.Bits())
	nextStart := addAddr(last.From(), lastSize)

	if nextStart.Cmp(e.poolPrefix.ToAddr()) < 0 {
		remainingPrefix := calcGapSize(nextStart, e.poolPrefix.ToAddr(), bitsLen)
		if remainingPrefix >= prefixSize {
			return nextStart.Prefix(prefixSize), nil
		}
	}

	return netip.Prefix{}, ErrPoolExhausted
}

// calcGapSize calculates the prefix size of the gap between start and end addresses.
func calcGapSize(start, end netip.Addr, bitsLen int) int {
	diff := end.Cmp(start)
	if diff <= 0 {
		return bitsLen
	}

	for p := bitsLen - 1; p >= 0; p-- {
		blockSize := 1 << (bitsLen - p)
		if diff < blockSize {
			return p
		}
	}
	return bitsLen
}

// calcSibling calculates the binary sibling of a CIDR block.
func calcSibling(prefix netip.Prefix) netip.Prefix {
	addr := prefix.From()
	bitsLen := addr.BitLen()
	blockSize := 1 << (bitsLen - prefix.Bits())

	siblingStart := addAddr(addr, blockSize)
	sibling, ok := siblingStart.PrefixPrefix(prefix.Bits())
	if !ok {
		return netip.Prefix{}
	}
	return sibling
}

// addAddr adds a value to an IP address.
func addAddr(addr netip.Addr, value int) netip.Addr {
	if addr.Is4() {
		ip4 := addr.As4()
		current := uint32(ip4[0])<<24 | uint32(ip4[1])<<16 | uint32(ip4[2])<<8 | uint32(ip4[3])
		current += uint32(value)
		return netip.AddrFrom4([4]byte{
			byte(current >> 24),
			byte(current >> 16),
			byte(current >> 8),
			byte(current),
		})
	}

	ip16 := addr.As16()
	current := uint64(ip16[0])<<56 | uint64(ip16[1])<<48 | uint64(ip16[2])<<40 | uint64(ip16[3])<<32 |
		uint64(ip16[4])<<24 | uint64(ip16[5])<<16 | uint64(ip16[6])<<8 | uint64(ip16[7])
	current += uint64(value)
	return netip.AddrFrom16([16]byte{
		byte(current >> 56),
		byte(current >> 48),
		byte(current >> 40),
		byte(current >> 32),
		byte(current >> 24),
		byte(current >> 16),
		byte(current >> 8),
		byte(current),
	})
}

// ipsInPrefix returns the number of IPs in a prefix.
func ipsInPrefix(prefix netip.Prefix) uint64 {
	bits := prefix.Bits()
	bitsLen := prefix.Addr().BitLen()
	return 1 << (bitsLen - bits)
}

// maxPrefix returns the maximum prefix size for the address family.
func (e *Engine) maxPrefix() int {
	if e.poolPrefix.Addr().Is4() {
		return 32
	}
	return 128
}
