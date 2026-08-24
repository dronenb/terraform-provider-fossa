// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/dronenb/terraform-provider-fossa/internal/fossaclient"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// defaultTeamMembersPageSize is the page size used when reading full team
// membership lists in a single request.
const defaultTeamMembersPageSize = 1000

// apiClient extracts the shared FOSSA API client configured by the provider.
func apiClientFromResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *fossaclient.APIClient {
	if req.ProviderData == nil {
		return nil
	}

	client, ok := req.ProviderData.(*fossaclient.APIClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *fossaclient.APIClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return nil
	}

	return client
}

func apiClientFromDataSource(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *fossaclient.APIClient {
	if req.ProviderData == nil {
		return nil
	}

	client, ok := req.ProviderData.(*fossaclient.APIClient)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *fossaclient.APIClient, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return nil
	}

	return client
}

// apiError converts an API error into a provider diagnostic message.
func apiError(action string, err error) string {
	return fmt.Sprintf("Unable to %s: %s", action, err.Error())
}

// isNotFound reports whether the API response indicates the entity is gone.
func isNotFound(resp *http.Response, err error) bool {
	if resp == nil || err == nil {
		return false
	}
	return resp.StatusCode == http.StatusNotFound
}

func timeStringValue(t *time.Time) basetypes.StringValue {
	if t == nil || t.IsZero() {
		return basetypes.NewStringNull()
	}
	return basetypes.NewStringValue(t.Format(time.RFC3339))
}

func int64Pointer(v basetypes.Int64Value) *int32 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	i := int32(v.ValueInt64())
	return &i
}

func int64Value(v *int32) basetypes.Int64Value {
	if v == nil {
		return basetypes.NewInt64Null()
	}
	return basetypes.NewInt64Value(int64(*v))
}

func stringPointer(v basetypes.StringValue) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

func stringValue(v *string) basetypes.StringValue {
	if v == nil {
		return basetypes.NewStringNull()
	}
	return basetypes.NewStringValue(*v)
}

func boolPointer(v basetypes.BoolValue) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

func boolValue(v *bool) basetypes.BoolValue {
	if v == nil {
		return basetypes.NewBoolNull()
	}
	return basetypes.NewBoolValue(*v)
}

// stringSlice converts a list of strings attribute into a Go string slice.
func stringSlice(ctx context.Context, list types.List, diags *diag.Diagnostics) []string {
	if list.IsNull() || list.IsUnknown() {
		return nil
	}

	var result []string
	diags.Append(list.ElementsAs(ctx, &result, false)...)
	if diags.HasError() {
		return nil
	}

	return result
}
