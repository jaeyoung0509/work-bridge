# TUI V2 Redesign Spec

**Goal:** Redesign the work-bridge TUI into a component-based architecture using Bubble Tea v2, improving maintainability, readability, and user experience.

**Problem:** Currently, the TUI is a monolithic file (`internal/tui/tui.go`) containing over 3,000 lines of code. It manages state, routing, and styling all in one place, making it fragile and hard to extend.

**Proposed Architecture:**
We will move to a Router-based component architecture using `charm.land/bubbletea/v2`.

1. **`internal/ui/`**: New package for the redesigned TUI.
2. **`internal/ui/styles/theme.go`**: Centralized Lip Gloss styles to ensure a consistent visual design across all views.
3. **`internal/ui/app.go`**: The main router model (`MainModel`). It will only hold the current state and delegate `Init`, `Update`, and `View` to the active child component.
4. **`internal/ui/views/session/session.go`**: The first screen. A component responsible solely for listing and selecting source sessions. We will use `github.com/charmbracelet/bubbles/list` for this to provide out-of-the-box filtering and pagination.

This spec focuses purely on the initial scaffolding and the first "Select Session" view. Later views (Preview, Apply) will be added in subsequent phases.
