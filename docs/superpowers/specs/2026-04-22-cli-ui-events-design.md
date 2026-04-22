# CLI UI Event Panel Design

## Goal

Adjust the terminal UI so it is split into two persistent sections:

- the upper section shows current runtime status
- the lower section shows the most recent 10 transfer events with timestamps

This change is intended to make recent file transfer history visible even after an upload finishes or the program returns to idle.

## Scope

This design only changes how runtime state is stored for UI purposes and how the terminal frame is rendered.

It does not change:

- file watching
- upload scheduling
- `rclone` invocation
- retry behavior
- cleanup behavior

## Current Problem

The current UI stores the latest parsed transfer event on each active job and renders that event inline with the job status lines.

That has two unwanted effects:

1. transfer events are visually mixed into the status section instead of being shown in a dedicated log area
2. events disappear quickly because they are tied to active jobs and short-lived event TTL logic

As a result, users cannot reliably see the recent transfer record once a job finishes or the program becomes idle.

## Desired Behavior

The terminal UI should always render two blocks.

### Status Block

The upper block shows the current runtime status:

- current timestamp
- whether the program is idle or transferring
- active job count against `max_parallel_uploads`
- waiting job count
- one summary line per active job

This block must contain only status information. Transfer event lines are no longer rendered here.

### Recent Events Block

The lower block shows the most recent 10 transfer events parsed from `rclone` output, with the local time when each event was recorded by the program.

Behavior rules:

- only parsed transfer events are recorded here
- events are stored globally, not per active job
- each event entry stores both timestamp and message
- the list keeps at most 10 entries
- when an 11th event arrives, drop the oldest event
- events remain visible while uploads are active
- events remain visible when the program is idle
- events are rendered in chronological order from oldest to newest so the newest item appears at the bottom
- each rendered event line uses the timestamp format `[YYYY-MM-DD HH:MM:SS]`
- if there are no events yet, render a placeholder such as `暂无事件`

## Data Model Changes

The service layer should stop treating recent events as ephemeral per-job UI state.

### Job Runtime State

`jobRuntime` continues to store:

- job config
- current summary line
- whether the job is active
- whether the job became dirty during upload

It no longer needs fields dedicated to temporary event display.

### Service-Level Event History

`Service` gains a global recent-event buffer shared by the whole UI.

Requirements:

- append a new entry whenever `rclone.ParseEvent` returns a payload
- record `time.Now()` together with the parsed payload at append time
- preserve insertion order
- trim the slice to the most recent 10 entries
- protect access with the existing service mutex

This model matches the requirement, which is about recent transfer history across the whole program rather than per-job status decoration.

## Rendering Changes

`internal/ui/renderer.go` should render a single frame with stable structure in both active and idle states.

Suggested layout:

```text
[2026-04-22 15:04:05] 当前状态：正在传输 | 活跃任务: 2/5 | 等待中: 1
[job-a] 832 MiB / 1000 MiB, 83%, 29.793 MiB/s, ETA 5s
[job-b] 12.4 GiB / 40.0 GiB, 31%, 48.2 MiB/s, ETA 9m12s

最近事件:
1. [2026-04-22 15:03:58] THIS_IS_TEST/file-01.mkv: Copied (new)
2. [2026-04-22 15:04:03] THIS_IS_TEST/file-02.mkv: Copied (new)
```

Idle layout should keep the same lower block:

```text
[2026-04-22 15:04:05] 当前状态：空闲

最近事件:
1. [2026-04-22 15:03:58] THIS_IS_TEST/file-01.mkv: Copied (new)
2. [2026-04-22 15:04:03] THIS_IS_TEST/file-02.mkv: Copied (new)
```

Formatting requirements:

- keep the status block first
- insert a blank line between the two blocks for readability
- always render the `最近事件:` header
- render each event line with a full datetime prefix in `[YYYY-MM-DD HH:MM:SS]` format
- render at most 10 event lines under that header
- render a placeholder line when the event list is empty

## Snapshot Semantics

The UI snapshot function should return two independent pieces of data:

- active job status rows
- recent event history

That snapshot must not drop event history when:

- there are zero active jobs
- a job has just completed
- an event is older than a short time threshold

This explicitly removes the existing TTL-based hiding behavior.

## Error Handling

This feature does not introduce new user-facing error paths.

If no events have been parsed yet, the UI shows the placeholder event line instead of failing or rendering an empty block.

## Testing

Required tests:

- service snapshot keeps recent events even when no job is active
- service snapshot keeps only the most recent 10 events
- service snapshot preserves event timestamps alongside event messages
- service snapshot no longer depends on event TTL behavior
- renderer output shows separate status and event blocks
- renderer output shows full datetime prefixes on recent event lines
- renderer output for active jobs does not include inline event lines
- renderer output for idle state still includes the recent events block

## Implementation Boundaries

Expected file changes:

- `internal/app/service.go`
- `internal/app/service_test.go`
- `internal/ui/renderer.go`
- `internal/ui/renderer_test.go`

No other package should need behavior changes for this feature.
