// Copyright (c) HashiCorp and Contributors. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// cidrblockProvider implements provider.Provider.
type cidrblockProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

var (
	_ provider.Provider = (*cidrblockProvider)(nil)
)

// New returns a provider function that returns a provider.Provider.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &cidrblockProvider{
			version: version,
		}
	}
}

// Metadata returns the provider type name and version.
func (p *cidrblockProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "cidrblock"
	resp.Version = p.version
}

// Schema defines the provider-level schema for configuration values.
func (p *cidrblockProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Logical IP Address Management provider. Tracks, allocates, and recycles subnets from a defined supernet pool.",
	}
}

// Configure prepares the provider for use.
func (p *cidrblockProvider) Configure(_ context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	// This is a purely logical provider with no provider-level configuration arguments.
	// We leave this empty since we do not need to initialize an external API client.
}

// DataSources defines the data sources implemented in the provider.
func (p *cidrblockProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewPoolDataSource,
	}
}

// Resources defines the resources implemented in the provider.
func (p *cidrblockProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewPoolResource,
	}
}

