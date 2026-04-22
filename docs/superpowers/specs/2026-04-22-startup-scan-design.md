# Startup Scan Responsiveness Design

## Goal

Improve startup responsiveness in two ways:

- show the live UI immediately when the program starts
- avoid waiting `stable_duration` again for files that already existed before startup

This change is intended to remove the long silent delay that currently happens when the source directory already contains media files.

## Scope

This design changes startup sequencing and initial catch-up scan behavior.

It does not change:

- runtime handling of newly created files
- upload scheduling rules
- terminal dashboard layout
- transfer event retention
- `rclone` execution behavior

## Current Problem

The current startup path performs all initial work before the UI loop starts.

Today `Run()` does this in order:

1. create each `link_dir`
2. add recursive watches
3. run `ScanAndLink(...)` for each job
4. only after that, start `eventLoop`, `dispatchLoop`, and `uiLoop`

That causes two user-visible problems:

1. the terminal stays blank during startup catch-up work
2. every existing media file is passed through `WaitStable(...)` again, even if it has already been sitting unchanged on disk for a long time

Because the default `stable_duration` is one minute, startup can pause for tens of seconds or more before any UI appears.

## Root Cause

The startup scanner uses the same stability rule as runtime file detection.

That is correct for files discovered from live filesystem events, because those files may still be downloading.

It is not appropriate for files found during startup catch-up scanning, because those files predate the current process. Requiring a fresh full-duration wait at process start is redundant and makes startup feel hung.

## Desired Behavior

### Immediate UI Startup

The live UI should start before the initial catch-up scan begins.

Behavior rules:

- the terminal dashboard appears immediately after process startup
- startup catch-up work happens while the UI is already active
- the user should no longer see a blank terminal while startup scanning is running

### Startup Scan Without Redundant Stability Delay

Initial catch-up scanning should treat pre-existing files differently from newly observed runtime files.

Behavior rules:

- files found during startup scan whose last modification time is already older than `stable_duration` do not wait again
- files found during startup scan whose last modification time is still within `stable_duration` must still pass a stability wait before linking
- startup scan still filters by allowed extension
- startup scan still creates hard links into `link_dir`
- if at least one file is linked or already exists, the job is marked dirty for upload
- runtime file handling continues to use `WaitStable(...)`

This keeps the safety check for files that may still be in progress at restart time, while removing unnecessary waiting for clearly old files that were already stable before startup.

## Runtime Model Changes

### Service Startup Sequence

`internal/app/service.go` should start the UI loop before the initial scan phase.

Suggested order:

1. create `link_dir` for each job
2. start `uiLoop`
3. add recursive watches
4. run startup catch-up scan for each job
5. start `eventLoop`
6. start `dispatchLoop`

This preserves a simple startup model while ensuring the dashboard is visible during catch-up work.

Initial scanning can remain serial across jobs. Parallel scanning is not required for this design.

### Scanner Behavior Split

`internal/watcher/scanner.go` should stop using a single behavior for both startup and runtime discovery.

Recommended approach:

- keep the existing runtime path unchanged
- add a dedicated startup scan path, or add an explicit option that applies a startup-specific stability rule

The important rule is:

- startup scan must skip `WaitStable(...)` only for files whose modification time is already older than `stable_duration`
- startup scan must still call `WaitStable(...)` for recently modified files
- runtime event-driven processing must still call `WaitStable(...)`

## UI Behavior During Startup

This design does not require a new dashboard section or a new renderer mode.

Acceptable behavior during startup:

- show `空闲` if there is not yet any queued or active work
- show `等待中` once startup scan marks jobs dirty

The key requirement is visibility, not a new startup-specific status label.

## Error Handling

This change does not introduce new user-facing error categories.

If startup scanning fails for a job, startup should still fail the same way it does today.

## Testing

Required tests:

- startup scan links sufficiently old existing files without calling stability waiting
- startup scan still waits for recently modified existing files
- runtime file handling still depends on `WaitStable(...)`
- service startup enters UI lifecycle before the catch-up scan completes
- startup scan marks jobs dirty when existing files are found

Tests may use small injectable helpers if needed to make startup ordering and scanner behavior observable.

## Implementation Boundaries

Expected file changes:

- `internal/app/service.go`
- `internal/app/service_test.go`
- `internal/watcher/scanner.go`
- `internal/watcher/scanner_test.go`

No renderer layout changes are required for this fix.
