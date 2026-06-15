// Copyright (c) Finext Networks. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package ipam

import (
	"net/netip"
	"testing"
)

func FuzzEngine_Allocation(f *testing.F) {
	// Seed Corpus: poolCIDR, size1, size2, reserveSibling, strategy
	f.Add("10.0.0.0/24", 28, 26, false, "FIRST")
	f.Add("2001:db8::/48", 64, 60, true, "SPARSE")
	f.Add("10.0.0.0/24", 26, 27, true, "BEST")
	f.Add("192.168.0.0/16", 24, 22, false, "FIRST")

	f.Fuzz(func(t *testing.T, poolCIDR string, size1 int, size2 int, reserveSibling bool, stratStr string) {
		eng, err := NewEngine(poolCIDR)
		if err != nil {
			return // Skip invalid base pool structures gracefully
		}

		strat := Strategy(stratStr)

		// 1. Execute heterogeneous allocations to build a fragmented prefix layout tree
		cidr1, err1 := eng.Allocate("fuzz_target_1", size1, reserveSibling, strat)
		_, err2 := eng.Allocate("fuzz_target_2", size2, reserveSibling, strat) // Fix: Discard unused string tracking token

		// 2. Property-Based Invariant Verification: IPAM Metrics Accounting Balance
		// Except for saturated IPv6 space bounds matching max mask ceilings, capacity math must perfectly align.
		m := eng.Metrics()
		if m.TotalIPs != ^uint64(0) {
			calculatedAvailable := m.TotalIPs - m.AllocatedIPs - m.ReservedIPs
			if m.AvailableIPs != calculatedAvailable {
				t.Fatalf("Metrics accounting mismatch! Total: %d, Allocated: %d, Reserved: %d, Expected Available: %d, Got: %d",
					m.TotalIPs, m.AllocatedIPs, m.ReservedIPs, calculatedAvailable, m.AvailableIPs)
			}
		}

		// 3. Trigger continuous available slice extraction parsing loops
		_ = eng.AvailableSlices()

		// 4. Anchor Mutation Validation Invariants
		if err1 == nil {
			oldPrefix := netip.MustParsePrefix(cidr1)

			// Attempt a size transformation up or down
			newSize := size1 + 1
			if size1 >= eng.maxPrefix() {
				newSize = size1 - 1
			}

			updateErr := eng.UpdateAllocation("fuzz_target_1", newSize, !reserveSibling, strat)
			if updateErr == nil {
				record, getErr := eng.GetAllocation("fuzz_target_1")
				if getErr != nil {
					t.Fatalf("Failed to retrieve successfully updated allocation: %v", getErr)
				}

				// CRITICAL INVARIANT: Base IP coordinate must never shift or float during an update operation
				newPrefix := netip.MustParsePrefix(record.AllocatedCIDR)
				if newPrefix.Addr() != oldPrefix.Addr() {
					t.Fatalf("Engine violated base-address immutability! Anchor shifted from %s to %s during size change",
						oldPrefix.Addr(), newPrefix.Addr())
				}
			}
		}

		// 5. Run standard cleanup cycles to ensure state maps purge cleanly without memory leakage
		if err1 == nil {
			if err := eng.Free("fuzz_target_1"); err != nil {
				t.Fatalf("Failed to release active allocation key: %v", err)
			}
		}
		if err2 == nil {
			if err := eng.Free("fuzz_target_2"); err != nil {
				t.Fatalf("Failed to release active allocation key: %v", err)
			}
		}
	})
}
