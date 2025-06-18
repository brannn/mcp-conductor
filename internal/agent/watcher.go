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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/watch"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcpv1 "github.com/mcp-conductor/mcp-conductor/api/v1"
	"github.com/mcp-conductor/mcp-conductor/internal/common"
)

// TaskWatcher watches for task assignments to this agent
type TaskWatcher struct {
	Client client.Client
	Config *common.AgentConfig
	Logger logr.Logger

	stopCh chan struct{}
}

// TaskHandler is called when a task is assigned to this agent
type TaskHandler func(ctx context.Context, task *mcpv1.Task) error

// NewTaskWatcher creates a new task watcher
func NewTaskWatcher(client client.Client, config *common.AgentConfig, logger logr.Logger) *TaskWatcher {
	return &TaskWatcher{
		Client: client,
		Config: config,
		Logger: logger,
		stopCh: make(chan struct{}),
	}
}

// Start begins watching for task assignments
func (w *TaskWatcher) Start(ctx context.Context, handler TaskHandler) error {
	w.Logger.Info("Starting task watcher", "agent", w.Config.AgentName)

	go w.watchLoop(ctx, handler)
	return nil
}

// Stop stops the task watcher
func (w *TaskWatcher) Stop() {
	w.Logger.Info("Stopping task watcher")
	close(w.stopCh)
}

// watchLoop continuously watches for task assignments
func (w *TaskWatcher) watchLoop(ctx context.Context, handler TaskHandler) {
	for {
		select {
		case <-w.stopCh:
			w.Logger.Info("Task watcher stopped")
			return
		case <-ctx.Done():
			w.Logger.Info("Task watcher context cancelled")
			return
		default:
			if err := w.watchTasks(ctx, handler); err != nil {
				w.Logger.Error(err, "Error watching tasks, retrying in 10 seconds")
				time.Sleep(10 * time.Second)
			}
		}
	}
}

// watchTasks performs the actual task watching
func (w *TaskWatcher) watchTasks(ctx context.Context, handler TaskHandler) error {
	// First, check for any existing assigned tasks
	if err := w.processExistingTasks(ctx, handler); err != nil {
		w.Logger.Error(err, "Failed to process existing tasks")
	}

	// Then start watching for new assignments
	return w.watchForNewAssignments(ctx, handler)
}

// processExistingTasks handles tasks that are already assigned to this agent
func (w *TaskWatcher) processExistingTasks(ctx context.Context, handler TaskHandler) error {
	taskList := &mcpv1.TaskList{}

	// Use label selector to efficiently find tasks assigned to this agent
	listOpts := []client.ListOption{
		client.InNamespace(w.Config.Namespace),
		client.MatchingLabels{"assignedAgent": w.Config.AgentName},
	}

	if err := w.Client.List(ctx, taskList, listOpts...); err != nil {
		return fmt.Errorf("failed to list existing tasks: %w", err)
	}

	for _, task := range taskList.Items {
		// Only process tasks that are assigned but not yet running
		if task.Status.Phase == mcpv1.TaskPhaseAssigned {
			w.Logger.Info("Processing existing assigned task", "task", task.Name)
			if err := handler(ctx, &task); err != nil {
				w.Logger.Error(err, "Failed to handle existing task", "task", task.Name)
			}
		}
	}

	return nil
}

// watchForNewAssignments watches for new task assignments using labels
func (w *TaskWatcher) watchForNewAssignments(ctx context.Context, handler TaskHandler) error {
	// Try to use watch API, fall back to polling if not supported
	watchClient, ok := w.Client.(client.WithWatch)
	if !ok {
		w.Logger.Info("Client does not support watching, falling back to polling")
		return w.pollForNewAssignments(ctx, handler)
	}

	// Set up watch options with label selector
	watchOpts := []client.ListOption{
		client.InNamespace(w.Config.Namespace),
		client.MatchingLabels{"assignedAgent": w.Config.AgentName},
	}

	// Start watching
	watcher, err := watchClient.Watch(ctx, &mcpv1.TaskList{}, watchOpts...)
	if err != nil {
		w.Logger.Info("Failed to start watch, falling back to polling", "error", err)
		return w.pollForNewAssignments(ctx, handler)
	}
	defer watcher.Stop()

	w.Logger.Info("Started watching for task assignments")

	for {
		select {
		case <-w.stopCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.ResultChan():
			if !ok {
				w.Logger.Info("Watch channel closed, restarting watch")
				return nil // This will cause the watch to restart
			}

			if err := w.handleWatchEvent(ctx, event, handler); err != nil {
				w.Logger.Error(err, "Failed to handle watch event")
			}
		}
	}
}

// handleWatchEvent processes a watch event
func (w *TaskWatcher) handleWatchEvent(ctx context.Context, event watch.Event, handler TaskHandler) error {
	task, ok := event.Object.(*mcpv1.Task)
	if !ok {
		return fmt.Errorf("unexpected object type: %T", event.Object)
	}

	// Only process domain-specific tasks
	if task.Spec.Domain != w.Config.Domain {
		return nil
	}

	switch event.Type {
	case watch.Added, watch.Modified:
		// Handle task assignment or updates
		if task.Status.Phase == mcpv1.TaskPhaseAssigned {
			w.Logger.Info("Task assigned to agent",
				"task", task.Name,
				"domain", task.Spec.Domain,
				"capabilities", task.Spec.RequiredCapabilities)

			return handler(ctx, task)
		}
	case watch.Deleted:
		// Task was deleted, nothing to do
		w.Logger.Info("Task deleted", "task", task.Name)
	case watch.Error:
		return fmt.Errorf("watch error event: %v", event.Object)
	}

	return nil
}

// pollForNewAssignments polls for new task assignments as fallback
func (w *TaskWatcher) pollForNewAssignments(ctx context.Context, handler TaskHandler) error {
	w.Logger.Info("Started polling for task assignments")

	ticker := time.NewTicker(5 * time.Second) // Poll every 5 seconds
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.pollForTasks(ctx, handler); err != nil {
				w.Logger.Error(err, "Failed to poll for tasks")
			}
		}
	}
}

// pollForTasks polls for tasks assigned to this agent
func (w *TaskWatcher) pollForTasks(ctx context.Context, handler TaskHandler) error {
	taskList := &mcpv1.TaskList{}

	// Use label selector for efficient filtering
	listOpts := []client.ListOption{
		client.InNamespace(w.Config.Namespace),
		client.MatchingLabels{"assignedAgent": w.Config.AgentName},
	}

	if err := w.Client.List(ctx, taskList, listOpts...); err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	for _, task := range taskList.Items {
		// Only process domain-specific tasks
		if task.Spec.Domain != w.Config.Domain {
			continue
		}

		// Handle task assignment
		if task.Status.Phase == mcpv1.TaskPhaseAssigned {
			w.Logger.V(1).Info("Found assigned task",
				"task", task.Name,
				"domain", task.Spec.Domain,
				"capabilities", task.Spec.RequiredCapabilities)

			if err := handler(ctx, &task); err != nil {
				w.Logger.Error(err, "Failed to handle assigned task", "task", task.Name)
			}
		}
	}

	return nil
}

// GetAssignedTasks returns all tasks currently assigned to this agent
func (w *TaskWatcher) GetAssignedTasks(ctx context.Context) ([]mcpv1.Task, error) {
	taskList := &mcpv1.TaskList{}

	listOpts := []client.ListOption{
		client.InNamespace(w.Config.Namespace),
		client.MatchingLabels{"assignedAgent": w.Config.AgentName},
	}

	if err := w.Client.List(ctx, taskList, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list assigned tasks: %w", err)
	}

	return taskList.Items, nil
}

// GetRunningTasks returns tasks currently being executed by this agent
func (w *TaskWatcher) GetRunningTasks(ctx context.Context) ([]mcpv1.Task, error) {
	tasks, err := w.GetAssignedTasks(ctx)
	if err != nil {
		return nil, err
	}

	var runningTasks []mcpv1.Task
	for _, task := range tasks {
		if task.Status.Phase == mcpv1.TaskPhaseRunning {
			runningTasks = append(runningTasks, task)
		}
	}

	return runningTasks, nil
}
