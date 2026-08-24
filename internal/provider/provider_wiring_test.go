// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestProviderSmokeWiring(t *testing.T) {
	p := New("test")()
	ctx := context.Background()
	mdReq := provider.MetadataRequest{}
	mdResp := &provider.MetadataResponse{}
	p.Metadata(ctx, mdReq, mdResp)
	if mdResp.TypeName != "fossa" {
		t.Fatalf("unexpected type name %q", mdResp.TypeName)
	}
	rs := p.Resources(ctx)
	want := []string{"fossa_team", "fossa_team_members", "fossa_oidc_provider", "fossa_oidc_trust_relationship", "fossa_service_account"}
	if len(rs) != len(want) {
		t.Fatalf("got %d resources, want %d", len(rs), len(want))
	}
	for i, r := range rs {
		rr := r()
		mResp := &resource.MetadataResponse{}
		rr.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "fossa"}, mResp)
		if mResp.TypeName != want[i] {
			t.Errorf("resource %d: got %q want %q", i, mResp.TypeName, want[i])
		}
		sReq := resource.SchemaRequest{}
		sResp := &resource.SchemaResponse{}
		rr.Schema(ctx, sReq, sResp)
		if sResp.Diagnostics.HasError() {
			t.Errorf("resource %s schema errors: %+v", mResp.TypeName, sResp.Diagnostics)
		}
	}
	ds := p.DataSources(ctx)
	wantDS := []string{"fossa_teams", "fossa_users", "fossa_roles", "fossa_team_members"}
	for i, d := range ds {
		dd := d()
		mResp := &datasource.MetadataResponse{}
		dd.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "fossa"}, mResp)
		if mResp.TypeName != wantDS[i] {
			t.Errorf("data source %d: got %q want %q", i, mResp.TypeName, wantDS[i])
		}
		sReq := datasource.SchemaRequest{}
		sResp := &datasource.SchemaResponse{}
		dd.Schema(ctx, sReq, sResp)
		if sResp.Diagnostics.HasError() {
			t.Errorf("data source %s schema errors: %+v", mResp.TypeName, sResp.Diagnostics)
		}
	}
}
