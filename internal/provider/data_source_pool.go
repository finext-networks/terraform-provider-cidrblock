// Copyright (c) Finext Networks. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"

	"github.com/finext-networks/terraform-provider-cidrblock/internal/ipam"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource = (*poolDataSource)(nil)
)

// poolDataSourceModel details tracking values exposed by the read-only data source block.
type poolDataSourceModel struct {
	ID                 types.String `tfsdk:"id"`
	CIDR               types.String `tfsdk:"cidr"`
	Organization       types.String `tfsdk:"organization"`
	Project            types.String `tfsdk:"project"`
	Network            types.String `tfsdk:"network"`
	AllocationStrategy types.String `tfsdk:"allocation_strategy"`
	Allocations        types.Map    `tfsdk:"allocations"`
	AvailableSlices    types.List   `tfsdk:"available_slices"`
	Metrics            types.Object `tfsdk:"metrics"`
}

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
				Description: "The calculated top-level business organization name parsed from the composite identifier token.",
				Computed:    true,
			},
			"project": schema.StringAttribute{
				Description: "The calculated infrastructure engineering project target scope parsed from the composite identifier token.",
				Computed:    true,
			},
			"network": schema.StringAttribute{
				Description: "The calculated segment name identifier parsed from the composite identifier token.",
				Computed:    true,
			},
			"allocation_strategy": schema.StringAttribute{
				Description: "The default or calculated algorithmic layout search criteria strategy active on the pool.",
				Computed:    true,
			},
			"allocations": schema.MapNestedAttribute{
				Description: "The complete map of nested subnet block structures packed inside this master routing container context.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"prefix_size": schema.Int64Attribute{
							Description: "The bit mask subnet size tier assigned to the allocation block.",
							Computed:    true,
						},
						"reserve_sibling": schema.BoolAttribute{
							Description: "Indicates whether an adjacent mathematical shadow buddy block was explicitly requested.",
							Computed:    true,
						},
						"allocated_cidr": schema.StringAttribute{
							Description: "The calculated absolute base network address coordinate calculated by the distribution logic.",
							Computed:    true,
						},
						"sibling_cidr": schema.StringAttribute{
							Description: "The calculated backup shadow companion reservation coordinate calculated by the distribution logic.",
							Computed:    true,
						},
					},
				},
			},
			"available_slices": schema.ListNestedAttribute{
				Description: "The serialized listing of all remaining contiguous unallocated address blocks discovered inside the grid matrix.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"start_cidr": schema.StringAttribute{
							Description: "The network base prefix boundary coordinate where the open space chunk begins.",
							Computed:    true,
						},
						"max_prefix_size": schema.Int64Attribute{
							Description: "The maximum individual bit mask length that can fit cleanly inside this isolated open slice.",
							Computed:    true,
						},
					},
				},
			},
			"metrics": schema.SingleNestedAttribute{
				Description: "The statistical usage summary metrics detailing total capacity distributions calculated across the matrix.",
				Computed:    true,
				Attributes: map[string]schema.Attribute{
					"total_ips": schema.Int64Attribute{
						Description: "The raw count of total possible address spaces contained globally across the supernet.",
						Computed:    true,
					},
					"allocated_ips": schema.Int64Attribute{
						Description: "The raw count of addresses consumed inside user-defined allocation blocks.",
						Computed:    true,
					},
					"reserved_ips": schema.Int64Attribute{
						Description: "The raw count of addresses consumed inside active buddy-block backup sibling allocations.",
						Computed:    true,
					},
					"available_ips": schema.Int64Attribute{
						Description: "The total residual capacity of freely assignable addresses remaining in the supernet bounds.",
						Computed:    true,
					},
				},
			},
		},
	}
}

func (d *poolDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data poolDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	poolID := data.ID.ValueString()
	cidr := data.CIDR.ValueString()

	if poolID == "" || cidr == "" {
		resp.Diagnostics.AddError("Missing Parameters", "Both ID and CIDR inputs must be provided.")
		return
	}

	// Deconstruct the synthetic token ID to rehydrate configuration attributes
	parts := strings.Split(poolID, ":")
	if len(parts) < 3 {
		resp.Diagnostics.AddAttributeError(path.Root("id"), "Invalid Pool ID", "Must contain organization:project:network")
		return
	}

	data.Organization = types.StringValue(parts[0])
	data.Project = types.StringValue(parts[1])
	data.Network = types.StringValue(parts[2])
	data.AllocationStrategy = types.StringValue("FIRST")

	eng, err := ipam.NewEngine(cidr)
	if err != nil {
		resp.Diagnostics.AddError("Engine Creation Failed", err.Error())
		return
	}

	// 1. Process and bind pool utilization math metrics objects
	m := eng.Metrics()
	metricTypes := map[string]attr.Type{
		"total_ips":     types.Int64Type,
		"allocated_ips": types.Int64Type,
		"reserved_ips":  types.Int64Type,
		"available_ips": types.Int64Type,
	}

	metricsObj, diags := types.ObjectValue(metricTypes, map[string]attr.Value{
		"total_ips":     types.Int64Value(int64(m.TotalIPs)),
		"allocated_ips": types.Int64Value(int64(m.AllocatedIPs)),
		"reserved_ips":  types.Int64Value(int64(m.ReservedIPs)),
		"available_ips": types.Int64Value(int64(m.AvailableIPs)),
	})
	resp.Diagnostics.Append(diags...)
	data.Metrics = metricsObj

	// 2. Fetch and serialize structural contiguous address spaces available lists
	slices := eng.AvailableSlices()
	sliceAttrTypes := map[string]attr.Type{
		"start_cidr":      types.StringType,
		"max_prefix_size": types.Int64Type,
	}

	sliceValues := make([]attr.Value, 0, len(slices))
	for _, s := range slices {
		objVal, diags := types.ObjectValue(sliceAttrTypes, map[string]attr.Value{
			"start_cidr":      types.StringValue(s.StartCIDR),
			"max_prefix_size": types.Int64Value(int64(s.MaxPrefixSize)),
		})
		resp.Diagnostics.Append(diags...)
		sliceValues = append(sliceValues, objVal)
	}

	listVal, diags := types.ListValue(types.ObjectType{AttrTypes: sliceAttrTypes}, sliceValues)
	resp.Diagnostics.Append(diags...)
	data.AvailableSlices = listVal

	// 3. Instantiate an empty allocation map token baseline
	allocTypes := map[string]attr.Type{
		"prefix_size":     types.Int64Type,
		"reserve_sibling": types.BoolType,
		"allocated_cidr":  types.StringType,
		"sibling_cidr":    types.StringType,
	}

	mapVal, diags := types.MapValue(types.ObjectType{AttrTypes: allocTypes}, map[string]attr.Value{})
	resp.Diagnostics.Append(diags...)
	data.Allocations = mapVal

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
