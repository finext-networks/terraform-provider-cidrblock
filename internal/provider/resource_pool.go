// Copyright (c) Finext Networks. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/finext-networks/terraform-provider-cidrblock/internal/ipam"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*poolResource)(nil)
	_ resource.ResourceWithImportState = (*poolResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*poolResource)(nil) // Enforces collection-aware plan modification lifecycle
)

type poolResource struct {
	preventSubnetDestruction bool
}

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
				Description: "The unique composite namespace identifier for the allocation pool managed by the provider engine.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cidr": schema.StringAttribute{
				Description: "The master base CIDR block assigned to this routing zone (e.g., 10.0.0.0/16). Modifying this forces pool replacement.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"organization": schema.StringAttribute{
				Description: "The top-level business organization name owning the network domain namespace context.",
				Required:    true,
			},
			"project": schema.StringAttribute{
				Description: "The infrastructure engineering project target scope linking the underlying networks.",
				Required:    true,
			},
			"network": schema.StringAttribute{
				Description: "The descriptive segment name identifier for the network footprint.",
				Required:    true,
			},
			"allocation_strategy": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Algorithmic layout search strategy choice used when packing blocks (FIRST, BEST, SPARSE). Defaults to FIRST.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"allocations": schema.MapNestedAttribute{
				Description: "The complete map of nested subnets to algorithmically pack inside the master pool bounds.",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"prefix_size": schema.Int64Attribute{
							Description: "The IPv4 or IPv6 subnet mask bit length representing the allocation size.",
							Required:    true,
						},
						"reserve_sibling": schema.BoolAttribute{
							Description: "Toggle to automatically reserve an adjacent mathematical buddy block. Defaults to false if omitted.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
						},
						"allocated_cidr": schema.StringAttribute{
							Description: "The calculated base network address block computed by the IPAM engine.",
							Computed:    true,
							// Static plan modifiers removed: handled dynamically via collection-aware ModifyPlan
						},
						"sibling_cidr": schema.StringAttribute{
							Description: "The calculated companion network reservation footprint computed by the IPAM engine.",
							Computed:    true,
							// Static plan modifiers removed: handled dynamically via collection-aware ModifyPlan
						},
					},
				},
			},
		},
	}
}

// ModifyPlan manages granular changes across map attributes without introducing plan-vs-apply semantic gaps.
func (r *poolResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// Use pointers so the framework can cleanly assign 'nil' if the entire State (on Create) or Plan (on Destroy) is Null
	var state *poolResourceModel
	var plan *poolResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// If the resource is being created fresh (state is null) or destroyed (plan is null), pass through standard mutations
	if state == nil || plan == nil {
		return
	}

	// Bypass logical patching if allocation boundaries are null or unassigned
	if state.Allocations.IsNull() || state.Allocations.IsUnknown() || plan.Allocations.IsNull() || plan.Allocations.IsUnknown() {
		return
	}

	var stateAllocs map[string]allocationModel
	var planAllocs map[string]allocationModel

	resp.Diagnostics.Append(state.Allocations.ElementsAs(ctx, &stateAllocs, false)...)
	resp.Diagnostics.Append(plan.Allocations.ElementsAs(ctx, &planAllocs, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	allocObjectType := types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"prefix_size":     types.Int64Type,
			"reserve_sibling": types.BoolType,
			"allocated_cidr":  types.StringType,
			"sibling_cidr":    types.StringType,
		},
	}

	resultElements := make(map[string]attr.Value)

	for k, planAlloc := range planAllocs {
		stateAlloc, exists := stateAllocs[k]

		if exists {
			// If configuration input states are identical, reuse old state coordinates to suppress plan diff noise
			if planAlloc.PrefixSize.ValueInt64() == stateAlloc.PrefixSize.ValueInt64() &&
				planAlloc.ReserveSibling.ValueBool() == stateAlloc.ReserveSibling.ValueBool() {
				planAlloc.AllocatedCIDR = stateAlloc.AllocatedCIDR
				planAlloc.SiblingCIDR = stateAlloc.SiblingCIDR
			} else {
				// Inputs changed: Force computation back to Unknown to accurately render (known after apply)
				planAlloc.AllocatedCIDR = types.StringUnknown()
				planAlloc.SiblingCIDR = types.StringUnknown()
			}
		} else {
			// Key is brand new: Maintain dynamic evaluation states so the execution pipeline compiles accurately
			planAlloc.AllocatedCIDR = types.StringUnknown()
			planAlloc.SiblingCIDR = types.StringUnknown()
		}

		objVal, diags := types.ObjectValue(allocObjectType.AttrTypes, map[string]attr.Value{
			"prefix_size":     planAlloc.PrefixSize,
			"reserve_sibling": planAlloc.ReserveSibling,
			"allocated_cidr":  planAlloc.AllocatedCIDR,
			"sibling_cidr":    planAlloc.SiblingCIDR,
		})
		resp.Diagnostics.Append(diags...)
		resultElements[k] = objVal
	}

	mapVal, diags := types.MapValue(allocObjectType, resultElements)
	resp.Diagnostics.Append(diags...)
	plan.Allocations = mapVal

	resp.Diagnostics.Append(resp.Plan.Set(ctx, plan)...)
}

func (r *poolResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	prevent, ok := req.ProviderData.(bool)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data Type",
			fmt.Sprintf("Expected a boolean provider data token, got: %T", req.ProviderData),
		)
		return
	}
	r.preventSubnetDestruction = prevent
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

	keys := make([]string, 0, len(planAllocs))
	for k := range planAllocs {
		keys = append(keys, k)
	}

	// ADVANCED SORTING: Sort keys by network size descending (largest block footprints first)
	// to implement First-Fit Decreasing alignment packing and fully eliminate dead-zone fragmentation.
	// Ties on identical sizing revert to alphabetical order for strict state determinism.
	sort.SliceStable(keys, func(i, j int) bool {
		sizeI := planAllocs[keys[i]].PrefixSize.ValueInt64()
		sizeJ := planAllocs[keys[j]].PrefixSize.ValueInt64()
		if sizeI == sizeJ {
			return keys[i] < keys[j]
		}
		return sizeI < sizeJ // Smaller prefix bit size implies a larger mathematical address block width
	})

	for _, k := range keys {
		v := planAllocs[k]
		cidr, err := eng.Allocate(k, int(v.PrefixSize.ValueInt64()), v.ReserveSibling.ValueBool(), strat)
		if err != nil {
			resp.Diagnostics.AddError("Allocation Failed", fmt.Sprintf("Key %s: %s", k, err.Error()))
			return
		}

		stateRecord, _ := eng.GetAllocation(k)

		// Map structural empty strings from the IPAM engine out to explicit Framework Null values
		// to enforce structural compliance with pre-computed planner layout shapes.
		siblingVal := types.StringNull()
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

	// Broad-scale address mutations require updating the synchronized global thread-safe registry
	// to expose the newly mapped network map to read-only data source consumers.
	registryAllocs := make(map[string]RegistryAllocation)
	for _, k := range keys {
		stateRecord, _ := eng.GetAllocation(k)
		registryAllocs[k] = RegistryAllocation{
			PrefixSize:     int64(stateRecord.PrefixSize),
			ReserveSibling: stateRecord.ReserveSibling,
			AllocatedCIDR:  stateRecord.AllocatedCIDR,
			SiblingCIDR:    stateRecord.SiblingCIDR,
		}
	}
	PublishPoolState(org+":"+proj+":"+netw+":"+plan.CIDR.ValueString(), RegistryPool{
		CIDR:               plan.CIDR.ValueString(),
		AllocationStrategy: plan.AllocationStrategy.ValueString(),
		Allocations:        registryAllocs,
	})

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

	if state.Organization.IsNull() || state.Organization.IsUnknown() || state.CIDR.IsNull() || state.CIDR.IsUnknown() {
		parts := strings.SplitN(state.ID.ValueString(), ":", 4)
		if len(parts) == 4 {
			state.Organization = types.StringValue(parts[0])
			state.Project = types.StringValue(parts[1])
			state.Network = types.StringValue(parts[2])
			state.CIDR = types.StringValue(parts[3])
		}
	}

	// --- Hydrate Global Registry for Read-Only Data Source Consumption ---
	// This ensures subsequent plans or multi-resource apply lifecycles possess
	// fully populated allocation matrices when new plugin processes are spawned.
	if !state.Allocations.IsNull() && !state.Allocations.IsUnknown() {
		var stateAllocs map[string]allocationModel
		diags := state.Allocations.ElementsAs(ctx, &stateAllocs, false)
		if !diags.HasError() {
			registryAllocs := make(map[string]RegistryAllocation)
			for k, v := range stateAllocs {
				registryAllocs[k] = RegistryAllocation{
					PrefixSize:     v.PrefixSize.ValueInt64(),
					ReserveSibling: v.ReserveSibling.ValueBool(),
					AllocatedCIDR:  v.AllocatedCIDR.ValueString(),
					SiblingCIDR:    v.SiblingCIDR.ValueString(),
				}
			}

			strat := "FIRST"
			if !state.AllocationStrategy.IsNull() && !state.AllocationStrategy.IsUnknown() {
				strat = state.AllocationStrategy.ValueString()
			}

			PublishPoolState(
				state.Organization.ValueString()+":"+state.Project.ValueString()+":"+state.Network.ValueString()+":"+state.CIDR.ValueString(),
				RegistryPool{
					CIDR:               state.CIDR.ValueString(),
					AllocationStrategy: strat,
					Allocations:        registryAllocs,
				},
			)
		}
	}
	// ---------------------------------------------------------------------

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

	// INTERCEPTION GUARDRAIL: If provider safety is active, block attempts to drop subnets from the map
	if r.preventSubnetDestruction {
		for k := range stateAllocs {
			if _, exists := planAllocs[k]; !exists {
				resp.Diagnostics.AddError(
					"Subnet Destruction Blocked",
					fmt.Sprintf("Allocation key %q was removed from the HCL map configuration. The provider safety flag 'prevent_subnet_destruction' is active and blocks this deletion to protect downstream cloud network interfaces.", k),
				)
				return
			}
		}
	}

	for k, v := range stateAllocs {
		if v.AllocatedCIDR.IsNull() || v.AllocatedCIDR.IsUnknown() {
			continue
		}
		eng.RegisterExistingAllocation(k, &ipam.Allocation{
			PrefixSize:     int(v.PrefixSize.ValueInt64()),
			ReserveSibling: v.ReserveSibling.ValueBool(),
			AllocatedCIDR:  v.AllocatedCIDR.ValueString(),
			SiblingCIDR:    v.SiblingCIDR.ValueString(),
		})
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

	// ADVANCED SORTING: Enforce identical size-descending and alphabetical tie-breaker logic
	// during updates to guarantee immutable stability across state hydration runs.
	sort.SliceStable(keys, func(i, j int) bool {
		sizeI := planAllocs[keys[i]].PrefixSize.ValueInt64()
		sizeJ := planAllocs[keys[j]].PrefixSize.ValueInt64()
		if sizeI == sizeJ {
			return keys[i] < keys[j]
		}
		return sizeI < sizeJ
	})

	for _, k := range keys {
		v := planAllocs[k]

		if _, err := eng.GetAllocation(k); err == nil {
			err = eng.UpdateAllocation(k, int(v.PrefixSize.ValueInt64()), v.ReserveSibling.ValueBool(), strat)
			if err != nil {
				resp.Diagnostics.AddError("Pool Allocation Update Failed", fmt.Sprintf("Key %s: %s", k, err.Error()))
				return
			}

			updated, _ := eng.GetAllocation(k)

			// Maintain layout synchronization by mapping empty sibling blocks to clear framework nulls
			siblingVal := types.StringNull()
			if updated.SiblingCIDR != "" {
				siblingVal = types.StringValue(updated.SiblingCIDR)
			}

			objVal, diags := types.ObjectValue(allocObjectType.AttrTypes, map[string]attr.Value{
				"prefix_size":     types.Int64Value(int64(updated.PrefixSize)),
				"reserve_sibling": types.BoolValue(updated.ReserveSibling),
				"allocated_cidr":  types.StringValue(updated.AllocatedCIDR),
				"sibling_cidr":    siblingVal,
			})
			resp.Diagnostics.Append(diags...)
			resultElements[k] = objVal
			continue
		}

		cidr, err := eng.Allocate(k, int(v.PrefixSize.ValueInt64()), v.ReserveSibling.ValueBool(), strat)
		if err != nil {
			resp.Diagnostics.AddError("Pool Allocation Failed", fmt.Sprintf("Key %s: %s", k, err.Error()))
			return
		}

		stateRecord, _ := eng.GetAllocation(k)

		// Maintain layout synchronization by mapping empty sibling blocks to clear framework nulls
		siblingVal := types.StringNull()
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

	// Update updates require the exact same registration serialization format to ensure
	// state mutations propagate flawlessly across back-to-back lifecycle reads.
	registryAllocs := make(map[string]RegistryAllocation)
	for _, k := range keys {
		stateRecord, _ := eng.GetAllocation(k)
		registryAllocs[k] = RegistryAllocation{
			PrefixSize:     int64(stateRecord.PrefixSize),
			ReserveSibling: stateRecord.ReserveSibling,
			AllocatedCIDR:  stateRecord.AllocatedCIDR,
			SiblingCIDR:    stateRecord.SiblingCIDR,
		}
	}
	PublishPoolState(plan.Organization.ValueString()+":"+plan.Project.ValueString()+":"+plan.Network.ValueString()+":"+plan.CIDR.ValueString(), RegistryPool{
		CIDR:               plan.CIDR.ValueString(),
		AllocationStrategy: plan.AllocationStrategy.ValueString(),
		Allocations:        registryAllocs,
	})

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
}

func (r *poolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
