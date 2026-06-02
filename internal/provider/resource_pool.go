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

// poolResourceModel utilizes explicit framework types to ensure stable protocol serialization.
type poolResourceModel struct {
	ID           types.String `tfsdk:"id"`
	CIDR         types.String `tfsdk:"cidr"`
	Organization types.String `tfsdk:"organization"`
	Project      types.String `tfsdk:"project"`
	Network      types.String `tfsdk:"network"`
	Allocations  types.Map    `tfsdk:"allocations"`
}

type allocationModel struct {
	PrefixSize     types.Int64  `tfsdk:"prefix_size"`
	ReserveSibling types.Bool   `tfsdk:"reserve_sibling"`
	AllocatedCIDR  types.String `tfsdk:"allocated_cidr"`
	SiblingCIDR    types.String `tfsdk:"sibling_cidr"`
}

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

	// Fix: Incorporate the CIDR into the ID to make the logical resource fully import-safe
	plan.ID = types.StringValue(org + ":" + proj + ":" + netw + ":" + plan.CIDR.ValueString())

	eng, err := ipam.NewEngine(plan.CIDR.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid Base Pool Prefix", err.Error())
		return
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
	sort.Strings(keys)

	for _, k := range keys {
		v := planAllocs[k]
		cidr, err := eng.Allocate(k, int(v.PrefixSize.ValueInt64()), v.ReserveSibling.ValueBool())
		if err != nil {
			resp.Diagnostics.AddError("Allocation Failed", fmt.Sprintf("Key %s: %s", k, err.Error()))
			return
		}

		stateRecord, _ := eng.GetAllocation(k)
		
		// Fix: Use an explicit empty string instead of Null to keep test assertions stable
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

	mapVal, diags := types.MapValue(allocObjectType, resultElements)
	resp.Diagnostics.Append(diags...)
	plan.Allocations = mapVal

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *poolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state poolResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Fix: Parse the 4-part ID using a split limit N=4 to handle IPv6 colons safely during imports
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

	// 1. Stateful Hydration: Pre-seed unaltered allocations straight from state memory
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

	// 2. Process all allocations and updates deterministically using sorted keys
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

		// Real-World Cloud Expansion Rule: If a user modifies a sibling reservation, 
		// execute an inline mutation update while keeping the baseline gateway CIDR stable.
		if existing, err := eng.GetAllocation(k); err == nil && int64(existing.PrefixSize) == v.PrefixSize.ValueInt64() {
			if v.ReserveSibling.ValueBool() != existing.ReserveSibling {
				err = eng.UpdateAllocation(k, int(v.PrefixSize.ValueInt64()), v.ReserveSibling.ValueBool())
				if err != nil {
					resp.Diagnostics.AddError("Sibling Reservation Modification Failed", err.Error())
					return
				}
				existing, _ = eng.GetAllocation(k)
			}

			// Fix: Use an explicit empty string instead of Null to keep test assertions stable
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

		// Otherwise, allocate a clean forward-aligned gap for new or expanded block configurations
		cidr, err := eng.Allocate(k, int(v.PrefixSize.ValueInt64()), v.ReserveSibling.ValueBool())
		if err != nil {
			resp.Diagnostics.AddError("Pool Allocation Failed", fmt.Sprintf("Key %s: %s", k, err.Error()))
			return
		}

		stateRecord, _ := eng.GetAllocation(k)
		
		// Fix: Use an explicit empty string instead of Null to keep test assertions stable
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

	mapVal, diags := types.MapValue(allocObjectType, resultElements)
	resp.Diagnostics.Append(diags...)
	plan.Allocations = mapVal

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *poolResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r *poolResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

