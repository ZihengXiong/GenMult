package flow

import (
	"testing"

	"github.com/ZihengXiong/GenMult/internal/db/postgres/sqlc"
	"github.com/ZihengXiong/GenMult/internal/models"
)

func TestSupportsToolCallFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		model    models.GetResponse
		provider sqlc.Provider
		want     bool
	}{
		{
			name: "explicit compatibility wins",
			model: models.GetResponse{
				Model: models.Model{
					Config: models.ModelConfig{Compatibilities: []string{models.CompatToolCall}},
				},
			},
			provider: sqlc.Provider{ClientType: string(models.ClientTypeOpenAICompletions)},
			want:     true,
		},
		{
			name: "empty compatibilities fall back to llm provider defaults",
			model: models.GetResponse{
				Model: models.Model{
					Config: models.ModelConfig{},
				},
			},
			provider: sqlc.Provider{ClientType: string(models.ClientTypeOpenAICompletions)},
			want:     true,
		},
		{
			name: "explicit no-tool compatibility stays disabled",
			model: models.GetResponse{
				Model: models.Model{
					Config: models.ModelConfig{Compatibilities: []string{models.CompatReasoning}},
				},
			},
			provider: sqlc.Provider{ClientType: string(models.ClientTypeOpenAICompletions)},
			want:     false,
		},
		{
			name: "speech provider does not fall back to tool calling",
			model: models.GetResponse{
				Model: models.Model{
					Config: models.ModelConfig{},
				},
			},
			provider: sqlc.Provider{ClientType: string(models.ClientTypeEdgeSpeech)},
			want:     false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := supportsToolCallFallback(tc.model, tc.provider); got != tc.want {
				t.Fatalf("supportsToolCallFallback() = %v, want %v", got, tc.want)
			}
		})
	}
}
