# Design QA — Rex native workspace

Source: https://www.superlogical.com/updates/public-testing-beginning, `rex-screenshot@2x.webp`.
Target: native Keel/Gio app at 1057 × 639 dp, scale 2, default paused state.
Evidence: `evidence/reference.png`, `evidence/native.png`, and the combined `evidence/comparison.png`.

Iteration 1 found actionable differences in font weight, transcript width, divider positions, toolbar glyphs, and game coordinates. Replaced the thin system variable monospace with Menlo; loaded the system interface font family; corrected pane ratios and 12.5 dp terminal text; extracted individual source icons; measured food and snake components in the original image to align game coordinates.

Iteration 2 found panel backgrounds too yellow, label rules running through text, a missing right game border, and square outer corners. Restored panel-specific gradients, opened the terminal frame rules at label positions, aligned all 58 border characters explicitly, and clipped the full native window to rounded corners. Rechecked the combined source/implementation image after these fixes.

Final result: pass for the reference workspace at the reference viewport. No remaining P0/P1/P2 visual issues in that scope. This is a visual recreation, not a claim of exact pixel equality. Minor remaining differences: Helvetica Neue interface text versus the original system font rendering, approximated translucent wallpaper tint, and subtle border / font rasterization differences. Responsive resizing and expanded panes are native adaptations without a supplied reference state.

Interaction verification passed: pause/resume, restart, Git staging/commit demo, pane expand/restore, new/close tab, and executing `printf` and `pwd` through the local shell. `go build` and `go vet` passed. macOS bundle built with the current Keel CLI and signed ad hoc. Desktop live screen QA is unavailable while the Mac is locked; the native renderer and automation screenshots were inspected instead.

## Functional revision — 0.2.0

The static transcript/command-preview implementation has been replaced by persistent PTY terminals and real Git data. Historical `native.png` / `comparison.png` show the earlier visual recreation; current evidence is `functional.png`, `git-functional.png`, `snake-pty.png`, and `vim-functional.png`. Session text, titles, file lists and diff content now depend on the live programs and project rather than matching the source's recorded demo data.

Native QA retained the source window tint, rounded panes, traffic lights, icon assets, titlebar/tab placement and initial three-pane geometry. Dynamic content and the native Git actions are intentional product changes required for a working terminal. This revision does not claim pixel equality with the original screenshot.

Functional checks: persistent environment / cwd; PTY tty detection; interrupt; resize; ANSI colors; alternate screen; independent sessions; process cleanup; Vim Unicode save; nonblocking large input; GUI tab/split/zoom/close; palette input focus and restoring terminal focus; installed Codex CLI execution; and actual Git stage, unstage and commit in a temporary repository. A close/read race was found and fixed by draining and closing the emulator's input pipe before final disposal. `go test -race`, `go vet`, and repository-wide `go build ./...` passed.

## Appearance correction — 0.2.1

The user identified opaque rectangular backgrounds in the screenshot-derived window controls and tab icons. Removed the bitmap loading and painting path from the app UI. Traffic lights, device symbol, tab stack, shell/Git icons and pane toolbar now use Gio vector geometry; only the package's app icon remains a raster asset. Traffic lights show hover marks and become gray when the window loses focus. Existing window actions and real PTY sessions are retained.

Inspected the actual native renderer output in `evidence/appearance.png`, including three tabs, inactive tab icons and the pane toolbar. The pale rectangular crop backgrounds are gone. Build, vet and whitespace validation passed.

## Native traffic lights — 0.2.2

The earlier vector traffic lights were still custom controls. macOS now renders its own NSWindow standard buttons over the transparent full-size titlebar. Rex does not draw or install Gio click regions for these buttons on macOS. Keel's opt-in NativeTrafficLights window option restores AppKit controls after Gio configures frameless mode and after subsequent resize/config events. Other windows keep their existing behavior.

The live AppKit acceptance fixture (`../keel/ui/window/testdata/nativebuttons`) verified the original NSTheme button/cell instances, their native selectors, visibility after resizing, native minimize and native close. Renderer-only/headless screenshots cannot include these AppKit views and intentionally leave their area empty.

## Configurable native button placement — 0.2.3

Rex uses a 44dp titlebar with native traffic lights centered at y=22dp, a 15dp left inset and 23dp center spacing. Keel exposes TrafficLightLayout (height, left inset, vertical offset and optional center spacing), plus SetTrafficLightLayout for live style changes. AppKit button/cell instances and their view hierarchy are preserved. A native update observer reapplies geometry after AppKit's own layout pass, which otherwise resets close/minimize positions.

The live acceptance fixture checks 36/44/64dp bars, both offset directions, custom spacing, layout changes without recreating a window, resize persistence, native button hit testing, click-through to content tabs, minimize and close.

## Git text and layout polish — 0.2.4

The user supplied evidence of wrapped status codes and tab characters rendered as missing glyph boxes. File rows now have a fixed 23dp height and an 18dp status column that cannot shrink; paths take the remaining width and truncate to one line. Diff tabs expand at 8-column stops, excluding unified diff's marker column, while plain file previews use source columns. Width calculation accounts for wide characters and grapheme clusters. Other control bytes are escaped as readable text. Long code lines remain intact in a two-axis scrolling viewport, and empty lines retain their height.

Removed duplicated cwd text from shell pane titles and clarified Stage/Unstage according to the selected file's actual index state. Reduced excessive unused space in the Git body. Inspected native wide (740dp) and narrow (360dp) fixtures, including long paths, tabs, added/removed lines and horizontal overflow. Evidence: `git-polish-wide.png`, `git-polish-narrow.png`.

## GoRex interaction revision — 0.3.0

Reference: https://github.com/egoist/gorex, inspected on 2026-10-07. Implemented a separate PTY/VT daemon, private Unix RPC, ordered atomic layout persistence, session reattachment, saved tab names/order/splits/focus/zoom, foreground process/cwd inspection, activity and attention indicators, focused program tiles, native menus, theme and font preferences, host information, and opt-in system notifications. UI symbols remain vector paths and traffic lights remain native NSWindow controls.

Interaction checks cover fuzzy commands and pane navigation; arrow/Enter/Escape handling; focus after repeated command and split actions; directory validation; Unicode tab names and terminal text; keyboard splitting, directional focus, divider adjustment, equalizing, zoom and close; confirmation for a running program; Unicode search across scrollback; actual staging, unstaging and commits in a private temporary Git repository; window close/reopen with the same daemon; and retaining alternate-screen sessions. Pointer-event tests verify inactive-tab dragging preserves the active workspace and repeated divider movements do not accumulate coordinate drift.

Two defects were found in persistence and focus handling. Optional layout reads initially serialized a JSON null that overwrote the saved layout; layout requests now omit absent payloads. Keel overlays restore their own editor tags, while the terminal uses a custom Gio tag; terminal focus requests now run after the outer view completes overlay handling. Source/diff tabs remain expanded, long filenames remain one row, and the terminal loads available system PingFang fonts for Chinese fallback.

Validation: `go test -race ./...`, `go vet ./...`, `python3 tools/qa_workspace.py`, arm64 App bundle build, version metadata, and ad-hoc signature verification. The QA script launches only in-memory Keel windows and private service processes; it cleans them after testing. It does not open native acceptance fixtures or touch normal app sessions. Evidence: `gorex-light.png`, `gorex-dark.png`, `gorex-search.png`, `gorex-restored.png`.

Service-failure QA also stops the private daemon while the UI remains open, then uses Restart to launch a new service and verify the replacement shell accepts commands. Native menu construction and notification integration compile in the bundle. Their OS presentation and permission flow were not exercised in this headless run. Sessions survive normal app closure while their daemon lives; an OS reboot or daemon failure requires starting new shells in the saved layout. This revision does not claim exact pixel equality with the static Rex reference or support terminal image protocols / an SSH connection manager.


## Internal button icon polish — 0.3.1

Redrew split, expand/restore, close, new-tab and command-palette glyphs on a shared 24-unit vector grid. Glyphs use rounded stroke ends, consistent optical padding and 16dp square bounds; header actions now center inside their hit targets instead of stretching 13×10 drawings. New-tab and palette glyphs are rendered inside the button itself, with hover feedback. A zoomed pane displays the restore glyph and an accurate accessible action name. Program cards share theme-aware outlines and separate glyph/frame rendering. The macOS bundle still uses the original application icon.

Inspected `buttons-light-1x.png`, `buttons-light-2x.png`, `buttons-dark-1x.png` and `buttons-dark-2x.png`. Fixed a Retina coordinate-scaling error in the curved command glyph found during inspection. Background interaction QA, race tests, vet, bundle build and signature verification passed; no native test windows were opened.

## GoRex icon alignment — 0.3.2

- Replaced hand-drawn control glyphs with gorex's Lucide SVG assets; retained ISC/CC0 notices.
- Added icons to Git actions, contextual menus, command results, search and directory headings.
- Program tiles use the reference's Simple Icons and colors; pane headers use bare glyphs.
- Pane controls use 26dp hit areas with centered 16dp glyphs; zoom menus reflect restore state.
- Normalized SVG geometry where the Keel parser needs explicit path commands, including rounded split corners and OpenAI internal gaps.
- Inspected light/dark previews at 1x/2x and ran headless workspace QA without opening AppKit windows.


## macOS native menu deadlock regression

AppKit invokes menu actions on its main thread. Calling `core.Update` directly from that callback can make Gio invalidate and flush events inline while its invalidation lock is held. The exported menu bridge now posts from a goroutine, so AppKit returns before Gio is awakened. A synchronous bridge reproduced the freeze at New tab; the asynchronous bridge completed New tab, Split right, Split down and Command palette with two tabs, three panes in the active tab and an open palette.

The optional `keelnativeqa` build tag exercises the actual NSMenu targets through `keel run`. The fixture requires a separate state directory and ends its disposable sessions on success. It exits with a stack dump after 12 seconds if the native loop stalls. Ordinary builds exclude the fixture.

```sh
rex_qa_state=$(mktemp -d /tmp/keel-rex-menu-state.XXXXXX)
REX_MENU_QA=1 KEEL_REX_DIR="$rex_qa_state" GOFLAGS=-tags=keelnativeqa go tool keel run
```

The fixture prints its artifact directory. `result.json` must report `tabs: 2`, `panes: 3`, and `palette: true`; a nonzero exit or `timeout.txt` is a failed run. Existing tests cover keyboard routing, tab shrinking, pointer reordering and close buttons separately.

Add `REX_ICON_QA=1` to capture AppKit’s actual Dock icon as `dock-icon.png` and compare it with the icon passed by `keel run`. The fixture normalizes Retina size and color profiles before comparing pixels. This verifies the `keel.json` icon path without packaging the app.


## Terminal IME composition

The custom terminal keeps composing text and rune-based replacement ranges locally, publishes the input snippet, selection and candidate geometry to Gio, and sends the final text to the PTY after composition ends. Cancelled preedit sends nothing. Printable spaces use text events; composition keystrokes do not reach the terminal application. The local overlay follows the terminal cursor in normal and alternate screens.

`TestTerminalIMEPreeditCommitAndCandidateGeometry` covers preedit replacement, candidate selection, commit, cancellation, consecutive Chinese/emoji input, normal/alternate screens and 1×/2× scale. The optional native fixture also checks AppKit's `setMarkedText` and `insertText` callbacks against a real PTY: `imepreedit` stays local and `你好中文` appears in the session after commit. Run the native-menu fixture above with `REX_IME_QA=1` added. This verifies native callback and PTY delivery; input-source switching and interaction with a specific candidate engine require manual QA.

Platform bindings live in Keel. Rex configures menus in `menu.go`, appearance in `appearance.go`, and terminal process queries in `process_native.go`, using Go APIs only. Native regression helpers are provided by `ui/window` under the `keelnativeqa` build tag.

On Windows the Go menu model uses Win32 menus; Linux uses Keel window menus on X11/Wayland. Rex terminal consumes `core.NextEditAction` for copy, paste and select all. Native desktop validation must be run on each target platform; cross-compilation alone does not confirm desktop interaction.
