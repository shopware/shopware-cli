package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"

	"github.com/shopware/shopware-cli/logging"
)

type GeminiClient struct {
	client *genai.Client
}

func newGeminiClient() (*GeminiClient, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")

	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY is not set")
	}

	client, err := genai.NewClient(context.Background(), option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}

	return &GeminiClient{client: client}, nil
}

func (c *GeminiClient) Generate(ctx context.Context, prompt string, options *LLMOptions) (string, error) {
	resp, err := c.client.GenerativeModel(options.Model).GenerateContent(ctx, genai.Text(options.SystemPrompt+"\n\n"+prompt))
	if err != nil {
		if strings.Contains(err.Error(), "Resource has been exhausted") {
			logging.FromContext(ctx).Warn("Resource exhausted, waiting 15 seconds before retrying")
			time.Sleep(15 * time.Second)

			return c.Generate(ctx, prompt, options)
		}

		return "", err
	}

	return string(resp.Candidates[0].Content.Parts[0].(genai.Text)), nil
}
