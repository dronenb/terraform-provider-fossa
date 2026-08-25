// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/dronenb/terraform-provider-fossa/internal/fossaclient"
	teammembersgen "github.com/dronenb/terraform-provider-fossa/internal/provider/resource_team_members"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &TeamMembersResource{}
	_ resource.ResourceWithConfigure = &TeamMembersResource{}
)

// NewTeamMembersResource is a helper function to simplify the provider implementation.
func NewTeamMembersResource() resource.Resource {
	return &TeamMembersResource{}
}

// TeamMembersResource manages the membership of a FOSSA team. Membership is
// fully managed: any user not listed in the configuration will be removed
// from the team.
type TeamMembersResource struct {
	client *fossaclient.APIClient
}

func (r *TeamMembersResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_members"
}

func (r *TeamMembersResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = teammembersgen.TeamMembersResourceSchema(ctx)
}

func (r *TeamMembersResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = apiClientFromResource(req, resp)
}

func (r *TeamMembersResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data teammembersgen.TeamMembersModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := int32(data.Id.ValueInt64())

	body, diags := r.membershipRequestBody(data.Users, "replace")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, err := r.client.TeamsAPI.UpdateTeamUsers(ctx, id).UpdateTeamUsersRequest(*body).Execute()
	if err != nil {
		if isNotFound(httpResp, err) {
			resp.Diagnostics.AddError(
				"Team Not Found",
				"The team was deleted outside of Terraform. Remove it from your configuration or recreate it in FOSSA.",
			)
			return
		}
		resp.Diagnostics.AddError("Client Error", apiError("set team members", err))
		return
	}

	if !r.applyTeamMembersRead(ctx, &data, id, &resp.Diagnostics) {
		if !resp.Diagnostics.HasError() {
			resp.Diagnostics.AddError("Client Error", "Unable to read team after setting members: team disappeared.")
		}
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TeamMembersResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data teammembersgen.TeamMembersModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := int32(data.Id.ValueInt64())

	if !r.applyTeamMembersRead(ctx, &data, id, &resp.Diagnostics) {
		if !resp.Diagnostics.HasError() {
			resp.State.RemoveResource(ctx)
		}
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TeamMembersResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data teammembersgen.TeamMembersModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := int32(data.Id.ValueInt64())

	body, diags := r.membershipRequestBody(data.Users, "replace")
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, httpResp, err := r.client.TeamsAPI.UpdateTeamUsers(ctx, id).UpdateTeamUsersRequest(*body).Execute()
	if err != nil {
		if isNotFound(httpResp, err) {
			resp.Diagnostics.AddError(
				"Team Not Found",
				"The team was deleted outside of Terraform. Remove it from your configuration or recreate it in FOSSA.",
			)
			return
		}
		resp.Diagnostics.AddError("Client Error", apiError("set team members", err))
		return
	}

	if !r.applyTeamMembersRead(ctx, &data, id, &resp.Diagnostics) {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete empties the team so that destroying this resource does not leave
// stale memberships behind.
func (r *TeamMembersResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data teammembersgen.TeamMembersModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := int32(data.Id.ValueInt64())
	body := fossaclient.NewUpdateTeamUsersRequest("replace")

	_, httpResp, err := r.client.TeamsAPI.UpdateTeamUsers(ctx, id).UpdateTeamUsersRequest(*body).Execute()
	if err != nil && !isNotFound(httpResp, err) {
		resp.Diagnostics.AddError("Client Error", apiError("clear team members", err))
		return
	}
}

// applyTeamMembersRead refreshes the members list from the API and returns
// false when the team no longer exists (no diagnostics are added then).
func (r *TeamMembersResource) applyTeamMembersRead(ctx context.Context, data *teammembersgen.TeamMembersModel, id int32, diags *diag.Diagnostics) bool {
	members, httpResp, err := r.client.TeamsAPI.GetTeamMembers(ctx, id).PageSize(defaultTeamMembersPageSize).Execute()
	if err != nil {
		if isNotFound(httpResp, err) {
			return false
		}
		diags.AddError("Client Error", apiError("read team members", err))
		return false
	}

	elements := make([]attr.Value, 0, len(members.Results))
	for _, m := range members.Results {
		elements = append(elements, teammembersgen.NewUsersValueMust(
			teammembersgen.UsersValue{}.AttributeTypes(ctx),
			map[string]attr.Value{
				"id":      basetypes.NewInt64Value(int64(m.UserId)),
				"role_id": basetypes.NewInt64Value(int64(m.RoleId)),
				"user_id": basetypes.NewInt64Null(),
			},
		))
	}

	list, lDiags := basetypes.NewListValue(teammembersgen.UsersType{}, elements)
	diags.Append(lDiags...)
	if diags.HasError() {
		return false
	}
	data.Users = list

	return true
}

// membershipRequestBody converts the configured users list into a PUT body.
func (r *TeamMembersResource) membershipRequestBody(users types.List, action string) (*fossaclient.UpdateTeamUsersRequest, diag.Diagnostics) {
	var diags diag.Diagnostics

	body := fossaclient.NewUpdateTeamUsersRequest(action)

	elems := users.Elements()

	apiUsers := make([]fossaclient.UpdateTeamUsersRequestUsersInner, 0, len(elems))
	for _, elem := range elems {
		user, ok := elem.(teammembersgen.UsersValue)
		if !ok {
			diags.AddError("Invalid Attribute Value",
				fmt.Sprintf("Expected users element to be of type teammembersgen.UsersValue, got: %T", elem))
			return nil, diags
		}
		apiUsers = append(apiUsers, fossaclient.UpdateTeamUsersRequestUsersInner{
			Id:     int32(user.Id.ValueInt64()),
			RoleId: int64Pointer(user.RoleId),
		})
	}
	body.Users = apiUsers

	return body, diags
}
