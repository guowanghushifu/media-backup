# Scheduler Events Design

## Goal

Expose task scheduling state changes in the existing terminal "最近事件" area so the operator can see when a job was queued, started, retried, or completed.

This is in addition to the existing file transfer events. Both kinds of events should share the same recent-event timeline.

## Scope

This change adds scheduler-facing event messages to the live UI.

It does not change:

- scheduler queue semantics
- upload concurrency rules
- transfer parsing
- event timestamp format
- recent event retention count

## Current Problem

The lower event area only shows transfer output such as copied files.

That leaves an important blind spot: operators cannot tell when a job became dirty, when it was dispatched, whether it is waiting for retry, or whether a completed upload was immediately re-queued because new files appeared during the run.

## Design Choice

Record scheduling events in `internal/app/service.go`, not inside `internal/queue/scheduler.go`.

Reasons:

- the service layer already owns the human-facing recent-event stream
- the service layer has the job name and the operational context needed for readable messages
- the queue package stays focused on state transitions and does not gain UI/event responsibilities

## Desired Behavior

### Shared Event Timeline

Scheduler events and transfer events are appended into the same `recentEvents` buffer.

Rules:

- keep the existing `maxRecentEvents = 10` limit
- preserve timestamps on every event
- keep the current UI rendering path unchanged apart from showing the new messages
- continue showing events while the program is idle

### Only Log Meaningful State Changes

Do not log noisy duplicate calls that do not result in a meaningful state transition.

Examples:

- if a job is already queued, another `MarkDirty`-equivalent action should not append another "待上传" event
- if a job is already marked to rerun after the current upload, repeated writes during the same run should not append repeated rerun events

The event stream should describe what changed, not every internal function call.

### Event Types

The service should emit human-readable events for these transitions:

1. startup scan finds existing files and queues a job
2. runtime file processing queues an idle job
3. runtime file processing marks an active job to rerun after the current upload
4. dispatcher starts an upload for a job
5. upload completes with no rerun pending
6. upload completes and immediately re-queues because new files arrived during the run
7. upload fails and enters retry waiting
8. retry delay expires and the job is re-queued

## Event Message Style

Messages should include the job name and the state transition outcome.

Recommended message set:

- `[JOB] 启动扫描发现 N 个文件，任务标记为待上传`
- `[JOB] 检测到新文件，任务标记为待上传`
- `[JOB] 检测到新文件，任务保持运行中，完成后将重新排队`
- `[JOB] 调度开始上传`
- `[JOB] 上传完成，任务清空`
- `[JOB] 上传完成，检测到新增文件，重新排队`
- `[JOB] 上传失败，进入重试等待`
- `[JOB] 到达重试时间，重新排队`

The exact wording may be adjusted slightly in implementation, but the semantic distinctions above must remain.

## Data Flow

### Startup Catch-Up

After startup scanning links one or more files for a job and the job is marked dirty, append a startup scheduling event.

If startup scanning finds zero files, emit no scheduling event.

### Runtime File Handling

After a new file is stabilized and linked:

- if the job is not currently active and the transition results in the job becoming queued, append the idle-to-queued event
- if the job is currently active and `dirtyDuringRun` changes from `false` to `true`, append the rerun-after-current-upload event

Repeated file arrivals that do not change those conditions should not append duplicates.

### Dispatch and Completion

When `dispatchLoop` successfully starts a ready job, append the start event.

When an upload finishes:

- if no rerun is pending and cleanup succeeds, append the completed-and-cleared event
- if `dirtyDuringRun` is true, append the completed-and-requeued event
- if the upload fails, append the retry-wait event
- if cleanup fails, keep existing retry behavior and treat it as a failed completion path for event purposes

### Retry Release

When retry delay expires and the job is actually moved back into the ready queue, append the retry-requeued event.

Do not append an event for jobs whose retry deadline was checked but not actually released.

## Testing

Required coverage:

- startup scan queues a job and records a startup scheduling event
- runtime file processing records the idle-to-queued event once
- runtime file processing records the rerun-after-current-upload event once while a job is active
- dispatch start records a scheduling event
- upload success records the correct completion event for both clean and rerun cases
- upload failure records the retry-wait event
- retry release records the re-queue event
- duplicate no-op transitions do not create duplicate scheduler events

Expected file changes:

- `internal/app/service.go`
- `internal/app/service_test.go`

No queue package API changes are required for this design.
