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

// Client represents an OpenAI API client.
type Client struct {
	host   string
	apiKey string
	client *http.Client
}

// ChatMessage represents a message in the chat completion request.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionRequest represents the request body for chat completion.
type ChatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
}

// ChatCompletionResponse represents the response from the chat completion endpoint.
type ChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int         `json:"index"`
		Message      ChatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// newOpenAIClient creates a new OpenAI client instance.
func newOpenAIClient() *Client {
	host := os.Getenv("OPENAI_API_HOST")
	ollamaHost := os.Getenv("OLLAMA_HOST")
	if host == "" {
		if ollamaHost != "" {
			host = ollamaHost
		} else {
			host = "https://api.openai.com"
		}
	}

	apiKey := os.Getenv("OPENAI_API_KEY")

	return &Client{
		host:   host,
		apiKey: apiKey,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Generate sends a chat completion request to the OpenAI API
func (c *Client) Generate(ctx context.Context, prompt string, options *LLMOptions) (string, error) {
	messages := []ChatMessage{
		{
			Role:    "user",
			Content: prompt,
		},
	}

	if options.SystemPrompt != "" {
		// Insert system message at the beginning
		messages = append([]ChatMessage{{
			Role:    "system",
			Content: options.SystemPrompt,
		}}, messages...)
	}

	reqBody := ChatCompletionRequest{
		Model:    options.Model,
		Messages: messages,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/v1/chat/completions", c.host), bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Warn("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read response body: %w", err)
		}
		return "", fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var response ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no completion choices returned")
	}

	return response.Choices[0].Message.Content, nil
}
