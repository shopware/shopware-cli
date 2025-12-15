package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/shopware/shopware-cli/logging"
)

// OpenRouterClient represents an OpenRouter API client.
type OpenRouterClient struct {
	client *http.Client
	apiKey string
}

// OpenRouterRequest represents the request body for text generation.
type OpenRouterRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenRouterResponse represents the response from the OpenRouter API.
type OpenRouterResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// newOpenRouterClient creates a new OpenRouter client instance.
func newOpenRouterClient() (*OpenRouterClient, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY is not set")
	}

	return &OpenRouterClient{
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}, nil
}

// Generate sends a generation request to the OpenRouter API.
func (c *OpenRouterClient) Generate(ctx context.Context, prompt string, options *LLMOptions) (string, error) {
	messages := []Message{
		{
			Role:    "system",
			Content: options.SystemPrompt,
		},
		{
			Role:    "user",
			Content: prompt,
		},
	}

	reqBody := OpenRouterRequest{
		Model:    options.Model,
		Messages: messages,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://openrouter.ai/api/v1/chat/completions", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("HTTP-Referer", "https://github.com/shopwareLabs/extension-verifier")
	req.Header.Set("X-Title", "Shopware Extension Verifier")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var response OpenRouterResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(response.Choices) == 0 {
		logging.FromContext(ctx).Error("no response choices returned", "body", string(body))
		return "", fmt.Errorf("no response choices returned")
	}

	return response.Choices[0].Message.Content, nil
}
