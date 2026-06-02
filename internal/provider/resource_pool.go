// Copyright (c) HashiCorp and Contributors. All rights reserved.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/finext/terraform-provider-cidrblock/internal/ipam"
)

// Ensure implementation satisfies interfaces.
var (
	_ resource.Resource                = (*poolResource)(nil)
	_ resource.ResourceWithConfigure   = (*poolResource)(nil)
)

// poolResourceModel maps resource schema data.
type poolResourceModel struct {
	ID           types.String `tfsdk:"id"`
	CIDR         types.String `tfsdk:"cidr"`
	Organization types.String `tfsdk:"organization"`
	Project      types.String `tfsdk:"project"`
	Network      types.String `tfsdk:"network"`
	Allocations  types.Map    `tfsdk:"allocations"`
}

// allocationModel maps allocation object attributes.
type allocationModel struct {
	PrefixSize     types.Int64 `tfsdk:"prefix_size"`
	ReserveSibling types.Bool  `tfsdk:"reserve_sibling"`
	AllocatedCIDR  types.String `tfsdk:"allocated_cidr"`
	SiblingCIDR    types.String `tfsdk:"sibling_cidr"`
}

// namespaceValidator validates namespace strings.
var namespaceRegex = regexp.MustCompile(`^[a-zA-Z0-9-_]+$`)

// NewPoolResource returns a cidrblock_pool resource.
func NewPoolResource() resource.Resource {
	return &poolResource{}
}

func (r *poolResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pool"
}

func (r *poolResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a CIDR block pool for IP Address Management (IPAM). " +
			"Tracks, allocates, and recycles subnets from a defined supernet pool.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Unique identifier for the pool (format: organization:project:network).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"cidr": schema.StringAttribute{
				Description: "The base IPv4/IPv6 supernet CIDR (e.g., 10.0.0.0/16 or 2001:db8::/32).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"organization": schema.StringAttribute{
				Description: "Top-level namespace for isolation (e.g., organization or environment). " +
					"Must match ^[a-zA-Z0-9-_]+$ and be 1-64 characters.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					namespaceValidator{},
				},
			},
			"project": schema.StringAttribute{
				Description: "Mid-level namespace for isolation (e.g., project or service). " +
					"Must match ^[a-zA-Z0-9-_]+$ and be 1-64 characters.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					namespaceValidator{},
				},
			},
			"network": schema.StringAttribute{
				Description: "Base-level namespace for isolation (e.g., network name). " +
					"Must match ^[a-zA-Z0-9-_]+$ and be 1-64 characters.",
				Required: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					namespaceValidator{},
				},
			},
			"allocations": schema.MapNestedAttribute{
				Description: "Map of named allocations. Key is the allocation name, value is the allocation configuration.",
				Optional:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"prefix_size": schema.Int64Attribute{
							Description: "The target subnet prefix size (e.g., 24 for IPv4 /24, 64 for IPv6 /64).",
							Required:    true,
						},
						"reserve_sibling": schema.BoolAttribute{
							Description: "If true, reserves the adjacent binary sibling block for future expansion. Default: false.",
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
						},
						"allocated_cidr": schema.StringAttribute{
							Description: "The computed CIDR block allocated to this entry.",
							Computed:    true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"sibling_cidr": schema.StringAttribute{
							Description: "The reserved sibling CIDR block (only set when reserve_sibling is true).",
							Computed:    true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
					},
				},
			},
		},
	}
}

func (r *poolResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan poolResourceModel
	diag := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Generate pool ID
	poolID := plan.Organization.ValueString() + ":" +
		plan.Project.ValueString() + ":" +
		plan.Network.ValueString()
	plan.ID = types.StringValue(poolID)

	// Initialize IPAM engine
	eng, err := ipam.NewEngine(plan.CIDR.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid CIDR", err.Error())
		return
	}

	// Process allocations
	allocMap, diag := plan.Allocations.ToElements(ctx)
	if diag.HasError() {
		resp.Diagnostics.Append(diag...)
		return
	}

	resultMap := make(map[string][]attr.Value)
	allocElements := plan.Allocations.Elements()

	for key, elem := range allocElements {
		allocObj := elem.(types.Object)

		prefixSizeAttr, _ := allocObj.Attributes()["prefix_size"]
		prefixSize := prefixSizeAttr.(types.Int64).ValueInt64()

		reserveSiblingAttr, _ := allocObj.Attributes()["reserve_sibling"]
		reserveSibling := reserveSiblingAttr.(types.Bool).ValueBool()

		allocatedCIDR, err := eng.Allocate(key, int(prefixSize), reserveSibling)
		if err != nil {
			resp.Diagnostics.AddError("Allocation failed",
				"Failed to allocate "+key+": "+err.Error())
			return
		}

		// Get the allocation state
		alloc, _ := eng.GetAllocation(key)

		// Build result element
		elemValues := []attr.Value{
			types.Int64Value(prefixSize),
			types.BoolValue(reserveSibling),
			types.StringValue(alloc.AllocatedCIDR),
			types.StringValue(alloc.SiblingCIDR),
		}
		resultMap[key] = elemValues
	}

	// Build result map
	elemType := types.TupleType{
		AttrTypes: []attr.Type{
			types.Int64Type{},
			types.BoolType{},
			types.StringType{},
			types.StringType{},
		},
	}

	resultMapType := types.MapType{
		ElemType: types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"prefix_size":     types.Int64Type{},
				"reserve_sibling": types.BoolType{},
				"allocated_cidr":  types.StringType{},
				"sibling_cidr":    types.StringType{},
			},
		},
	}

	// Convert back to map
	newAllocs := make(map[string]types.Object)
	for key, values := range resultMap {
		obj, d := types.ObjectValue(
			map[string]attr.Type{
				"prefix_size":     types.Int64Type{},
				"reserve_sibling": types.BoolType{},
				"allocated_cidr":  types.StringType{},
				"sibling_cidr":    types.StringType{},
			},
			map[string]attr.Value{
				"prefix_size":     values[0],
				"reserve_sibling": values[1],
				"allocated_cidr":  values[2],
				"sibling_cidr":    values[3],
			},
		)
		diag = d
		if diag.HasError() {
			resp.Diagnostics.Append(diag...)
			return
		}
		newAllocs[key] = obj
	}

	resultMapVal, d := resultMapType.ValueFrom(ctx, newAllocs)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.Allocations = resultMapVal.(types.Map)

	diag = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diag...)
}

func (r *poolResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state poolResourceModel
	diag := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	// For a logical provider, state is authoritative.
	// No external API to check against.
}

func (r *poolResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan poolResourceModel
	diag := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state poolResourceModel
	diag = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Initialize IPAM engine
	eng, err := ipam.NewEngine(plan.CIDR.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid CIDR", err.Error())
		return
	}

	// Build current state map
	stateAllocs := make(map[string]ipam.Allocation)
	if !state.Allocations.IsNull() && !state.Allocations.IsUnknown() {
		for key, elem := range state.Allocations.Elements() {
			allocObj := elem.(types.Object)
			prefixSizeAttr, _ := allocObj.Attributes()["prefix_size"]
			reserveSiblingAttr, _ := allocObj.Attributes()["reserve_sibling"]
			stateAllocs[key] = ipam.Allocation{
				PrefixSize:     int(prefixSizeAttr.(types.Int64).ValueInt64()),
				ReserveSibling: reserveSiblingAttr.(types.Bool).ValueBool(),
			}
		}
	}

	// Load existing allocations into engine
	for key, alloc := range stateAllocs {
		eng.Allocate(key, alloc.PrefixSize, alloc.ReserveSibling)
	}

	// Process plan allocations
	resultMap := make(map[string]types.Object)

	for key, elem := range plan.Allocations.Elements() {
		allocObj := elem.(types.Object)

		prefixSizeAttr, _ := allocObj.Attributes()["prefix_size"]
		prefixSize := prefixSizeAttr.(types.Int64).ValueInt64()

		reserveSiblingAttr, _ := allocObj.Attributes()["reserve_sibling"]
		reserveSibling := reserveSiblingAttr.(types.Bool).ValueBool()

		// Check if this is an existing allocation being updated
		if existing, ok := stateAllocs[key]; ok {
			// Update existing
			if err := eng.UpdateAllocation(key, int(prefixSize), reserveSibling); err != nil {
				resp.Diagnostics.AddError("Update allocation failed",
					"Failed to update "+key+": "+err.Error())
				return
			}
		} else {
			// New allocation
			_, err := eng.Allocate(key, int(prefixSize), reserveSibling)
			if err != nil {
				resp.Diagnostics.AddError("Allocation failed",
					"Failed to allocate "+key+": "+err.Error())
				return
			}
		}

		alloc, _ := eng.GetAllocation(key)

		obj, d := types.ObjectValue(
			map[string]attr.Type{
				"prefix_size":     types.Int64Type{},
				"reserve_sibling": types.BoolType{},
				"allocated_cidr":  types.StringType{},
				"sibling_cidr":    types.StringType{},
			},
			map[string]attr.Value{
				"prefix_size":     types.Int64Value(prefixSize),
				"reserve_sibling": types.BoolValue(reserveSibling),
				"allocated_cidr":  types.StringValue(alloc.AllocatedCIDR),
				"sibling_cidr":    types.StringValue(alloc.SiblingCIDR),
			},
		)
		resp.Diagnostics.Append(d...)
		if resp.Diagnostics.HasError() {
			return
		}
		resultMap[key] = obj
	}

	// Free removed allocations
	for key := range stateAllocs {
		if _, exists := resultMap[key]; !exists {
			eng.Free(key)
		}
	}

	resultMapVal, d := types.MapValue(
		types.ObjectType{
			AttrTypes: map[string]attr.Type{
				"prefix_size":     types.Int64Type{},
				"reserve_sibling": types.BoolType{},
				"allocated_cidr":  types.StringType{},
				"sibling_cidr":    types.StringType{},
			},
		},
		resultMap,
	)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.Allocations = resultMapVal
	diag = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diag...)
}

func (r *poolResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state poolResourceModel
	diag := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diag...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Clear state
	resp.State.RemoveResource(ctx)
}

func (r *poolResource) Configure(_ context.Context, _ resource.ConfigureRequest, _ *resource.ConfigureResponse) {
}

// namespaceValidator implements string validation for namespace strings.
type namespaceValidator struct{}

func (v namespaceValidator) Description(_ context.Context) string {
	return "Namespace must contain only alphanumeric characters, hyphens, and underscores (1-64 characters)."
}

func (v namespaceValidator) MarkdownDescription(_ context.Context) string {
	return "Must match `^[a-zA-Z0-9-_]+$` and be 1-64 characters."
}

func (v namespaceValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	input := req.ConfigValue.ValueString()

	if input == "" {
		return
	}

	if len(input) > 64 {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Namespace Length",
			"Namespace must be 1-64 characters, got "+string(rune(len(input)))+".",
		)
		return
	}

	if !namespaceRegex.MatchString(input) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid Namespace Format",
			"Namespace must match ^[a-zA-Z0-9-_]+$, got: "+input,
		)
		return
	}
}

// allocationElementTypes defines the allocation object attribute types.
func allocationElementTypes() map[string]attr.Type {
	return map[string]attr.Type{
		"prefix_size":     types.Int64Type{},
		"reserve_sibling": types.BoolType{},
		"allocated_cidr":  types.StringType{},
		"sibling_cidr":    types.StringType{},
	}
}

// helper to get allocation values from a map element.
func getAllocationValues(ctx context.Context, elem types.Object) (int64, bool, error) {
	prefixSizeAttr, _ := elem.Attributes()["prefix_size"]
	reserveSiblingAttr, _ := elem.Attributes()["reserve_sibling"]

	prefixSize := prefixSizeAttr.(types.Int64).ValueInt64()
	reserveSibling := reserveSiblingAttr.(types.Bool).ValueBool()

	return prefixSize, reserveSibling, nil
}

// unused import guard
var _ = path.MatchRoot
