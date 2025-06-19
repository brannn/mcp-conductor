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

	// MCP 2025-06-18: Check for MCP-Protocol-Version header in subsequent requests
	// (after initial handshake, this header should be present)
	protocolVersion := r.Header.Get("MCP-Protocol-Version")
	if protocolVersion != "" {
		// Validate the protocol version
		supportedVersions := []string{"2024-11-05", "2025-03-26", "2025-06-18"}
		isSupported := false
		for _, version := range supportedVersions {
			if version == protocolVersion {
				isSupported = true
				break
			}
		}
		if !isSupported {
			s.Logger.Info("Unsupported protocol version in header", "version", protocolVersion)
			http.Error(w, fmt.Sprintf("Unsupported protocol version: %s", protocolVersion), http.StatusBadRequest)
			return
		}
		// Store the negotiated version for this request
		s.protocolVersion = protocolVersion
	}

	// Read request body
	body, err := json.RawMessage{}, error(nil)
	if err = json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.Logger.Error(err, "Failed to decode JSON body")
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// MCP 2025-06-18: JSON-RPC batching is no longer supported
	// Check if it's a batch request and reject it
	if len(body) > 0 && body[0] == '[' {
		s.Logger.Info("Rejected batch request - batching not supported in MCP 2025-06-18")
		http.Error(w, "JSON-RPC batching is not supported", http.StatusBadRequest)
		return
	}

	s.handleStreamableSingle(w, r, body)
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

		// Handle single requests (batching no longer supported in MCP 2025-06-18)
		s.handleStdioMessage(ctx, line)
	}

	return scanner.Err()
}

// handleStdioMessage handles single messages for stdio (batching no longer supported)
func (s *Server) handleStdioMessage(ctx context.Context, line string) {
	// MCP 2025-06-18: JSON-RPC batching is no longer supported
	// Check if it's a batch request and reject it
	if len(line) > 0 && line[0] == '[' {
		s.Logger.Info("Rejected stdio batch request - batching not supported in MCP 2025-06-18")
		// Send error response for batch requests
		errorResponse := MCPResponse{
			JSONRPC: "2.0",
			Error: &MCPError{
				Code:    -32600,
				Message: "JSON-RPC batching is not supported",
			},
			ID: nil,
		}
		responseBytes, _ := json.Marshal(errorResponse)
		fmt.Print(string(responseBytes))
		return
	}

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
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data,omitempty"`
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
	case "initialized":
		// MCP 2025-06-18: Lifecycle operations are now mandatory
		response := s.handleInitialized(req)
		response.JSONRPC = jsonrpcVersion
		return response
	case "elicitation/create":
		// MCP Draft: Elicitation support - servers send elicitation/create to clients
		// Note: This should not be called in our server implementation
		// as we are the server, not the client
		return MCPResponse{
			JSONRPC: jsonrpcVersion,
			Error: &MCPError{
				Code:    -32601,
				Message: "elicitation/create is sent by servers to clients, not the reverse",
			},
			ID: req.ID,
		}
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

	// Support 2024-11-05, 2025-03-26, and 2025-06-18
	supportedVersions := []string{"2024-11-05", "2025-03-26", "2025-06-18"}
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

// handleInitialized handles the initialized notification (MCP 2025-06-18 mandatory lifecycle)
func (s *Server) handleInitialized(req MCPRequest) MCPResponse {
	s.Logger.Info("Client initialization completed", "protocolVersion", s.protocolVersion)

	// This is a notification, so we return an empty response
	// The ID should be null for notifications
	return MCPResponse{
		ID: req.ID,
	}
}

// Note: Elicitation support removed - our understanding was incorrect.
// According to MCP specification, servers send elicitation/create TO clients,
// not the other way around. This requires a different implementation approach
// that would need to be integrated into tool execution flows.

// handleMCPToolsList returns the list of available tools for MCP protocol
func (s *Server) handleMCPToolsList(req MCPRequest) MCPResponse {
	// Check if we should include annotations (2025-03-26 and later)
	includeAnnotations := s.protocolVersion == "2025-03-26" || s.protocolVersion == "2025-06-18"

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

// createToolDefinition creates a tool definition with optional annotations and output schema
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

	// MCP 2025-06-18: Add output schema for structured content
	if s.protocolVersion == "2025-06-18" {
		tool["outputSchema"] = s.getOutputSchemaForTool(name)
	}

	return tool
}

// getOutputSchemaForTool returns the output schema for a specific tool (MCP 2025-06-18)
func (s *Server) getOutputSchemaForTool(toolName string) map[string]interface{} {
	switch toolName {
	case "list_kubernetes_pods":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"success": map[string]interface{}{
					"type": "boolean",
					"description": "Whether the operation was successful",
				},
				"data": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"count": map[string]interface{}{
							"type": "integer",
							"description": "Number of pods found",
						},
						"pods": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"name":      map[string]interface{}{"type": "string"},
									"namespace": map[string]interface{}{"type": "string"},
									"phase":     map[string]interface{}{"type": "string"},
									"ready":     map[string]interface{}{"type": "boolean"},
									"restarts":  map[string]interface{}{"type": "integer"},
									"age":       map[string]interface{}{"type": "string"},
									"node":      map[string]interface{}{"type": "string"},
								},
							},
						},
					},
				},
			},
		}
	case "list_kubernetes_namespaces":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"success": map[string]interface{}{
					"type": "boolean",
					"description": "Whether the operation was successful",
				},
				"data": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"count": map[string]interface{}{
							"type": "integer",
							"description": "Number of namespaces found",
						},
						"namespaces": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"name":   map[string]interface{}{"type": "string"},
									"status": map[string]interface{}{"type": "string"},
									"age":    map[string]interface{}{"type": "string"},
								},
							},
						},
					},
				},
			},
		}
	case "list_kubernetes_nodes":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"success": map[string]interface{}{
					"type": "boolean",
					"description": "Whether the operation was successful",
				},
				"data": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"count": map[string]interface{}{
							"type": "integer",
							"description": "Number of nodes found",
						},
						"nodes": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"name":   map[string]interface{}{"type": "string"},
									"status": map[string]interface{}{"type": "string"},
									"roles":  map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
									"age":    map[string]interface{}{"type": "string"},
									"version": map[string]interface{}{"type": "string"},
								},
							},
						},
					},
				},
			},
		}
	default:
		// Generic schema for other tools
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"success": map[string]interface{}{
					"type": "boolean",
					"description": "Whether the operation was successful",
				},
				"data": map[string]interface{}{
					"type": "object",
					"description": "Tool-specific result data",
				},
			},
		}
	}
}

// generateResourceLinksForTool creates resource links for tool responses (MCP 2025-06-18)
func (s *Server) generateResourceLinksForTool(toolName string, params map[string]interface{}, structuredData interface{}) []map[string]interface{} {
	var links []map[string]interface{}

	switch toolName {
	case "list_kubernetes_pods":
		// Create resource links for each pod
		if data, ok := structuredData.(map[string]interface{}); ok {
			if podData, ok := data["data"].(map[string]interface{}); ok {
				if pods, ok := podData["pods"].([]interface{}); ok {
					for _, podInterface := range pods {
						if pod, ok := podInterface.(map[string]interface{}); ok {
							if name, ok := pod["name"].(string); ok && name != "" {
								namespace := "default"
								if ns, ok := pod["namespace"].(string); ok && ns != "" {
									namespace = ns
								}
								links = append(links, map[string]interface{}{
									"type": "resource",
									"resource": map[string]interface{}{
										"uri":         fmt.Sprintf("kubernetes://pod/%s/%s", namespace, name),
										"name":        fmt.Sprintf("Pod: %s", name),
										"description": fmt.Sprintf("Kubernetes pod %s in namespace %s", name, namespace),
									},
								})
							}
						}
					}
				}
			}
		}
	case "list_kubernetes_namespaces":
		// Create resource links for each namespace
		if data, ok := structuredData.(map[string]interface{}); ok {
			if nsData, ok := data["data"].(map[string]interface{}); ok {
				if namespaces, ok := nsData["namespaces"].([]interface{}); ok {
					for _, nsInterface := range namespaces {
						if ns, ok := nsInterface.(map[string]interface{}); ok {
							if name, ok := ns["name"].(string); ok && name != "" {
								links = append(links, map[string]interface{}{
									"type": "resource",
									"resource": map[string]interface{}{
										"uri":         fmt.Sprintf("kubernetes://namespace/%s", name),
										"name":        fmt.Sprintf("Namespace: %s", name),
										"description": fmt.Sprintf("Kubernetes namespace %s", name),
									},
								})
							}
						}
					}
				}
			}
		}
	case "list_kubernetes_nodes":
		// Create resource links for each node
		if data, ok := structuredData.(map[string]interface{}); ok {
			if nodeData, ok := data["data"].(map[string]interface{}); ok {
				if nodes, ok := nodeData["nodes"].([]interface{}); ok {
					for _, nodeInterface := range nodes {
						if node, ok := nodeInterface.(map[string]interface{}); ok {
							if name, ok := node["name"].(string); ok && name != "" {
								links = append(links, map[string]interface{}{
									"type": "resource",
									"resource": map[string]interface{}{
										"uri":         fmt.Sprintf("kubernetes://node/%s", name),
										"name":        fmt.Sprintf("Node: %s", name),
										"description": fmt.Sprintf("Kubernetes node %s", name),
									},
								})
							}
						}
					}
				}
			}
		}
	}

	return links
}

// updateTaskElicitationResponse updates a task with the user's elicitation response
func (s *Server) updateTaskElicitationResponse(ctx context.Context, elicitationID, response string, cancelled bool) error {
	// Find the task that has this elicitation request
	taskList := &mcpv1.TaskList{}
	if err := s.Client.List(ctx, taskList); err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	var targetTask *mcpv1.Task
	for i := range taskList.Items {
		task := &taskList.Items[i]
		if task.Status.ElicitationRequest != nil && task.Status.ElicitationRequest.ID == elicitationID {
			targetTask = task
			break
		}
	}

	if targetTask == nil {
		return fmt.Errorf("task with elicitation ID %s not found", elicitationID)
	}

	// Update the task with the elicitation response
	now := metav1.Now()
	targetTask.Status.ElicitationResponse = &mcpv1.TaskElicitationResponse{
		ID:           elicitationID,
		Response:     response,
		ResponseTime: &now,
		Cancelled:    cancelled,
	}

	// Clear the elicitation request and resume task execution
	targetTask.Status.ElicitationRequest = nil
	targetTask.Status.Phase = mcpv1.TaskPhaseRunning

	// Update the task status
	if err := s.Client.Status().Update(ctx, targetTask); err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	s.Logger.Info("Updated task with elicitation response",
		"task", targetTask.Name,
		"elicitationID", elicitationID,
		"cancelled", cancelled)

	return nil
}

// handleTaskElicitation handles a task that is waiting for elicitation (MCP 2025-06-18)
func (s *Server) handleTaskElicitation(ctx context.Context, task *mcpv1.Task) (string, interface{}, error) {
	elicitationReq := task.Status.ElicitationRequest
	if elicitationReq == nil {
		return "", nil, fmt.Errorf("task is waiting for elicitation but no elicitation request found")
	}

	// Return an elicitation response that the MCP client can handle
	elicitationResponse := map[string]interface{}{
		"type": "elicitation",
		"elicitation": map[string]interface{}{
			"id":           elicitationReq.ID,
			"prompt":       elicitationReq.Prompt,
			"promptType":   elicitationReq.PromptType,
			"choices":      elicitationReq.Choices,
			"defaultValue": elicitationReq.DefaultValue,
			"required":     elicitationReq.Required,
			"requestTime":  elicitationReq.RequestTime,
		},
	}

	// Format as text for human readability
	textResponse := fmt.Sprintf("Task is waiting for user input:\n\nPrompt: %s\nType: %s\nElicitation ID: %s\n\nPlease respond using the elicitation/respond method.",
		elicitationReq.Prompt,
		elicitationReq.PromptType,
		elicitationReq.ID)

	return textResponse, elicitationResponse, nil
}

// createEnhancedError creates an enhanced error response with context (MCP 2025-06-18)
func (s *Server) createEnhancedError(err error, toolName string, params map[string]interface{}) *MCPError {
	// Base error
	mcpError := &MCPError{
		Code:    -32603,
		Message: fmt.Sprintf("Task execution failed: %v", err),
	}

	// MCP 2025-06-18: Add enhanced error context
	if s.protocolVersion == "2025-06-18" {
		mcpError.Data = map[string]interface{}{
			"toolName":  toolName,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"context": map[string]interface{}{
				"parameters": params,
				"errorType":  fmt.Sprintf("%T", err),
			},
		}

		// Add recovery suggestions based on error type
		if strings.Contains(err.Error(), "timeout") {
			mcpError.Data["recoverySuggestion"] = "Consider increasing the task timeout or checking system load"
		} else if strings.Contains(err.Error(), "not found") {
			mcpError.Data["recoverySuggestion"] = "Verify that the requested resource exists and is accessible"
		} else if strings.Contains(err.Error(), "permission") || strings.Contains(err.Error(), "forbidden") {
			mcpError.Data["recoverySuggestion"] = "Check that the agent has sufficient permissions for this operation"
		} else if strings.Contains(err.Error(), "connection") || strings.Contains(err.Error(), "network") {
			mcpError.Data["recoverySuggestion"] = "Check network connectivity and service availability"
		} else {
			mcpError.Data["recoverySuggestion"] = "Review the error details and tool parameters"
		}
	}

	return mcpError
}

// validateToolInput validates tool input parameters against schema (MCP 2025-06-18)
func (s *Server) validateToolInput(toolName string, params map[string]interface{}) error {
	// Basic validation for known tools
	switch toolName {
	case "list_kubernetes_pods":
		if namespace, ok := params["namespace"]; ok {
			if ns, ok := namespace.(string); !ok || ns == "" {
				return fmt.Errorf("namespace must be a non-empty string")
			}
		}
	case "get_kubernetes_pod":
		if name, ok := params["name"]; !ok {
			return fmt.Errorf("name parameter is required")
		} else if n, ok := name.(string); !ok || n == "" {
			return fmt.Errorf("name must be a non-empty string")
		}
		if namespace, ok := params["namespace"]; ok {
			if ns, ok := namespace.(string); !ok || ns == "" {
				return fmt.Errorf("namespace must be a non-empty string")
			}
		}
	case "list_kubernetes_namespaces", "list_kubernetes_nodes", "get_cluster_summary":
		// These tools don't require parameters, but validate no unexpected params
		for key := range params {
			return fmt.Errorf("unexpected parameter: %s", key)
		}
	}

	return nil
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

	// MCP 2025-06-18: Validate input parameters against schema
	if s.protocolVersion == "2025-06-18" {
		if validationErr := s.validateToolInput(toolName, params); validationErr != nil {
			return MCPResponse{
				Error: &MCPError{
					Code:    -32602,
					Message: fmt.Sprintf("Input validation failed: %v", validationErr),
					Data: map[string]interface{}{
						"toolName":        toolName,
						"validationError": validationErr.Error(),
						"providedParams":  params,
					},
				},
				ID: req.ID,
			}
		}
	}

	// Create and execute task
	result, structuredData, err := s.executeTaskWithStructuredOutput(ctx, toolName, capability, domain, params)
	if err != nil {
		// MCP 2025-06-18: Enhanced error handling with context
		errorResponse := s.createEnhancedError(err, toolName, params)
		return MCPResponse{
			Error: errorResponse,
			ID:    req.ID,
		}
	}

	// Build response based on protocol version
	content := []map[string]interface{}{
		{
			"type": "text",
			"text": result,
		},
	}

	// MCP 2025-06-18: Add resource links if available
	if s.protocolVersion == "2025-06-18" {
		resourceLinks := s.generateResourceLinksForTool(toolName, params, structuredData)
		content = append(content, resourceLinks...)
	}

	response := map[string]interface{}{
		"content":  content,
		"isError":  false,
	}

	// MCP 2025-06-18: Add structured content if available and protocol supports it
	if s.protocolVersion == "2025-06-18" && structuredData != nil {
		response["structuredContent"] = structuredData
	}

	return MCPResponse{
		Result: response,
		ID:     req.ID,
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

// executeTaskWithStructuredOutput creates a Kubernetes task and waits for completion, returning both text and structured data
func (s *Server) executeTaskWithStructuredOutput(ctx context.Context, toolName, capability, domain string, params map[string]interface{}) (string, interface{}, error) {
	// Create task payload
	payloadBytes, err := json.Marshal(params)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal params: %w", err)
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
		return "", nil, fmt.Errorf("failed to create task: %w", err)
	}

	// Wait for task completion
	return s.waitForTaskCompletionWithStructuredOutput(ctx, taskName)
}

// waitForTaskCompletionWithStructuredOutput waits for a task to complete and returns both text and structured data
func (s *Server) waitForTaskCompletionWithStructuredOutput(ctx context.Context, taskName string) (string, interface{}, error) {
	// Optimized: Check immediately first (some tasks complete very quickly)
	task := &mcpv1.Task{}
	key := types.NamespacedName{Name: taskName, Namespace: "default"}

	if err := s.Client.Get(ctx, key, task); err == nil {
		switch task.Status.Phase {
		case mcpv1.TaskPhaseCompleted:
			textResult, structuredResult := s.formatTaskResultWithStructuredOutput(task)
			return textResult, structuredResult, nil
		case mcpv1.TaskPhaseFailed:
			return "", nil, fmt.Errorf("task failed: %s", task.Status.Error)
		case mcpv1.TaskPhaseCancelled:
			return "", nil, fmt.Errorf("task was cancelled")
		}
	}

	timeout := time.After(90 * time.Second)
	// Optimized: Use 500ms polling for faster response times
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return "", nil, fmt.Errorf("task execution timed out")
		case <-ticker.C:
			if err := s.Client.Get(ctx, key, task); err != nil {
				s.Logger.Error(err, "Failed to get task", "task", taskName)
				continue
			}

			switch task.Status.Phase {
			case mcpv1.TaskPhaseCompleted:
				textResult, structuredResult := s.formatTaskResultWithStructuredOutput(task)
				return textResult, structuredResult, nil
			case mcpv1.TaskPhaseFailed:
				return "", nil, fmt.Errorf("task failed: %s", task.Status.Error)
			case mcpv1.TaskPhaseCancelled:
				return "", nil, fmt.Errorf("task was cancelled")
			case mcpv1.TaskPhaseWaitingForElicitation:
				// MCP 2025-06-18: Handle elicitation requests
				if s.protocolVersion == "2025-06-18" && task.Status.ElicitationRequest != nil {
					return s.handleTaskElicitation(ctx, task)
				}
				s.Logger.V(1).Info("Task waiting for elicitation", "task", taskName)
			default:
				s.Logger.V(1).Info("Task still running", "task", taskName, "phase", task.Status.Phase)
			}
		}
	}
}

// formatTaskResultWithStructuredOutput formats the task result for MCP response, returning both text and structured data
func (s *Server) formatTaskResultWithStructuredOutput(task *mcpv1.Task) (string, interface{}) {
	if task.Status.Result == nil {
		return "Task completed successfully (no result data)", nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(task.Status.Result.Raw, &result); err != nil {
		return fmt.Sprintf("Task completed but result parsing failed: %v", err), nil
	}

	// Format the result nicely for text content
	resultBytes, err := json.MarshalIndent(result, "", "  ")
	textResult := ""
	if err != nil {
		textResult = fmt.Sprintf("Task completed: %v", result)
	} else {
		textResult = string(resultBytes)
	}

	// Return both text and structured data
	// The structured data is the parsed result without JSON formatting
	return textResult, result
}

// formatTaskResult formats the task result for MCP response (backward compatibility)
func (s *Server) formatTaskResult(task *mcpv1.Task) string {
	textResult, _ := s.formatTaskResultWithStructuredOutput(task)
	return textResult
}
