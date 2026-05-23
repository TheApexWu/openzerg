package evolve

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/TheApexWu/openzerg/backend/internal/attacks"
	"github.com/TheApexWu/openzerg/backend/internal/openrouter"
)

// LLMMutationBudget caps OpenRouter calls across the whole run, per PRD's
// "Hard cap of 32 OpenRouter calls per run". It is safe for sequential use
// from a single goroutine; the generation loop is sequential so we do not
// need a mutex.
type LLMMutationBudget struct {
	Remaining int
}

// MutateLLM asks Gemma 4 (or the configured model) to suggest mutated
// variants of the surviving genomes. Output is validated as a JSON array of
// genome objects; on any parse failure or empty output the caller falls
// back to pure-Go Mutate. Budget is decremented even on failure to bound
// total cost.
func MutateLLM(
	ctx context.Context,
	client *openrouter.Client,
	model string,
	survivors []attacks.Genome,
	requestedCount int,
	targetURL string,
	budget *LLMMutationBudget,
) ([]attacks.Genome, error) {
	if budget == nil || budget.Remaining <= 0 {
		return nil, fmt.Errorf("evolve.MutateLLM: budget exhausted")
	}
	if client == nil || client.APIKey == "" {
		return nil, fmt.Errorf("evolve.MutateLLM: no openrouter client")
	}
	if len(survivors) == 0 || requestedCount <= 0 {
		return nil, fmt.Errorf("evolve.MutateLLM: no survivors or zero count")
	}
	budget.Remaining--

	systemPrompt := "You mutate JSON attack genomes for an evolutionary web-app red-team swarm. " +
		"Reply with ONLY a JSON array of new genome objects matching the input schema. No prose."
	userPrompt := buildLLMMutationUserPrompt(survivors, requestedCount, targetURL)

	response, err := client.CreateChatCompletion(ctx, openrouter.ChatRequest{
		Model: model,
		Messages: []openrouter.ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.7,
		MaxTokens:   1024,
	})
	if err != nil {
		return nil, fmt.Errorf("evolve.MutateLLM: %w", err)
	}
	content := strings.TrimSpace(response.FirstMessageContent())
	if content == "" {
		return nil, fmt.Errorf("evolve.MutateLLM: empty completion")
	}
	mutated, err := parseGenomeJSONArray(content)
	if err != nil {
		return nil, fmt.Errorf("evolve.MutateLLM: parse: %w", err)
	}
	if len(mutated) > requestedCount {
		mutated = mutated[:requestedCount]
	}
	return mutated, nil
}

func buildLLMMutationUserPrompt(survivors []attacks.Genome, requestedCount int, targetURL string) string {
	encoded, err := json.MarshalIndent(survivors, "", "  ")
	if err != nil {
		encoded = []byte("[]")
	}
	return fmt.Sprintf(
		"Target: %s\n"+
			"These attacks partially worked or revealed information against the target:\n%s\n\n"+
			"Generate %d mutated variants. Each must be a JSON object with fields: "+
			"vector (string), category (one of injection,auth,access_control,xss,data_exposure,validation), "+
			"technique (string), target_path (string), params (object), hint (string). "+
			"Return ONLY a JSON array. No markdown, no commentary.",
		targetURL, string(encoded), requestedCount,
	)
}

// parseGenomeJSONArray accepts the assistant content and tries to find a
// JSON array of genomes in it. Gemma occasionally wraps output in a
// ```json``` fence even when told not to; we strip fences before parsing.
func parseGenomeJSONArray(content string) ([]attacks.Genome, error) {
	stripped := stripCodeFence(content)
	stripped = strings.TrimSpace(stripped)
	if !strings.HasPrefix(stripped, "[") {
		// Try to find the first '[' if the model added a leading sentence
		// despite instructions.
		if i := strings.Index(stripped, "["); i >= 0 {
			stripped = stripped[i:]
		}
	}
	if !strings.HasSuffix(stripped, "]") {
		if i := strings.LastIndex(stripped, "]"); i >= 0 {
			stripped = stripped[:i+1]
		}
	}
	var genomes []attacks.Genome
	if err := json.Unmarshal([]byte(stripped), &genomes); err != nil {
		return nil, err
	}
	return genomes, nil
}

func stripCodeFence(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		if newlineIndex := strings.Index(content, "\n"); newlineIndex > 0 {
			content = content[newlineIndex+1:]
		}
		if closingIndex := strings.LastIndex(content, "```"); closingIndex >= 0 {
			content = content[:closingIndex]
		}
	}
	return content
}
