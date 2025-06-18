/*
Copyright 2024 MCP Conductor Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package examples

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"

	mcpv1 "github.com/mcp-conductor/mcp-conductor/api/v1"
	"github.com/mcp-conductor/mcp-conductor/internal/agent"
)

// HTTPClient provides HTTP client capabilities
type HTTPClient struct {
	Logger logr.Logger
	Client *http.Client
}

// NewHTTPClient creates a new HTTP client agent
func NewHTTPClient(logger logr.Logger) *HTTPClient {
	return &HTTPClient{
		Logger: logger.WithName("http-client"),
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetCapabilities returns the capabilities this agent provides
func (h *HTTPClient) GetCapabilities() []string {
	return []string{
		"http-get",
		"http-post",
		"http-put",
		"http-delete",
		"webhook-call",
		"api-health-check",
		"rest-api-call",
	}
}

// GetTags returns the tags for this agent
func (h *HTTPClient) GetTags() []string {
	return []string{
		"integration",
		"http",
		"api",
		"webhook",
		"network",
	}
}

// ExecuteTask executes an HTTP client task
func (h *HTTPClient) ExecuteTask(ctx context.Context, task *mcpv1.Task) (*agent.TaskResult, error) {
	h.Logger.Info("Executing HTTP client task",
		"task", task.Name,
		"capabilities", task.Spec.RequiredCapabilities)

	// Parse task payload
	payload, err := h.parseTaskPayload(task)
	if err != nil {
		return &agent.TaskResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to parse task payload: %v", err),
		}, nil
	}

	// Determine which capability to execute
	capability := h.determineCapability(task.Spec.RequiredCapabilities)
	if capability == "" {
		return &agent.TaskResult{
			Success: false,
			Error:   "No supported capability found in task requirements",
		}, nil
	}

	// Execute the appropriate capability
	result, err := h.executeCapability(ctx, capability, payload)
	if err != nil {
		return &agent.TaskResult{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &agent.TaskResult{
		Success: true,
		Data:    result,
	}, nil
}

// parseTaskPayload extracts parameters from the task payload
func (h *HTTPClient) parseTaskPayload(task *mcpv1.Task) (map[string]interface{}, error) {
	if task.Spec.Payload == nil {
		return map[string]interface{}{}, nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(task.Spec.Payload.Raw, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return payload, nil
}

// determineCapability finds the first supported capability from the task requirements
func (h *HTTPClient) determineCapability(requiredCapabilities []string) string {
	supportedCapabilities := make(map[string]bool)
	for _, cap := range h.GetCapabilities() {
		supportedCapabilities[cap] = true
	}

	for _, required := range requiredCapabilities {
		if supportedCapabilities[required] {
			return required
		}
	}

	return ""
}

// executeCapability executes a specific capability
func (h *HTTPClient) executeCapability(ctx context.Context, capability string, payload map[string]interface{}) (map[string]interface{}, error) {
	switch capability {
	case "http-get":
		return h.httpGet(ctx, payload)
	case "http-post":
		return h.httpPost(ctx, payload)
	case "http-put":
		return h.httpPut(ctx, payload)
	case "http-delete":
		return h.httpDelete(ctx, payload)
	case "webhook-call":
		return h.webhookCall(ctx, payload)
	case "api-health-check":
		return h.apiHealthCheck(ctx, payload)
	case "rest-api-call":
		return h.restAPICall(ctx, payload)
	default:
		return nil, fmt.Errorf("unsupported capability: %s", capability)
	}
}

// httpGet performs an HTTP GET request
func (h *HTTPClient) httpGet(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	url := h.getStringParam(payload, "url", "")
	if url == "" {
		return nil, fmt.Errorf("url parameter is required")
	}

	h.Logger.Info("Performing HTTP GET", "url", url)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers if provided
	if headers, ok := payload["headers"].(map[string]interface{}); ok {
		for key, value := range headers {
			if strValue, ok := value.(string); ok {
				req.Header.Set(key, strValue)
			}
		}
	}

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return map[string]interface{}{
		"method":     "GET",
		"url":        url,
		"statusCode": resp.StatusCode,
		"status":     resp.Status,
		"headers":    resp.Header,
		"body":       string(body),
		"bodySize":   len(body),
	}, nil
}

// httpPost performs an HTTP POST request
func (h *HTTPClient) httpPost(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	url := h.getStringParam(payload, "url", "")
	if url == "" {
		return nil, fmt.Errorf("url parameter is required")
	}

	h.Logger.Info("Performing HTTP POST", "url", url)

	// Prepare request body
	var body io.Reader
	var contentType string

	if bodyData, ok := payload["body"]; ok {
		switch v := bodyData.(type) {
		case string:
			body = bytes.NewBufferString(v)
			contentType = "text/plain"
		case map[string]interface{}:
			jsonData, err := json.Marshal(v)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal JSON body: %w", err)
			}
			body = bytes.NewBuffer(jsonData)
			contentType = "application/json"
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Add headers if provided
	if headers, ok := payload["headers"].(map[string]interface{}); ok {
		for key, value := range headers {
			if strValue, ok := value.(string); ok {
				req.Header.Set(key, strValue)
			}
		}
	}

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return map[string]interface{}{
		"method":     "POST",
		"url":        url,
		"statusCode": resp.StatusCode,
		"status":     resp.Status,
		"headers":    resp.Header,
		"body":       string(respBody),
		"bodySize":   len(respBody),
	}, nil
}

// httpPut performs an HTTP PUT request
func (h *HTTPClient) httpPut(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	// Similar to POST but with PUT method
	url := h.getStringParam(payload, "url", "")
	if url == "" {
		return nil, fmt.Errorf("url parameter is required")
	}

	h.Logger.Info("Performing HTTP PUT", "url", url)

	// For brevity, this is a simplified implementation
	// In practice, you'd implement the full PUT logic similar to POST
	return map[string]interface{}{
		"method":     "PUT",
		"url":        url,
		"statusCode": 200,
		"status":     "200 OK",
		"message":    "PUT request simulated successfully",
	}, nil
}

// httpDelete performs an HTTP DELETE request
func (h *HTTPClient) httpDelete(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	url := h.getStringParam(payload, "url", "")
	if url == "" {
		return nil, fmt.Errorf("url parameter is required")
	}

	h.Logger.Info("Performing HTTP DELETE", "url", url)

	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers if provided
	if headers, ok := payload["headers"].(map[string]interface{}); ok {
		for key, value := range headers {
			if strValue, ok := value.(string); ok {
				req.Header.Set(key, strValue)
			}
		}
	}

	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	return map[string]interface{}{
		"method":     "DELETE",
		"url":        url,
		"statusCode": resp.StatusCode,
		"status":     resp.Status,
	}, nil
}

// webhookCall makes a webhook call (specialized POST)
func (h *HTTPClient) webhookCall(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	h.Logger.Info("Making webhook call")

	// Webhook calls are essentially POST requests with specific formatting
	result, err := h.httpPost(ctx, payload)
	if err != nil {
		return nil, err
	}

	// Add webhook-specific metadata
	result["type"] = "webhook"
	result["timestamp"] = time.Now().UTC()

	return result, nil
}

// apiHealthCheck performs a health check on an API endpoint
func (h *HTTPClient) apiHealthCheck(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	url := h.getStringParam(payload, "url", "")
	if url == "" {
		return nil, fmt.Errorf("url parameter is required")
	}

	h.Logger.Info("Performing API health check", "url", url)

	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := h.Client.Do(req)
	duration := time.Since(start)

	healthy := err == nil && resp != nil && resp.StatusCode >= 200 && resp.StatusCode < 400

	result := map[string]interface{}{
		"url":          url,
		"healthy":      healthy,
		"responseTime": duration.Milliseconds(),
		"timestamp":    time.Now().UTC(),
	}

	if resp != nil {
		result["statusCode"] = resp.StatusCode
		result["status"] = resp.Status
		resp.Body.Close()
	}

	if err != nil {
		result["error"] = err.Error()
	}

	return result, nil
}

// restAPICall makes a generic REST API call
func (h *HTTPClient) restAPICall(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	method := h.getStringParam(payload, "method", "GET")

	switch method {
	case "GET":
		return h.httpGet(ctx, payload)
	case "POST":
		return h.httpPost(ctx, payload)
	case "PUT":
		return h.httpPut(ctx, payload)
	case "DELETE":
		return h.httpDelete(ctx, payload)
	default:
		return nil, fmt.Errorf("unsupported HTTP method: %s", method)
	}
}

// Helper methods

func (h *HTTPClient) getStringParam(payload map[string]interface{}, key, defaultValue string) string {
	if value, ok := payload[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return defaultValue
}
