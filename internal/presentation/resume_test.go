package presentation

import (
	"strings"
	"testing"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
)

func TestReadinessLabelMapsAppliedToReadyAndErrorToBlocked(t *testing.T) {
	t.Parallel()

	if got := ReadinessLabel(domain.SwitchStateApplied); got != "READY" {
		t.Fatalf("expected APPLIED to render as READY, got %q", got)
	}
	if got := ReadinessLabel(domain.SwitchStateError); got != "BLOCKED" {
		t.Fatalf("expected ERROR to render as BLOCKED, got %q", got)
	}
}

func TestRecommendedTargetUsesOpinionatedDefaults(t *testing.T) {
	t.Parallel()

	cases := map[domain.Tool]domain.Tool{
		domain.ToolCodex:    domain.ToolGemini,
		domain.ToolGemini:   domain.ToolCodex,
		domain.ToolClaude:   domain.ToolCodex,
		domain.ToolOpenCode: domain.ToolCodex,
	}
	for source, want := range cases {
		if got := RecommendedTarget(source); got != want {
			t.Fatalf("expected %s -> %s, got %s", source, want, got)
		}
	}
}

func TestDescribePlanTranslatesWarningsIntoManualChecks(t *testing.T) {
	t.Parallel()

	guide := DescribePlan(domain.SwitchPlan{
		TargetTool: domain.ToolOpenCode,
		Status:     domain.SwitchStatePartial,
		Session:    domain.SwitchComponentPlan{State: domain.SwitchStateReady, Summary: "2 managed session artifacts"},
		Skills:     domain.SwitchComponentPlan{State: domain.SwitchStateReady, Summary: "1 skill bundle"},
		MCP:        domain.SwitchComponentPlan{State: domain.SwitchStatePartial, Summary: "1 managed MCP server"},
		Warnings:   []string{`OpenCode MCP config does not support cwd for server "github"; omitting it`},
	}, "preview")

	if guide.Headline == "" || !strings.Contains(guide.Headline, "continue in OPENCODE") {
		t.Fatalf("expected preview headline, got %#v", guide)
	}
	if len(guide.Keeps) != 3 {
		t.Fatalf("expected all three preserved sections, got %#v", guide.Keeps)
	}
	if len(guide.ManualChecks) != 1 || !strings.Contains(guide.ManualChecks[0], "working directory") {
		t.Fatalf("expected translated manual check, got %#v", guide.ManualChecks)
	}
}

func TestDescribeResultAddsNextStepsForProjectResume(t *testing.T) {
	t.Parallel()

	guide := DescribeResult(
		domain.SwitchPlan{
			TargetTool:      domain.ToolClaude,
			Mode:            domain.SwitchModeProject,
			DestinationRoot: "/repo/project",
			ManagedRoot:     "/repo/project/.work-bridge/claude",
		},
		&domain.ApplyReport{
			Status:          domain.SwitchStatePartial,
			AppliedMode:     "project",
			DestinationRoot: "/repo/project",
			Session:         domain.ApplyComponentResult{State: domain.SwitchStateApplied, Summary: "2 session files applied"},
			Skills:          domain.ApplyComponentResult{State: domain.SwitchStateApplied, Summary: "1 skill files applied"},
			MCP:             domain.ApplyComponentResult{State: domain.SwitchStatePartial, Summary: "1 MCP files applied"},
			Warnings:        []string{`Global MCP server "github" already exists in the target config with different settings; keeping the existing target entry`},
		},
		"switch",
	)

	if guide.Readiness != ResumeReadinessPartial {
		t.Fatalf("expected partial readiness, got %#v", guide)
	}
	if len(guide.NextSteps) < 3 || !strings.Contains(strings.Join(guide.NextSteps, " "), "CLAUDE.md") {
		t.Fatalf("expected project next steps to mention CLAUDE.md, got %#v", guide.NextSteps)
	}
	if len(guide.ManualChecks) == 0 || !strings.Contains(guide.ManualChecks[0], "target already has MCP server") {
		t.Fatalf("expected target-side MCP warning translation, got %#v", guide.ManualChecks)
	}
}
