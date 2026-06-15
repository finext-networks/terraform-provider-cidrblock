// Copyright (c) Finext Networks. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// cidrblockProvider implements provider.Provider.
type cidrblockProvider struct {
	version string
}

// cidrblockProviderModel maps the provider-level configuration fields.
type cidrblockProviderModel struct {
	PreventSubnetDestruction types.Bool `tfsdk:"prevent_subnet_destruction"`
}

var (
	_ provider.Provider = (*cidrblockProvider)(nil)
)

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &cidrblockProvider{
			version: version,
		}
	}
}

func (p *cidrblockProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "cidrblock"
	resp.Version = p.version
}

func (p *cidrblockProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Logical IP Address Management provider. Tracks, allocates, and recycles subnets from a defined supernet pool.",
		Attributes: map[string]schema.Attribute{
			"prevent_subnet_destruction": schema.BoolAttribute{
				Optional:    true,
				Description: "When true, explicitly blocks any configuration updates that attempt to remove existing allocation keys from a pool.",
			},
		},
	}
}

func (p *cidrblockProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config cidrblockProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	preventDestruction := false
	if !config.PreventSubnetDestruction.IsNull() && !config.PreventSubnetDestruction.IsUnknown() {
		preventDestruction = config.PreventSubnetDestruction.ValueBool()
	}

	// Share the safety configuration flag with downstream resources
	resp.ResourceData = preventDestruction
}

func (p *cidrblockProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewPoolDataSource,
	}
}

func (p *cidrblockProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewPoolResource,
	}
}
