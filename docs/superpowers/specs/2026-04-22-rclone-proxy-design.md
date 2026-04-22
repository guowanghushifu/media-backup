# Rclone Proxy Configuration Design

## Goal

Add global HTTP proxy support for `rclone` uploads so every configured job can use the same outbound proxy, including proxies that require username and password authentication.

## Scope

This change only affects how the program builds and launches the `rclone` subprocess. It does not change file watching, hard-link creation, queueing, concurrency, cleanup, or terminal rendering.

The proxy configuration is global. It applies uniformly to all jobs because the process already treats `rclone_args`, retry timing, and upload concurrency as global runtime settings.

## Configuration

The YAML configuration gains an optional top-level `proxy` block:

```yaml
proxy:
  enabled: true
  scheme: http
  host: 127.0.0.1
  port: 7890
  username: myuser
  password: mypass
```

Field rules:

- `enabled` controls whether proxy injection is active.
- `scheme` is required when `enabled=true`. The initial implementation supports `http` and `https`.
- `host` is required when `enabled=true`.
- `port` is required when `enabled=true` and must be a positive integer.
- `username` is optional.
- `password` is optional.

Behavior rules:

- If `proxy` is omitted, the program behaves exactly as it does today.
- If `proxy.enabled=false`, the program behaves exactly as it does today.
- If `username` is set and `password` is empty, the program still includes the username in the proxy URL.
- If special characters appear in `username` or `password`, they must be URL-encoded before being placed into the proxy URL.

## Runtime Behavior

Before starting `rclone`, the program constructs a proxy URL from the parsed config:

```text
<scheme>://<username>:<password>@<host>:<port>
```

Credentials are omitted when not configured. The proxy URL is injected into the `rclone` subprocess environment only:

- `HTTP_PROXY`
- `HTTPS_PROXY`
- `http_proxy`
- `https_proxy`

The main Go process does not mutate its own environment globally. This keeps the proxy effect narrowly scoped to `rclone` execution and avoids accidental side effects on unrelated code.

## Validation

Validation occurs during config loading:

- reject unsupported proxy schemes
- reject missing host when enabled
- reject missing or non-positive port when enabled

Validation errors should clearly identify that the invalid field belongs to the global proxy configuration.

## Implementation Boundaries

The change should remain localized to:

- `internal/config/config.go`
  add proxy config parsing, defaults, normalization, and validation
- `internal/config/config_test.go`
  add coverage for valid proxy config, disabled proxy config, and invalid proxy config
- `internal/rclone/command.go`
  add proxy environment construction and inject it into `exec.CommandContext`
- `internal/rclone/runner_test.go`
  add coverage for proxy environment generation if command execution helpers are split for testability
- `configs/config.example.yaml`
  document the new optional `proxy` block

No job-level proxy override is included in this design.

## Error Handling

- Invalid proxy config prevents startup, the same as other config validation failures.
- If the proxy is valid but unreachable, `rclone` fails normally and existing retry behavior remains unchanged.
- The program does not attempt to probe proxy reachability during startup.

## Testing

Required tests:

- config parsing succeeds with a fully populated proxy block
- config parsing succeeds with `enabled=false`
- config validation fails for missing host, missing port, invalid port, and unsupported scheme
- proxy URL generation correctly encodes username and password
- command environment injection adds proxy variables only when proxy is enabled

## Non-Goals

- per-job proxy configuration
- SOCKS proxy support
- dynamic proxy reload without restart
- proxy-specific UI indicators
