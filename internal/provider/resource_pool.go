// Copyright (c) HashiCorp and Contributors. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/finext/terraform-provider-cidrblock/internal/ipam"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*poolResource)(nil)
	_ resource.ResourceWithImportState = (*poolResource)(nil)
)

type poolResource struct{}

// poolResourceModel establishes configuration map layout fields for the framework schema.
type poolResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	CIDR               types.String `tfsdk:"cidr"`
	Organization       types.String `tfsdk:"organization"`
	Project            types.String `tfsdk:"project"`
	Network            types.String `tfsdk:"network"`
	AllocationStrategy types.String `tfsdk:"allocation_strategy"`
	Allocations        types.Map    `tfsdk:"allocations"`
}

// allocationModel builds structure schemas matching internal map metrics attributes.
type allocationModel struct {
	PrefixSize     types.Int64  `tfsdk:"prefix_size"`
	ReserveSibling types.Bool   `tfsdk:"reserve_sibling"`
	AllocatedCIDR  types.String `tfsdk:"allocated_cidr"`
	SiblingCIDR    types.String `tfsdk:"sibling_cidr"`
}

// namespaceRegex prevents colon collisions inside composite identifier strings
var namespaceRegex = regexp.MustCompile(`^[a-zA-Z0-9-_]+$`)

func NewPoolResource() resource.Resource {
	return &poolResource{}
}

func (r *poolResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pool"
}

func (r *poolResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages an atomic IP address prefix pool allocation layout grid.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cidr": schema.StringAttribute{
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"organization": schema.StringAttribute{
				Required: true,
			},
			"project": schema.StringAttribute{
				Required: true,
			},
			"network": schema.StringAttribute{
				Required: true,
			},
			"allocation_strategy": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Algorithmic layout search strategy choice (FIRST, BEST, SPARSE). Defaults to FIRST.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"allocations": schema.MapNestedAttribute{
				Optional: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"prefix_size": schema.Int64Attribute{
							Required: true,
						},
						"reserve_sibling": schema.BoolAttribute{
							Optional: true,
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
		},
	}
}

func (r *poolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan poolResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	org := plan.Organization.ValueString()
	proj := plan.Project.ValueString()
	netw := plan.Network.ValueString()

	if !namespaceRegex.MatchString(org) || !namespaceRegex.MatchString(proj) || !namespaceRegex.MatchString(netw) {
		resp.Diagnostics.AddError("Invalid Namespace Boundary", "Allowed characters: alphanumeric, hyphens, and underscores.")
		return
	}

	// ID builds structural synthetic keys required by Terraform lifecycle interfaces
	plan.ID = types.StringValue(org + ":" + proj + ":" + netw + ":" + plan.CIDR.ValueString())

	eng, err := ipam.NewEngine(plan.CIDR.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid Base Pool Prefix", err.Error())
		return
	}

	strat := ipam.StrategyFirst
	if !plan.AllocationStrategy.IsNull() && !plan.AllocationStrategy.IsUnknown() {
		strat = ipam.Strategy(plan.AllocationStrategy.ValueString())
	} else {
		plan.AllocationStrategy = types.StringValue(string(strat))
	}

	var planAllocs map[string]allocationModel
	if !plan.Allocations.IsNull() && !plan.Allocations.IsUnknown() {
		resp.Diagnostics.Append(plan.Allocations.ElementsAs(ctx, &planAllocs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	} else {
		planAllocs = make(map[string]allocationModel)
	}

	resultElements := make(map[string]attr.Value)
	allocObjectType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"prefix_size":     types.Int64Type,
			"reserve_sibling": types.BoolType,
			"allocated_cidr":  types.StringType,
			"sibling_cidr":    types.StringType,
		},
	}

	// Process keys in alphabetical order to maintain state file determinism
	keys := make([]string, 0, len(planAllocs))
	for k := range planAllocs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := planAllocs[k]
		cidr, err := eng.Allocate(k, int(v.PrefixSize.ValueInt64()), v.ReserveSibling.ValueBool(), strat)
		if err != nil {
			resp.Diagnostics.AddError("Allocation Failed", fmt.Sprintf("Key %s: %s", k, err.Error()))
			return
		}

		stateRecord, _ := eng.GetAllocation(k)
		siblingVal := types.StringValue("")
		if stateRecord.SiblingCIDR != "" {
			siblingVal = types.StringValue(stateRecord.SiblingCIDR)
		}

		objVal, diags := types.ObjectValue(allocObjectType.AttrTypes, map[string]attr.Value{
			"prefix_size":     types.Int64Value(int64(stateRecord.PrefixSize)),
			"reserve_sibling": types.BoolValue(stateRecord.ReserveSibling),
			"allocated_cidr":  types.StringValue(cidr),
			"sibling_cidr":    siblingVal,
		})
		resp.Diagnostics.Append(diags...)
		resultElements[k] = objVal
	}

	// Fix: Null map checks maintain exact model symmetry, eliminating inconsistent state panics
	if plan.Allocations.IsNull() {
		plan.Allocations = types.MapNull(allocObjectType)
	} else {
		mapVal, diags := types.MapValue(allocObjectType, resultElements)
		resp.Diagnostics.Append(diags...)
		plan.Allocations = mapVal
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *poolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state poolResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Rehydrate state attributes from composite IDs during logical imports
	if state.Organization.IsNull() || state.Organization.IsUnknown() || state.CIDR.IsNull() || state.CIDR.IsUnknown() {
		parts := strings.SplitN(state.ID.ValueString(), ":", 4)
		if len(parts) == 4 {
			state.Organization = types.StringValue(parts[0])
			state.Project = types.StringValue(parts[1])
			state.Network = types.StringValue(parts[2])
			state.CIDR = types.StringValue(parts[3])
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *poolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan poolResourceModel
	var state poolResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	eng, _ := ipam.NewEngine(plan.CIDR.ValueString())

	strat := ipam.StrategyFirst
	if !plan.AllocationStrategy.IsNull() && !plan.AllocationStrategy.IsUnknown() {
		strat = ipam.Strategy(plan.AllocationStrategy.ValueString())
	} else {
		plan.AllocationStrategy = types.StringValue(string(strat))
	}

	var stateAllocs map[string]allocationModel
	if !state.Allocations.IsNull() && !state.Allocations.IsUnknown() {
		resp.Diagnostics.Append(state.Allocations.ElementsAs(ctx, &stateAllocs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	} else {
		stateAllocs = make(map[string]allocationModel)
	}

	var planAllocs map[string]allocationModel
	if !plan.Allocations.IsNull() && !plan.Allocations.IsUnknown() {
		resp.Diagnostics.Append(plan.Allocations.ElementsAs(ctx, &planAllocs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	} else {
		planAllocs = make(map[string]allocationModel)
	}

	// Re-register static allocations to prevent shifts across update evaluations
	for k, v := range stateAllocs {
		if v.AllocatedCIDR.IsNull() || v.AllocatedCIDR.IsUnknown() {
			continue
		}

		planVal, existsInPlan := planAllocs[k]
		if existsInPlan && planVal.PrefixSize.ValueInt64() == v.PrefixSize.ValueInt64() {
			eng.RegisterExistingAllocation(k, &ipam.Allocation{
				PrefixSize:     int(v.PrefixSize.ValueInt64()),
				ReserveSibling: v.ReserveSibling.ValueBool(),
				AllocatedCIDR:  v.AllocatedCIDR.ValueString(),
				SiblingCIDR:    v.SiblingCIDR.ValueString(),
			})
		}
	}

	resultElements := make(map[string]attr.Value)
	allocObjectType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"prefix_size":     types.Int64Type,
			"reserve_sibling": types.BoolType,
			"allocated_cidr":  types.StringType,
			"sibling_cidr":    types.StringType,
		},
	}

	keys := make([]string, 0, len(planAllocs))
	for k := range planAllocs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := planAllocs[k]

		// Execute inline modification if the specific key token already exists
		if existing, err := eng.GetAllocation(k); err == nil && int64(existing.PrefixSize) == v.PrefixSize.ValueInt64() {
			if v.ReserveSibling.ValueBool() != existing.ReserveSibling {
				err = eng.UpdateAllocation(k, int(v.PrefixSize.ValueInt64()), v.ReserveSibling.ValueBool(), strat)
				if err != nil {
					resp.Diagnostics.AddError("Sibling Reservation Modification Failed", err.Error())
					return
				}
				existing, _ = eng.GetAllocation(k)
			}

			siblingVal := types.StringValue("")
			if existing.SiblingCIDR != "" {
				siblingVal = types.StringValue(existing.SiblingCIDR)
			}

			objVal, diags := types.ObjectValue(allocObjectType.AttrTypes, map[string]attr.Value{
				"prefix_size":     types.Int64Value(int64(existing.PrefixSize)),
				"reserve_sibling": types.BoolValue(existing.ReserveSibling),
				"allocated_cidr":  types.StringValue(existing.AllocatedCIDR),
				"sibling_cidr":    siblingVal,
			})
			resp.Diagnostics.Append(diags...)
			resultElements[k] = objVal
			continue
		}

		// Otherwise, process as a new subnet entry allocation step
		cidr, err := eng.Allocate(k, int(v.PrefixSize.ValueInt64()), v.ReserveSibling.ValueBool(), strat)
		if err != nil {
			resp.Diagnostics.AddError("Pool Allocation Failed", fmt.Sprintf("Key %s: %s", k, err.Error()))
			return
		}

		stateRecord, _ := eng.GetAllocation(k)
		siblingVal := types.StringValue("")
		if stateRecord.SiblingCIDR != "" {
			siblingVal = types.StringValue(stateRecord.SiblingCIDR)
		}

		objVal, diags := types.ObjectValue(allocObjectType.AttrTypes, map[string]attr.Value{
			"prefix_size":     types.Int64Value(int64(stateRecord.PrefixSize)),
			"reserve_sibling": types.BoolValue(stateRecord.ReserveSibling),
			"allocated_cidr":  types.StringValue(cidr),
			"sibling_cidr":    siblingVal,
		})
		resp.Diagnostics.Append(diags...)
		resultElements[k] = objVal
	}

	// Fix: Ensure null updates maintain correct structural configuration formatting
	if plan.Allocations.IsNull() {
		plan.Allocations = types.MapNull(allocObjectType)
	} else {
		mapVal, diags := types.MapValue(allocObjectType, resultElements)
		resp.Diagnostics.Append(diags...)
		plan.Allocations = mapVal
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *poolResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
	// Pure logical component requires no remote state teardown orchestration API sweeps
}

func (r *poolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

