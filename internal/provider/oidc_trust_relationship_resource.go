// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/dronenb/terraform-provider-fossa/internal/fossaclient"
	oidctrustgen "github.com/dronenb/terraform-provider-fossa/internal/provider/resource_oidc_trust_relationship"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource              = &OIDCTrustRelationshipResource{}
	_ resource.ResourceWithConfigure = &OIDCTrustRelationshipResource{}
)

// NewOIDCTrustRelationshipResource is a helper function to simplify the provider implementation.
func NewOIDCTrustRelationshipResource() resource.Resource {
	return &OIDCTrustRelationshipResource{}
}

// OIDCTrustRelationshipResource manages a FOSSA OIDC trust relationship.
type OIDCTrustRelationshipResource struct {
	client *fossaclient.APIClient
}

func (r *OIDCTrustRelationshipResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_oidc_trust_relationship"
}

func (r *OIDCTrustRelationshipResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = oidctrustgen.OidcTrustRelationshipResourceSchema(ctx)
}

func (r *OIDCTrustRelationshipResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = apiClientFromResource(req, resp)
}

func (r *OIDCTrustRelationshipResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data oidctrustgen.OidcTrustRelationshipModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	audiences := stringSlice(ctx, data.Audiences, &resp.Diagnostics)
	claims := requiredClaimsRequest(data.RequiredClaims, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	body := fossaclient.NewCreateOIDCTrustRelationshipRequest(
		int32(data.UserId.ValueInt64()),
		int32(data.ProviderId.ValueInt64()),
		data.Scope.ValueString(),
		audiences,
		claims,
	)
	body.ScopeId = int64Pointer(data.ScopeId)

	created, _, err := r.client.OIDCAPI.CreateOIDCTrustRelationship(ctx).CreateOIDCTrustRelationshipRequest(*body).Execute()
	if err != nil {
		resp.Diagnostics.AddError("Client Error", apiError("create OIDC trust relationship", err))
		return
	}

	r.applyTrustRelationshipRead(ctx, &data, &resp.Diagnostics,
		trReadData{
			userID:      created.UserId,
			providerID:  created.ProviderId,
			audiences:   created.Audiences,
			claims:      created.RequiredClaims,
			scope:       stringValue(created.Scope),
			scopeID:     int64Value(created.ScopeId),
			createdAt:   &created.CreatedAt,
			updatedAt:   &created.UpdatedAt,
			hasIdentity: false,
		})
	if resp.Diagnostics.HasError() {
		return
	}
	data.Id = int64Value(&created.Id)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OIDCTrustRelationshipResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data oidctrustgen.OidcTrustRelationshipModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := int32(data.Id.ValueInt64())

	tr, httpResp, err := r.client.OIDCAPI.GetOIDCTrustRelationship(ctx, id).Execute()
	if err != nil {
		if isNotFound(httpResp, err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Client Error", apiError("read OIDC trust relationship", err))
		return
	}

	r.applyTrustRelationshipRead(ctx, &data, &resp.Diagnostics,
		trReadData{
			userID:      tr.UserId,
			providerID:  tr.ProviderId,
			audiences:   tr.Audiences,
			claims:      tr.RequiredClaims,
			scope:       stringValue(tr.Scope),
			scopeID:     int64Value(tr.ScopeId),
			createdAt:   &tr.CreatedAt,
			updatedAt:   &tr.UpdatedAt,
			username:    stringValue(&tr.Username),
			email:       stringValue(tr.Email),
			hasIdentity: true,
		})
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OIDCTrustRelationshipResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data oidctrustgen.OidcTrustRelationshipModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := int32(data.Id.ValueInt64())

	body := fossaclient.NewUpdateOIDCTrustRelationshipRequest()
	body.Audiences = stringSlice(ctx, data.Audiences, &resp.Diagnostics)
	body.RequiredClaims = requiredClaimsRequest(data.RequiredClaims, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, _, err := r.client.OIDCAPI.UpdateOIDCTrustRelationship(ctx, id).UpdateOIDCTrustRelationshipRequest(*body).Execute()
	if err != nil {
		resp.Diagnostics.AddError("Client Error", apiError("update OIDC trust relationship", err))
		return
	}

	r.applyTrustRelationshipRead(ctx, &data, &resp.Diagnostics,
		trReadData{
			userID:      updated.UserId,
			providerID:  updated.ProviderId,
			audiences:   updated.Audiences,
			claims:      updated.RequiredClaims,
			scope:       stringValue(updated.Scope),
			scopeID:     int64Value(updated.ScopeId),
			createdAt:   &updated.CreatedAt,
			updatedAt:   &updated.UpdatedAt,
			hasIdentity: false,
		})
	if resp.Diagnostics.HasError() {
		return
	}
	data.Id = int64Value(&updated.Id)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *OIDCTrustRelationshipResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data oidctrustgen.OidcTrustRelationshipModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	httpResp, err := r.client.OIDCAPI.DeleteOIDCTrustRelationship(ctx, int32(data.Id.ValueInt64())).Execute()
	if err != nil && !isNotFound(httpResp, err) {
		resp.Diagnostics.AddError("Client Error", apiError("delete OIDC trust relationship", err))
		return
	}
}

// trReadData carries the API response fields needed to refresh state.
type trReadData struct {
	userID      int32
	providerID  int32
	audiences   []string
	claims      []fossaclient.ListOIDCTrustRelationships200ResponseResultsInnerRequiredClaimsInner
	scope       basetypes.StringValue
	scopeID     basetypes.Int64Value
	createdAt   *time.Time
	updatedAt   *time.Time
	username    basetypes.StringValue
	email       basetypes.StringValue
	hasIdentity bool
}

// applyTrustRelationshipRead fills attributes on the model from an API response.
func (r *OIDCTrustRelationshipResource) applyTrustRelationshipRead(ctx context.Context, data *oidctrustgen.OidcTrustRelationshipModel, diags *diag.Diagnostics, src trReadData) {
	data.UserId = basetypes.NewInt64Value(int64(src.userID))
	data.ProviderId = basetypes.NewInt64Value(int64(src.providerID))
	data.Scope = src.scope
	data.ScopeId = src.scopeID

	audienceVals := make([]attr.Value, 0, len(src.audiences))
	for _, a := range src.audiences {
		audienceVals = append(audienceVals, basetypes.NewStringValue(a))
	}
	audiencesList, aDiags := basetypes.NewListValue(basetypes.StringType{}, audienceVals)
	diags.Append(aDiags...)
	data.Audiences = audiencesList

	claimVals := make([]attr.Value, 0, len(src.claims))
	for _, c := range src.claims {
		claimVals = append(claimVals, oidctrustgen.NewRequiredClaimsValueMust(
			oidctrustgen.RequiredClaimsValue{}.AttributeTypes(ctx),
			map[string]attr.Value{
				"claim":         basetypes.NewStringValue(c.Claim),
				"value":         basetypes.NewStringValue(c.Value),
				"has_wildcards": boolValue(c.HasWildcards),
			},
		))
	}
	claimsList, cDiags := basetypes.NewListValue(oidctrustgen.RequiredClaimsType{}, claimVals)
	diags.Append(cDiags...)
	data.RequiredClaims = claimsList

	data.CreatedAt = timeStringValue(src.createdAt)
	data.UpdatedAt = timeStringValue(src.updatedAt)
	if src.hasIdentity {
		data.Username = src.username
		data.Email = src.email
	} else {
		data.Username = basetypes.NewStringNull()
		data.Email = basetypes.NewStringNull()
	}
}

// requiredClaimsRequest converts the configured required_claims list into an
// API request slice.
func requiredClaimsRequest(claims types.List, diags *diag.Diagnostics) []fossaclient.ListOIDCTrustRelationships200ResponseResultsInnerRequiredClaimsInner {
	if claims.IsNull() || claims.IsUnknown() {
		return nil
	}

	elems := claims.Elements()

	result := make([]fossaclient.ListOIDCTrustRelationships200ResponseResultsInnerRequiredClaimsInner, 0, len(elems))
	for _, elem := range elems {
		claim, ok := elem.(oidctrustgen.RequiredClaimsValue)
		if !ok {
			diags.AddError("Invalid Attribute Value",
				fmt.Sprintf("Expected required_claims element to be of type oidctrustgen.RequiredClaimsValue, got: %T", elem))
			return nil
		}
		result = append(result, fossaclient.ListOIDCTrustRelationships200ResponseResultsInnerRequiredClaimsInner{
			Claim:        claim.Claim.ValueString(),
			Value:        claim.Value.ValueString(),
			HasWildcards: boolPointer(claim.HasWildcards),
		})
	}

	return result
}
