package styles

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestButtonStylesKeepSameWidth(t *testing.T) {
	t.Parallel()

	label := "  ▶ Apply Handoff  "
	secondaryWidth := lipgloss.Width(ButtonSecondary.Render(label))
	activeWidth := lipgloss.Width(ButtonActive.Render(label))

	if secondaryWidth != activeWidth {
		t.Fatalf("expected button widths to match, got secondary=%d active=%d", secondaryWidth, activeWidth)
	}
}
