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

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcpv1 "github.com/mcp-conductor/mcp-conductor/api/v1"
	"github.com/mcp-conductor/mcp-conductor/internal/common"
)

// Manager handles agent lifecycle and coordination
type Manager struct {
	Client   client.Client
	Config   *common.AgentConfig
	Logger   logr.Logger
	Executor TaskExecutor
	Watcher  *TaskWatcher

	// Internal state
	agentResource   *mcpv1.Agent
	stopCh          chan struct{}
	heartbeatTicker *time.Ticker
}

// TaskExecutor defines the interface for task execution
type TaskExecutor interface {
	ExecuteTask(ctx context.Context, task *mcpv1.Task) (*TaskResult, error)
	GetCapabilities() []string
	GetTags() []string
}

// TaskResult represents the result of task execution
type TaskResult struct {
	Success bool
	Data    map[string]interface{}
	Error   string
}

// NewManager creates a new agent manager
func NewManager(client client.Client, config *common.AgentConfig, executor TaskExecutor, logger logr.Logger) *Manager {
	return &Manager{
		Client:   client,
		Config:   config,
		Logger:   logger.WithName("agent-manager"),
		Executor: executor,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the agent lifecycle
func (m *Manager) Start(ctx context.Context) error {
	m.Logger.Info("Starting MCP Agent",
		"name", m.Config.AgentName,
		"domain", m.Config.Domain,
		"capabilities", m.Executor.GetCapabilities(),
		"tags", m.Executor.GetTags())

	// Register agent with the cluster
	if err := m.registerAgent(ctx); err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}

	// Start heartbeat
	m.startHeartbeat(ctx)

	// Start task watcher
	m.Watcher = NewTaskWatcher(m.Client, m.Config, m.Logger.WithName("task-watcher"))
	if err := m.Watcher.Start(ctx, m.handleTaskAssignment); err != nil {
		return fmt.Errorf("failed to start task watcher: %w", err)
	}

	m.Logger.Info("Agent started successfully")
	return nil
}

// Stop gracefully shuts down the agent
func (m *Manager) Stop(ctx context.Context) error {
	m.Logger.Info("Stopping MCP Agent")

	// Signal stop
	close(m.stopCh)

	// Stop heartbeat
	if m.heartbeatTicker != nil {
		m.heartbeatTicker.Stop()
	}

	// Stop task watcher
	if m.Watcher != nil {
		m.Watcher.Stop()
	}

	// Update agent status to terminating
	if m.agentResource != nil {
		m.agentResource.Status.Phase = mcpv1.AgentPhaseTerminating
		if err := m.Client.Status().Update(ctx, m.agentResource); err != nil {
			m.Logger.Error(err, "Failed to update agent status to terminating")
		}
	}

	m.Logger.Info("Agent stopped")
	return nil
}

// registerAgent creates or updates the agent resource in the cluster
func (m *Manager) registerAgent(ctx context.Context) error {
	agent := &mcpv1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.Config.AgentName,
			Namespace: m.Config.Namespace,
			Labels: map[string]string{
				"app":       "mcp-conductor",
				"component": "agent",
				"domain":    m.Config.Domain,
			},
		},
		Spec: mcpv1.AgentSpec{
			Domain:             m.Config.Domain,
			Capabilities:       m.Executor.GetCapabilities(),
			Tags:               m.Executor.GetTags(),
			MaxConcurrentTasks: m.Config.MaxConcurrentTasks,
			HeartbeatInterval:  metav1.Duration{Duration: m.Config.HeartbeatInterval},
			Resources: mcpv1.AgentResources{
				CPU:     m.Config.Resources.CPU,
				Memory:  m.Config.Resources.Memory,
				GPU:     m.Config.Resources.GPU,
				Storage: m.Config.Resources.Storage,
			},
		},
	}

	// Try to get existing agent
	existing := &mcpv1.Agent{}
	key := types.NamespacedName{Name: agent.Name, Namespace: agent.Namespace}
	err := m.Client.Get(ctx, key, existing)

	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Agent doesn't exist, create it
			if err := m.Client.Create(ctx, agent); err != nil {
				return fmt.Errorf("failed to create agent resource: %w", err)
			}
			m.Logger.Info("Agent resource created")
		} else {
			return fmt.Errorf("failed to get existing agent: %w", err)
		}
	} else {
		// Agent exists, update it
		existing.Spec = agent.Spec
		if err := m.Client.Update(ctx, existing); err != nil {
			return fmt.Errorf("failed to update agent resource: %w", err)
		}
		m.Logger.Info("Agent resource updated")
		agent = existing
	}

	m.agentResource = agent
	return nil
}

// startHeartbeat begins sending periodic heartbeats
func (m *Manager) startHeartbeat(ctx context.Context) {
	m.heartbeatTicker = time.NewTicker(m.Config.HeartbeatInterval)

	go func() {
		defer m.heartbeatTicker.Stop()

		for {
			select {
			case <-m.heartbeatTicker.C:
				if err := m.sendHeartbeat(ctx); err != nil {
					m.Logger.Error(err, "Failed to send heartbeat")
				}
			case <-m.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// sendHeartbeat updates the agent's last heartbeat timestamp
func (m *Manager) sendHeartbeat(ctx context.Context) error {
	if m.agentResource == nil {
		return fmt.Errorf("agent resource not initialized")
	}

	// Get the latest version of the agent
	key := types.NamespacedName{Name: m.agentResource.Name, Namespace: m.agentResource.Namespace}
	if err := m.Client.Get(ctx, key, m.agentResource); err != nil {
		return fmt.Errorf("failed to get agent resource: %w", err)
	}

	// Update heartbeat timestamp
	now := metav1.NewTime(time.Now())
	m.agentResource.Status.LastHeartbeat = &now

	// Update status
	if err := m.Client.Status().Update(ctx, m.agentResource); err != nil {
		return fmt.Errorf("failed to update heartbeat: %w", err)
	}

	m.Logger.V(1).Info("Heartbeat sent")
	return nil
}

// handleTaskAssignment processes a task assignment
func (m *Manager) handleTaskAssignment(ctx context.Context, task *mcpv1.Task) error {
	m.Logger.Info("Received task assignment",
		"task", task.Name,
		"domain", task.Spec.Domain,
		"capabilities", task.Spec.RequiredCapabilities)

	// Update task status to running
	task.Status.Phase = mcpv1.TaskPhaseRunning
	task.Status.StartTime = &metav1.Time{Time: time.Now()}

	if err := m.Client.Status().Update(ctx, task); err != nil {
		m.Logger.Error(err, "Failed to update task status to running")
		return err
	}

	// Execute task in background (make a copy to avoid race conditions)
	taskCopy := task.DeepCopy()
	go m.executeTaskAsync(ctx, taskCopy)

	return nil
}

// executeTaskAsync executes a task asynchronously
func (m *Manager) executeTaskAsync(ctx context.Context, task *mcpv1.Task) {
	logger := m.Logger.WithValues("task", task.Name)
	logger.Info("Starting task execution")

	// Create timeout context
	taskCtx, cancel := context.WithTimeout(ctx, task.Spec.Timeout.Duration)
	defer cancel()

	// Execute the task
	result, err := m.Executor.ExecuteTask(taskCtx, task)

	// Update task status based on result
	if err := m.updateTaskResult(ctx, task, result, err); err != nil {
		logger.Error(err, "Failed to update task result")
	}

	logger.Info("Task execution completed",
		"success", result != nil && result.Success,
		"error", err)
}

// updateTaskResult updates the task with execution results
func (m *Manager) updateTaskResult(ctx context.Context, task *mcpv1.Task, result *TaskResult, execErr error) error {
	// Get the latest version of the task
	key := types.NamespacedName{Name: task.Name, Namespace: task.Namespace}
	if err := m.Client.Get(ctx, key, task); err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}

	// Update completion time
	task.Status.CompletionTime = &metav1.Time{Time: time.Now()}

	if execErr != nil {
		// Task failed
		task.Status.Phase = mcpv1.TaskPhaseFailed
		task.Status.Error = execErr.Error()
	} else if result != nil && result.Success {
		// Task succeeded
		task.Status.Phase = mcpv1.TaskPhaseCompleted
		if result.Data != nil {
			// Convert result data to RawExtension
			resultData := map[string]interface{}{
				"success": true,
				"data":    result.Data,
			}

			jsonData, err := json.Marshal(resultData)
			if err != nil {
				m.Logger.Error(err, "Failed to marshal result data")
				task.Status.Result = &runtime.RawExtension{
					Raw: []byte(`{"success": true, "data": "result_marshal_error"}`),
				}
			} else {
				task.Status.Result = &runtime.RawExtension{
					Raw: jsonData,
				}
			}
		}
	} else {
		// Task failed without explicit error
		task.Status.Phase = mcpv1.TaskPhaseFailed
		if result != nil {
			task.Status.Error = result.Error
		} else {
			task.Status.Error = "Task execution failed with unknown error"
		}
	}

	// Update task status
	if err := m.Client.Status().Update(ctx, task); err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	return nil
}
