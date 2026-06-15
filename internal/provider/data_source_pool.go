// Copyright (c) Finext Networks. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"strings"
	"sync"

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

// RegistryAllocation maps layout specs natively for memory serialization.
type RegistryAllocation struct {
	PrefixSize     int64
	ReserveSibling bool
	AllocatedCIDR  string
	SiblingCIDR    string
}

// RegistryPool aggregates master pool specs across lifecycle boundaries.
type RegistryPool struct {
	CIDR               string
	AllocationStrategy string
	Allocations        map[string]RegistryAllocation
}

var (
	poolRegistryMu sync.RWMutex
	poolRegistry   = make(map[string]RegistryPool)
)

// PublishPoolState allows mutable resources to broadcast allocation topologies.
func PublishPoolState(registryKey string, pool RegistryPool) {
	poolRegistryMu.Lock()
	defer poolRegistryMu.Unlock()
	poolRegistry[registryKey] = pool
}

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
				Description: "The unique composite identifier string for the pool. Optional if organization, project, and network are provided.",
				Optional:    true,
				Computed:    true,
			},
			"cidr": schema.StringAttribute{
				Description: "The base IPv4/IPv6 supernet CIDR block assigned to this routing container. Required if 'id' is omitted.",
				Optional:    true,
				Computed:    true,
			},
			"organization": schema.StringAttribute{
				Description: "The top-level business organization name owning the network domain context.",
				Optional:    true,
				Computed:    true,
			},
			"project": schema.StringAttribute{
				Description: "The infrastructure engineering project target scope.",
				Optional:    true,
				Computed:    true,
			},
			"network": schema.StringAttribute{
				Description: "The descriptive segment name identifier for the network grid.",
				Optional:    true,
				Computed:    true,
			},
			"allocation_strategy": schema.StringAttribute{
				Description: "The active search criteria packing algorithm assigned to this pool context (FIRST, BEST, SPARSE).",
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

	var registryKey string
	var cidrStr string

	// Check if a non-empty composite ID token string has been explicitly provided
	idProvided := !data.ID.IsNull() && !data.ID.IsUnknown() && data.ID.ValueString() != ""

	if idProvided {
		parts := strings.Split(data.ID.ValueString(), ":")
		if len(parts) < 4 {
			resp.Diagnostics.AddAttributeError(path.Root("id"), "Invalid Pool ID", "Must conform to standard 4-part format: organization:project:network:cidr")
			return
		}
		// Capture the exact 4-part identifier from the composite token split
		registryKey = parts[0] + ":" + parts[1] + ":" + parts[2] + ":" + parts[3]
		cidrStr = parts[3]

		// If an explicit CIDR configuration attribute is also provided, prioritize it to validate formatting
		if !data.CIDR.IsNull() && !data.CIDR.IsUnknown() && data.CIDR.ValueString() != "" {
			if data.CIDR.ValueString() != cidrStr {
				cidrStr = data.CIDR.ValueString()
			}
		}

		data.Organization = types.StringValue(parts[0])
		data.Project = types.StringValue(parts[1])
		data.Network = types.StringValue(parts[2])
		data.CIDR = types.StringValue(cidrStr)
	} else {
		// Discrete attributes route: Ensure all required parameters are non-empty and present
		if data.Organization.IsNull() || data.Organization.IsUnknown() || data.Organization.ValueString() == "" ||
			data.Project.IsNull() || data.Project.IsUnknown() || data.Project.ValueString() == "" ||
			data.Network.IsNull() || data.Network.IsUnknown() || data.Network.ValueString() == "" ||
			data.CIDR.IsNull() || data.CIDR.IsUnknown() || data.CIDR.ValueString() == "" {
			resp.Diagnostics.AddError(
				"Missing Parameters",
				"To execute a pool query, you must either provide a non-empty composite token 'id' OR all four matching discrete coordinates ('organization', 'project', 'network', 'cidr').",
			)
			return
		}

		cidrStr = data.CIDR.ValueString()
		registryKey = data.Organization.ValueString() + ":" + data.Project.ValueString() + ":" + data.Network.ValueString() + ":" + cidrStr
		data.ID = types.StringValue(registryKey)
	}

	var strategyStr string
	var allocRecords map[string]RegistryAllocation

	// Query the global memory pool using the comprehensive 4-part key contract
	poolRegistryMu.RLock()
	pool, exists := poolRegistry[registryKey]
	poolRegistryMu.RUnlock()

	if exists {
		strategyStr = pool.AllocationStrategy
		allocRecords = pool.Allocations
	} else {
		// Clean fallback protection for early cold-start planning phases
		strategyStr = "FIRST"
		allocRecords = make(map[string]RegistryAllocation)
	}

	data.AllocationStrategy = types.StringValue(strategyStr)

	eng, err := ipam.NewEngine(cidrStr)
	if err != nil {
		resp.Diagnostics.AddError("Engine Creation Failed", err.Error())
		return
	}

	// Rehydrate the isolated calculation engine with live parameters found in the shared store
	for k, v := range allocRecords {
		eng.RegisterExistingAllocation(k, &ipam.Allocation{
			PrefixSize:     int(v.PrefixSize),
			ReserveSibling: v.ReserveSibling,
			AllocatedCIDR:  v.AllocatedCIDR,
			SiblingCIDR:    v.SiblingCIDR,
		})
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

	// 3. Hydrate and map live allocations back into the data source state collection
	allocTypes := map[string]attr.Type{
		"prefix_size":     types.Int64Type,
		"reserve_sibling": types.BoolType,
		"allocated_cidr":  types.StringType,
		"sibling_cidr":    types.StringType,
	}

	resultElements := make(map[string]attr.Value)
	for k, v := range allocRecords {
		siblingVal := types.StringNull()
		if v.SiblingCIDR != "" {
			siblingVal = types.StringValue(v.SiblingCIDR)
		}

		objVal, diags := types.ObjectValue(allocTypes, map[string]attr.Value{
			"prefix_size":     types.Int64Value(v.PrefixSize),
			"reserve_sibling": types.BoolValue(v.ReserveSibling),
			"allocated_cidr":  types.StringValue(v.AllocatedCIDR),
			"sibling_cidr":    siblingVal,
		})
		resp.Diagnostics.Append(diags...)
		resultElements[k] = objVal
	}

	mapVal, diags := types.MapValue(types.ObjectType{AttrTypes: allocTypes}, resultElements)
	resp.Diagnostics.Append(diags...)
	data.Allocations = mapVal

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
