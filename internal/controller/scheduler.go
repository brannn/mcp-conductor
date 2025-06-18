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
	"sort"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mcpv1 "github.com/mcp-conductor/mcp-conductor/api/v1"
)

// TaskScheduler handles task-to-agent assignment logic
type TaskScheduler struct {
	Client client.Client
}

// SchedulingResult represents the result of a scheduling attempt
type SchedulingResult struct {
	Agent  *mcpv1.Agent
	Score  int
	Reason string
}

// FindSuitableAgent finds the best agent for a given task
func (s *TaskScheduler) FindSuitableAgent(ctx context.Context, task *mcpv1.Task) (*mcpv1.Agent, error) {
	logger := log.FromContext(ctx).WithValues("task", task.Name, "domain", task.Spec.Domain)

	// List all agents in the same domain
	agents := &mcpv1.AgentList{}
	listOpts := []client.ListOption{
		client.InNamespace(task.Namespace),
	}

	if err := s.Client.List(ctx, agents, listOpts...); err != nil {
		logger.Error(err, "Failed to list agents")
		return nil, err
	}

	// Filter and score agents
	candidates := []SchedulingResult{}
	for _, agent := range agents.Items {
		result := s.scoreAgent(task, &agent)
		if result.Score > 0 {
			candidates = append(candidates, result)
		}
	}

	if len(candidates) == 0 {
		logger.Info("No suitable agents found for task")
		return nil, nil
	}

	// Sort by score (highest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	bestCandidate := candidates[0]
	logger.Info("Selected agent for task",
		"agent", bestCandidate.Agent.Name,
		"score", bestCandidate.Score,
		"reason", bestCandidate.Reason)

	return bestCandidate.Agent, nil
}

// scoreAgent scores an agent's suitability for a task
func (s *TaskScheduler) scoreAgent(task *mcpv1.Task, agent *mcpv1.Agent) SchedulingResult {
	score := 0
	reasons := []string{}

	// Domain matching (required)
	if agent.Spec.Domain != task.Spec.Domain {
		return SchedulingResult{
			Agent:  agent,
			Score:  0,
			Reason: "domain mismatch",
		}
	}
	score += 100
	reasons = append(reasons, "domain match")

	// Agent must be ready
	if agent.Status.Phase != mcpv1.AgentPhaseReady {
		return SchedulingResult{
			Agent:  agent,
			Score:  0,
			Reason: "agent not ready",
		}
	}
	score += 50
	reasons = append(reasons, "agent ready")

	// Capability matching (required)
	if !s.hasRequiredCapabilities(agent, task.Spec.RequiredCapabilities) {
		return SchedulingResult{
			Agent:  agent,
			Score:  0,
			Reason: "missing required capabilities",
		}
	}
	score += 200
	reasons = append(reasons, "capabilities match")

	// Required tags matching (required)
	if !s.hasRequiredTags(agent, task.Spec.RequiredTags) {
		return SchedulingResult{
			Agent:  agent,
			Score:  0,
			Reason: "missing required tags",
		}
	}
	score += 50
	reasons = append(reasons, "required tags match")

	// Preferred tags matching (bonus)
	preferredTagMatches := s.countPreferredTagMatches(agent, task.Spec.PreferredTags)
	score += preferredTagMatches * 10
	if preferredTagMatches > 0 {
		reasons = append(reasons, "preferred tags match")
	}

	// Load balancing - prefer agents with fewer current tasks
	maxTasks := agent.Spec.MaxConcurrentTasks
	currentTasks := agent.Status.CurrentTasks

	if currentTasks >= maxTasks {
		return SchedulingResult{
			Agent:  agent,
			Score:  0,
			Reason: "agent at capacity",
		}
	}

	// Higher score for agents with more available capacity
	availableCapacity := maxTasks - currentTasks
	capacityScore := int(availableCapacity) * 5
	score += capacityScore
	reasons = append(reasons, "available capacity")

	// Priority bonus - higher priority tasks get slight preference for better agents
	if task.Spec.Priority > 5 {
		score += int(task.Spec.Priority-5) * 2
		reasons = append(reasons, "high priority task")
	}

	return SchedulingResult{
		Agent:  agent,
		Score:  score,
		Reason: joinReasons(reasons),
	}
}

// hasRequiredCapabilities checks if agent has all required capabilities
func (s *TaskScheduler) hasRequiredCapabilities(agent *mcpv1.Agent, requiredCapabilities []string) bool {
	agentCapabilities := make(map[string]bool)
	for _, capability := range agent.Spec.Capabilities {
		agentCapabilities[capability] = true
	}

	for _, required := range requiredCapabilities {
		if !agentCapabilities[required] {
			return false
		}
	}

	return true
}

// hasRequiredTags checks if agent has all required tags
func (s *TaskScheduler) hasRequiredTags(agent *mcpv1.Agent, requiredTags []string) bool {
	if len(requiredTags) == 0 {
		return true
	}

	agentTags := make(map[string]bool)
	for _, tag := range agent.Spec.Tags {
		agentTags[tag] = true
	}

	for _, required := range requiredTags {
		if !agentTags[required] {
			return false
		}
	}

	return true
}

// countPreferredTagMatches counts how many preferred tags the agent has
func (s *TaskScheduler) countPreferredTagMatches(agent *mcpv1.Agent, preferredTags []string) int {
	if len(preferredTags) == 0 {
		return 0
	}

	agentTags := make(map[string]bool)
	for _, tag := range agent.Spec.Tags {
		agentTags[tag] = true
	}

	matches := 0
	for _, preferred := range preferredTags {
		if agentTags[preferred] {
			matches++
		}
	}

	return matches
}

// joinReasons joins multiple reasons into a single string
func joinReasons(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	if len(reasons) == 1 {
		return reasons[0]
	}

	result := reasons[0]
	for i := 1; i < len(reasons); i++ {
		result += ", " + reasons[i]
	}
	return result
}

// GetAgentWorkload returns the current workload information for an agent
func (s *TaskScheduler) GetAgentWorkload(ctx context.Context, agent *mcpv1.Agent) (int32, int32, error) {
	// In a real implementation, we might query running tasks
	// For now, we use the status information
	return agent.Status.CurrentTasks, agent.Spec.MaxConcurrentTasks, nil
}

// CanScheduleTask checks if a task can be scheduled (has available agents)
func (s *TaskScheduler) CanScheduleTask(ctx context.Context, task *mcpv1.Task) (bool, error) {
	agent, err := s.FindSuitableAgent(ctx, task)
	if err != nil {
		return false, err
	}
	return agent != nil, nil
}
