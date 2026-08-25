// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/dronenb/terraform-provider-fossa/internal/fossaclient"
	rolesds "github.com/dronenb/terraform-provider-fossa/internal/provider/datasource_roles"
	teammembersds "github.com/dronenb/terraform-provider-fossa/internal/provider/datasource_team_members"
	teamsds "github.com/dronenb/terraform-provider-fossa/internal/provider/datasource_teams"
	usersds "github.com/dronenb/terraform-provider-fossa/internal/provider/datasource_users"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// ---------------------------------------------------------------------------
// fossa_teams
// ---------------------------------------------------------------------------

var (
	_ datasource.DataSource              = &TeamsDataSource{}
	_ datasource.DataSourceWithConfigure = &TeamsDataSource{}
)

func NewTeamsDataSource() datasource.DataSource {
	return &TeamsDataSource{}
}

type TeamsDataSource struct {
	client *fossaclient.APIClient
}

func (d *TeamsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_teams"
}

func (d *TeamsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = teamsds.TeamsDataSourceSchema(ctx)
}

func (d *TeamsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = apiClientFromDataSource(req, resp)
}

func (d *TeamsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data teamsds.TeamsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	list, httpResp, err := d.client.TeamsAPI.GetAllTeamsV2(ctx).Execute()
	if err != nil {
		if isNotFound(httpResp, err) {
			return
		}
		resp.Diagnostics.AddError("Client Error", apiError("read teams", err))
		return
	}

	elements := make([]attr.Value, 0, len(list.Results))
	for _, t := range list.Results {
		elements = append(elements, teamsds.NewTeamsValueMust(
			teamsds.TeamsValue{}.AttributeTypes(ctx),
			map[string]attr.Value{
				"auto_add_users":            boolValue(t.AutoAddUsers),
				"created_at":                timeStringValue(t.CreatedAt),
				"default_role_id":           int64Value(t.DefaultRoleId),
				"id":                        int64Value(t.Id),
				"name":                      stringValue(t.Name),
				"organization_id":           int64Value(t.OrganizationId),
				"team_projects_count":       int64Value(t.TeamProjectsCount),
				"team_release_groups_count": int64Value(t.TeamReleaseGroupsCount),
				"team_users":                basetypes.NewListNull(basetypes.ObjectType{AttrTypes: teamsds.TeamsValue{}.AttributeTypes(ctx)}),
				"unique_identifier":         stringValue(t.UniqueIdentifier),
				"updated_at":                timeStringValue(t.UpdatedAt),
			},
		))
	}

	set, sDiags := basetypes.NewSetValue(teamsds.TeamsType{}, elements)
	resp.Diagnostics.Append(sDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Teams = set

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// ---------------------------------------------------------------------------
// fossa_users
// ---------------------------------------------------------------------------

var (
	_ datasource.DataSource              = &UsersDataSource{}
	_ datasource.DataSourceWithConfigure = &UsersDataSource{}
)

func NewUsersDataSource() datasource.DataSource {
	return &UsersDataSource{}
}

type UsersDataSource struct {
	client *fossaclient.APIClient
}

func (d *UsersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_users"
}

func (d *UsersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = usersds.UsersDataSourceSchema(ctx)
}

func (d *UsersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = apiClientFromDataSource(req, resp)
}

func (d *UsersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data usersds.UsersModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	listReq := d.client.UsersAPI.GetAllUsersV2(ctx)
	if !data.Search.IsNull() && !data.Search.IsUnknown() {
		listReq = listReq.Search(data.Search.ValueString())
	}
	if !data.Sort.IsNull() && !data.Sort.IsUnknown() {
		listReq = listReq.Sort(data.Sort.ValueString())
	}

	list, httpResp, err := listReq.Execute()
	if err != nil {
		if isNotFound(httpResp, err) {
			return
		}
		resp.Diagnostics.AddError("Client Error", apiError("read users", err))
		return
	}

	nullObject := func(attrTypes map[string]attr.Type) basetypes.ObjectValue {
		return basetypes.NewObjectNull(attrTypes)
	}
	githubAttrTypes := usersds.GithubValue{}.AttributeTypes(ctx)

	elements := make([]attr.Value, 0, len(list.Results))
	for _, u := range list.Results {
		elements = append(elements, usersds.NewResultsValueMust(
			usersds.ResultsValue{}.AttributeTypes(ctx),
			map[string]attr.Value{
				"bitbucket_cloud":    nullObject(usersds.BitbucketCloudValue{}.AttributeTypes(ctx)),
				"created_at":         timeStringValue(u.CreatedAt),
				"demo":               boolValue(u.Demo),
				"email":              stringValue(u.Email),
				"email_verified":     boolValue(u.EmailVerified),
				"enabled":            boolValue(u.Enabled),
				"full_name":          stringValue(u.FullName),
				"github":             nullObject(githubAttrTypes),
				"has_set_password":   boolValue(u.HasSetPassword),
				"id":                 int64Value(u.Id),
				"install_admin":      boolValue(u.InstallAdmin),
				"is_service_account": boolValue(u.IsServiceAccount),
				"joined":             timeStringValue(u.Joined),
				"last_visit":         timeStringValue(u.LastVisit),
				"organization":       nullObject(usersds.OrganizationValue{}.AttributeTypes(ctx)),
				"organization_id":    int64Value(u.OrganizationId),
				"phone":              stringValue(u.Phone),
				"role":               stringValue(u.Role),
				"sso_only":           boolValue(u.SsoOnly),
				"super":              boolValue(u.Super),
				"teams_count":        int64Value(u.TeamsCount),
				"terms_agreed":       timeStringValue(u.TermsAgreed),
				"tokens":             basetypes.NewListNull(basetypes.ObjectType{AttrTypes: usersds.TokensValue{}.AttributeTypes(ctx)}),
				"updated_at":         timeStringValue(u.UpdatedAt),
				"user_role":          nullObject(usersds.UserRoleValue{}.AttributeTypes(ctx)),
				"username":           stringValue(u.Username),
			},
		))
	}

	resultsList, rDiags := basetypes.NewListValue(usersds.ResultsType{}, elements)
	resp.Diagnostics.Append(rDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Results = resultsList

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// ---------------------------------------------------------------------------
// fossa_roles
// ---------------------------------------------------------------------------

var (
	_ datasource.DataSource              = &RolesDataSource{}
	_ datasource.DataSourceWithConfigure = &RolesDataSource{}
)

func NewRolesDataSource() datasource.DataSource {
	return &RolesDataSource{}
}

type RolesDataSource struct {
	client *fossaclient.APIClient
}

func (d *RolesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_roles"
}

func (d *RolesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = rolesds.RolesDataSourceSchema(ctx)
}

func (d *RolesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = apiClientFromDataSource(req, resp)
}

func (d *RolesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data rolesds.RolesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	roles, httpResp, err := d.client.RolesAPI.GetAllRoles(ctx).Execute()
	if err != nil {
		if isNotFound(httpResp, err) {
			return
		}
		resp.Diagnostics.AddError("Client Error", apiError("read roles", err))
		return
	}

	elements := make([]attr.Value, 0, len(roles))
	for _, role := range roles {
		permVals := make([]attr.Value, 0, len(role.Permissions))
		for _, p := range role.Permissions {
			permVals = append(permVals, rolesds.NewPermissionsValueMust(
				rolesds.PermissionsValue{}.AttributeTypes(ctx),
				map[string]attr.Value{
					"action":        stringValue(p.Action),
					"resource_type": stringValue(p.ResourceType),
				},
			))
		}
		permsList, pDiags := basetypes.NewListValue(rolesds.PermissionsType{}, permVals)
		resp.Diagnostics.Append(pDiags...)
		if resp.Diagnostics.HasError() {
			return
		}

		elements = append(elements, rolesds.NewRolesValueMust(
			rolesds.RolesValue{}.AttributeTypes(ctx),
			map[string]attr.Value{
				"created_at":      timeStringValue(role.CreatedAt),
				"description":     stringValue(role.Description),
				"id":              int64Value(role.Id),
				"is_custom":       boolValue(role.IsCustom),
				"name":            stringValue(role.Name),
				"organization_id": int64Value(role.OrganizationId),
				"permissions":     permsList,
				"scope":           stringValue(role.Scope),
				"updated_at":      timeStringValue(role.UpdatedAt),
			},
		))
	}

	rolesSet, sDiags := basetypes.NewSetValue(rolesds.RolesType{}, elements)
	resp.Diagnostics.Append(sDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Roles = rolesSet

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// ---------------------------------------------------------------------------
// fossa_team_members
// ---------------------------------------------------------------------------

var (
	_ datasource.DataSource              = &TeamMembersDataSource{}
	_ datasource.DataSourceWithConfigure = &TeamMembersDataSource{}
)

func NewTeamMembersDataSource() datasource.DataSource {
	return &TeamMembersDataSource{}
}

type TeamMembersDataSource struct {
	client *fossaclient.APIClient
}

func (d *TeamMembersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_team_members"
}

func (d *TeamMembersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = teammembersds.TeamMembersDataSourceSchema(ctx)
}

func (d *TeamMembersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = apiClientFromDataSource(req, resp)
}

func (d *TeamMembersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data teammembersds.TeamMembersModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := int32(data.Id.ValueInt64())

	listReq := d.client.TeamsAPI.GetTeamMembers(ctx, id).PageSize(defaultTeamMembersPageSize)
	if !data.Search.IsNull() && !data.Search.IsUnknown() {
		listReq = listReq.Search(data.Search.ValueString())
	}

	members, httpResp, err := listReq.Execute()
	if err != nil {
		if isNotFound(httpResp, err) {
			return
		}
		resp.Diagnostics.AddError("Client Error", apiError("read team members", err))
		return
	}

	elements := make([]attr.Value, 0, len(members.Results))
	for _, m := range members.Results {
		elements = append(elements, teammembersds.NewResultsValueMust(
			teammembersds.ResultsValue{}.AttributeTypes(ctx),
			map[string]attr.Value{
				"email":              stringValue(&m.Email),
				"is_service_account": boolValue(m.IsServiceAccount),
				"role_id":            basetypes.NewInt64Value(int64(m.RoleId)),
				"user_id":            basetypes.NewInt64Value(int64(m.UserId)),
				"username":           stringValue(&m.Username),
			},
		))
	}

	resultsList, rDiags := basetypes.NewListValue(teammembersds.ResultsType{}, elements)
	resp.Diagnostics.Append(rDiags...)
	if resp.Diagnostics.HasError() {
		return
	}
	data.Results = resultsList

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
