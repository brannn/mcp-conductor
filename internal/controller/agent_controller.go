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

// AgentReconciler reconciles an Agent object
type AgentReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Config   *common.ControllerConfig
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=mcp.io,resources=agents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mcp.io,resources=agents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mcp.io,resources=agents/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile handles Agent resource changes
func (r *AgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("agent", req.NamespacedName)

	// Fetch the Agent instance
	agent := &mcpv1.Agent{}
	if err := r.Get(ctx, req.NamespacedName, agent); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Agent resource not found, likely deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Agent")
		return ctrl.Result{}, err
	}

	// Check if this controller should handle this agent (domain filtering)
	if r.Config.Domain != "" && agent.Spec.Domain != r.Config.Domain {
		logger.V(1).Info("Skipping agent from different domain",
			"agentDomain", agent.Spec.Domain,
			"controllerDomain", r.Config.Domain)
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if agent.DeletionTimestamp != nil {
		return r.handleAgentDeletion(ctx, agent, logger)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(agent, "mcp.io/agent-finalizer") {
		controllerutil.AddFinalizer(agent, "mcp.io/agent-finalizer")
		if err := r.Update(ctx, agent); err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Initialize status if needed
	if agent.Status.Phase == "" {
		agent.Status.Phase = mcpv1.AgentPhaseNotReady
		agent.Status.CurrentTasks = 0
		agent.Status.CompletedTasks = 0
		agent.Status.FailedTasks = 0
		agent.Status.ObservedGeneration = agent.Generation

		r.setAgentCondition(agent, mcpv1.AgentConditionReady, metav1.ConditionFalse,
			"Initializing", "Agent is being initialized")

		if err := r.Status().Update(ctx, agent); err != nil {
			logger.Error(err, "Failed to initialize agent status")
			return ctrl.Result{}, err
		}

		r.Recorder.Event(agent, "Normal", "Initializing", "Agent is being initialized")
		return ctrl.Result{RequeueAfter: time.Second * 5}, nil
	}

	// Check agent health based on heartbeat
	now := metav1.NewTime(time.Now())
	heartbeatTimeout := time.Duration(agent.Spec.HeartbeatInterval.Duration) * 2

	if agent.Status.LastHeartbeat != nil {
		timeSinceHeartbeat := now.Time.Sub(agent.Status.LastHeartbeat.Time)
		if timeSinceHeartbeat > heartbeatTimeout {
			// Agent is unhealthy
			if agent.Status.Phase != mcpv1.AgentPhaseNotReady {
				logger.Info("Agent heartbeat timeout, marking as not ready",
					"lastHeartbeat", agent.Status.LastHeartbeat,
					"timeout", heartbeatTimeout)

				agent.Status.Phase = mcpv1.AgentPhaseNotReady
				r.setAgentCondition(agent, mcpv1.AgentConditionHealthy, metav1.ConditionFalse,
					"HeartbeatTimeout", fmt.Sprintf("No heartbeat for %v", timeSinceHeartbeat))

				if err := r.Status().Update(ctx, agent); err != nil {
					logger.Error(err, "Failed to update agent health status")
					return ctrl.Result{}, err
				}

				r.Recorder.Event(agent, "Warning", "Unhealthy", "Agent heartbeat timeout")
			}
		} else {
			// Agent is healthy
			if agent.Status.Phase != mcpv1.AgentPhaseReady {
				logger.Info("Agent is healthy, marking as ready")

				agent.Status.Phase = mcpv1.AgentPhaseReady
				r.setAgentCondition(agent, mcpv1.AgentConditionHealthy, metav1.ConditionTrue,
					"Healthy", "Agent is responding to heartbeats")
				r.setAgentCondition(agent, mcpv1.AgentConditionReady, metav1.ConditionTrue,
					"Ready", "Agent is ready to accept tasks")

				if err := r.Status().Update(ctx, agent); err != nil {
					logger.Error(err, "Failed to update agent ready status")
					return ctrl.Result{}, err
				}

				r.Recorder.Event(agent, "Normal", "Ready", "Agent is ready to accept tasks")
			}
		}
	}

	// Update observed generation
	if agent.Status.ObservedGeneration != agent.Generation {
		agent.Status.ObservedGeneration = agent.Generation
		if err := r.Status().Update(ctx, agent); err != nil {
			logger.Error(err, "Failed to update observed generation")
			return ctrl.Result{}, err
		}
	}

	// Requeue for next health check
	nextCheck := time.Duration(agent.Spec.HeartbeatInterval.Duration)
	logger.V(1).Info("Agent reconciliation complete", "nextCheck", nextCheck)

	return ctrl.Result{RequeueAfter: nextCheck}, nil
}

// handleAgentDeletion handles the deletion of an agent
func (r *AgentReconciler) handleAgentDeletion(ctx context.Context, agent *mcpv1.Agent, logger logr.Logger) (ctrl.Result, error) {
	logger.Info("Handling agent deletion")

	// Cancel any running tasks assigned to this agent
	if err := r.cancelAgentTasks(ctx, agent, logger); err != nil {
		logger.Error(err, "Failed to cancel agent tasks")
		return ctrl.Result{RequeueAfter: time.Second * 30}, err
	}

	// Remove the finalizer to allow deletion
	controllerutil.RemoveFinalizer(agent, "mcp.io/agent-finalizer")
	if err := r.Update(ctx, agent); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	r.Recorder.Event(agent, "Normal", "Deleted", "Agent has been deleted")
	logger.Info("Agent deletion complete")

	return ctrl.Result{}, nil
}

// setAgentCondition sets or updates an agent condition
func (r *AgentReconciler) setAgentCondition(agent *mcpv1.Agent, conditionType mcpv1.AgentConditionType, status metav1.ConditionStatus, reason, message string) {
	now := metav1.NewTime(time.Now())

	// Find existing condition
	for i, condition := range agent.Status.Conditions {
		if condition.Type == conditionType {
			if condition.Status != status {
				agent.Status.Conditions[i].Status = status
				agent.Status.Conditions[i].LastTransitionTime = now
				agent.Status.Conditions[i].Reason = reason
				agent.Status.Conditions[i].Message = message
			}
			return
		}
	}

	// Add new condition
	agent.Status.Conditions = append(agent.Status.Conditions, mcpv1.AgentCondition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	})
}

// cancelAgentTasks cancels all running tasks assigned to the agent
func (r *AgentReconciler) cancelAgentTasks(ctx context.Context, agent *mcpv1.Agent, logger logr.Logger) error {
	// List all tasks assigned to this agent
	taskList := &mcpv1.TaskList{}
	listOpts := []client.ListOption{
		client.InNamespace(agent.Namespace),
		client.MatchingFields{"status.assignedAgent": agent.Name},
	}

	if err := r.List(ctx, taskList, listOpts...); err != nil {
		return fmt.Errorf("failed to list agent tasks: %w", err)
	}

	// Cancel running tasks
	for _, task := range taskList.Items {
		if task.Status.Phase == mcpv1.TaskPhaseRunning || task.Status.Phase == mcpv1.TaskPhasePending {
			logger.Info("Cancelling task due to agent deletion", "task", task.Name)

			task.Status.Phase = mcpv1.TaskPhaseCancelled
			task.Status.Error = "Agent was deleted"
			task.Status.AssignedAgent = ""

			if err := r.Status().Update(ctx, &task); err != nil {
				logger.Error(err, "Failed to cancel task", "task", task.Name)
				continue
			}

			r.Recorder.Event(&task, "Warning", "Cancelled", "Task cancelled due to agent deletion")
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *AgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("agent-controller")

	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1.Agent{}).
		Complete(r)
}
