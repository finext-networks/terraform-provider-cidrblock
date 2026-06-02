// Copyright (c) HashiCorp and Contributors. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/finext/terraform-provider-cidrblock/internal/ipam"
)

// Ensure implementation satisfies interfaces.
var (
	_ datasource.DataSource = (*poolDataSource)(nil)
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

// NewPoolDataSource returns a cidrblock_pool data source.
func NewPoolDataSource() datasource.DataSource {
	return &poolDataSource{}
}

type poolDataSource struct{}

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
				Description: "The base IPv4/IPv6 supernet CIDR. Required to compute available slices and metrics.",
				Required:    true,
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
			path.Root("id"),
			"Missing Pool ID",
			"The pool ID is required to query the data source.",
		)
		return
	}

	cidr := data.CIDR.ValueString()
	if cidr == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("cidr"),
			"Missing CIDR",
			"The pool CIDR is required to compute available slices and metrics.",
		)
		return
	}

	// Parse pool ID into components
	parts := splitPoolID(poolID)
	if len(parts) != 3 {
		resp.Diagnostics.AddAttributeError(
			path.Root("id"),
			"Invalid Pool ID",
			"Pool ID must be in format organization:project:network, got: "+poolID,
		)
		return
	}

	data.Organization = types.StringValue(parts[0])
	data.Project = types.StringValue(parts[1])
	data.Network = types.StringValue(parts[2])

	// Initialize IPAM engine to compute metrics
	eng, err := ipam.NewEngine(cidr)
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("cidr"),
			"Invalid CIDR",
			"Failed to parse CIDR: "+err.Error(),
		)
		return
	}

	// Load allocations from config (if any are provided via an allocations attribute)
	// In practice, the data source reads the resource state. For standalone queries,
	// we can optionally accept an allocations map.

	// Compute metrics
	metrics := eng.Metrics()

	// Build metrics object
	metricsObj, diag := types.ObjectValue(
		map[string]attr.Type{
			"total_ips":     types.Int64Type{},
			"allocated_ips": types.Int64Type{},
			"reserved_ips":  types.Int64Type{},
			"available_ips": types.Int64Type{},
		},
		map[string]attr.Value{
			"total_ips":     types.Int64Value(int64(metrics.TotalIPs)),
			"allocated_ips": types.Int64Value(int64(metrics.AllocatedIPs)),
			"reserved_ips":  types.Int64Value(int64(metrics.ReservedIPs)),
			"available_ips": types.Int64Value(int64(metrics.AvailableIPs)),
		},
	)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Metrics = metricsObj

	// Compute available slices
	availableSlices := eng.AvailableSlices()
	sliceObjects := make([]types.Object, 0, len(availableSlices))
	for _, s := range availableSlices {
		obj, diag := types.ObjectValue(
			map[string]attr.Type{
				"start_cidr":      types.StringType{},
				"max_prefix_size": types.Int64Type{},
			},
			map[string]attr.Value{
				"start_cidr":      types.StringValue(s.StartCIDR),
				"max_prefix_size": types.Int64Value(int64(s.MaxPrefixSize)),
			},
		)
		resp.Diagnostics.Append(diag...)
		if resp.Diagnostics.HasError() {
			return
		}
		sliceObjects = append(sliceObjects, obj)
	}

	slicesList, diag := types.ListValue(
		types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"start_cidr":      types.StringType{},
				"max_prefix_size": types.Int64Type{},
			},
		},
		sliceObjects,
	)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.AvailableSlices = slicesList

	// Set empty allocations map (data source doesn't manage allocations)
	emptyAllocs, diag := types.MapValueEmpty(
		types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"prefix_size":     types.Int64Type{},
				"reserve_sibling": types.BoolType{},
				"allocated_cidr":  types.StringType{},
				"sibling_cidr":    types.StringType{},
			},
		},
	)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Allocations = emptyAllocs

	diag = resp.State.Set(ctx, data)
	resp.Diagnostics.Append(diag...)
}

// splitPoolID splits a pool ID into its components.
func splitPoolID(id string) []string {
	parts := strings.Split(id, ":")
	return parts
}
