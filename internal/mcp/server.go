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

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcpv1 "github.com/mcp-conductor/mcp-conductor/api/v1"
)

// Server implements the MCP server protocol over HTTP
type Server struct {
	Client          client.Client
	Logger          logr.Logger
	Port            int
	protocolVersion string // Store negotiated protocol version
}

// NewServer creates a new MCP server
func NewServer(client client.Client, logger logr.Logger) *Server {
	return &Server{
		Client: client,
		Logger: logger.WithName("mcp-server"),
		Port:   8080,
	}
}

// StartHTTP begins the Streamable HTTP MCP server
func (s *Server) StartHTTP(ctx context.Context) error {
	s.Logger.Info("Starting Streamable HTTP MCP server", "port", s.Port)

	mux := http.NewServeMux()

	// Single MCP endpoint (Streamable HTTP specification)
	mux.HandleFunc("/", s.handleStreamableHTTP)

	// Health endpoints
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/readyz", s.handleReady)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.Port), // Bind to all interfaces for Kubernetes
		Handler: s.withSecurityHeaders(mux),
	}

	// Start server in goroutine
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.Logger.Error(err, "HTTP server failed")
		}
	}()

	s.Logger.Info("Streamable HTTP MCP server started", "port", s.Port, "endpoint", "http://localhost:"+fmt.Sprintf("%d", s.Port))

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return server.Shutdown(shutdownCtx)
}

// withSecurityHeaders adds security headers and Origin validation
func (s *Server) withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip origin validation for health endpoints
		if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
			next.ServeHTTP(w, r)
			return
		}

		// Validate Origin header to prevent DNS rebinding attacks for MCP endpoints
		origin := r.Header.Get("Origin")
		if origin != "" {
			// Allow localhost origins only for MCP endpoints
			if origin != "http://localhost:"+fmt.Sprintf("%d", s.Port) &&
				origin != "http://127.0.0.1:"+fmt.Sprintf("%d", s.Port) {
				s.Logger.Info("Rejected request with invalid origin", "origin", origin)
				http.Error(w, "Invalid origin", http.StatusForbidden)
				return
			}
		}

		// Add security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		next.ServeHTTP(w, r)
	})
}

// handleStreamableHTTP implements the Streamable HTTP transport specification
func (s *Server) handleStreamableHTTP(w http.ResponseWriter, r *http.Request) {
	// Skip health endpoints
	if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.handleStreamablePost(w, r)
	case http.MethodGet:
		s.handleStreamableGet(w, r)
	case http.MethodDelete:
		s.handleSessionDelete(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleStreamablePost handles POST requests (sending messages to server)
func (s *Server) handleStreamablePost(w http.ResponseWriter, r *http.Request) {
	// Validate Accept header
	accept := r.Header.Get("Accept")
	if !strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/event-stream") {
		http.Error(w, "Accept header must include application/json or text/event-stream", http.StatusBadRequest)
		return
	}

	// Read request body
	body, err := json.RawMessage{}, error(nil)
	if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.Logger.Error(err, "Failed to decode JSON body")
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Check if it's a batch or single request
	if len(body) > 0 && body[0] == '[' {
		s.handleStreamableBatch(w, r, body)
	} else {
		s.handleStreamableSingle(w, r, body)
	}
}

// handleStreamableSingle handles single JSON-RPC requests
func (s *Server) handleStreamableSingle(w http.ResponseWriter, r *http.Request, body json.RawMessage) {
	var req MCPRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.Logger.Error(err, "Failed to decode MCP request")
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// If it's a notification (no ID), return 202 Accepted
	if req.ID == nil {
		s.handleRequest(r.Context(), req)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// For requests, we can return either JSON or start an SSE stream
	// For simplicity, we'll return JSON (SSE streaming can be added later)
	response := s.handleRequest(r.Context(), req)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.Logger.Error(err, "Failed to encode response")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleStreamableBatch handles JSON-RPC batch requests
func (s *Server) handleStreamableBatch(w http.ResponseWriter, r *http.Request, body json.RawMessage) {
	var requests []MCPRequest
	if err := json.Unmarshal(body, &requests); err != nil {
		s.Logger.Error(err, "Failed to decode batch request")
		http.Error(w, "Invalid batch JSON", http.StatusBadRequest)
		return
	}

	// Check if all are notifications or responses
	hasRequests := false
	for _, req := range requests {
		if req.ID != nil {
			hasRequests = true
			break
		}
	}

	// If no requests (all notifications/responses), return 202 Accepted
	if !hasRequests {
		for _, req := range requests {
			s.handleRequest(r.Context(), req)
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Process batch and return responses
	responses := make([]MCPResponse, 0, len(requests))
	for _, req := range requests {
		if req.ID == nil {
			s.handleRequest(r.Context(), req) // Process notification
			continue
		}
		response := s.handleRequest(r.Context(), req)
		responses = append(responses, response)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(responses); err != nil {
		s.Logger.Error(err, "Failed to encode batch response")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// handleStreamableGet handles GET requests (listening for server messages)
func (s *Server) handleStreamableGet(w http.ResponseWriter, r *http.Request) {
	// Validate Accept header
	accept := r.Header.Get("Accept")
	if !strings.Contains(accept, "text/event-stream") {
		http.Error(w, "Method not allowed - GET requires text/event-stream Accept header", http.StatusMethodNotAllowed)
		return
	}

	// For now, we don't implement server-initiated messages
	// This would be where SSE streaming would be implemented
	http.Error(w, "Server-initiated messages not implemented", http.StatusNotImplemented)
}

// handleSessionDelete handles DELETE requests (session termination)
func (s *Server) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		http.Error(w, "Missing Mcp-Session-Id header", http.StatusBadRequest)
		return
	}

	// For now, we don't implement session management
	// This would be where session cleanup would happen
	s.Logger.Info("Session termination requested", "sessionId", sessionID)
	w.WriteHeader(http.StatusOK)
}

// StartStdio begins the stdio MCP server (for Claude Desktop)
func (s *Server) StartStdio(ctx context.Context) error {
	s.Logger.Info("Starting stdio MCP server")

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		s.Logger.V(1).Info("Received stdio request", "line", line)

		// Handle both single requests and batches
		s.handleStdioMessage(ctx, line)
	}

	return scanner.Err()
}

// handleStdioMessage handles both single and batch messages for stdio
func (s *Server) handleStdioMessage(ctx context.Context, line string) {
	// Check if it's a batch (starts with '[') or single request
	if len(line) > 0 && line[0] == '[' {
		// Handle batch
		var requests []MCPRequest
		if err := json.Unmarshal([]byte(line), &requests); err != nil {
			s.Logger.Error(err, "Failed to parse stdio batch request")
			return
		}

		responses := make([]MCPResponse, 0, len(requests))
		for _, req := range requests {
			// Skip notifications (no ID)
			if req.ID == nil {
				s.handleRequest(ctx, req)
				continue
			}
			response := s.handleRequest(ctx, req)
			responses = append(responses, response)
		}

		// Output batch response
		if len(responses) > 0 {
			responseBytes, err := json.Marshal(responses)
			if err != nil {
				s.Logger.Error(err, "Failed to marshal stdio batch response")
				return
			}
			fmt.Print(string(responseBytes))
		}
	} else {
		// Handle single request
		var req MCPRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.Logger.Error(err, "Failed to parse stdio request")
			return
		}

		response := s.handleRequest(ctx, req)
		responseBytes, err := json.Marshal(response)
		if err != nil {
			s.Logger.Error(err, "Failed to marshal stdio response")
			return
		}

		fmt.Print(string(responseBytes))
		s.Logger.V(1).Info("Sent stdio response", "response", string(responseBytes))
	}
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

// handleReady handles readiness check requests
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "Ready")
}

// MCPRequest represents an incoming MCP request
type MCPRequest struct {
	JSONRPC string                 `json:"jsonrpc,omitempty"`
	Method  string                 `json:"method"`
	Params  map[string]interface{} `json:"params,omitempty"`
	ID      interface{}            `json:"id,omitempty"` // Optional for notifications
}

// MCPResponse represents an MCP response
type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// MCPError represents an MCP error
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// handleRequest processes an MCP request
func (s *Server) handleRequest(ctx context.Context, req MCPRequest) MCPResponse {
	// Determine if we should include JSON-RPC version
	jsonrpcVersion := ""
	if req.JSONRPC != "" {
		jsonrpcVersion = "2.0"
	}

	switch req.Method {
	case "tools/list":
		response := s.handleMCPToolsList(req)
		response.JSONRPC = jsonrpcVersion
		return response
	case "tools/call":
		response := s.handleToolsCall(ctx, req)
		response.JSONRPC = jsonrpcVersion
		return response
	case "initialize":
		response := s.handleInitialize(req)
		response.JSONRPC = jsonrpcVersion
		return response
	case "resources/list":
		response := s.handleResourcesList(req)
		response.JSONRPC = jsonrpcVersion
		return response
	case "prompts/list":
		response := s.handlePromptsList(req)
		response.JSONRPC = jsonrpcVersion
		return response
	default:
		return MCPResponse{
			JSONRPC: jsonrpcVersion,
			Error: &MCPError{
				Code:    -32601,
				Message: fmt.Sprintf("Method not found: %s", req.Method),
			},
			ID: req.ID,
		}
	}
}

// handleInitialize handles the initialize request
func (s *Server) handleInitialize(req MCPRequest) MCPResponse {
	// Extract client's requested protocol version
	clientProtocolVersion := "2024-11-05" // Default to older version for compatibility

	// The protocolVersion is directly in the params object
	if req.Params != nil {
		if version, ok := req.Params["protocolVersion"].(string); ok {
			clientProtocolVersion = version
		}
	}

	// Support both 2024-11-05 and 2025-03-26
	supportedVersions := []string{"2024-11-05", "2025-03-26"}
	negotiatedVersion := "2024-11-05" // Default to older for compatibility

	for _, version := range supportedVersions {
		if version == clientProtocolVersion {
			negotiatedVersion = version
			break
		}
	}

	s.Logger.Info("Protocol version negotiated", "client", clientProtocolVersion, "negotiated", negotiatedVersion)

	// Store the negotiated protocol version
	s.protocolVersion = negotiatedVersion

	return MCPResponse{
		Result: map[string]interface{}{
			"protocolVersion": negotiatedVersion,
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{
					"listChanged": false,
				},
				"resources": map[string]interface{}{
					"subscribe":   false,
					"listChanged": false,
				},
				"prompts": map[string]interface{}{
					"listChanged": false,
				},
				"completions": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "mcp-conductor",
				"version": "0.1.0",
			},
		},
		ID: req.ID,
	}
}

// handleMCPToolsList returns the list of available tools for MCP protocol
func (s *Server) handleMCPToolsList(req MCPRequest) MCPResponse {
	// Check if we should include annotations (2025-03-26 and later)
	includeAnnotations := s.protocolVersion == "2025-03-26"

	tools := []map[string]interface{}{
		s.createToolDefinition("list_kubernetes_pods", "List Kubernetes pods in a namespace", map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"namespace": map[string]interface{}{
					"type":        "string",
					"description": "Kubernetes namespace (empty for all namespaces)",
					"default":     "",
				},
				"labelSelector": map[string]interface{}{
					"type":        "string",
					"description": "Label selector to filter pods",
					"default":     "",
				},
			},
		}, includeAnnotations),
		s.createToolDefinition("list_kubernetes_nodes", "List Kubernetes cluster nodes", map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}, includeAnnotations),
		s.createToolDefinition("get_cluster_summary", "Get a high-level summary of the Kubernetes cluster", map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}, includeAnnotations),
		s.createToolDefinition("list_kubernetes_deployments", "List Kubernetes deployments in a namespace", map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"namespace": map[string]interface{}{
					"type":        "string",
					"description": "Kubernetes namespace (empty for all namespaces)",
					"default":     "",
				},
			},
		}, includeAnnotations),
		s.createToolDefinition("list_kubernetes_services", "List Kubernetes services in a namespace", map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"namespace": map[string]interface{}{
					"type":        "string",
					"description": "Kubernetes namespace (empty for all namespaces)",
					"default":     "",
				},
			},
		}, includeAnnotations),
		s.createToolDefinition("list_kubernetes_daemonsets", "List Kubernetes daemonsets in a namespace", map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"namespace": map[string]interface{}{
					"type":        "string",
					"description": "Kubernetes namespace (empty for all namespaces)",
					"default":     "",
				},
			},
		}, includeAnnotations),
		s.createToolDefinition("get_kubernetes_pod", "Get detailed information about a specific Kubernetes pod", map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Pod name",
				},
				"namespace": map[string]interface{}{
					"type":        "string",
					"description": "Kubernetes namespace",
					"default":     "default",
				},
			},
			"required": []string{"name"},
		}, includeAnnotations),
		s.createToolDefinition("get_kubernetes_node_status", "Get detailed status information about a specific Kubernetes node", map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Node name",
				},
			},
			"required": []string{"name"},
		}, includeAnnotations),
		s.createToolDefinition("list_kubernetes_namespaces", "List all Kubernetes namespaces in the cluster", map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}, includeAnnotations),
	}

	return MCPResponse{
		Result: map[string]interface{}{
			"tools": tools,
		},
		ID: req.ID,
	}
}

// createToolDefinition creates a tool definition with optional annotations
func (s *Server) createToolDefinition(name, description string, inputSchema map[string]interface{}, includeAnnotations bool) map[string]interface{} {
	tool := map[string]interface{}{
		"name":        name,
		"description": description,
		"inputSchema": inputSchema,
	}

	if includeAnnotations {
		tool["annotations"] = map[string]interface{}{
			"audience": []string{"user", "assistant"},
			"level":    "info",
		}
	}

	return tool
}

// handleResourcesList returns an empty list of resources
func (s *Server) handleResourcesList(req MCPRequest) MCPResponse {
	return MCPResponse{
		Result: map[string]interface{}{
			"resources": []interface{}{},
		},
		ID: req.ID,
	}
}

// handlePromptsList returns an empty list of prompts
func (s *Server) handlePromptsList(req MCPRequest) MCPResponse {
	return MCPResponse{
		Result: map[string]interface{}{
			"prompts": []interface{}{},
		},
		ID: req.ID,
	}
}

// handleToolsCall executes a tool by creating a Kubernetes task
func (s *Server) handleToolsCall(ctx context.Context, req MCPRequest) MCPResponse {
	params, ok := req.Params["arguments"].(map[string]interface{})
	if !ok {
		params = make(map[string]interface{})
	}

	toolName, ok := req.Params["name"].(string)
	if !ok {
		return MCPResponse{
			Error: &MCPError{
				Code:    -32602,
				Message: "Missing tool name",
			},
			ID: req.ID,
		}
	}

	s.Logger.Info("Executing tool", "tool", toolName, "params", params)

	// Map tool names to capabilities and create task
	capability, domain := s.mapToolToCapability(toolName)
	if capability == "" {
		return MCPResponse{
			Error: &MCPError{
				Code:    -32602,
				Message: fmt.Sprintf("Unknown tool: %s", toolName),
			},
			ID: req.ID,
		}
	}

	// Create and execute task
	result, err := s.executeTask(ctx, toolName, capability, domain, params)
	if err != nil {
		return MCPResponse{
			Error: &MCPError{
				Code:    -32603,
				Message: fmt.Sprintf("Task execution failed: %v", err),
			},
			ID: req.ID,
		}
	}

	return MCPResponse{
		Result: map[string]interface{}{
			"content": []map[string]interface{}{
				{
					"type": "text",
					"text": result,
				},
			},
			"isError": false,
		},
		ID: req.ID,
	}
}

// mapToolToCapability maps MCP tool names to agent capabilities
func (s *Server) mapToolToCapability(toolName string) (capability, domain string) {
	switch toolName {
	case "list_kubernetes_pods":
		return "k8s-list-pods", "infrastructure"
	case "list_kubernetes_nodes":
		return "k8s-list-nodes", "infrastructure"
	case "get_cluster_summary":
		return "k8s-cluster-summary", "infrastructure"
	case "list_kubernetes_deployments":
		return "k8s-list-deployments", "infrastructure"
	case "list_kubernetes_services":
		return "k8s-list-services", "infrastructure"
	case "list_kubernetes_daemonsets":
		return "k8s-list-daemonsets", "infrastructure"
	case "get_kubernetes_pod":
		return "k8s-get-pod", "infrastructure"
	case "get_kubernetes_node_status":
		return "k8s-get-node-status", "infrastructure"
	case "list_kubernetes_namespaces":
		return "k8s-list-namespaces", "infrastructure"
	default:
		return "", ""
	}
}

// executeTask creates a Kubernetes task and waits for completion
func (s *Server) executeTask(ctx context.Context, toolName, capability, domain string, params map[string]interface{}) (string, error) {
	// Create task payload
	payloadBytes, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("failed to marshal params: %w", err)
	}

	// Generate unique task name
	taskName := fmt.Sprintf("mcp-%s-%d", strings.ReplaceAll(toolName, "_", "-"), time.Now().Unix())

	// Create task
	task := &mcpv1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      taskName,
			Namespace: "default",
			Labels: map[string]string{
				"app":       "mcp-conductor",
				"component": "mcp-task",
				"domain":    domain,
				"tool":      toolName,
			},
		},
		Spec: mcpv1.TaskSpec{
			Domain:               domain,
			RequiredCapabilities: []string{capability},
			Priority:             8,
			Payload: &runtime.RawExtension{
				Raw: payloadBytes,
			},
			Timeout: metav1.Duration{Duration: 60 * time.Second},
			RetryPolicy: &mcpv1.TaskRetryPolicy{
				MaxRetries:      2,
				BackoffStrategy: mcpv1.BackoffStrategyExponential,
				InitialDelay:    metav1.Duration{Duration: 5 * time.Second},
				MaxDelay:        metav1.Duration{Duration: 30 * time.Second},
			},
			RequiredTags: []string{domain},
		},
	}

	s.Logger.Info("Creating task", "task", taskName, "capability", capability, "domain", domain)

	if err := s.Client.Create(ctx, task); err != nil {
		return "", fmt.Errorf("failed to create task: %w", err)
	}

	// Wait for task completion
	return s.waitForTaskCompletion(ctx, taskName)
}

// waitForTaskCompletion waits for a task to complete and returns the result
func (s *Server) waitForTaskCompletion(ctx context.Context, taskName string) (string, error) {
	timeout := time.After(90 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return "", fmt.Errorf("task execution timed out")
		case <-ticker.C:
			task := &mcpv1.Task{}
			key := types.NamespacedName{Name: taskName, Namespace: "default"}

			if err := s.Client.Get(ctx, key, task); err != nil {
				s.Logger.Error(err, "Failed to get task", "task", taskName)
				continue
			}

			switch task.Status.Phase {
			case mcpv1.TaskPhaseCompleted:
				return s.formatTaskResult(task), nil
			case mcpv1.TaskPhaseFailed:
				return "", fmt.Errorf("task failed: %s", task.Status.Error)
			case mcpv1.TaskPhaseCancelled:
				return "", fmt.Errorf("task was cancelled")
			default:
				s.Logger.V(1).Info("Task still running", "task", taskName, "phase", task.Status.Phase)
			}
		}
	}
}

// formatTaskResult formats the task result for MCP response
func (s *Server) formatTaskResult(task *mcpv1.Task) string {
	if task.Status.Result == nil {
		return "Task completed successfully (no result data)"
	}

	var result map[string]interface{}
	if err := json.Unmarshal(task.Status.Result.Raw, &result); err != nil {
		return fmt.Sprintf("Task completed but result parsing failed: %v", err)
	}

	// Format the result nicely
	resultBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Sprintf("Task completed: %v", result)
	}

	return string(resultBytes)
}
