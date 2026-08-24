// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/dronenb/terraform-provider-fossa/internal/fossaclient"
	teamgen "github.com/dronenb/terraform-provider-fossa/internal/provider/resource_team"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &TeamResource{}
	_ resource.ResourceWithConfigure = &TeamResource{}
)

// NewTeamResource is a helper function to simplify the provider implementation.
func NewTeamResource() resource.Resource {
	return &TeamResource{}
}

// TeamResource manages a FOSSA team.
type TeamResource struct {
	client *fossaclient.APIClient
}

func (r *TeamResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team"
}

func (r *TeamResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = teamgen.TeamResourceSchema(ctx)
}

func (r *TeamResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = apiClientFromResource(req, resp)
}

func (r *TeamResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data teamgen.TeamModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := fossaclient.NewCreateTeamRequest(data.Name.ValueString(), int32(data.DefaultRoleId.ValueInt64()))
	body.AutoAddUsers = boolPointer(data.AutoAddUsers)
	body.UniqueIdentifier = stringPointer(data.UniqueIdentifier)

	created, _, err := r.client.TeamsAPI.CreateTeam(ctx).CreateTeamRequest(*body).Execute()
	if err != nil {
		resp.Diagnostics.AddError("Client Error", apiError("create team", err))
		return
	}

	data.Id = types.Int64Value(int64(*created.Id))
	if !r.applyTeamRead(ctx, &data, *created.Id, &resp.Diagnostics) {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TeamResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data teamgen.TeamModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.applyTeamRead(ctx, &data, int32(data.Id.ValueInt64()), &resp.Diagnostics) {
		if !resp.Diagnostics.HasError() {
			resp.State.RemoveResource(ctx)
		}
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TeamResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data teamgen.TeamModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := int32(data.Id.ValueInt64())

	body := fossaclient.NewUpdateTeamRequest()
	body.Name = stringPointer(data.Name)
	body.AutoAddUsers = boolPointer(data.AutoAddUsers)
	body.DefaultRoleId = int64Pointer(data.DefaultRoleId)
	body.UniqueIdentifier = stringPointer(data.UniqueIdentifier)

	_, _, err := r.client.TeamsAPI.UpdateTeam(ctx, id).UpdateTeamRequest(*body).Execute()
	if err != nil {
		resp.Diagnostics.AddError("Client Error", apiError("update team", err))
		return
	}

	if !r.applyTeamRead(ctx, &data, id, &resp.Diagnostics) {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TeamResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data teamgen.TeamModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	httpResp, err := r.client.TeamsAPI.DeleteTeam(ctx, int32(data.Id.ValueInt64())).Execute()
	if err != nil && !isNotFound(httpResp, err) {
		resp.Diagnostics.AddError("Client Error", apiError("delete team", err))
		return
	}
}

// applyTeamRead refreshes attributes from the API and returns false when the
// team no longer exists (in which case no diagnostics are added).
func (r *TeamResource) applyTeamRead(ctx context.Context, data *teamgen.TeamModel, id int32, diags *diag.Diagnostics) bool {
	team, httpResp, err := r.client.TeamsAPI.GetTeamByIdV2(ctx, id).Execute()
	if err != nil {
		if isNotFound(httpResp, err) {
			return false
		}
		diags.AddError("Client Error", apiError("read team", err))
		return false
	}

	data.Id = int64Value(team.Id)
	data.OrganizationId = int64Value(team.OrganizationId)
	data.Name = stringValue(team.Name)
	data.DefaultRoleId = int64Value(team.DefaultRoleId)
	data.AutoAddUsers = boolValue(team.AutoAddUsers)
	data.UniqueIdentifier = stringValue(team.UniqueIdentifier)
	data.CreatedAt = timeStringValue(team.CreatedAt)
	data.UpdatedAt = timeStringValue(team.UpdatedAt)
	data.TeamType = stringValue(team.TeamType)
	data.TeamMembersCount = int64Value(team.TeamMembersCount)
	data.TeamOidcprovidersCount = int64Value(team.TeamOIDCProvidersCount)
	data.TeamProjectsCount = int64Value(team.TeamProjectsCount)
	data.TeamReleaseGroupsCount = int64Value(team.TeamReleaseGroupsCount)

	return true
}
