# Debian Media Backup Design

## Goal

Build a Go CLI program for Debian that watches one or more source directories, creates hard-link backups for newly completed media files, uploads the hard-link trees to Google Drive through `rclone`, and cleans uploaded link content without deleting the configured link root directories.

## Scope

The program runs in the foreground as a single process. It reads configuration from a file, performs an initial catch-up scan, monitors source directories recursively, and processes upload jobs through one serial queue.

Supported media extensions by default:

- `.mkv`
- `.mp4`
- `.m2ts`
- `.ts`

## Configuration

The program reads a YAML configuration file. Global options stay minimal. Each source directory maps to exactly one hard-link directory and one `rclone` destination.

Example:

```yaml
poll_interval: 1s

jobs:
  - name: 4k-remux-sgnb
    source_dir: /dld/upload/4K.REMUX.SGNB
    link_dir: /dld/gd_upload/4K.REMUX.SGNB
    rclone_remote: gd1:/sync/Movie/4K.REMUX.SGNB
    stable_duration: 60s
    retry_interval: 10m
    extensions: [".mkv", ".mp4", ".m2ts", ".ts"]
    rclone_args:
      - --drive-chunk-size=256M
      - --checkers=5
      - --transfers=5
      - --drive-stop-on-upload-limit
      - --stats=1s
      - --stats-one-line
      - -v
```

Configuration rules:

- `source_dir`, `link_dir`, and `rclone_remote` are required.
- One `source_dir` corresponds to one unique `link_dir` and one unique `rclone_remote`.
- If `extensions` is omitted, use the default extension list above.
- If `stable_duration` is omitted, default to `60s`.
- If `retry_interval` is omitted, default to `10m`.
- If `rclone_args` is omitted, use a safe default matching the sample above.

## Runtime Model

The program has five logical components:

1. `config`: parse and validate YAML configuration.
2. `watcher`: recursive directory watch, initial scan, stable-file detection, hard-link creation.
3. `queue`: deduplicated serial upload scheduling across all jobs.
4. `rclone`: invoke `rclone copy`, read progress output, return success or failure.
5. `ui`: print idle state and transfer state to the terminal.

Suggested package layout:

- `cmd/media-backup/main.go`
- `internal/config`
- `internal/watcher`
- `internal/queue`
- `internal/rclone`
- `internal/ui`

## File Discovery

### Initial Catch-up Scan

On startup, the program recursively scans every configured `source_dir`.

For each file:

- Ignore directories.
- Ignore files whose extension is not in the allowed media list.
- Check whether the file is stable.
- Compute its relative path from `source_dir`.
- Ensure the parent directory exists under `link_dir`.
- Create a hard link in `link_dir` using the same relative path if the link target does not already exist.

If at least one valid file is linked or already exists in `link_dir`, enqueue the corresponding job for upload.

This startup scan is required so a restart does not miss media files that already exist on disk.

### Recursive Watch

After the initial scan, the program registers watches for:

- each configured `source_dir`
- all existing subdirectories under each `source_dir`

It must handle:

- file create
- file rename into place
- file close after write
- directory create
- directory rename into place

When a new subdirectory appears, the program must:

- register a watch for that subdirectory
- recursively register watches for nested subdirectories if they already exist
- scan that subtree immediately to avoid missing files created before watch registration completes

## Stable File Detection

A file must not be linked or uploaded until it is considered complete.

The chosen rule is:

1. observe a relevant filesystem event such as `close_write`, `create`, or `rename`
2. wait for the file to exist and not be a directory
3. verify the file size remains unchanged for `stable_duration`

Only then is the file treated as ready.

This supports downloaders that write to a temporary name and rename on completion, and also avoids linking a still-growing file.

## Hard-link Behavior

For each ready media file:

1. derive the relative path under `source_dir`
2. join that relative path onto `link_dir`
3. create missing parent directories inside `link_dir`
4. create a hard link pointing to the source file

Example:

- source file:
  `/dld/upload/4K.REMUX.SGNB/爆裂鼓手 (2014) {tmdb-244786}/爆裂鼓手 (2014) {tmdb-244786} - 2160p.BluRay.HDR.HEVC.TrueHD.7.1-ROBOT.mkv`
- linked file:
  `/dld/gd_upload/4K.REMUX.SGNB/爆裂鼓手 (2014) {tmdb-244786}/爆裂鼓手 (2014) {tmdb-244786} - 2160p.BluRay.HDR.HEVC.TrueHD.7.1-ROBOT.mkv`

If the target file already exists, the program should treat it as already linked and continue without error.

## Queue and Scheduling

Uploads are serialized globally. The queue item is a whole configured job, not an individual file.

Each job maintains three scheduling flags:

- `queued`: the job is already waiting in the queue
- `running`: the job is currently executing `rclone`
- `dirty`: new content appeared while the job was queued or running

Rules:

1. First new linked file for a job:
   if `queued=false` and `running=false`, enqueue the job and set `queued=true`.
2. New file while already queued:
   do not enqueue again; set `dirty=true`.
3. New file while `rclone` is running:
   do not interrupt `rclone`; set `dirty=true`.
4. After a successful upload round:
   if `dirty=true`, immediately enqueue the job again for another round.
5. After a failed upload round:
   keep all files in `link_dir`, clear `running`, and re-enqueue the job after `retry_interval`.

This satisfies the requirement that a second file created during an active upload is not expected to be included in the first `rclone` run, but must be sent by a later run of the same job.

## Rclone Execution

The program invokes `rclone copy` directly because `rclone` is already installed and configured.

Base command:

```bash
rclone copy <link_dir> <rclone_remote>
```

Recommended default arguments:

```text
--drive-chunk-size=256M
--checkers=5
--transfers=5
--drive-stop-on-upload-limit
--stats=1s
--stats-one-line
-v
```

`--stats=1s --stats-one-line` is preferred over `--progress` because it produces stable line-based status that is easier for the Go process to capture and display.

`rclone` runs under `exec.CommandContext`. The program continuously reads output from `stdout` and `stderr`, extracts the latest transfer statistics line, and keeps the full raw output for logging.

## Terminal Output

When no upload work is active, print once per second:

```text
[YYYY-MM-DD HH:MM:SS] 当前状态：空闲
```

When `rclone` is active, print:

```text
[YYYY-MM-DD HH:MM:SS] 当前状态：正在传输
```

Then print the most recent parsed `rclone` stats line, for example:

```text
Transferred: 1.234 TiB / 3.560 TiB, 35%, 82.114 MiB/s, ETA 6h12m
```

If no newer stats line is available for the current second, repeat the latest known line.

The terminal output is for current state visibility, not full audit logging.

## Logging

Write detailed logs to a local file such as:

```text
./logs/media-backup.log
```

Log items include:

- startup configuration summary
- watcher registration
- link creation
- queue transitions
- `rclone` command start and exit code
- `rclone` raw output
- retry scheduling
- cleanup actions
- shutdown events

## Cleanup Rules

Cleanup happens only after an upload round succeeds and the job is not dirty.

Cleanup must obey these restrictions:

- delete files under `link_dir`
- remove empty subdirectories under `link_dir`
- never delete the `link_dir` root directory itself

Required cleanup algorithm:

1. before cleanup, re-check the job state
2. if `dirty=true`, skip cleanup and re-enqueue the job
3. otherwise, walk only the contents inside `link_dir`
4. delete linked files found under `link_dir`
5. remove empty child directories under `link_dir` from deepest to shallowest
6. stop directory removal at `link_dir` and keep it present even if empty

This avoids deleting the configured root link directory while still clearing completed content.

## Failure Handling

If `rclone copy` fails because of network interruption, quota stop, or process error:

- do not delete anything from `link_dir`
- keep the root `link_dir` directory
- schedule the same job for retry after `retry_interval`
- if more files arrive before the retry starts, do not create duplicate queue entries; only mark the job dirty

## Signals and Shutdown

The program runs as a foreground CLI process.

On `SIGINT` or `SIGTERM`, it should:

- stop accepting new watch events
- stop the idle printer
- cancel any active `rclone` process through context cancellation
- flush logs
- exit with a clear shutdown message

Daemonization is out of scope. Background execution can be handled externally through `systemd`, `nohup`, or similar tools.

## Build and Packaging

Provide a build script that creates a static Linux executable.

Recommended output:

- `dist/media-backup-linux-amd64`

Recommended build command:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/media-backup-linux-amd64 ./cmd/media-backup
```

This produces a pure Go statically linked binary suitable for Debian deployment.

An optional second target for `linux/arm64` can be added later if needed, but is not required for the first version.

## Testing Strategy

Implementation should cover:

- config parsing and validation
- extension filtering
- relative-path mapping from `source_dir` to `link_dir`
- hard-link creation with parent directory creation
- job deduplication state transitions
- retry scheduling
- cleanup that removes child content but never removes the `link_dir` root
- `rclone` output parsing for status display

Watch integration tests should use temporary directories where possible. The `rclone` process should be abstracted behind an interface so tests can replace it with a fake executor.

## Non-goals

The first version does not need:

- a web UI
- daemon mode
- parallel uploads across jobs
- database persistence
- resumable state files beyond what filesystem scanning already reconstructs
