package orchestrator

import (
	"fmt"
	"strings"

	"github.com/experience-gateway/apps/core-engine/internal/vision/catalog"
	"github.com/experience-gateway/apps/core-engine/internal/vision/domain"
)

const (
	warningUnsupportedCapability = "UNSUPPORTED_CAPABILITY"
	warningNoApplicableZone      = "NO_APPLICABLE_ZONE"
)

// capabilityWarnings explains every planned capability that produced no score:
// either no model is uploaded for it, or none of its zones yielded a score.
func capabilityWarnings(plan *catalog.Plan, unsupported []string, scored map[string]bool) []domain.WarningItem {
	unsupportedSet := make(map[string]bool, len(unsupported))
	for _, capability := range unsupported {
		unsupportedSet[capability] = true
	}

	var warnings []domain.WarningItem
	for _, capability := range plan.Capabilities {
		if scored[capability] {
			continue
		}
		affected := strings.Join(conditionsUsing(plan, capability), ", ")
		if unsupportedSet[capability] {
			warnings = append(warnings, domain.WarningItem{
				Code:    warningUnsupportedCapability,
				Message: fmt.Sprintf("No model is uploaded for capability %q, so these skin conditions have no score: %s", capability, affected),
			})
			continue
		}
		warnings = append(warnings, domain.WarningItem{
			Code:    warningNoApplicableZone,
			Message: fmt.Sprintf("Capability %q produced no score in any usable zone; affected skin conditions: %s", capability, affected),
		})
	}
	return warnings
}

func conditionsUsing(plan *catalog.Plan, capability string) []string {
	var codes []string
	for _, condition := range plan.Conditions {
		for _, c := range condition.Capabilities {
			if c == capability {
				codes = append(codes, condition.Code)
				break
			}
		}
	}
	return codes
}
