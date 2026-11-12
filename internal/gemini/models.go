package gemini

import (
	"fmt"
	"strings"
)

// DefaultQueryModel is the current general-purpose File Search generation model.
const DefaultQueryModel = "gemini-4-hydrogen"

// ResolveQueryModel normalizes API resource names and rejects the old generation
// families removed from ragujuary. Other explicit IDs remain usable as Google
// releases new models; unsupported IDs are reported by the API, never substituted.
func ResolveQueryModel(model string) (string, error) {
	model = strings.TrimPrefix(strings.TrimSpace(model), "models/")
	if model == "" {
		return DefaultQueryModel, nil
	}
	if strings.HasPrefix(model, "gemini-1.") || strings.HasPrefix(model, "gemini-2.") {
		return "", fmt.Errorf("legacy generation model %q is no longer supported; use %s, gemini-4-argon, or gemini-4-carbon", model, DefaultQueryModel)
	}
	return model, nil
}
