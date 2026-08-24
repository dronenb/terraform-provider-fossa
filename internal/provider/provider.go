// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"os"

	"github.com/dronenb/terraform-provider-fossa/internal/fossaclient"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	defaultEndpoint = "https://app.fossa.com/api"
	apiTokenEnvVar  = "FOSSA_API_TOKEN"
)

// Ensure FossaProvider satisfies various provider interfaces.
var _ provider.Provider = &FossaProvider{}

// FossaProvider defines the provider implementation.
type FossaProvider struct {
	// version is set to the provider version on release, "dev" when the
	// provider is built and ran locally, and "test" when running acceptance
	// testing.
	version string
}

// FossaProviderModel describes the provider data model.
type FossaProviderModel struct {
	APIToken types.String `tfsdk:"api_token"`
	Endpoint types.String `tfsdk:"endpoint"`
}

func (p *FossaProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "fossa"
	resp.Version = p.version
}

func (p *FossaProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"api_token": schema.StringAttribute{
				MarkdownDescription: "FOSSA API token. Can also be set via the `FOSSA_API_TOKEN` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"endpoint": schema.StringAttribute{
				MarkdownDescription: "Base URL of the FOSSA API. Defaults to `https://app.fossa.com/api`.",
				Optional:            true,
			},
		},
	}
}

func (p *FossaProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data FossaProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	token := ""
	if !data.APIToken.IsNull() {
		token = data.APIToken.ValueString()
	}
	if token == "" {
		token = os.Getenv(apiTokenEnvVar)
	}
	if token == "" {
		resp.Diagnostics.AddError(
			"Missing FOSSA API Token",
			"Set the api_token provider attribute or the "+apiTokenEnvVar+" environment variable.",
		)
		return
	}

	endpoint := defaultEndpoint
	if !data.Endpoint.IsNull() {
		endpoint = data.Endpoint.ValueString()
	}

	cfg := fossaclient.NewConfiguration()
	cfg.DefaultHeader["Authorization"] = "Bearer " + token
	if endpoint != "" && endpoint != defaultEndpoint {
		if len(cfg.Servers) > 0 {
			cfg.Servers[0].URL = endpoint
		} else {
			cfg.Servers = fossaclient.ServerConfigurations{{URL: endpoint}}
		}
	}

	client := fossaclient.NewAPIClient(cfg)

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *FossaProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewTeamResource,
		NewTeamMembersResource,
		NewOIDCProviderResource,
		NewOIDCTrustRelationshipResource,
		NewServiceAccountResource,
	}
}

func (p *FossaProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewTeamsDataSource,
		NewUsersDataSource,
		NewRolesDataSource,
		NewTeamMembersDataSource,
	}
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &FossaProvider{
			version: version,
		}
	}
}
