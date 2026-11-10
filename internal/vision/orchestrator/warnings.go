package orchestrator

import (
	"fmt"
	"strings"

	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/catalog"
	"gitlab.com/paragon-is/solution-delivery-operations/bbs-experiences/seagull-core-engine/internal/vision/domain"
)

const (
	warningUnsupportedCapability = "UNSUPPORTED_CAPABILITY"
	warningNoApplicableZone      = "NO_APPLICABLE_ZONE"
	warningCapabilityUnavailable = "CAPABILITY_UNAVAILABLE"
)

// capabilityWarnings explains every planned capability that produced no score:
// its bound model could not be loaded, no model is uploaded for it, or none of
// its zones yielded a score. An unloadable capability is also listed as
// unsupported by the capability worker; it gets only the unavailable warning.
func capabilityWarnings(plan *catalog.Plan, unsupported []string, unavailable map[string]string, scored map[string]bool) []domain.WarningItem {
	unsupportedSet := make(map[string]bool, len(unsupported))
	for _, capability := range unsupported {
		unsupportedSet[capability] = true
	}

	var warnings []domain.WarningItem
	for _, capability := range plan.Capabilities {
		if scored[capability] {
			continue
		}
		if reason, ok := unavailable[capability]; ok {
			warnings = append(warnings, domain.WarningItem{
				Code:    warningCapabilityUnavailable,
				Message: fmt.Sprintf("Model for capability %q could not be loaded: %s", capability, reason),
			})
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

// noCapabilityWorkerWarnings explains every planned capability when no worker
// is configured to score capabilities (VISION_CAPABILITY_URL unset).
func noCapabilityWorkerWarnings(plan *catalog.Plan) []domain.WarningItem {
	warnings := make([]domain.WarningItem, 0, len(plan.Capabilities))
	for _, capability := range plan.Capabilities {
		warnings = append(warnings, domain.WarningItem{
			Code: warningUnsupportedCapability,
			Message: fmt.Sprintf("No worker is configured to score capability %q (VISION_CAPABILITY_URL), so these skin conditions have no score: %s",
				capability, strings.Join(conditionsUsing(plan, capability), ", ")),
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
