# CLI UI Alternate Screen Refresh Design

## Goal

Make the live terminal UI resilient to terminal window resizing by moving the dashboard into the terminal alternate screen buffer and repainting the full screen on each refresh.

This change is intended to stop the corrupted output that appears when the terminal width changes and previously rendered lines are reflowed by the terminal emulator.

## Scope

This design changes only the terminal refresh mechanism used by the live dashboard.

It does not change:

- file watching
- upload scheduling
- `rclone` invocation
- status or event content
- event retention rules
- dashboard layout structure

## Current Problem

The current refresh logic rewrites the dashboard by:

1. tracking how many logical lines were printed in the previous frame
2. moving the cursor upward by that many lines
3. clearing downward from that position
4. printing the new frame

That approach breaks when the terminal window is resized.

Root cause:

- the code tracks logical newline count, not actual visual rows on screen
- when the terminal width changes, previously printed long lines are rewrapped into a different number of visual rows
- cursor-up movement based on the old logical row count no longer lands at the true frame origin
- the next repaint starts in the wrong location, so old and new output become interleaved

This is why the UI looks correct until the user resizes the Termius window.

## Desired Behavior

The live dashboard should behave like a self-contained full-screen panel while the program is running.

Behavior rules:

- when the live UI starts, switch to the terminal alternate screen buffer
- hide the normal shell content while the dashboard is active
- on every refresh, move the cursor to the top-left corner of the alternate screen
- clear from that point to the end of the screen
- print the full dashboard frame again
- when the program exits, restore the normal screen buffer
- return the user to the original shell screen cleanly

The dashboard contents themselves remain unchanged by this design. Only the repaint mechanism changes.

## Why Alternate Screen

The alternate screen buffer is the right primitive here because it gives the dashboard a stable drawing surface that is independent of the shell scrollback area.

This avoids the main weaknesses of the current approach:

- no dependence on previous visual line count
- no need to infer how the terminal wrapped older lines
- no accumulation of partial frames in shell history
- full-screen redraw remains valid after a resize

The accepted tradeoff is that the live dashboard will not remain in the main terminal scrollback after the program exits.

## Rejected Alternatives

### Continue Using Relative Cursor Rewind

Possible mitigation:

- detect resize events
- reset frame bookkeeping
- keep using cursor-up by remembered line count

Why not:

- still relies on line-count estimation
- still sensitive to wrap differences across terminals
- only reduces the failure rate instead of removing the root cause

### Fixed Region Repaint in Main Screen

Possible approach:

- reserve a known screen region
- clear and repaint only that region

Why not:

- still depends on terminal geometry assumptions
- more fragile when the dashboard height changes
- more complex without solving the core resize problem as directly as alternate screen redraw

## Rendering Changes

`internal/ui/live.go` should stop treating the previous frame height as part of the redraw contract.

Instead, the refresh layer should support three responsibilities:

1. enter alternate screen
2. rewrite the current frame from a fixed screen origin
3. leave alternate screen

Suggested ANSI behavior:

- enter alternate screen: `ESC[?1049h`
- move cursor home: `ESC[H`
- clear to screen end: `ESC[J`
- leave alternate screen: `ESC[?1049l`

The frame rewrite operation should produce output equivalent to:

```text
ESC[H ESC[J <content>
```

This means the redraw path no longer needs `previousLines`.

## App Loop Changes

`internal/app/service.go` should manage live screen lifecycle around `uiLoop`.

Behavior:

- before the first repaint, enter alternate screen
- during each tick, render the dashboard and repaint from the fixed origin
- when the loop exits, leave alternate screen
- preserve the final newline behavior after restoration so the shell prompt appears normally

The loop should continue to repaint once per second exactly as it does today.

## Error Handling

This feature does not add new runtime error branches.

If the terminal does not interpret alternate-screen ANSI sequences, the output may degrade to raw escape-code behavior, but that is acceptable for this implementation because the current code already depends on ANSI cursor control.

## Testing

Required tests:

- entering alternate screen returns the expected ANSI sequence
- leaving alternate screen returns the expected ANSI sequence
- frame rewrite returns home-cursor plus clear-screen plus content
- redraw no longer depends on previous line count

Existing renderer and service behavior tests should remain unchanged unless interface adjustments require mechanical updates.

## Implementation Boundaries

Expected file changes:

- `internal/ui/live.go`
- `internal/ui/live_test.go`
- `internal/app/service.go`

No other package should need behavioral changes for this fix.
