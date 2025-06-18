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
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcpv1 "github.com/mcp-conductor/mcp-conductor/api/v1"
	"github.com/mcp-conductor/mcp-conductor/internal/agent"
)

// K8sReporter provides Kubernetes resource reporting capabilities
type K8sReporter struct {
	Client client.Client
	Logger logr.Logger
}

// NewK8sReporter creates a new Kubernetes resource reporter
func NewK8sReporter(client client.Client, logger logr.Logger) *K8sReporter {
	return &K8sReporter{
		Client: client,
		Logger: logger.WithName("k8s-reporter"),
	}
}

// GetCapabilities returns the capabilities this agent provides
func (k *K8sReporter) GetCapabilities() []string {
	return []string{
		"k8s-list-pods",
		"k8s-list-deployments",
		"k8s-list-daemonsets",
		"k8s-list-services",
		"k8s-list-nodes",
		"k8s-list-namespaces",
		"k8s-get-pod",
		"k8s-get-deployment",
		"k8s-get-node-status",
		"k8s-cluster-summary",
	}
}

// GetTags returns the tags for this agent
func (k *K8sReporter) GetTags() []string {
	return []string{
		"kubernetes",
		"infrastructure",
		"reporting",
		"cluster-info",
	}
}

// ExecuteTask executes a Kubernetes reporting task
func (k *K8sReporter) ExecuteTask(ctx context.Context, task *mcpv1.Task) (*agent.TaskResult, error) {
	k.Logger.Info("Executing K8s reporting task",
		"task", task.Name,
		"capabilities", task.Spec.RequiredCapabilities)

	// Parse task payload
	payload, err := k.parseTaskPayload(task)
	if err != nil {
		return &agent.TaskResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to parse task payload: %v", err),
		}, nil
	}

	// Determine which capability to execute
	capability := k.determineCapability(task.Spec.RequiredCapabilities)
	if capability == "" {
		return &agent.TaskResult{
			Success: false,
			Error:   "No supported capability found in task requirements",
		}, nil
	}

	// Execute the appropriate capability
	result, err := k.executeCapability(ctx, capability, payload)
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
func (k *K8sReporter) parseTaskPayload(task *mcpv1.Task) (map[string]interface{}, error) {
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
func (k *K8sReporter) determineCapability(requiredCapabilities []string) string {
	supportedCapabilities := make(map[string]bool)
	for _, cap := range k.GetCapabilities() {
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
func (k *K8sReporter) executeCapability(ctx context.Context, capability string, payload map[string]interface{}) (map[string]interface{}, error) {
	switch capability {
	case "k8s-list-pods":
		return k.listPods(ctx, payload)
	case "k8s-list-deployments":
		return k.listDeployments(ctx, payload)
	case "k8s-list-daemonsets":
		return k.listDaemonSets(ctx, payload)
	case "k8s-list-services":
		return k.listServices(ctx, payload)
	case "k8s-list-nodes":
		return k.listNodes(ctx, payload)
	case "k8s-list-namespaces":
		return k.listNamespaces(ctx, payload)
	case "k8s-get-pod":
		return k.getPod(ctx, payload)
	case "k8s-get-deployment":
		return k.getDeployment(ctx, payload)
	case "k8s-get-node-status":
		return k.getNodeStatus(ctx, payload)
	case "k8s-cluster-summary":
		return k.getClusterSummary(ctx, payload)
	default:
		return nil, fmt.Errorf("unsupported capability: %s", capability)
	}
}

// listPods lists pods in a namespace
func (k *K8sReporter) listPods(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	namespace := k.getStringParam(payload, "namespace", "")
	labelSelector := k.getStringParam(payload, "labelSelector", "")

	pods := &corev1.PodList{}
	listOpts := []client.ListOption{}

	if namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}

	if labelSelector != "" {
		selector, err := labels.Parse(labelSelector)
		if err != nil {
			return nil, fmt.Errorf("invalid label selector: %w", err)
		}
		listOpts = append(listOpts, client.MatchingLabelsSelector{Selector: selector})
	}

	if err := k.Client.List(ctx, pods, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	// Convert to simplified format
	podSummaries := make([]map[string]interface{}, 0, len(pods.Items))
	for _, pod := range pods.Items {
		podSummaries = append(podSummaries, map[string]interface{}{
			"name":      pod.Name,
			"namespace": pod.Namespace,
			"phase":     string(pod.Status.Phase),
			"node":      pod.Spec.NodeName,
			"ready":     k.isPodReady(&pod),
			"restarts":  k.getPodRestartCount(&pod),
			"age":       pod.CreationTimestamp.Time,
		})
	}

	return map[string]interface{}{
		"pods":  podSummaries,
		"count": len(podSummaries),
	}, nil
}

// listDeployments lists deployments in a namespace
func (k *K8sReporter) listDeployments(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	namespace := k.getStringParam(payload, "namespace", "")

	deployments := &appsv1.DeploymentList{}
	listOpts := []client.ListOption{}

	if namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}

	if err := k.Client.List(ctx, deployments, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	// Convert to simplified format
	deploymentSummaries := make([]map[string]interface{}, 0, len(deployments.Items))
	for _, deployment := range deployments.Items {
		deploymentSummaries = append(deploymentSummaries, map[string]interface{}{
			"name":      deployment.Name,
			"namespace": deployment.Namespace,
			"replicas":  deployment.Status.Replicas,
			"ready":     deployment.Status.ReadyReplicas,
			"available": deployment.Status.AvailableReplicas,
			"age":       deployment.CreationTimestamp.Time,
		})
	}

	return map[string]interface{}{
		"deployments": deploymentSummaries,
		"count":       len(deploymentSummaries),
	}, nil
}

// listDaemonSets lists daemonsets in a namespace
func (k *K8sReporter) listDaemonSets(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	namespace := k.getStringParam(payload, "namespace", "")

	daemonsets := &appsv1.DaemonSetList{}
	listOpts := []client.ListOption{}

	if namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}

	if err := k.Client.List(ctx, daemonsets, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list daemonsets: %w", err)
	}

	// Convert to simplified format
	daemonsetSummaries := make([]map[string]interface{}, 0, len(daemonsets.Items))
	for _, ds := range daemonsets.Items {
		daemonsetSummaries = append(daemonsetSummaries, map[string]interface{}{
			"name":      ds.Name,
			"namespace": ds.Namespace,
			"desired":   ds.Status.DesiredNumberScheduled,
			"current":   ds.Status.CurrentNumberScheduled,
			"ready":     ds.Status.NumberReady,
			"available": ds.Status.NumberAvailable,
			"age":       ds.CreationTimestamp.Time,
		})
	}

	return map[string]interface{}{
		"daemonsets": daemonsetSummaries,
		"count":      len(daemonsetSummaries),
	}, nil
}

// listServices lists services in a namespace
func (k *K8sReporter) listServices(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	namespace := k.getStringParam(payload, "namespace", "")

	services := &corev1.ServiceList{}
	listOpts := []client.ListOption{}

	if namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}

	if err := k.Client.List(ctx, services, listOpts...); err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// Convert to simplified format
	serviceSummaries := make([]map[string]interface{}, 0, len(services.Items))
	for _, service := range services.Items {
		serviceSummaries = append(serviceSummaries, map[string]interface{}{
			"name":       service.Name,
			"namespace":  service.Namespace,
			"type":       string(service.Spec.Type),
			"clusterIP":  service.Spec.ClusterIP,
			"externalIP": service.Status.LoadBalancer.Ingress,
			"ports":      service.Spec.Ports,
			"age":        service.CreationTimestamp.Time,
		})
	}

	return map[string]interface{}{
		"services": serviceSummaries,
		"count":    len(serviceSummaries),
	}, nil
}

// listNodes lists cluster nodes
func (k *K8sReporter) listNodes(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	nodes := &corev1.NodeList{}

	if err := k.Client.List(ctx, nodes); err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	// Convert to simplified format
	nodeSummaries := make([]map[string]interface{}, 0, len(nodes.Items))
	for _, node := range nodes.Items {
		nodeSummaries = append(nodeSummaries, map[string]interface{}{
			"name":    node.Name,
			"ready":   k.isNodeReady(&node),
			"roles":   k.getNodeRoles(&node),
			"version": node.Status.NodeInfo.KubeletVersion,
			"os":      node.Status.NodeInfo.OSImage,
			"age":     node.CreationTimestamp.Time,
		})
	}

	return map[string]interface{}{
		"nodes": nodeSummaries,
		"count": len(nodeSummaries),
	}, nil
}

// listNamespaces lists all namespaces in the cluster
func (k *K8sReporter) listNamespaces(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	namespaces := &corev1.NamespaceList{}

	if err := k.Client.List(ctx, namespaces); err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	// Convert to simplified format
	namespaceSummaries := make([]map[string]interface{}, 0, len(namespaces.Items))
	for _, ns := range namespaces.Items {
		namespaceSummaries = append(namespaceSummaries, map[string]interface{}{
			"name":   ns.Name,
			"status": string(ns.Status.Phase),
			"age":    ns.CreationTimestamp.Time,
		})
	}

	return map[string]interface{}{
		"namespaces": namespaceSummaries,
		"count":      len(namespaceSummaries),
	}, nil
}

// getPod gets detailed information about a specific pod
func (k *K8sReporter) getPod(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	name := k.getStringParam(payload, "name", "")
	namespace := k.getStringParam(payload, "namespace", "default")

	if name == "" {
		return nil, fmt.Errorf("pod name is required")
	}

	pod := &corev1.Pod{}
	key := client.ObjectKey{Name: name, Namespace: namespace}

	if err := k.Client.Get(ctx, key, pod); err != nil {
		return nil, fmt.Errorf("failed to get pod: %w", err)
	}

	return map[string]interface{}{
		"name":       pod.Name,
		"namespace":  pod.Namespace,
		"phase":      string(pod.Status.Phase),
		"node":       pod.Spec.NodeName,
		"ready":      k.isPodReady(pod),
		"restarts":   k.getPodRestartCount(pod),
		"containers": k.getPodContainerInfo(pod),
		"conditions": pod.Status.Conditions,
		"age":        pod.CreationTimestamp.Time,
	}, nil
}

// getDeployment gets detailed information about a specific deployment
func (k *K8sReporter) getDeployment(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	name := k.getStringParam(payload, "name", "")
	namespace := k.getStringParam(payload, "namespace", "default")

	if name == "" {
		return nil, fmt.Errorf("deployment name is required")
	}

	deployment := &appsv1.Deployment{}
	key := client.ObjectKey{Name: name, Namespace: namespace}

	if err := k.Client.Get(ctx, key, deployment); err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	return map[string]interface{}{
		"name":        deployment.Name,
		"namespace":   deployment.Namespace,
		"replicas":    deployment.Status.Replicas,
		"ready":       deployment.Status.ReadyReplicas,
		"available":   deployment.Status.AvailableReplicas,
		"unavailable": deployment.Status.UnavailableReplicas,
		"conditions":  deployment.Status.Conditions,
		"strategy":    deployment.Spec.Strategy.Type,
		"age":         deployment.CreationTimestamp.Time,
	}, nil
}

// getNodeStatus gets detailed status of a specific node
func (k *K8sReporter) getNodeStatus(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	name := k.getStringParam(payload, "name", "")

	if name == "" {
		return nil, fmt.Errorf("node name is required")
	}

	node := &corev1.Node{}
	key := client.ObjectKey{Name: name}

	if err := k.Client.Get(ctx, key, node); err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	return map[string]interface{}{
		"name":        node.Name,
		"ready":       k.isNodeReady(node),
		"roles":       k.getNodeRoles(node),
		"version":     node.Status.NodeInfo.KubeletVersion,
		"os":          node.Status.NodeInfo.OSImage,
		"capacity":    node.Status.Capacity,
		"allocatable": node.Status.Allocatable,
		"conditions":  node.Status.Conditions,
		"addresses":   node.Status.Addresses,
		"age":         node.CreationTimestamp.Time,
	}, nil
}

// getClusterSummary provides a high-level cluster overview
func (k *K8sReporter) getClusterSummary(ctx context.Context, payload map[string]interface{}) (map[string]interface{}, error) {
	summary := map[string]interface{}{}

	// Count nodes
	nodes := &corev1.NodeList{}
	if err := k.Client.List(ctx, nodes); err == nil {
		readyNodes := 0
		for _, node := range nodes.Items {
			if k.isNodeReady(&node) {
				readyNodes++
			}
		}
		summary["nodes"] = map[string]interface{}{
			"total": len(nodes.Items),
			"ready": readyNodes,
		}
	}

	// Count pods across all namespaces
	pods := &corev1.PodList{}
	if err := k.Client.List(ctx, pods); err == nil {
		runningPods := 0
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				runningPods++
			}
		}
		summary["pods"] = map[string]interface{}{
			"total":   len(pods.Items),
			"running": runningPods,
		}
	}

	// Count deployments
	deployments := &appsv1.DeploymentList{}
	if err := k.Client.List(ctx, deployments); err == nil {
		summary["deployments"] = len(deployments.Items)
	}

	// Count services
	services := &corev1.ServiceList{}
	if err := k.Client.List(ctx, services); err == nil {
		summary["services"] = len(services.Items)
	}

	return summary, nil
}

// Helper methods

func (k *K8sReporter) getStringParam(payload map[string]interface{}, key, defaultValue string) string {
	if value, ok := payload[key]; ok {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return defaultValue
}

func (k *K8sReporter) isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (k *K8sReporter) getPodRestartCount(pod *corev1.Pod) int32 {
	var restarts int32
	for _, containerStatus := range pod.Status.ContainerStatuses {
		restarts += containerStatus.RestartCount
	}
	return restarts
}

func (k *K8sReporter) getPodContainerInfo(pod *corev1.Pod) []map[string]interface{} {
	containers := make([]map[string]interface{}, 0, len(pod.Spec.Containers))
	for i, container := range pod.Spec.Containers {
		containerInfo := map[string]interface{}{
			"name":  container.Name,
			"image": container.Image,
		}

		// Add status if available
		if i < len(pod.Status.ContainerStatuses) {
			status := pod.Status.ContainerStatuses[i]
			containerInfo["ready"] = status.Ready
			containerInfo["restartCount"] = status.RestartCount
			containerInfo["state"] = status.State
		}

		containers = append(containers, containerInfo)
	}
	return containers
}

func (k *K8sReporter) isNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func (k *K8sReporter) getNodeRoles(node *corev1.Node) []string {
	roles := []string{}
	for label := range node.Labels {
		if strings.HasPrefix(label, "node-role.kubernetes.io/") {
			role := strings.TrimPrefix(label, "node-role.kubernetes.io/")
			if role != "" {
				roles = append(roles, role)
			}
		}
	}
	if len(roles) == 0 {
		roles = append(roles, "worker")
	}
	return roles
}
