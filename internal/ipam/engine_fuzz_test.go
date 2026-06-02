// Copyright (c) HashiCorp and Contributors. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package ipam

import (
	"testing"
)

func FuzzEngine_Allocation(f *testing.F) {
	f.Add("10.0.0.0/24", 28, false, "FIRST")
	f.Add("2001:db8::/48", 64, true, "SPARSE")
	f.Add("10.0.0.0/24", 26, true, "BEST")

	f.Fuzz(func(t *testing.T, poolCIDR string, prefixSize int, reserveSibling bool, stratStr string) {
		eng, err := NewEngine(poolCIDR)
		if err != nil {
			return
		}

		strat := Strategy(stratStr)

		_, _ = eng.Allocate("fuzz_target_1", prefixSize, reserveSibling, strat)
		_, _ = eng.Allocate("fuzz_target_2", prefixSize, reserveSibling, strat)

		_ = eng.AvailableSlices()
		_ = eng.Metrics()

		_ = eng.UpdateAllocation("fuzz_target_1", prefixSize+1, !reserveSibling, strat)
		_ = eng.UpdateAllocation("fuzz_target_1", prefixSize, reserveSibling, strat)

		_ = eng.Free("fuzz_target_1")
	})
}

