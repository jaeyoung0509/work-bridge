package switcher

import (
	"fmt"

	"github.com/jaeyoung0509/work-bridge/internal/domain"
)

func (a *projectAdapter) previewNative(payload domain.SwitchPayload, projectRoot string, destinationOverride string) (domain.SwitchPlan, error) {
	switch a.target {
	case domain.ToolCodex:
		return a.previewNativeCodex(payload, projectRoot, destinationOverride)
	case domain.ToolGemini:
		return a.previewNativeGemini(payload, projectRoot, destinationOverride)
	case domain.ToolClaude:
		return a.previewNativeClaude(payload, projectRoot, destinationOverride)
	case domain.ToolOpenCode:
		return a.previewNativeOpenCode(payload, projectRoot, destinationOverride)
	default:
		return domain.SwitchPlan{}, fmt.Errorf("unsupported native target %q", a.target)
	}
}

func (a *projectAdapter) applyNative(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	switch a.target {
	case domain.ToolCodex:
		return a.applyNativeCodex(payload, plan)
	case domain.ToolGemini:
		return a.applyNativeGemini(payload, plan)
	case domain.ToolClaude:
		return a.applyNativeClaude(payload, plan)
	case domain.ToolOpenCode:
		return a.applyNativeOpenCode(payload, plan)
	default:
		return domain.ApplyReport{}, fmt.Errorf("unsupported native target %q", a.target)
	}
}

func (a *projectAdapter) exportNative(payload domain.SwitchPayload, plan domain.SwitchPlan) (domain.ApplyReport, error) {
	switch a.target {
	case domain.ToolCodex:
		return a.exportNativeCodex(payload, plan)
	case domain.ToolGemini:
		return a.exportNativeGemini(payload, plan)
	case domain.ToolClaude:
		return a.exportNativeClaude(payload, plan)
	case domain.ToolOpenCode:
		return a.exportNativeOpenCode(payload, plan)
	default:
		return domain.ApplyReport{}, fmt.Errorf("unsupported native target %q", a.target)
	}
}

