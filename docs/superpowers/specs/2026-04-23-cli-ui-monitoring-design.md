# CLI Monitoring UI Design

## Goal

Polish the current terminal dashboard into a denser, more legible monitoring console for remote use over SSH, while keeping it passive and stable in Termius.

## Context

Current UI behavior:

- The application renders a passive dashboard in the alternate screen.
- Output is currently single-column plain text.
- The dashboard shows one summary line, active job lines, and a recent event list.
- The dashboard is used over SSH on a remote server, with Termius as the client.

This environment changes the design constraints:

- Rendering must remain robust in a remote terminal.
- Layout should avoid fragile multi-column grids or heavy Unicode dependence.
- The dashboard should feel closer to tools like `btop`, `k9s`, or `lazygit`, but with less layout risk.
- The UI remains read-only. No keyboard interaction is required.

## Requirements

1. Keep the dashboard passive: display-only, no interactive controls.
2. Preserve `Recent Events` as the dominant area of the screen.
3. Improve visual hierarchy so the screen reads as a monitoring console rather than formatted logs.
4. Work well in SSH + Termius with moderate terminal widths.
5. Use ANSI color, emphasis, and limited Unicode box drawing only where they materially improve readability.
6. Avoid aggressive layout complexity that could break alignment in remote terminals.

## Non-Goals

- No interactive TUI controls
- No mouse support
- No multiple tabbed views
- No heavy dependency on third-party UI frameworks
- No highly dynamic layout that depends on exact terminal width measurement

## Proposed Direction

Adopt a `steady monitoring console` style:

- high information density
- light framed sections
- restrained color palette
- strong emphasis on summary metrics
- large event stream area

The design should borrow the structural clarity of strong terminal dashboards without copying highly complex layouts that are fragile in SSH clients.

## Layout

Use a three-section vertical layout:

### 1. Summary Bar

A compact fixed header at the top of the screen.

Purpose:

- give immediate system health context
- expose the current state without reading long sentences
- surface the few numbers that matter most

Content:

- overall state
- active jobs
- queued jobs
- max parallel uploads
- last refresh time

Preferred expression:

- label-based metrics instead of prose
- examples: `STATE RUNNING`, `ACTIVE 2/5`, `QUEUE 1`, `UPDATED 15:04:05`

This section should be one or two lines maximum.

### 2. Active Jobs

A medium-height panel under the summary bar.

Purpose:

- show current work in progress
- make active transfers comparable at a glance

Each job should render in a stable column layout rather than free-form sentences.

Suggested fields:

- job name
- progress percent
- transferred / total
- speed
- ETA
- stage or state label

Preferred style:

- fixed ordering
- aligned columns where practical
- optional short progress bar only if it remains stable in remote terminals

Example shape:

`movies-a   83%   832 MiB / 1.0 GiB   29.8 MiB/s   ETA 00:05   UPLOADING`

or

`movies-a   [████████░░] 83%   29.8 MiB/s   ETA 00:05`

The shorter progress-bar variant is preferred only if alignment remains reliable in Termius.

### 3. Recent Events

This is the primary panel and should occupy the largest area.

Purpose:

- show the narrative of system activity
- make it easy to answer “what just happened?”

Design principles:

- newest events first
- each event split into time, tag, and message
- event stream should look like a structured feed, not raw log output

Suggested fields:

- time
- event tag
- message

Example shape:

- `15:04:03  DONE    movie/file-02.mkv copied`
- `15:04:05  WAIT    queue full, pending`
- `15:04:07  ERR     rclone exited with code 1`

The panel title should expose count context, for example:

- `RECENT EVENTS (20)`
- `RECENT EVENTS  last 20`

## Visual Language

### Framing

Use light box-drawing characters:

- `┌ ┐ └ ┘ │ ─`

Do not use:

- dense double-line borders
- decorative ASCII art headers
- many nested boxes

The goal is structural separation, not ornamental complexity.

### Color

Use a restrained palette with a small number of semantic roles:

- cyan or blue for panel titles and structural accents
- green for healthy or successful states
- yellow for waiting, queued, or in-progress states
- red for failures or errors
- default foreground for normal content

Do not color entire lines by default. Color should guide scanning, not dominate the screen.

### Emphasis

Highlight only the information that deserves immediate attention:

- state labels
- active and queued counts
- transfer speed
- ETA
- error tags

De-emphasize:

- separators
- timestamps
- repeated framing characters

## State Model Presentation

Current natural-language status lines should shift toward label-based dashboard metrics.

Recommended vocabulary:

- `IDLE`
- `RUNNING`
- `QUEUED`
- `ERROR`
- `DONE`

This change makes the interface feel more like a monitoring surface and less like translated prose output.

## Event Stream Presentation

The event stream is the visual centerpiece, so it needs more structure than the current “timestamp + full message” form.

### Event Tags

Introduce lightweight event categories where possible:

- `INFO`
- `DONE`
- `WARN`
- `ERR`
- `SCAN`
- `LINK`
- `UPLOAD`

These tags do not need a full event taxonomy refactor in phase one. They can be inferred from existing messages where practical, with fallback to a generic tag.

### Timestamp Formatting

Prefer compact time-only display for same-day events:

- `15:04:03`

Use full date only when needed, such as older retained events or cross-day context.

### Message Compaction

Long paths should be shortened when needed so the event stream remains readable in narrower terminals.

Examples:

- `TV/.../Episode-08.mkv`
- `Movies/Inception.mkv`

The compaction rule should preserve the filename and enough path context to remain useful.

### Empty State

When there are no events, keep the panel visually complete rather than dropping to a bare placeholder line.

Examples:

- `No recent events`
- `Watching for new files...`

## Compatibility Strategy

Because the target environment is remote SSH with Termius:

- prefer a mostly vertical layout
- avoid deep nested grids
- avoid requiring exact terminal width calculations in phase one
- keep Unicode usage light and conventional
- keep redraw logic simple and deterministic

The interface should degrade gracefully in narrower windows by truncating content rather than breaking the frame structure.

## Phased Rollout

### Phase 1

Highest-value, lowest-risk polish:

- add panel titles and light borders
- redesign summary bar into label-based metrics
- restructure active jobs into stable columns
- redesign event stream into tagged recent events
- keep event panel as the dominant area

### Phase 2

Optional refinements after the new layout is stable:

- short textual progress bars
- smarter truncation rules
- richer status coloring
- compact same-day vs full-date timestamp rules

## Testing

Add renderer tests for:

1. Idle state frame
2. Running state summary metrics
3. Active jobs panel with aligned fields
4. Event stream ordering with newest first
5. Empty-state event panel
6. Narrow-width fallback behavior if width-dependent formatting is added
7. ANSI enable/disable behavior if color is made configurable

Manual verification should also check:

- Termius rendering over SSH
- alternate-screen repaint behavior
- readability with a small number of events and with a dense event list

## Implementation Notes

Likely impacted files:

- `internal/ui/renderer.go`
- `internal/ui/renderer_test.go`
- `internal/ui/live.go` only if rendering helpers need small support changes
- `internal/app/service.go` only if event ordering or event tagging data needs adjustment

Keep the implementation centered on the current renderer-first design. This work should remain a targeted UI polish, not a TUI framework rewrite.
