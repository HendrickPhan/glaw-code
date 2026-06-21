package entity

import (
	"fmt"
	"strings"
	"sync"

	"github.com/hieu-glaw/glaw-code/internal/shared/api"
)

// ModelPricing holds per-token cost information.
type ModelPricing struct {
	InputCostPerMillion         float64
	OutputCostPerMillion        float64
	CacheCreationCostPerMillion float64
	CacheReadCostPerMillion     float64
}

// UsageTracker tracks token usage across turns.
type UsageTracker struct {
	LatestTurn api.Usage
	Cumulative api.Usage
	Turns      int
	mu         sync.Mutex
}

// NewUsageTracker creates a new usage tracker.
func NewUsageTracker() *UsageTracker {
	return &UsageTracker{}
}

// Record adds a usage entry.
func (t *UsageTracker) Record(usage api.Usage) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.LatestTurn = usage
	t.Cumulative.InputTokens += usage.InputTokens
	t.Cumulative.OutputTokens += usage.OutputTokens
	t.Cumulative.CacheCreationInputTokens += usage.CacheCreationInputTokens
	t.Cumulative.CacheReadInputTokens += usage.CacheReadInputTokens
	t.Turns++
}

// EstimateCost calculates the estimated cost in USD.
func (t *UsageTracker) EstimateCost(model string) (float64, float64, float64) {
	pricing := PricingForModel(model)
	input := float64(t.Cumulative.InputTokens) * pricing.InputCostPerMillion / 1_000_000
	output := float64(t.Cumulative.OutputTokens) * pricing.OutputCostPerMillion / 1_000_000
	cacheCreate := float64(t.Cumulative.CacheCreationInputTokens) * pricing.CacheCreationCostPerMillion / 1_000_000
	cacheRead := float64(t.Cumulative.CacheReadInputTokens) * pricing.CacheReadCostPerMillion / 1_000_000
	total := input + output + cacheCreate + cacheRead
	return input, output, total
}

// PricingForModel returns pricing for the given model.
func PricingForModel(model string) ModelPricing {
	lower := strings.ToLower(model)

	switch {
	case contains(lower, "haiku"):
		return ModelPricing{1.0, 5.0, 1.25, 0.1}
	case contains(lower, "opus"):
		return ModelPricing{15.0, 75.0, 18.75, 1.5}
	case contains(lower, "sonnet"):
		return ModelPricing{3.0, 15.0, 3.75, 0.3}
	}

	switch {
	case contains(lower, "gpt-4.1"):
		return ModelPricing{2.0, 8.0, 0, 0}
	case contains(lower, "gpt-4o"):
		return ModelPricing{2.5, 10.0, 0, 0}
	case contains(lower, "o3") || contains(lower, "o4"):
		return ModelPricing{10.0, 40.0, 0, 0}
	}

	switch {
	case contains(lower, "gemini-2.5-pro"):
		return ModelPricing{1.25, 10.0, 0, 0}
	case contains(lower, "gemini"):
		return ModelPricing{0.15, 0.60, 0, 0}
	}

	if contains(lower, "ollama") {
		return ModelPricing{0, 0, 0, 0}
	}

	if contains(lower, "openrouter:") && contains(lower, ":free") {
		return ModelPricing{0, 0, 0, 0}
	}

	return ModelPricing{3.0, 15.0, 3.75, 0.3}
}

// FormatUSD formats a float as USD string.
func FormatUSD(amount float64) string {
	return fmt.Sprintf("$%.4f", amount)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsLower(s, substr))
}

func containsLower(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
