// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"

	"github.com/dronenb/terraform-provider-fossa/internal/fossaclient"
	oidcprovidergen "github.com/dronenb/terraform-provider-fossa/internal/provider/resource_oidc_provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &OIDCProviderResource{}
	_ resource.ResourceWithConfigure = &OIDCProviderResource{}
)

// NewOIDCProviderResource is a helper function to simplify the provider implementation.
func NewOIDCProviderResource() resource.Resource {
	return &OIDCProviderResource{}
}

// OIDCProviderResource manages a FOSSA OIDC provider.
type OIDCProviderResource struct {
	client *fossaclient.APIClient
}

func (r *OIDCProviderResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oidc_provider"
}

func (r *OIDCProviderResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = oidcprovidergen.OidcProviderResourceSchema(ctx)
}

func (r *OIDCProviderResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = apiClientFromResource(req, resp)
}

func (r *OIDCProviderResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data oidcprovidergen.OidcProviderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := fossaclient.NewCreateOIDCProviderRequest(
		data.Issuer.ValueString(),
		data.Scope.ValueString(),
	)
	body.ScopeId = int64Pointer(data.ScopeId)

	created, _, err := r.client.OIDCAPI.CreateOIDCProvider(ctx).CreateOIDCProviderRequest(*body).Execute()
	if err != nil {
		resp.Diagnostics.AddError("Client Error", apiError("create OIDC provider", err))
		return
	}

	r.applyOIDCProviderRead(&data, created)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OIDCProviderResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data oidcprovidergen.OidcProviderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := int32(data.Id.ValueInt64())

	provider, httpResp, err := r.client.OIDCAPI.GetOIDCProvider(ctx, id).Execute()
	if err != nil {
		if isNotFound(httpResp, err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", apiError("read OIDC provider", err))
		return
	}

	r.applyOIDCProviderRead(&data, provider)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update reports that OIDC providers cannot be updated in place: the FOSSA
// API does not expose an update operation. Use `terraform apply -replace` to
// recreate the provider.
func (r *OIDCProviderResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"OIDC Provider Cannot Be Updated In Place",
		"The FOSSA API does not support updating OIDC providers. To change this provider, "+
			"recreate it with: terraform apply -replace="+req.Config.Raw.String(),
	)
}

// Delete removes the OIDC provider.
func (r *OIDCProviderResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data oidcprovidergen.OidcProviderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	httpResp, err := r.client.OIDCAPI.DeleteOIDCProvider(ctx, int32(data.Id.ValueInt64())).Execute()
	if err != nil && !isNotFound(httpResp, err) {
		resp.Diagnostics.AddError("Client Error", apiError("delete OIDC provider", err))
		return
	}
}

func (r *OIDCProviderResource) applyOIDCProviderRead(data *oidcprovidergen.OidcProviderModel, provider *fossaclient.CreateOIDCProvider201Response) {
	data.Id = int64Value(&provider.Id)
	data.OrganizationId = int64Value(&provider.OrganizationId)
	data.Issuer = stringValue(&provider.Issuer)
	data.Scope = stringValue(&provider.Scope)
	data.ScopeId = int64Value(&provider.ScopeId)
	data.CreatedAt = timeStringValue(&provider.CreatedAt)
	data.UpdatedAt = timeStringValue(&provider.UpdatedAt)
}
