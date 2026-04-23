# File-Level Rclone Upload Design

## Goal

Replace the current directory-level upload jobs with file-level upload jobs so each scheduler slot handles one linked media file at a time. This allows multiple files from the same source directory tree to upload concurrently without serial re-dispatch of a whole directory job.

## Current Problem

The current runtime model creates one upload job per configured `source_dir`. When a file becomes stable, it is hard-linked into the matching `link_dir`, but the scheduler only marks the directory job dirty. The upload command then sends the entire `link_dir` to rclone:

`rclone copy <link_dir> <rclone_remote> ...`

This creates three issues:

1. Files under the same directory tree are serialized behind one runtime job.
2. If new files appear while a directory upload is running, the service must requeue the directory job and rescan later.
3. Cleanup removes everything under `link_dir`, which is incompatible with file-level retry and per-file completion.

## Required Behavior

### File-Level Task Model

- Keep config matching by `source_dir` and `link_dir`.
- Replace runtime directory jobs with runtime file tasks keyed by the absolute linked file path.
- A task represents exactly one linked media file ready to upload.
- Startup scanning must enqueue one task per existing linked media file.
- Runtime watch events must enqueue one task per newly stable media file after hard-link creation succeeds.
- The scheduler continues to enforce `max_parallel_uploads`, but slots now represent files, not directories.

### Rclone Command

Each task must call rclone with a single linked file as the source and the file's target remote directory as the destination:

`rclone copy <linked-file> <remote-directory-with-trailing-slash> ...`

Rules:

- The destination must always end with `/`.
- The remote directory must preserve the relative subdirectory structure between `source_dir` and the source file.
- The remote directory must be computed from config, not guessed from the linked file path alone.

Example:

- `source_dir`: `/dld/upload/4K.REMUX.SGNB`
- `link_dir`: `/dld/gd_upload/4K.REMUX.SGNB`
- `rclone_remote`: `gd1:/sync/Movie/4K.REMUX.SGNB`
- source file: `/dld/upload/4K.REMUX.SGNB/爆裂鼓手 (2014) {tmdb-244786}/爆裂鼓手 (2014) {tmdb-244786} - 2160p.BluRay.HDR.HEVC.TrueHD.7.1-ROBOT.mkv`
- linked file: `/dld/gd_upload/4K.REMUX.SGNB/爆裂鼓手 (2014) {tmdb-244786}/爆裂鼓手 (2014) {tmdb-244786} - 2160p.BluRay.HDR.HEVC.TrueHD.7.1-ROBOT.mkv`
- computed remote directory: `gd1:/sync/Movie/4K.REMUX.SGNB/爆裂鼓手 (2014) {tmdb-244786}/`

### Completion and Cleanup

When a file upload succeeds:

1. Remove only that linked file.
2. Attempt to remove now-empty parent directories.
3. Stop directory cleanup at the configured `link_dir` root.
4. Never remove non-empty directories.
5. Never remove the `link_dir` root itself.

When a file upload fails:

1. Keep the linked file in place.
2. Mark only that file task for retry.
3. Do not block uploads for sibling files in the same directory.
4. Do not remove parent directories while the failed file still exists.

### UI Task Naming

The active task name can no longer use config job names because multiple file tasks may come from the same config entry.

New rule:

- Display the uploading file's basename, truncated to 40 display columns.
- Truncation must respect East Asian character width so Chinese characters consume two columns.
- The truncation logic must keep the dashboard table aligned for mixed Chinese and ASCII names.

Events may still include additional context such as config name or relative path when that helps diagnosis, but the active task name shown in the dashboard should be derived from the file name.

## Design

### Runtime Structures

Introduce a file-oriented runtime record that stores:

- the matched `config.JobConfig`
- the task key (absolute linked file path)
- the source file path
- the linked file path
- the computed remote directory
- the latest upload summary
- active/retry state owned by the service and scheduler

The service should maintain runtime tasks in a map keyed by linked file path. Config-level matching remains separate and is still based on `source_dir`.

### Discovery Flow

Startup:

1. Ensure each configured `link_dir` exists.
2. Add recursive watches under each `source_dir`.
3. Scan each `source_dir` for matching media files.
4. Ensure each matching source file has a hard link under `link_dir`.
5. Scan each `link_dir` for matching media files.
6. Register one runtime task for each linked media file found.

Runtime:

1. Watch source tree changes.
2. When a matching file becomes stable, create or confirm the hard link.
3. Materialize a file task from the linked file path.
4. Queue that single file task if it is not already running or waiting for retry.

Duplicate events for the same file must remain idempotent.

### Remote Directory Calculation

For a given source file:

1. Compute `rel = filepath.Rel(source_dir, source_file)`.
2. Extract `relDir = filepath.Dir(rel)`.
3. If `relDir` is `.`, use the configured `rclone_remote` as the destination directory.
4. Otherwise append `relDir` to `rclone_remote` using remote-path joining semantics.
5. Ensure the final destination ends with `/`.

The same relative-path logic must be reproducible from an existing linked file by converting it back to its `source_dir`-relative path using `link_dir`.

### Scheduler Semantics

The scheduler may stay generic, but its keys now represent files. This removes directory-level dirty rescheduling during a run:

- no `dirtyDuringRun` bookkeeping for sibling file arrivals
- no need to rerun a whole directory task because another file landed mid-transfer
- retries are scoped to a single linked file

Waiting count in the UI should reflect queued file tasks, not queued config entries.

### Logging and Event Binding

Rclone stats output is still bound to the active file task that launched the process.

Service logs and recent events should identify the task in a way that remains understandable with many files from the same config entry. Prefer either:

- `[<config-name>] <relative-path>: ...`
- or a similarly compact file-oriented label

### Testing

Tests must cover:

- startup scan creates file tasks for multiple files in one directory tree
- scheduler can start multiple sibling files concurrently up to the global limit
- rclone command uses linked file as source
- destination remote directory is computed correctly and always ends with `/`
- successful upload deletes only the uploaded link file
- cleanup removes only empty parent directories and stops at `link_dir`
- failed upload preserves the link file and schedules retry only for that file
- duplicate filesystem events do not create duplicate file tasks
- UI task naming truncates to 40 display columns without misaligning Chinese names

## Non-Goals

- Changing config schema
- Changing allowed media extension rules
- Introducing per-job concurrency limits separate from the global scheduler
- Uploading directories as a batch fallback
