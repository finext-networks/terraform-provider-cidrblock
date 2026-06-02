// Copyright (c) HashiCorp and Contributors. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package provider

// NewPoolResource returns a resource.Pool resource.
func NewPoolResource() interface{} {
	return &poolResource{}
}

// poolResource is the resource implementation.
type poolResource struct{}
