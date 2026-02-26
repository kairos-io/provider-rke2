# CLAUDE.md — provider-rke2

## What This Repo Is

This is the RKE2 cluster provider plugin for the Kairos immutable OS platform. It implements the `clusterplugin.ClusterProvider` interface from the `kairos-sdk`, producing `yip.YipConfig` objects that describe cloud-init-style boot stages for configuring and starting RKE2 on a node. The binary (`agent-provider-rke2`) is placed at `/system/providers/` inside a Kairos OS image and is invoked by the Kairos agent at boot via the go-pluggable event bus.

## Package Structure

```
main.go                         — entry point, wires plugin to provider and reset handler
pkg/
  constants/constants.go        — all hardcoded paths, system names, k8s defaults
  log/log.go                    — logrus setup with lumberjack rotation and version tagging
  provider/provider.go          — ClusterProvider implementation (the core logic)
  provider/reset.go             — HandleClusterReset handler (cluster teardown)
  types/rke2.go                 — RKE2Config struct (token, server, tls-san)
  types/mount.go                — MountPoint struct
  version/version.go            — version string injected at build time via ldflags
```

## Provider Implementation Pattern

The entire provider is a single function `ClusterProvider(cluster clusterplugin.Cluster) yip.YipConfig`. It is not a struct with methods. It takes the cluster description, builds a slice of `yip.Stage` values in order, and returns a `yip.YipConfig` with those stages under the `"boot.before"` key.

Stages are appended to a `[]yip.Stage` slice one at a time, in execution order:
1. Disable swap
2. Write config files
3. (conditionally) Import local images
4. Sleep to allow content extraction
5. Enable and restart the systemd service

Never collapse these into a single stage unless the upstream pattern explicitly does so.

## Role Handling

Roles come from `clusterplugin` constants — use them exactly:
- `clusterplugin.RoleInit` — initializes etcd, runs as RKE2 server, `Server` field must be empty string
- `clusterplugin.RoleControlPlane` — joins as RKE2 server pointing at `ControlPlaneHost:9345`
- `clusterplugin.RoleWorker` — runs `rke2-agent` instead of `rke2-server`

The `systemName` variable is determined by role. Worker nodes use `constants.AgentSystemName` ("rke2-agent"), all others use `constants.ServerSystemName` ("rke2-server"). This single variable drives both the containerd env config path and the systemctl commands.

For HA (multiple control plane nodes), the `RoleInit` node has `rke2Config.Server` set to empty string. All `RoleControlPlane` nodes point at `https://<ControlPlaneHost>:9345`. HA is handled implicitly by the cluster config — there is no special branching beyond the `RoleInit` empty-server check.

## Config File Generation

RKE2 config is written as multiple YAML files under `/etc/rancher/rke2/config.d/` and then merged with `jq` into `/etc/rancher/rke2/config.yaml` at stage execution time:
- `90_userdata.yaml` — user-supplied options (from `cluster.Options`, converted from YAML to JSON)
- `99_userdata.yaml` — provider-generated config (token, server URL, tls-san)

The merge command is a `jq` one-liner that deep-merges all `*.yaml` files in config.d. Do not change this pattern; it is the mechanism by which user config overrides provider defaults in a predictable priority order.

The `RKE2Config` struct uses yaml struct tags with RKE2's exact key names (`token`, `server`, `tls-san`). Never use camelCase keys for RKE2 config.

## Error Handling

Errors are not returned from `ClusterProvider` — the function signature is fixed by the `clusterplugin` interface. Errors surface only in `HandleClusterReset`, which returns a `pluggable.EventResponse` with the `Error` field set as a plain string.

Error messages follow this format exactly:
```go
response.Error = fmt.Sprintf("failed to <verb> <thing>: %s", err.Error())
```

No `fmt.Errorf`, no `errors.Wrap`, no sentinel errors. Direct string formatting into the response error field.

For non-critical errors (unmarshal of user options, interface address lookup), errors are silently ignored with `_` or a `fmt.Println` to stdout. Reserve `logrus.Fatal` for startup failures only (in `main`).

When a function can produce an error that is not actionable, assign to `_`:
```go
_ = yaml.NewEncoder(&providerConfig).Encode(&rke2Config)
```

## Code Style Rules

**Functions are short and named by what they return or do.** Helper functions are package-private and named with `get` prefix when they return a computed value (`getClusterRootPath`, `getDefaultNoProxy`, `getNodeCIDR`, `getSwapDisableStage`), and without a prefix when they perform a side-effecting action or build a string (`proxyEnv`).

**No named return values.** All returns are explicit.

**Variable declarations use `var` at the top of the function for zero-value initialization**, then assign later. Multiple `var` declarations are grouped:
```go
var payload bus.EventPayload
var config clusterplugin.Config
var response pluggable.EventResponse
```

Short `:=` declarations are used for values that are immediately assigned from a computation.

**Inline struct literals are used for `yip.Stage` and `yip.File`** — no intermediate builder variables unless the stage or file depends on a conditional.

**Slices are declared with `var` and grown with `append`**, not pre-allocated with `make` unless length is known.

**String formatting for shell commands uses `fmt.Sprintf`** with the full shell command as the format string. Commands are not abstracted into helpers unless they appear more than once.

**Constants are grouped by domain** using separate `const (...)` blocks in the same file, not interleaved with variables.

**No blank lines between closely related lines** (e.g., consecutive appends to the same slice, consecutive struct field assignments). Blank lines separate logical sections within a function.

**Imports are grouped**: stdlib, then internal packages, then external packages. The blank-line grouping is enforced. An underscore import (`_ "embed"`) goes in the stdlib group.

## Naming Conventions

- Types: PascalCase, noun (`RKE2Config`, `MountPoint`, `RKE2Logger`)
- Functions: camelCase for unexported, PascalCase for exported
- Constants: PascalCase (`ConfigurationPath`, `ServerSystemName`, `ClusterRootPath`)
- Local variables: camelCase, short and descriptive (`systemName`, `proxyValues`, `clusterRootPath`)
- No Hungarian notation, no type suffixes on variable names

Package names are single lowercase words (`provider`, `constants`, `types`, `log`, `version`).

## Airgap / Local Images Pattern

Airgap support is opt-in via `cluster.ImportLocalImages`. When true, a stage is inserted that calls the `import.sh` script from the cluster root path. The `If` field on the stage guards execution with a directory existence check. Default path is `constants.LocalImagesPath` (`/opt/content/images`) and is only overridden if the cluster config provides a non-empty `LocalImagesPath`.

```go
if cluster.ImportLocalImages {
    if cluster.LocalImagesPath == "" {
        cluster.LocalImagesPath = constants.LocalImagesPath
    }
    importStage := yip.Stage{
        Commands: []string{...},
        If: fmt.Sprintf("[  -d %s ]", cluster.LocalImagesPath),
    }
    stages = append(stages, importStage)
}
```

## Proxy Handling

Proxy environment variables are written to `/etc/default/<systemName>` as a flat `KEY=value` file, one per line. Both the plain env var and the `CONTAINERD_`-prefixed variant are written for each proxy setting. `NO_PROXY` is built by combining default k8s no-proxy entries (cluster CIDR, service CIDR, node CIDR, `.svc` suffixes) with user-supplied `NO_PROXY`. If no proxy is configured, no file is written.

## Reset Handler Pattern

`HandleClusterReset` follows the exact same parse-then-act pattern as the SDK's own `onBoot`:
1. Unmarshal event data into `bus.EventPayload`
2. Unmarshal payload config into `clusterplugin.Config`
3. Guard on nil cluster
4. Execute the action, capture combined output
5. Set `response.Error` if the command failed, return response

The uninstall is a shell script invocation via `exec.Command("/bin/sh", "-c", ...)`, not a Go reimplementation of the cleanup.

## Build and FIPS

The binary is built with `go-build-static.sh` for a statically linked binary. For FIPS, `go-build-fips.sh` is used instead and the result is validated with `assert-fips.sh` and `assert-static.sh`. The version string is injected via ldflags: `-X github.com/kairos-io/provider-rke2/pkg/version.Version=${VERSION}`. The `version.Version` variable in `pkg/version/version.go` is intentionally empty — it is always set at build time.

## Testing

There are no test files in this repository. Do not add tests unless explicitly asked. If tests are added, they should be in `_test.go` files within the same package they test (white-box style), not in a separate `_test` package.

## Patterns to Avoid

- Do not return errors from `ClusterProvider` — the interface does not allow it.
- Do not use `errors.New` or `fmt.Errorf` — error messages go directly into `response.Error` as formatted strings.
- Do not split the stage-building logic across multiple files — keep `ClusterProvider` in `provider.go`.
- Do not use interface types or dependency injection — functions take concrete structs.
- Do not add configuration structs for provider behavior — use `constants` for all hardcoded values.
- Do not use `os.Exit` or `log.Fatal` outside of `main.go`.
- Do not use `context.Context` — the plugin model is synchronous and event-driven.
- Do not use goroutines — all execution is sequential within a single event handler invocation.
- Do not write YAML config files directly in Go — use `yaml.NewEncoder` + a struct with proper yaml tags.
- Do not use `map[string]interface{}` for config unless unmarshalling unknown user input (as in `getDefaultNoProxy`).
- Do not create wrapper types around the `clusterplugin.Cluster` struct — use it directly.

## Function Design & Testability

- **Every function does one thing and fits in ~20–30 lines.** If it grows beyond that, extract named helpers.
- **Write functions so they can be unit tested in isolation** — no hidden side effects, no global state access, no I/O buried inside business logic.
- **Most business logic must be unit testable** without spinning up a server, database, or Kubernetes cluster. Separate I/O at the boundary.
- **Use guard clauses / early returns** to reduce nesting. Flat code is easier to read and test than deeply nested.
- **Accept interfaces, return concrete types.** This makes callers mockable without reflection or code generation.
- **Keep interfaces small** — 1–3 methods. Large interfaces are hard to mock and signal poor separation of concerns.

## General Go Practices

- **Dependency injection over globals.** Pass dependencies via constructors or function parameters — not package-level singletons (except logging).
- **`context.Context` is always the first parameter** on any function that performs I/O. Never store it in a struct field.
- **Table-driven tests** for any function with multiple input/output cases: `[]struct{ name, input, expected }` with `t.Run`.
- **Test naming:** `TestFuncName_Scenario` — e.g. `TestCreateCluster_MissingName`.
- **Prefer `switch` over long `if/else if` chains.**
- **Short variable names in small scopes** (`i`, `v`, `err`) are idiomatic; use descriptive names in wider scopes.
- **No goroutines unless concurrency is genuinely required.** Sequential code is easier to test and reason about.
- **Avoid `init()` for anything except registering handlers or loggers.** Never use it for config loading or side-effectful setup.
- **Respect context cancellation** in any loop that calls external services.
- **Import grouping:** stdlib / external / internal — separated by blank lines, sorted by `goimports`.
- **Don't over-abstract.** Don't create an interface or wrapper until there are ≥2 concrete implementations or a clear testing need.
- **No naked `panic` in library code.** Panics are only acceptable in `main` or test setup for truly unrecoverable state.
