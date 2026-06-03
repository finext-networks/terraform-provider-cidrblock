// Copyright (c) HashiCorp and Contributors. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

// Package main provides the entry point for the Terraform Provider.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/finext-networks/terraform-provider-cidrblock/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// Run directory holds a generated copy of the provider for use by the Terraform Plugin framework ("terraform-plugin-framework").
// Documentation versioning, checking, etc. is handled through that generated copy.
var (
	// these will be set by the goreleaser configuration
	// to appropriate values for the built binary.
	version string = "dev"
)

// Generate documentation for the provider.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name cidrblock

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	// Fix: Removed the '&' to instantiate as a value type instead of a reference pointer
	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/finext-networks/cidrblock",
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), provider.New(version), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
}
