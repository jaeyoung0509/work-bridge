package styles

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestButtonStylesKeepSameWidth(t *testing.T) {
	t.Parallel()

	label := " ▶ Prepare Resume "
	secondaryWidth := lipgloss.Width(ButtonSecondary.Render(label))
	activeWidth := lipgloss.Width(ButtonActive.Render(label))
	secondaryHeight := lipgloss.Height(ButtonSecondary.Render(label))
	activeHeight := lipgloss.Height(ButtonActive.Render(label))

	if secondaryWidth != activeWidth {
		t.Fatalf("expected button widths to match, got secondary=%d active=%d", secondaryWidth, activeWidth)
	}
	if secondaryHeight != activeHeight {
		t.Fatalf("expected button heights to match, got secondary=%d active=%d", secondaryHeight, activeHeight)
	}
}
