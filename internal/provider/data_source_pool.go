// Copyright (c) HashiCorp and Contributors. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"

	"github.com/finext/terraform-provider-cidrblock/internal/ipam"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource = (*poolDataSource)(nil)
)

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
							Computed: true,
						},
						"reserve_sibling": schema.BoolAttribute{
							Computed: true,
						},
						"allocated_cidr": schema.StringAttribute{
							Computed: true,
						},
						"sibling_cidr": schema.StringAttribute{
							Computed: true,
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
							Computed: true,
						},
						"max_prefix_size": schema.Int64Attribute{
							Computed: true,
						},
					},
				},
			},
			"metrics": schema.SingleNestedAttribute{
				Description: "Pool usage metrics.",
				Computed:    true,
				Attributes: map[string]schema.Attribute{
					"total_ips": schema.Int64Attribute{
						Computed: true,
					},
					"allocated_ips": schema.Int64Attribute{
						Computed: true,
					},
					"reserved_ips": schema.Int64Attribute{
						Computed: true,
					},
					"available_ips": schema.Int64Attribute{
						Computed: true,
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

	parts := strings.Split(poolID, ":")
	if len(parts) != 3 {
		resp.Diagnostics.AddAttributeError(path.Root("id"), "Invalid Pool ID", "Must be organization:project:network")
		return
	}

	data.Organization = types.StringValue(parts[0])
	data.Project = types.StringValue(parts[1])
	data.Network = types.StringValue(parts[2])

	eng, err := ipam.NewEngine(cidr)
	if err != nil {
		resp.Diagnostics.AddError("Engine Creation Failed", err.Error())
		return
	}

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

