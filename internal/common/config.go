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

package common

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// ControllerConfig holds configuration for the MCP Controller
type ControllerConfig struct {
	// MetricsAddr is the address the metric endpoint binds to
	MetricsAddr string

	// ProbeAddr is the address the probe endpoint binds to
	ProbeAddr string

	// LeaderElection enables leader election for controller manager
	LeaderElection bool

	// LeaderElectionID is the name of the leader election resource
	LeaderElectionID string

	// ReconcileInterval is the interval for reconciling resources
	ReconcileInterval time.Duration

	// MaxConcurrentReconciles is the maximum number of concurrent reconciles
	MaxConcurrentReconciles int

	// Domain is the domain this controller instance manages (optional, empty means all domains)
	Domain string

	// Namespace is the namespace to watch (empty means all namespaces)
	Namespace string
}

// AgentConfig holds configuration for MCP Agents
type AgentConfig struct {
	// AgentName is the name of this agent instance
	AgentName string

	// Domain is the domain this agent belongs to
	Domain string

	// Capabilities is the list of capabilities this agent provides
	Capabilities []string

	// Tags are additional metadata tags for this agent
	Tags []string

	// MaxConcurrentTasks is the maximum number of tasks this agent can handle simultaneously
	MaxConcurrentTasks int32

	// HeartbeatInterval is how often the agent reports its health
	HeartbeatInterval time.Duration

	// TaskTimeout is the default timeout for task execution
	TaskTimeout time.Duration

	// Namespace is the namespace this agent operates in
	Namespace string

	// MetricsAddr is the address the metric endpoint binds to
	MetricsAddr string

	// ProbeAddr is the address the probe endpoint binds to
	ProbeAddr string

	// Resources defines the resource allocation for this agent
	Resources AgentResources
}

// AgentResources defines resource requirements for an agent
type AgentResources struct {
	CPU     string
	Memory  string
	GPU     bool
	Storage string
}

// NewControllerConfig creates a new controller configuration from environment variables
func NewControllerConfig() *ControllerConfig {
	return &ControllerConfig{
		MetricsAddr:             getEnvOrDefault("METRICS_ADDR", ":8080"),
		ProbeAddr:               getEnvOrDefault("PROBE_ADDR", ":8081"),
		LeaderElection:          getEnvBoolOrDefault("LEADER_ELECTION", true),
		LeaderElectionID:        getEnvOrDefault("LEADER_ELECTION_ID", "mcp-controller-leader"),
		ReconcileInterval:       getEnvDurationOrDefault("RECONCILE_INTERVAL", 30*time.Second),
		MaxConcurrentReconciles: getEnvIntOrDefault("MAX_CONCURRENT_RECONCILES", 10),
		Domain:                  getEnvOrDefault("CONTROLLER_DOMAIN", ""),
		Namespace:               getEnvOrDefault("WATCH_NAMESPACE", ""),
	}
}

// NewAgentConfig creates a new agent configuration from environment variables
func NewAgentConfig() *AgentConfig {
	return &AgentConfig{
		AgentName:          getEnvOrDefault("AGENT_NAME", generateAgentName()),
		Domain:             getEnvOrDefault("AGENT_DOMAIN", "default"),
		Capabilities:       getEnvSliceOrDefault("AGENT_CAPABILITIES", []string{"generic"}),
		Tags:               getEnvSliceOrDefault("AGENT_TAGS", []string{}),
		MaxConcurrentTasks: int32(getEnvIntOrDefault("MAX_CONCURRENT_TASKS", 5)),
		HeartbeatInterval:  getEnvDurationOrDefault("HEARTBEAT_INTERVAL", 30*time.Second),
		TaskTimeout:        getEnvDurationOrDefault("TASK_TIMEOUT", 300*time.Second),
		Namespace:          getEnvOrDefault("NAMESPACE", "default"),
		MetricsAddr:        getEnvOrDefault("METRICS_ADDR", ":8080"),
		ProbeAddr:          getEnvOrDefault("PROBE_ADDR", ":8081"),
		Resources: AgentResources{
			CPU:     getEnvOrDefault("AGENT_CPU", "500m"),
			Memory:  getEnvOrDefault("AGENT_MEMORY", "1Gi"),
			GPU:     getEnvBoolOrDefault("AGENT_GPU", false),
			Storage: getEnvOrDefault("AGENT_STORAGE", ""),
		},
	}
}

// Helper functions for environment variable parsing

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvDurationOrDefault(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvSliceOrDefault(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		// Simple comma-separated parsing
		result := []string{}
		for _, item := range splitAndTrim(value, ",") {
			if item != "" {
				result = append(result, item)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}

func splitAndTrim(s, sep string) []string {
	parts := []string{}
	for _, part := range splitString(s, sep) {
		trimmed := trimString(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func splitString(s, sep string) []string {
	if s == "" {
		return []string{}
	}

	parts := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
			parts = append(parts, s[start:i])
			start = i + len(sep)
			i += len(sep) - 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trimString(s string) string {
	start := 0
	end := len(s)

	// Trim leading whitespace
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}

	// Trim trailing whitespace
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}

	return s[start:end]
}

func generateAgentName() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Convert to lowercase and replace invalid characters
	name := "agent-" + strings.ToLower(hostname)
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// Ensure it starts and ends with alphanumeric
	if len(name) > 63 {
		name = name[:63]
	}

	return name
}
