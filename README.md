# tinx-providers

Monorepo for Tinx providers built around the current normalized package model.

## Layout

- `providers/setup-kubectl`: Tinx provider for `kubectl`, backed by a bundled Go installer.

## Common flows

Validate a provider:

```bash
cd providers/setup-kubectl
go test ./...
tinx release --manifest provider.yaml --main ./cmd/tinx-provider-kubectl --dist dist --output oci --tag validate
```

Publish a provider:

```bash
tinx release --manifest provider.yaml --main ./cmd/tinx-provider-kubectl --dist dist --output oci --tag v0.1.0 --push ghcr.io/<org>/tinx-setup-kubectl:v0.1.0
```

Run a local workspace check:

```bash
workspace_dir=$(mktemp -d)
cat > "$workspace_dir/tinx.yaml" <<EOF
apiVersion: tinx.io/v1
kind: Workspace
workspace: demo
providers:
	kubectl:
		source: $(pwd)/providers/setup-kubectl/oci
EOF

tinx init "$workspace_dir"
KUBECTL_VERSION=v1.30.6 tinx -w "$workspace_dir" exec -- kubectl version --client -o json
```

## CI model

The GitHub Actions setup is intentionally minimal:

- `ci.yml` runs `go test`, validates `tinx release`, and smoke-tests both transient providers and a workspace manifest through `sourceplane/tinx-action@v2`.
- `release.yml` installs `tinx` through `sourceplane/tinx-action@v2` and publishes the provider with `tinx release --push`.
