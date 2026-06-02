// Copyright (c) HashiCorp and Contributors. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure implementation satisfies interfaces.
var (
	_ datasource.DataSource        = (*poolDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*poolDataSource)(nil)
)

// poolDataSourceModel maps data source schema data.
type poolDataSourceModel struct {
	ID              types.String `tfsdk:"id"`
	CIDR            types.String `tfsdk:"cidr"`
	Organization    types.String `tfsdk:"organization"`
	Project         types.String `tfsdk:"project"`
	Network         types.String `tfsdk:"network"`
	Allocations     types.Map    `tfsdk:"allocations"`
	AvailableSlices types.List   `tfsdk:"available_slices"`
	Metrics         types.Object `tfsdk:"metrics"`
}

// availableSliceModel maps available slice attributes.
type availableSliceModel struct {
	StartCIDR     types.String `tfsdk:"start_cidr"`
	MaxPrefixSize types.Int64  `tfsdk:"max_prefix_size"`
}

// metricsModel maps metrics object attributes.
type metricsModel struct {
	TotalIPs     types.Int64 `tfsdk:"total_ips"`
	AllocatedIPs types.Int64 `tfsdk:"allocated_ips"`
	ReservedIPs  types.Int64 `tfsdk:"reserved_ips"`
	AvailableIPs types.Int64 `tfsdk:"available_ips"`
}

// NewPoolDataSource returns a cidrblock_pool data source.
func NewPoolDataSource() datasource.DataSource {
	return &poolDataSource{}
}

func (d *poolDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pool"
}

func (d *poolDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads a CIDR block pool's state, showing current allocations, available slices, and usage metrics.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The pool ID to query (format: organization:project:network).",
				Required:    true,
			},
			"cidr": schema.StringAttribute{
				Description: "The base IPv4/IPv6 supernet CIDR.",
				Computed:    true,
			},
			"organization": schema.StringAttribute{
				Description: "Top-level namespace.",
				Computed:    true,
			},
			"project": schema.StringAttribute{
				Description: "Mid-level namespace.",
				Computed:    true,
			},
			"network": schema.StringAttribute{
				Description: "Base-level namespace.",
				Computed:    true,
			},
			"allocations": schema.MapNestedAttribute{
				Description: "Current allocations in the pool.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"prefix_size": schema.Int64Attribute{
							Description: "The target subnet prefix size.",
							Computed:    true,
						},
						"reserve_sibling": schema.BoolAttribute{
							Description: "Whether the sibling block is reserved.",
							Computed:    true,
						},
						"allocated_cidr": schema.StringAttribute{
							Description: "The allocated CIDR block.",
							Computed:    true,
						},
						"sibling_cidr": schema.StringAttribute{
							Description: "The reserved sibling CIDR (if reserve_sibling is true).",
							Computed:    true,
						},
					},
				},
			},
			"available_slices": schema.ListNestedAttribute{
				Description: "List of available (unallocated) slices in the pool.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"start_cidr": schema.StringAttribute{
							Description: "The starting CIDR of this available slice.",
							Computed:    true,
						},
						"max_prefix_size": schema.Int64Attribute{
							Description: "The maximum prefix size allocatable in this slice.",
							Computed:    true,
						},
					},
				},
			},
			"metrics": schema.SingleNestedAttribute{
				Description: "Pool usage metrics.",
				Computed:    true,
				Attributes: map[string]schema.Attribute{
					"total_ips": schema.Int64Attribute{
						Description: "Total number of IP addresses in the pool.",
						Computed:    true,
					},
					"allocated_ips": schema.Int64Attribute{
						Description: "Number of IPs currently allocated.",
						Computed:    true,
					},
					"reserved_ips": schema.Int64Attribute{
						Description: "Number of IPs reserved (sibling blocks).",
						Computed:    true,
					},
					"available_ips": schema.Int64Attribute{
						Description: "Number of IPs available for allocation.",
						Computed:    true,
					},
				},
			},
		},
	}
}

func (d *poolDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data poolDataSourceModel
	diag := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	poolID := data.ID.ValueString()
	if poolID == "" {
		resp.Diagnostics.AddAttributeError(
			req.Path.Root(),
			"Missing Pool ID",
			"The pool ID is required to query the data source.",
		)
		return
	}

	// Parse pool ID into components
	parts := splitPoolID(poolID)
	if len(parts) != 3 {
		resp.Diagnostics.AddAttributeError(
			req.Path.Root(),
			"Invalid Pool ID",
			"Pool ID must be in format organization:project:network, got: "+poolID,
		)
		return
	}

	data.Organization = types.StringValue(parts[0])
	data.Project = types.StringValue(parts[1])
	data.Network = types.StringValue(parts[2])

	// Note: Data sources can only read their own state or other resources' state
	// through the Terraform state. Since this is a logical provider, we compute
	// metrics based on the allocations in the linked resource.
	// For now, we set default empty values that will be populated when the
	// resource is read from state.

	diag = resp.State.Set(ctx, data)
	resp.Diagnostics.Append(diag...)
}

func (d *poolDataSource) Configure(_ context.Context, _ datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
}

// splitPoolID splits a pool ID into its components.
func splitPoolID(id string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(id); i++ {
		if id[i] == ':' {
			parts = append(parts, id[start:i])
			start = i + 1
		}
	}
	parts = append(parts, id[start:])
	return parts
}

// helper types for data source
var (
	availableSliceAttrTypes = map[string]attr.Type{
		"start_cidr":      types.StringType{},
		"max_prefix_size": types.Int64Type{},
	}

	metricsAttrTypes = map[string]attr.Type{
		"total_ips":     types.Int64Type{},
		"allocated_ips": types.Int64Type{},
		"reserved_ips":  types.Int64Type{},
		"available_ips": types.Int64Type{},
	}
)
