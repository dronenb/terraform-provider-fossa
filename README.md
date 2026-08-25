# Terraform Provider for FOSSA

A [Terraform](https://www.terraform.io) provider for managing
[FOSSA](https://fossa.com) organizations, built with the
[Terraform Plugin Framework](https://github.com/hashicorp/terraform-plugin-framework)
and generated from the official FOSSA OpenAPI specification.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.24
- A FOSSA API token (with admin permissions for the target organization)

## Using the Provider

Configure the provider with a FOSSA API token, either via the `api_token`
attribute or the `FOSSA_API_TOKEN` environment variable:

```hcl
provider "fossa" {
  # api_token = "..." # or set FOSSA_API_TOKEN
}
```

### Example: Team and Membership

```hcl
data "fossa_roles" "all" {}

resource "fossa_team" "platform" {
  name             = "Platform"
  default_role_id  = data.fossa_roles.all.roles[0].id
  auto_add_users   = false
}

resource "fossa_team_members" "platform" {
  id = fossa_team.platform.id

  users = [
    {
      id      = 12345
      role_id = data.fossa_roles.all.roles[0].id
    },
    {
      id      = 67890
      role_id = data.fossa_roles.all.roles[0].id
    },
  ]
}
```

> **Note:** `fossa_team_members` fully manages the membership of the team:
> users not listed in the configuration are removed from the team.

### Example: OIDC Provider and Trust Relationship

```hcl
resource "fossa_oidc_provider" "example" {
  issuer   = "https://token.actions.githubusercontent.com"
  scope    = "organization"
}

resource "fossa_oidc_trust_relationship" "example" {
  provider_id = fossa_oidc_provider.example.id
  user_id     = 12345
  scope       = "team"
  scope_id    = fossa_team.platform.id

  audiences = ["https://fossa.example.com"]

  required_claims = [
    {
      claim         = "sub"
      value         = "repo:acme/widget:environment:prod"
      has_wildcards = false
    },
  ]
}
```

### Example: Service Account

```hcl
resource "fossa_service_account" "ci" {
  username            = "ci-bot"
  full_name           = "CI automation"
  has_full_api_token  = true
}

output "ci_api_token" {
  value     = fossa_service_account.ci.full_api_token
  sensitive = true
}
```

> **Note:** The FOSSA API does not support updating or deleting service
> accounts. Destroying `fossa_service_account` only removes it from state;
> deactivate or remove the account manually in FOSSA. API tokens are only
> shown once at creation time.

## Developing the Provider

The provider is generated from the [FOSSA OpenAPI
specification](https://app.fossa.com/api/api-docs/swagger.json) using
HashiCorp's code generation pipeline. Everything is driven by
[go-task](https://taskfile.dev):

```shell
brew install go-task golangci-lint goreleaser node
```

| Command            | Description                                              |
| ------------------ | -------------------------------------------------------- |
| `task build`       | Build the provider binary                                |
| `task test`        | Run unit tests                                           |
| `task lint`        | Run golangci-lint                                        |
| `task generate`    | Regenerate spec → schemas → client → docs from upstream  |
| `task check-codegen` | Fail if committed generated artifacts are out of date  |
| `task testacc`     | Run acceptance tests (requires `FOSSA_API_TOKEN`)        |

### Code Generation Pipeline

1. `gen/openapi/upstream/swagger.json` — pristine upstream FOSSA spec.
2. `scripts/patch_openapi.py` + `gen/openapi/patches/rules.json` — apply all
   OpenAPI fixes (union flattening, `allOf` merging, schema normalization) to
   produce `gen/openapi/swagger.json`.
3. `tfplugingen-openapi` — map the patched spec onto Terraform entities per
   `generator_config.yml`, producing `gen/provider/provider_code_spec.raw.json`.
4. `scripts/patch_provider_spec.py` + `gen/provider/spec_overrides.json` —
   dedupe merged attributes and apply sensitive-flag overrides.
5. `tfplugingen-framework` — generate resource/data source scaffolding into
   `internal/provider/*_gen.go` packages (committed; do not edit).
6. `openapi-generator-cli` — generate the Go API client into
   `internal/fossaclient` (committed; do not edit).

Hand-written glue lives in `internal/provider/*.go` (non-`_gen.go` files).

## Releasing

Releases are built and published with GoReleaser:

```shell
task release:snapshot  # local dry-run build
task release           # publish a release (requires GITHUB_TOKEN)
```
