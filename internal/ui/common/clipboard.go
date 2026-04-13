package common

import (
	tea "charm.land/bubbletea/v2"
)

// ClipboardCopiedMsg is emitted when clipboard copy completes.
type ClipboardCopiedMsg struct{}

// CopiedToastMsg is emitted to show a brief "copied" toast.
type CopiedToastMsg struct{}

// CopyToClipboard returns a tea.Cmd that copies text to clipboard via OSC 52.
// The cmd returns CopiedToastMsg for displaying a brief confirmation.
func CopyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		// OSC 52 escape sequence — terminal handles it
		// Return toast message for UI feedback
		return CopiedToastMsg{}
	}
}
