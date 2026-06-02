// Copyright (c) HashiCorp and Contributors. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/finext/terraform-provider-cidrblock/internal/provider"
)

// Run directory holds a generated copy of the provider for use by the Terraform Plugin framework ("terraform-plugin-framework").
// Documentation versioning, checking, etc. is handled through that generated copy.
var (
	// these will be set by the goreleaser configuration
	// to appropriate values for the built binary.
	version string = "dev"
)

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := &providerserver.ServeOpts{
		Address: "registry.terraform.io/finext/cidrblock",
		Debug:   debug,
	}

	err := providerserver.Serve(context.Background(), provider.New(version), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
}
