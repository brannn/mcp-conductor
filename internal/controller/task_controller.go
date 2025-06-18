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

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mcpv1 "github.com/mcp-conductor/mcp-conductor/api/v1"
	"github.com/mcp-conductor/mcp-conductor/internal/common"
)

// TaskReconciler reconciles a Task object
type TaskReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Config    *common.ControllerConfig
	Recorder  record.EventRecorder
	Scheduler *TaskScheduler
}

//+kubebuilder:rbac:groups=mcp.io,resources=tasks,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mcp.io,resources=tasks/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mcp.io,resources=tasks/finalizers,verbs=update
//+kubebuilder:rbac:groups=mcp.io,resources=agents,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=mcp.io,resources=agents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile handles Task resource changes
func (r *TaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("task", req.NamespacedName)

	// Fetch the Task instance
	task := &mcpv1.Task{}
	if err := r.Get(ctx, req.NamespacedName, task); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Task resource not found, likely deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Task")
		return ctrl.Result{}, err
	}

	// Check if this controller should handle this task (domain filtering)
	if r.Config.Domain != "" && task.Spec.Domain != r.Config.Domain {
		logger.V(1).Info("Skipping task from different domain",
			"taskDomain", task.Spec.Domain,
			"controllerDomain", r.Config.Domain)
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if task.DeletionTimestamp != nil {
		return r.handleTaskDeletion(ctx, task, logger)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(task, "mcp.io/task-finalizer") {
		controllerutil.AddFinalizer(task, "mcp.io/task-finalizer")
		if err := r.Update(ctx, task); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Initialize status if needed
	if task.Status.Phase == "" {
		task.Status.Phase = mcpv1.TaskPhasePending
		task.Status.RetryCount = 0
		task.Status.ObservedGeneration = task.Generation

		r.setTaskCondition(task, mcpv1.TaskConditionAssigned, metav1.ConditionFalse,
			"Pending", "Task is waiting for assignment")

		if err := r.Status().Update(ctx, task); err != nil {
			logger.Error(err, "Failed to initialize task status")
			return ctrl.Result{}, err
		}

		r.Recorder.Event(task, "Normal", "Created", "Task has been created")
		return ctrl.Result{RequeueAfter: time.Second * 2}, nil
	}

	// Handle task assignment
	if task.Status.Phase == mcpv1.TaskPhasePending {
		return r.handleTaskAssignment(ctx, task, logger)
	}

	// Handle task timeout
	if r.isTaskTimedOut(task) {
		return r.handleTaskTimeout(ctx, task, logger)
	}

	// Handle task deadline
	if r.isTaskPastDeadline(task) {
		return r.handleTaskDeadline(ctx, task, logger)
	}

	// Handle failed tasks that need retry
	if task.Status.Phase == mcpv1.TaskPhaseFailed && r.shouldRetryTask(task) {
		return r.handleTaskRetry(ctx, task, logger)
	}

	// Handle task completion - decrement agent task counter
	if (task.Status.Phase == mcpv1.TaskPhaseCompleted || task.Status.Phase == mcpv1.TaskPhaseFailed) &&
		task.Status.AssignedAgent != "" && !r.hasDecrementedTaskCount(task) {
		return r.handleTaskCompletion(ctx, task, logger)
	}

	// Update observed generation
	if task.Status.ObservedGeneration != task.Generation {
		task.Status.ObservedGeneration = task.Generation
		if err := r.Status().Update(ctx, task); err != nil {
			logger.Error(err, "Failed to update observed generation")
			return ctrl.Result{}, err
		}
	}

	// Determine next reconcile time
	nextReconcile := r.getNextReconcileTime(task)
	if nextReconcile > 0 {
		logger.V(1).Info("Task reconciliation complete", "nextReconcile", nextReconcile)
		return ctrl.Result{RequeueAfter: nextReconcile}, nil
	}

	return ctrl.Result{}, nil
}

// handleTaskAssignment attempts to assign a task to a suitable agent
func (r *TaskReconciler) handleTaskAssignment(ctx context.Context, task *mcpv1.Task, logger logr.Logger) (ctrl.Result, error) {
	logger.Info("Attempting to assign task to agent")

	// Find suitable agent
	agent, err := r.Scheduler.FindSuitableAgent(ctx, task)
	if err != nil {
		logger.Error(err, "Failed to find suitable agent")
		r.setTaskCondition(task, mcpv1.TaskConditionAssigned, metav1.ConditionFalse,
			"SchedulingError", fmt.Sprintf("Failed to find agent: %v", err))

		if updateErr := r.Status().Update(ctx, task); updateErr != nil {
			logger.Error(updateErr, "Failed to update task status after scheduling error")
		}
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	if agent == nil {
		logger.Info("No suitable agent available, will retry")
		r.setTaskCondition(task, mcpv1.TaskConditionAssigned, metav1.ConditionFalse,
			"NoAgentAvailable", "No suitable agent available for this task")

		if err := r.Status().Update(ctx, task); err != nil {
			logger.Error(err, "Failed to update task status")
			return ctrl.Result{}, err
		}

		r.Recorder.Event(task, "Warning", "NoAgent", "No suitable agent available")
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	// Assign task to agent using both label and status
	// Add label for efficient agent filtering
	if task.Labels == nil {
		task.Labels = make(map[string]string)
	}
	task.Labels["assignedAgent"] = agent.Name

	// Update the task resource with the label
	if err := r.Update(ctx, task); err != nil {
		logger.Error(err, "Failed to update task with agent assignment label")
		return ctrl.Result{}, err
	}

	// Update status
	task.Status.Phase = mcpv1.TaskPhaseAssigned
	task.Status.AssignedAgent = agent.Name
	task.Status.StartTime = &metav1.Time{Time: time.Now()}

	r.setTaskCondition(task, mcpv1.TaskConditionAssigned, metav1.ConditionTrue,
		"Assigned", fmt.Sprintf("Task assigned to agent %s", agent.Name))

	if err := r.Status().Update(ctx, task); err != nil {
		logger.Error(err, "Failed to update task assignment status")
		return ctrl.Result{}, err
	}

	// Update agent's current task count
	agent.Status.CurrentTasks++
	if err := r.Status().Update(ctx, agent); err != nil {
		logger.Error(err, "Failed to update agent task count")
		// Don't fail the reconciliation for this
	}

	r.Recorder.Event(task, "Normal", "Assigned", fmt.Sprintf("Task assigned to agent %s", agent.Name))
	logger.Info("Task assigned to agent", "agent", agent.Name)

	return ctrl.Result{RequeueAfter: time.Second * 10}, nil
}

// handleTaskTimeout handles tasks that have exceeded their timeout
func (r *TaskReconciler) handleTaskTimeout(ctx context.Context, task *mcpv1.Task, logger logr.Logger) (ctrl.Result, error) {
	logger.Info("Task has timed out")

	task.Status.Phase = mcpv1.TaskPhaseFailed
	task.Status.Error = fmt.Sprintf("Task timed out after %v", task.Spec.Timeout.Duration)
	task.Status.CompletionTime = &metav1.Time{Time: time.Now()}

	r.setTaskCondition(task, mcpv1.TaskConditionCompleted, metav1.ConditionFalse,
		"Timeout", "Task execution timed out")

	if err := r.Status().Update(ctx, task); err != nil {
		logger.Error(err, "Failed to update task timeout status")
		return ctrl.Result{}, err
	}

	// Update agent task count
	if task.Status.AssignedAgent != "" {
		if err := r.decrementAgentTaskCount(ctx, task.Status.AssignedAgent, task.Namespace); err != nil {
			logger.Error(err, "Failed to decrement agent task count")
		}
	}

	r.Recorder.Event(task, "Warning", "Timeout", "Task execution timed out")
	return ctrl.Result{}, nil
}

// handleTaskDeadline handles tasks that have passed their deadline
func (r *TaskReconciler) handleTaskDeadline(ctx context.Context, task *mcpv1.Task, logger logr.Logger) (ctrl.Result, error) {
	logger.Info("Task has passed its deadline")

	task.Status.Phase = mcpv1.TaskPhaseFailed
	task.Status.Error = fmt.Sprintf("Task missed deadline %v", task.Spec.Deadline)
	task.Status.CompletionTime = &metav1.Time{Time: time.Now()}

	r.setTaskCondition(task, mcpv1.TaskConditionCompleted, metav1.ConditionFalse,
		"DeadlineExceeded", "Task missed its deadline")

	if err := r.Status().Update(ctx, task); err != nil {
		logger.Error(err, "Failed to update task deadline status")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(task, "Warning", "DeadlineExceeded", "Task missed its deadline")
	return ctrl.Result{}, nil
}

// handleTaskRetry handles retrying failed tasks
func (r *TaskReconciler) handleTaskRetry(ctx context.Context, task *mcpv1.Task, logger logr.Logger) (ctrl.Result, error) {
	logger.Info("Retrying failed task", "retryCount", task.Status.RetryCount)

	// Clear assignment label
	if err := r.removeTaskAssignmentLabel(ctx, task); err != nil {
		logger.Error(err, "Failed to remove assignment label during retry")
		return ctrl.Result{}, err
	}

	task.Status.Phase = mcpv1.TaskPhasePending
	task.Status.AssignedAgent = ""
	task.Status.StartTime = nil
	task.Status.Error = ""
	task.Status.RetryCount++

	r.setTaskCondition(task, mcpv1.TaskConditionAssigned, metav1.ConditionFalse,
		"Retrying", fmt.Sprintf("Retrying task (attempt %d)", task.Status.RetryCount+1))

	if err := r.Status().Update(ctx, task); err != nil {
		logger.Error(err, "Failed to update task retry status")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(task, "Normal", "Retrying", fmt.Sprintf("Retrying task (attempt %d)", task.Status.RetryCount+1))

	// Calculate retry delay
	retryDelay := r.calculateRetryDelay(task)
	return ctrl.Result{RequeueAfter: retryDelay}, nil
}

// handleTaskDeletion handles the deletion of a task
func (r *TaskReconciler) handleTaskDeletion(ctx context.Context, task *mcpv1.Task, logger logr.Logger) (ctrl.Result, error) {
	logger.Info("Handling task deletion")

	// Update agent task count if task was assigned
	if task.Status.AssignedAgent != "" {
		if err := r.decrementAgentTaskCount(ctx, task.Status.AssignedAgent, task.Namespace); err != nil {
			logger.Error(err, "Failed to decrement agent task count during deletion")
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(task, "mcp.io/task-finalizer")
	if err := r.Update(ctx, task); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(task, "Normal", "Deleted", "Task has been deleted")
	logger.Info("Task deletion complete")

	return ctrl.Result{}, nil
}

// Helper methods

// isTaskTimedOut checks if a task has exceeded its timeout
func (r *TaskReconciler) isTaskTimedOut(task *mcpv1.Task) bool {
	if task.Status.StartTime == nil {
		return false
	}

	timeout := task.Spec.Timeout.Duration
	if timeout == 0 {
		return false
	}

	elapsed := time.Since(task.Status.StartTime.Time)
	return elapsed > timeout
}

// isTaskPastDeadline checks if a task has passed its deadline
func (r *TaskReconciler) isTaskPastDeadline(task *mcpv1.Task) bool {
	if task.Spec.Deadline == nil {
		return false
	}

	return time.Now().After(task.Spec.Deadline.Time)
}

// shouldRetryTask determines if a failed task should be retried
func (r *TaskReconciler) shouldRetryTask(task *mcpv1.Task) bool {
	if task.Spec.RetryPolicy == nil {
		return false
	}

	return task.Status.RetryCount < task.Spec.RetryPolicy.MaxRetries
}

// calculateRetryDelay calculates the delay before retrying a task
func (r *TaskReconciler) calculateRetryDelay(task *mcpv1.Task) time.Duration {
	if task.Spec.RetryPolicy == nil {
		return time.Second * 30
	}

	retryPolicy := task.Spec.RetryPolicy
	baseDelay := retryPolicy.InitialDelay.Duration
	maxDelay := retryPolicy.MaxDelay.Duration

	var delay time.Duration
	switch retryPolicy.BackoffStrategy {
	case mcpv1.BackoffStrategyExponential:
		// Exponential backoff: delay = baseDelay * 2^retryCount
		multiplier := 1 << task.Status.RetryCount // 2^retryCount
		delay = time.Duration(multiplier) * baseDelay
	case mcpv1.BackoffStrategyLinear:
		// Linear backoff: delay = baseDelay * (retryCount + 1)
		delay = time.Duration(task.Status.RetryCount+1) * baseDelay
	case mcpv1.BackoffStrategyFixed:
		// Fixed delay
		delay = baseDelay
	default:
		delay = baseDelay
	}

	// Cap at max delay
	if maxDelay > 0 && delay > maxDelay {
		delay = maxDelay
	}

	return delay
}

// getNextReconcileTime determines when to reconcile the task next
func (r *TaskReconciler) getNextReconcileTime(task *mcpv1.Task) time.Duration {
	switch task.Status.Phase {
	case mcpv1.TaskPhaseAssigned, mcpv1.TaskPhaseRunning:
		// Check for timeout/deadline more frequently
		return time.Second * 30
	case mcpv1.TaskPhaseCompleted, mcpv1.TaskPhaseFailed, mcpv1.TaskPhaseCancelled:
		// No need to reconcile completed tasks
		return 0
	default:
		// Pending tasks - check less frequently
		return time.Minute
	}
}

// decrementAgentTaskCount decrements the current task count for an agent
func (r *TaskReconciler) decrementAgentTaskCount(ctx context.Context, agentName, namespace string) error {
	agent := &mcpv1.Agent{}
	key := client.ObjectKey{Name: agentName, Namespace: namespace}

	if err := r.Get(ctx, key, agent); err != nil {
		if errors.IsNotFound(err) {
			// Agent was deleted, nothing to update
			return nil
		}
		return err
	}

	if agent.Status.CurrentTasks > 0 {
		agent.Status.CurrentTasks--
		return r.Status().Update(ctx, agent)
	}

	return nil
}

// removeTaskAssignmentLabel removes the assignedAgent label from a task
func (r *TaskReconciler) removeTaskAssignmentLabel(ctx context.Context, task *mcpv1.Task) error {
	if task.Labels != nil {
		delete(task.Labels, "assignedAgent")
		return r.Update(ctx, task)
	}
	return nil
}

// setTaskCondition sets or updates a task condition
func (r *TaskReconciler) setTaskCondition(task *mcpv1.Task, conditionType mcpv1.TaskConditionType, status metav1.ConditionStatus, reason, message string) {
	now := metav1.NewTime(time.Now())

	// Find existing condition
	for i, condition := range task.Status.Conditions {
		if condition.Type == conditionType {
			if condition.Status != status {
				task.Status.Conditions[i].Status = status
				task.Status.Conditions[i].LastTransitionTime = now
				task.Status.Conditions[i].Reason = reason
				task.Status.Conditions[i].Message = message
			}
			return
		}
	}

	// Add new condition
	task.Status.Conditions = append(task.Status.Conditions, mcpv1.TaskCondition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	})
}

// handleTaskCompletion handles task completion and decrements agent task counter
func (r *TaskReconciler) handleTaskCompletion(ctx context.Context, task *mcpv1.Task, logger logr.Logger) (ctrl.Result, error) {
	logger.Info("Handling task completion", "phase", task.Status.Phase, "agent", task.Status.AssignedAgent)

	// Decrement agent task count
	if err := r.decrementAgentTaskCount(ctx, task.Status.AssignedAgent, task.Namespace); err != nil {
		logger.Error(err, "Failed to decrement agent task count")
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	}

	// Mark that we've decremented the task count by adding an annotation
	if task.Annotations == nil {
		task.Annotations = make(map[string]string)
	}
	task.Annotations["mcp.io/task-count-decremented"] = "true"

	if err := r.Update(ctx, task); err != nil {
		logger.Error(err, "Failed to update task with decrement annotation")
		return ctrl.Result{RequeueAfter: time.Second * 5}, err
	}

	logger.Info("Task completion handled successfully", "agent", task.Status.AssignedAgent)
	return ctrl.Result{}, nil
}

// hasDecrementedTaskCount checks if the task counter has already been decremented
func (r *TaskReconciler) hasDecrementedTaskCount(task *mcpv1.Task) bool {
	if task.Annotations == nil {
		return false
	}
	return task.Annotations["mcp.io/task-count-decremented"] == "true"
}

// SetupWithManager sets up the controller with the Manager
func (r *TaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("task-controller")
	r.Scheduler = &TaskScheduler{Client: mgr.GetClient()}

	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1.Task{}).
		Complete(r)
}
