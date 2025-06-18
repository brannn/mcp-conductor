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

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	mcpv1 "github.com/mcp-conductor/mcp-conductor/api/v1"
	"github.com/mcp-conductor/mcp-conductor/internal/agent"
	"github.com/mcp-conductor/mcp-conductor/internal/agent/examples"
	"github.com/mcp-conductor/mcp-conductor/internal/common"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = common.SetupAgentLogger("mcp-agent")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(mcpv1.AddToScheme(scheme))
}

func main() {
	var agentType string
	var configFile string

	flag.StringVar(&agentType, "type", "k8s-reporter",
		"Type of agent to run (k8s-reporter, data-processor, http-client)")
	flag.StringVar(&configFile, "config", "",
		"The agent will load its initial configuration from this file. "+
			"Omit this flag to use the default configuration values. "+
			"Command-line flags override configuration from this file.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Load configuration
	config := common.NewAgentConfig()
	setupLog.Info("Starting MCP Agent",
		"type", agentType,
		"name", config.AgentName,
		"domain", config.Domain,
		"namespace", config.Namespace,
		"maxConcurrentTasks", config.MaxConcurrentTasks,
		"heartbeatInterval", config.HeartbeatInterval,
	)

	// Create Kubernetes client
	k8sConfig, err := common.GetKubernetesConfig()
	if err != nil {
		setupLog.Error(err, "Failed to get Kubernetes config")
		os.Exit(1)
	}

	k8sClient, err := client.New(k8sConfig, client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "Failed to create Kubernetes client")
		os.Exit(1)
	}

	// Create task executor based on agent type
	executor, err := createExecutor(agentType, k8sClient, config)
	if err != nil {
		setupLog.Error(err, "Failed to create task executor", "type", agentType)
		os.Exit(1)
	}

	// Update config with executor capabilities and tags
	config.Capabilities = executor.GetCapabilities()
	config.Tags = append(config.Tags, executor.GetTags()...)

	setupLog.Info("Agent capabilities configured",
		"capabilities", config.Capabilities,
		"tags", config.Tags)

	// Create agent manager
	manager := agent.NewManager(k8sClient, config, executor, setupLog)

	// Setup context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start agent
	if err := manager.Start(ctx); err != nil {
		setupLog.Error(err, "Failed to start agent")
		os.Exit(1)
	}

	setupLog.Info("Agent started successfully, waiting for tasks...")

	// Wait for shutdown signal
	<-sigCh
	setupLog.Info("Received shutdown signal, stopping agent...")

	// Graceful shutdown
	if err := manager.Stop(ctx); err != nil {
		setupLog.Error(err, "Error during agent shutdown")
		os.Exit(1)
	}

	setupLog.Info("Agent stopped successfully")
}

// createExecutor creates the appropriate task executor based on agent type
func createExecutor(agentType string, k8sClient client.Client, config *common.AgentConfig) (agent.TaskExecutor, error) {
	logger := setupLog.WithValues("agentType", agentType)

	switch strings.ToLower(agentType) {
	case "k8s-reporter", "kubernetes-reporter":
		return examples.NewK8sReporter(k8sClient, logger), nil

	case "data-processor", "data-processing":
		return examples.NewDataProcessor(logger), nil

	case "http-client", "http":
		return examples.NewHTTPClient(logger), nil

	case "multi", "combined":
		// Create a multi-capability agent that combines multiple executors
		return createMultiExecutor(k8sClient, config, logger)

	default:
		return nil, fmt.Errorf("unsupported agent type: %s. Supported types: k8s-reporter, data-processor, http-client, multi", agentType)
	}
}

// createMultiExecutor creates an executor that combines multiple capabilities
func createMultiExecutor(k8sClient client.Client, config *common.AgentConfig, logger logr.Logger) (agent.TaskExecutor, error) {
	multiExec := NewMultiExecutor(logger)

	// Add different executors for different capability domains
	k8sExecutor := examples.NewK8sReporter(k8sClient, logger)
	multiExec.AddExecutor(k8sExecutor.GetCapabilities(), k8sExecutor)

	dataExecutor := examples.NewDataProcessor(logger)
	multiExec.AddExecutor(dataExecutor.GetCapabilities(), dataExecutor)

	httpExecutor := examples.NewHTTPClient(logger)
	multiExec.AddExecutor(httpExecutor.GetCapabilities(), httpExecutor)

	return multiExec, nil
}

// MultiExecutor combines multiple executors (example implementation)
type MultiExecutor struct {
	executors map[string]agent.TaskExecutor
	logger    logr.Logger
}

// NewMultiExecutor creates a new multi-capability executor
func NewMultiExecutor(logger logr.Logger) *MultiExecutor {
	return &MultiExecutor{
		executors: make(map[string]agent.TaskExecutor),
		logger:    logger,
	}
}

// AddExecutor adds an executor for specific capabilities
func (m *MultiExecutor) AddExecutor(capabilities []string, executor agent.TaskExecutor) {
	for _, capability := range capabilities {
		m.executors[capability] = executor
	}
}

// GetCapabilities returns all capabilities from all executors
func (m *MultiExecutor) GetCapabilities() []string {
	capabilities := make([]string, 0, len(m.executors))
	for capability := range m.executors {
		capabilities = append(capabilities, capability)
	}
	return capabilities
}

// GetTags returns combined tags from all executors
func (m *MultiExecutor) GetTags() []string {
	tagSet := make(map[string]bool)
	for _, executor := range m.executors {
		for _, tag := range executor.GetTags() {
			tagSet[tag] = true
		}
	}

	tags := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tags = append(tags, tag)
	}
	return tags
}

// ExecuteTask executes a task using the appropriate executor
func (m *MultiExecutor) ExecuteTask(ctx context.Context, task *mcpv1.Task) (*agent.TaskResult, error) {
	// Find the first executor that can handle one of the required capabilities
	for _, capability := range task.Spec.RequiredCapabilities {
		if executor, exists := m.executors[capability]; exists {
			m.logger.Info("Delegating task to executor",
				"capability", capability,
				"task", task.Name)
			return executor.ExecuteTask(ctx, task)
		}
	}

	return &agent.TaskResult{
		Success: false,
		Error:   fmt.Sprintf("No executor found for capabilities: %v", task.Spec.RequiredCapabilities),
	}, nil
}
