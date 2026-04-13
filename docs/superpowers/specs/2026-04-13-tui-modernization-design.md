# TUI Modernization Design

**Date:** 2026-04-13  
**Author:** User + AI  
**Status:** Approved for implementation

## Purpose

Modernize the work-bridge TUI to support session migration, global skills, and MCP management with a simple, elegant, and user-friendly interface. Leverages modern terminal capabilities (TrueColor, hyperlinks, mouse, OSC 52 clipboard) while maintaining graceful degradation.

## Architecture

### State Machine

```
Loading → SessionList → SessionPreview → Applying → Done
           ↑    ↓            ↓              ↓
           └────┴────────────┴──────────────┘ (back navigation)
```

### States

| State | View | Purpose |
|-------|------|---------|
| `StateLoading` | loading.View | Spinner + "Discovering sessions..." |
| `StateSessionList` | session.List | Browse/filter available sessions |
| `StateSessionPreview` | session.Preview | Show session metadata before migration |
| `StateApplying` | result.Applying | Spinner during migration |
| `StateResult` | result.View | Migration complete confirmation |
| `StateError` | error.View | Error display with retry option |

### File Structure

```
internal/ui/
├── app.go                    # MainModel, state machine, ProgramOptions
├── styles/
│   ├── theme.go              # TrueColor palette, tool brand colors
│   └── palette.go            # Color profile adaptation
├── views/
│   ├── loading/
│   │   └── loading.go        # Spinner + loading message
│   ├── session/
│   │   ├── list.go           # Session list with custom delegate
│   │   └── preview.go        # Session detail card
│   ├── result/
│   │   └── result.go         # Applying spinner + Migration complete screen
│   └── error/
│       └── error.go          # Error display with retry
└── common/
    ├── help.go               # Reusable help bar
    └── clipboard.go          # OSC 52 clipboard helper
```

### Key Decisions

- `MainModel` holds state, delegates to sub-models
- Each view is its own package with clean boundaries
- Shared styles in `styles/`, shared components in `common/`
- Error view is a separate state — errors never silently swallowed
- Existing `cli/root_tui.go` entry point unchanged

## Visual Design

### TrueColor Palette

| Token | Color | Usage |
|-------|-------|-------|
| `bgPrimary` | `#1a1b26` | Main background |
| `accent` | `#7aa2f7` | Primary accent (focus, borders) |
| `textPrimary` | `#c0caf5` | Main text |
| `textSecondary` | `#565f89` | Subtitles, metadata |
| `success` | `#9ece6a` | Success states |
| `warning` | `#e0af68` | Warning states |
| `error` | `#f7768e` | Error states |

### Tool Brand Colors

| Tool | Color | Usage |
|------|-------|-------|
| Claude | `#66b366` (green) | Dot indicator, highlights |
| Gemini | `#8b5cf6` (purple) | Dot indicator, highlights |
| Codex | `#f5a623` (orange) | Dot indicator, highlights |
| OpenCode | `#4fc3f7` (cyan) | Dot indicator, highlights |

### Color Profile Fallback

Use `colorprofile` to detect terminal color support:
- TrueColor (direct) → use full palette
- 256-color → map to nearest 256-color index
- 16-color → use ANSI basic colors

## Modern Terminal Features

| Feature | Protocol | Usage |
|---------|----------|-------|
| TrueColor | RGB escapes | All colors, gradients on borders |
| Hyperlinks | OSC 8 | File paths, project roots clickable |
| Clipboard | OSC 52 | `c` copies session ID to system clipboard |
| Mouse | tea.MouseMsg | Click to select, scroll wheel navigation |
| Keyboard Enhancement | bubbletea v2 | Distinguish `Esc` from `Ctrl+[`, fast key chords |
| Spinner | bubbles/spinner | Dots animation during loading |
| AltScreen | ProgramOption | Clean fullscreen, restore on quit |
| Color Profile | colorprofile | Auto-detect, degrade gracefully |

## Component Details

### Loading View

- Centered spinner (`bubbles/spinner`, dot animation)
- Message: "Discovering sessions..."
- If no tools detected: "No AI tools found. Make sure Claude/Gemini/Codex/OpenCode is installed."
- Auto-transitions to SessionList when data loads

### Session List View

- Custom list delegate with TrueColor dot per tool
- Filtering enabled (default `/` trigger)
- Help bar at bottom
- Key bindings:
  - `enter` / `space` → Preview
  - `/` → Filter
  - `c` → Copy session ID (OSC 52 + flash toast)
  - `tab` → Switch to Skills/MCP pane (future)
  - `q` / `ctrl+c` → Quit

**List Item Format:**
```
  ● Claude  • abc123  ChatExport
    /path/to/project  •  142 msgs  •  2026-04-10
```

The `●` dot is TrueColor — green for Claude, purple for Gemini, etc.

### Preview View

- Card layout with rounded borders (lipgloss `BorderStyle`)
- Metadata rows: Tool, Created, Updated, Messages, Skills, MCPs
- Hyperlink on project root path
- Actions: `space` to migrate, `esc` to go back

### Applying View (during migration)

- Centered spinner (`bubbles/spinner`)
- Message: "Migrating session..."
- Non-interactive until migration completes or fails

### Result View (after migration)

- Big `✓` in green
- "Migration Complete"
- Destination path as hyperlink
- `enter` to select another, `q` to quit

### Error View

- Red `✗` icon
- Error message in full (no truncation)
- `r` to retry, `q` to quit

### Help Bar Format (always visible at bottom)

```
 ─────────────────────────────────────────────────────────
  ↵ select  / filter  c copy  tab tools  q quit
```

## Message Flow

```
LoadingView.Init()     → start background load
workspaceLoadedMsg     → SessionList (success) or ErrorView (fail)
SessionSelectedMsg     → PreviewView
MigrationStartedMsg    → Applying state (spinner)
MigrationCompleteMsg   → ResultView
MigrationFailedMsg     → ErrorView
```

## Error Handling

| Scenario | Behavior |
|----------|----------|
| No sessions found | List shows "No sessions found. Run an AI tool in this project first." with `q` to quit |
| Load fails (DB error) | ErrorView with full message, `r` retry, `q` quit |
| Migration fails | ErrorView shows target path + error, `esc` back to preview |
| Terminal too small | Show "Terminal too small, resize and press `q`" message |
| Non-TrueColor terminal | `colorprofile` detects, falls back to 256-color palette |
| Mouse not supported | Graceful — keyboard still works, no error |
| Clipboard not supported | `c` shows "Clipboard not available" toast |

## Testing Strategy

- **Unit**: Each view's `Update` function tested with message sequences
- **Integration**: Full flow test (Loading → List → Preview → Result) with mocked backend
- **Manual**: `go run ./cmd/work-bridge` — verify in iTerm2, Terminal.app, Alacritty, Kitty
