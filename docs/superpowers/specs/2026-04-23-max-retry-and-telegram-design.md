# Max Retry And Telegram Failure Design

## Goal

Add a configurable maximum retry count for upload failures so a file stops retrying after a fixed number of consecutive failures, keeps its linked file in `link_dir`, and sends a final failure notification to Telegram when Telegram is configured.

## Current State

- Global config supports `retry_interval` but not a retry limit
- Upload failures are tracked in `internal/app/service.go` using `retryDue`
- A failed file is re-queued forever as long as it keeps failing
- Successful uploads remove the linked file from `link_dir`
- The repository does not currently define Telegram notification configuration or a Telegram sender

## Requirements

1. Add a global config field `max_retry_count`.
2. `max_retry_count` applies per file task, keyed by the existing file task key in the service.
3. The retry counter is a consecutive failure count, not a lifetime total.
4. A successful upload resets the file's failure count.
5. A same-path replacement file should not inherit the previous file's failure history.
6. When a file reaches `max_retry_count`, it must stop entering the retry queue.
7. When retrying stops because the max count is reached, the file in `link_dir` must be kept in place.
8. When Telegram is configured, reaching the retry limit must send one final failure notification to the configured Telegram channel.
9. When Telegram is not configured, retry-limit exhaustion must still behave correctly without failing the service.
10. Existing behavior should remain compatible when `max_retry_count` is unset.

## Design

### Retry Limit Semantics

- `max_retry_count` means the maximum number of consecutive failed upload attempts allowed for one file task
- Default value: `0`, meaning unlimited retries and preserving current behavior
- Failure counts are tracked by the existing file task key, which is the file's `linkPath`
- The count increments only when an actual upload attempt fails
- The count resets when:
  - the upload eventually succeeds
  - a same-path replacement supersedes the previous active or waiting task
  - a waiting retry task is replaced by a new file version at the same path

### Service State

Extend `internal/app.Service` with a failure-count map keyed by task key.

Responsibilities:

- Increment the count during failed upload completion
- Decide whether the task should re-enter `retryDue`
- Clear the count on success and on same-path replacement
- Keep failure accounting in the service layer, not inside `internal/queue`, because the retry limit affects business behavior and notifications, not generic scheduling

### Failure Completion Behavior

For a failed upload:

1. Mark the task inactive as today.
2. Increment the consecutive failure count for that file key.
3. If `max_retry_count == 0`, preserve today's unlimited retry behavior.
4. If the new failure count is below `max_retry_count`, preserve today's retry scheduling behavior.
5. If the new failure count reaches or exceeds `max_retry_count`:
   - do not add the task to `retryDue`
   - do not re-queue it in the scheduler
   - keep the task registered so the linked file remains visible and available
   - add a recent event explaining that retrying stopped because the maximum retry count was reached
   - attempt to send a Telegram final-failure notification

This means the terminal state after retry exhaustion is “failed and retained,” not “failed and pending retry.”

### Success Behavior

For a successful upload:

- Clear any retry counter for that file
- Keep the existing linked-file cleanup behavior
- Keep the existing task-removal behavior

### Same-Path Replacement Behavior

When a file at the same path is updated or replaced:

- Clear any retry counter for that file key before the new task continues
- Treat the replacement as a fresh file version
- Preserve the existing supersede behavior for active uploads and retry-waiting tasks

This avoids permanently poisoning a path because an older version failed several times.

### Telegram Configuration

Add optional Telegram config as a new top-level config section.

Suggested fields:

- `enabled`
- `bot_token`
- `chat_id`

Behavior:

- If `enabled` is `false`, no Telegram sending occurs
- If `enabled` is `true`, `bot_token` and `chat_id` are required
- A notification send failure must not crash the service or alter retry-limit behavior
- Send failures should be logged

### Telegram Notification Trigger

Only send Telegram for the terminal failure state when the retry limit is reached.

Do not send Telegram on every ordinary retryable failure. This avoids flooding the channel for transient issues.

Notification content should include at least:

- job name
- linked file path
- reached retry count
- last upload error

### Testing

Add tests for:

1. Config loading defaulting `max_retry_count` to `0`
2. Config validation accepting disabled Telegram config and rejecting incomplete enabled Telegram config
3. A failed upload below the retry limit still enters `retryDue`
4. A failed upload that reaches the retry limit stops retrying
5. Reaching the retry limit keeps the linked file in place
6. Successful upload clears the failure count
7. Same-path replacement clears the previous failure count
8. Telegram final-failure notification is attempted exactly once when the retry limit is reached
9. Telegram notification errors are logged but do not break service behavior

## Versioning And Compatibility

- Existing configs remain valid because `max_retry_count` defaults to `0`
- Existing behavior remains unchanged unless users explicitly set a positive retry limit
- Telegram is fully optional

## Out of Scope

- Persisting retry counters across process restarts
- Per-job retry limits
- Recovered-success Telegram notifications
- Repeated reminder notifications after retry exhaustion
- More advanced alert routing beyond Telegram
