// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/dronenb/terraform-provider-fossa/internal/fossaclient"
	serviceaccountgen "github.com/dronenb/terraform-provider-fossa/internal/provider/resource_service_account"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &ServiceAccountResource{}
	_ resource.ResourceWithConfigure = &ServiceAccountResource{}
)

// NewServiceAccountResource is a helper function to simplify the provider implementation.
func NewServiceAccountResource() resource.Resource {
	return &ServiceAccountResource{}
}

// ServiceAccountResource manages a FOSSA service account.
//
// The FOSSA API does not support updating service accounts. Changes to
// username, email, or full_name require replacement of the whole account.
type ServiceAccountResource struct {
	client *fossaclient.APIClient
}

func (r *ServiceAccountResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_account"
}

func (r *ServiceAccountResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = serviceaccountgen.ServiceAccountResourceSchema(ctx)
}

func (r *ServiceAccountResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = apiClientFromResource(req, resp)
}

func (r *ServiceAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data serviceaccountgen.ServiceAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := fossaclient.NewCreateServiceAccountRequest(data.Username.ValueString())
	body.Email = stringPointer(data.Email)
	body.FullName = stringPointer(data.FullName)
	body.OrgRoleId = int64Pointer(data.OrgRoleId)
	body.HasPushOnlyApiToken = boolPointer(data.HasPushOnlyApiToken)
	body.HasFullApiToken = boolPointer(data.HasFullApiToken)
	if !data.Team.IsNull() && !data.Team.IsUnknown() {
		team := fossaclient.NewCreateServiceAccountRequestTeam(
			int32(data.Team.Id.ValueInt64()),
			int32(data.Team.RoleId.ValueInt64()),
		)
		body.Team = team
	}

	created, _, err := r.client.UsersAPI.CreateServiceAccount(ctx).CreateServiceAccountRequest(*body).Execute()
	if err != nil {
		resp.Diagnostics.AddError("Client Error", apiError("create service account", err))
		return
	}

	data.Id = types.Int64Value(int64(created.Id))
	data.Username = stringValue(&created.Username)
	data.Email = stringValue(created.Email)
	data.FullName = stringValue(created.FullName)
	data.FullApiToken = stringValue(created.FullApiToken)
	data.PushOnlyApiToken = stringValue(created.PushOnlyApiToken)
	data.IsServiceAccount = basetypes.NewBoolValue(true)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ServiceAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data serviceaccountgen.ServiceAccountModel

	// Preserve attributes that are not returned by the user read endpoint:
	// configured role/team flags and one-time API tokens.
	var priorTokens [2]types.String
	var priorOrgRoleID types.Int64
	var priorTeam serviceaccountgen.TeamValue
	preserve := func(src serviceaccountgen.ServiceAccountModel) {
		priorTokens[0] = src.FullApiToken
		priorTokens[1] = src.PushOnlyApiToken
		priorOrgRoleID = src.OrgRoleId
		priorTeam = src.Team
	}

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	preserve(data)

	user, httpResp, err := r.client.UsersAPI.GetUser(ctx, int32(data.Id.ValueInt64())).Execute()
	if err != nil {
		if isNotFound(httpResp, err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", apiError("read service account", err))
		return
	}

	data.Id = int64Value(user.Id)
	data.Username = stringValue(user.Username)
	data.Email = stringValue(user.Email)
	data.FullName = stringValue(user.FullName)
	data.OrganizationId = int64Value(user.OrganizationId)
	if user.IsServiceAccount != nil {
		data.IsServiceAccount = boolValue(user.IsServiceAccount)
	}

	// Restore non-readable attributes.
	if data.FullApiToken.IsNull() {
		data.FullApiToken = priorTokens[0]
	}
	if data.PushOnlyApiToken.IsNull() {
		data.PushOnlyApiToken = priorTokens[1]
	}
	if data.OrgRoleId.IsNull() {
		data.OrgRoleId = priorOrgRoleID
	}
	if data.Team.IsNull() && !priorTeam.IsNull() {
		data.Team = priorTeam
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update reports that service accounts cannot be updated in place: the FOSSA
// API does not expose an update operation. Use `terraform apply -replace` to
// recreate the account (which also rotates its API tokens).
func (r *ServiceAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"Service Account Cannot Be Updated In Place",
		"The FOSSA API does not support updating service accounts. To change this service account, "+
			"recreate it with: terraform apply -replace="+req.Config.Raw.String(),
	)
}

// Delete removes the service account via the user delete endpoint.
func (r *ServiceAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data serviceaccountgen.ServiceAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	httpResp, err := r.client.UsersAPI.DeleteUser(ctx, int32(data.Id.ValueInt64())).Execute()
	if err != nil && !isNotFound(httpResp, err) {
		resp.Diagnostics.AddError("Client Error", apiError("delete service account", err))
		return
	}
}
