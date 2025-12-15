package llm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGenerate(t *testing.T) {
	// Create a test server to mock the OpenAI API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check request method and path
		assert.Equal(t, http.MethodPost, r.Method, "Request method should be POST")
		assert.Equal(t, "/v1/chat/completions", r.URL.Path, "Request path should be /v1/chat/completions")

		// Check for authorization header
		authHeader := r.Header.Get("Authorization")
		assert.Equal(t, "Bearer test-api-key", authHeader, "Authorization header should be properly set")

		// Decode request body to verify content
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		if !assert.NoError(t, err, "Should decode request body without error") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Check model
		model, ok := reqBody["model"].(string)
		assert.True(t, ok, "Model should be a string")
		assert.Equal(t, "gpt-3.5-turbo", model, "Model should be gpt-3.5-turbo")

		// Check messages
		messages, ok := reqBody["messages"].([]interface{})
		assert.True(t, ok, "Messages should be an array")

		// Different response based on request type
		var responseContent string
		switch len(messages) {
		case 1:
			// Regular prompt
			responseContent = "This is a test response to a regular prompt"
		case 2:
			// With system prompt
			responseContent = "This is a test response with system context"
		default:
			assert.Failf(t, "Unexpected messages length", "Got %d messages, expected 1 or 2", len(messages))
			responseContent = "Unexpected configuration"
		}

		// Send a successful response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Simplified mock response that matches the ChatCompletionResponse structure
		responseJSON := fmt.Sprintf(`{
			"id": "test-id",
			"object": "chat.completion",
			"created": 1677652288,
			"model": "gpt-3.5-turbo",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "%s"
				},
				"finish_reason": "stop"
			}],
			"usage": {
				"prompt_tokens": 10,
				"completion_tokens": 5,
				"total_tokens": 15
			}
		}`, responseContent)

		_, err = w.Write([]byte(responseJSON))
		assert.NoError(t, err, "Should write response without error")
	}))
	defer server.Close()

	// Set environment variables for the test
	t.Setenv("OPENAI_API_HOST", server.URL)
	t.Setenv("OPENAI_API_KEY", "test-api-key")

	// Get an OpenAI client instance
	client, err := NewLLMClient("openai")
	require.NoError(t, err, "Creating OpenAI client should not error")

	// Test cases
	tests := []struct {
		name         string
		prompt       string
		options      *LLMOptions
		expected     string
		expectError  bool
		errorMessage string
	}{
		{
			name:     "basic prompt",
			prompt:   "Hello, world!",
			options:  &LLMOptions{Model: "gpt-3.5-turbo"},
			expected: "This is a test response to a regular prompt",
		},
		{
			name:     "with system prompt",
			prompt:   "Hello, assistant!",
			options:  &LLMOptions{Model: "gpt-3.5-turbo", SystemPrompt: "You are a helpful assistant"},
			expected: "This is a test response with system context",
		},
	}

	// Run test cases
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := client.Generate(t.Context(), tt.prompt, tt.options)

			if tt.expectError {
				assert.Error(t, err, "Should return an error")
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage, "Error should contain expected message")
				}
				return
			}

			assert.NoError(t, err, "Should not return an error")
			assert.Equal(t, tt.expected, result, "Should return expected result")
		})
	}
}

func TestOpenAIGenerateErrors(t *testing.T) {
	// Test case 1: Server error
	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, err := w.Write([]byte(`{"error": {"message": "Server error", "type": "server_error"}}`))
			assert.NoError(t, err, "Should write response without error")
		}))
		defer server.Close()

		// Set environment variables for the test
		t.Setenv("OPENAI_API_HOST", server.URL)
		t.Setenv("OPENAI_API_KEY", "test-key")

		client, err := NewLLMClient("openai")
		assert.NoError(t, err, "Creating OpenAI client should not error")

		_, err = client.Generate(t.Context(), "Test prompt", &LLMOptions{
			Model: "gpt-3.5-turbo",
		})

		assert.Error(t, err, "Should return an error when server returns error")
		assert.Contains(t, err.Error(), "unexpected status code", "Error should mention unexpected status code")
	})

	// Test case 2: Empty choices
	t.Run("empty choices", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte(`{"id": "test-id", "object": "chat.completion", "choices": [], "usage": {"prompt_tokens": 10, "completion_tokens": 0, "total_tokens": 10}}`))
			assert.NoError(t, err, "Should write response without error")
		}))
		defer server.Close()

		// Set environment variables for the test
		t.Setenv("OPENAI_API_HOST", server.URL)
		t.Setenv("OPENAI_API_KEY", "test-key")

		client, err := NewLLMClient("openai")
		require.NoError(t, err, "Creating OpenAI client should not error")

		_, err = client.Generate(t.Context(), "Test prompt", &LLMOptions{
			Model: "gpt-3.5-turbo",
		})

		assert.Error(t, err, "Should return an error when response has empty choices")
		assert.Contains(t, err.Error(), "no completion choices", "Error should mention no completion choices")
	})
}
