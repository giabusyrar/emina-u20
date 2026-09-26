package client

import (
	"context"
)

type EngineBridge struct {
	scoreEngineURL string
	matchEngineURL string
}

func NewEngineBridge(scoreURL, matchURL string) *EngineBridge {
	return &EngineBridge{
		scoreEngineURL: scoreURL,
		matchEngineURL: matchURL,
	}
}

// ForwardVisionSignals forwards extracted visual traits to score-engine.
func (b *EngineBridge) ForwardVisionSignals(ctx context.Context, brandId string, scores map[string]float64, missedZones []string) error {
	// Vision engine is a pure extractor; signals are forwarded to score-engine / gateway
	return nil
}
