// Copyright (c) HashiCorp and Contributors. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package provider

// NewPoolDataSource returns a data source.Pool data source.
func NewPoolDataSource() interface{} {
	return &poolDataSource{}
}

// poolDataSource is the data source implementation.
type poolDataSource struct{}
