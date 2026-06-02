// Copyright (c) HashiCorp and Contributors. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package main

// This exists so we can "go get" the plugins we need to generate documentation.

//go:build tools

import (
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)
