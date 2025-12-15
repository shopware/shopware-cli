package llm

import "context"

type LLMOptions struct {
	Model        string
	SystemPrompt string
}

type LLMClient interface {
	Generate(ctx context.Context, prompt string, options *LLMOptions) (string, error)
}

func NewLLMClient(provider string) (LLMClient, error) {
	switch provider {
	case "ollama":
		return newOpenAIClient(), nil
	case "openai":
		return newOpenAIClient(), nil
	case "gemini":
		return newGeminiClient()
	case "openrouter":
		return newOpenRouterClient()
	}

	panic("Invalid provider: " + provider)
}
