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
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	mcpv1 "github.com/mcp-conductor/mcp-conductor/api/v1"
)

// K8sClient wraps Kubernetes client functionality
type K8sClient struct {
	Client    client.Client
	Clientset kubernetes.Interface
	Config    *rest.Config
	Logger    logr.Logger
}

// NewK8sClient creates a new Kubernetes client
func NewK8sClient(logger logr.Logger) (*K8sClient, error) {
	config, err := GetKubernetesConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes config: %w", err)
	}

	// Create controller-runtime client
	scheme := runtime.NewScheme()
	if err := mcpv1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add MCP scheme: %w", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add core scheme: %w", err)
	}

	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Create clientset for additional functionality
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	return &K8sClient{
		Client:    k8sClient,
		Clientset: clientset,
		Config:    config,
		Logger:    logger,
	}, nil
}

// GetKubernetesConfig returns the Kubernetes configuration
func GetKubernetesConfig() (*rest.Config, error) {
	// Try in-cluster config first
	config, err := rest.InClusterConfig()
	if err == nil {
		return config, nil
	}

	// Fall back to kubeconfig
	kubeconfig := clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
	}

	return config, nil
}

// CreateEvent creates a Kubernetes event
func (k *K8sClient) CreateEvent(ctx context.Context, obj client.Object, eventType, reason, message string) error {
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s.%x", obj.GetName(), time.Now().UnixNano()),
			Namespace: obj.GetNamespace(),
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
			APIVersion: obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
			UID:        obj.GetUID(),
		},
		Reason:  reason,
		Message: message,
		Type:    eventType,
		Source: corev1.EventSource{
			Component: "mcp-conductor",
		},
		FirstTimestamp: metav1.NewTime(time.Now()),
		LastTimestamp:  metav1.NewTime(time.Now()),
		Count:          1,
	}

	return k.Client.Create(ctx, event)
}

// UpdateAgentStatus updates an agent's status
func (k *K8sClient) UpdateAgentStatus(ctx context.Context, agent *mcpv1.Agent) error {
	return k.Client.Status().Update(ctx, agent)
}

// UpdateTaskStatus updates a task's status
func (k *K8sClient) UpdateTaskStatus(ctx context.Context, task *mcpv1.Task) error {
	return k.Client.Status().Update(ctx, task)
}

// ListAgents lists agents with optional domain filtering
func (k *K8sClient) ListAgents(ctx context.Context, namespace, domain string) (*mcpv1.AgentList, error) {
	agents := &mcpv1.AgentList{}
	opts := []client.ListOption{}

	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}

	if domain != "" {
		opts = append(opts, client.MatchingFields{"spec.domain": domain})
	}

	err := k.Client.List(ctx, agents, opts...)
	return agents, err
}

// ListTasks lists tasks with optional domain and status filtering
func (k *K8sClient) ListTasks(ctx context.Context, namespace, domain string, phase mcpv1.TaskPhase) (*mcpv1.TaskList, error) {
	tasks := &mcpv1.TaskList{}
	opts := []client.ListOption{}

	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}

	if domain != "" {
		opts = append(opts, client.MatchingFields{"spec.domain": domain})
	}

	if phase != "" {
		opts = append(opts, client.MatchingFields{"status.phase": string(phase)})
	}

	err := k.Client.List(ctx, tasks, opts...)
	return tasks, err
}

// GetAgent retrieves an agent by name and namespace
func (k *K8sClient) GetAgent(ctx context.Context, name, namespace string) (*mcpv1.Agent, error) {
	agent := &mcpv1.Agent{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	err := k.Client.Get(ctx, key, agent)
	return agent, err
}

// GetTask retrieves a task by name and namespace
func (k *K8sClient) GetTask(ctx context.Context, name, namespace string) (*mcpv1.Task, error) {
	task := &mcpv1.Task{}
	key := client.ObjectKey{Name: name, Namespace: namespace}
	err := k.Client.Get(ctx, key, task)
	return task, err
}

// CreateOrUpdateAgent creates or updates an agent
func (k *K8sClient) CreateOrUpdateAgent(ctx context.Context, agent *mcpv1.Agent) error {
	existing := &mcpv1.Agent{}
	key := client.ObjectKey{Name: agent.Name, Namespace: agent.Namespace}

	err := k.Client.Get(ctx, key, existing)
	if err != nil {
		if client.IgnoreNotFound(err) == nil {
			// Agent doesn't exist, create it
			return k.Client.Create(ctx, agent)
		}
		return err
	}

	// Agent exists, update it
	agent.ResourceVersion = existing.ResourceVersion
	return k.Client.Update(ctx, agent)
}

// DeleteAgent deletes an agent
func (k *K8sClient) DeleteAgent(ctx context.Context, name, namespace string) error {
	agent := &mcpv1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	return k.Client.Delete(ctx, agent)
}

// WatchTasks creates a watch for tasks assigned to a specific agent
func (k *K8sClient) WatchTasks(ctx context.Context, agentName, namespace string) (client.WithWatch, error) {
	watchClient, ok := k.Client.(client.WithWatch)
	if !ok {
		return nil, fmt.Errorf("client does not support watching")
	}
	return watchClient, nil
}

// SetupManagerWithClient sets up a controller manager with the K8s client
func (k *K8sClient) SetupManagerWithClient(mgr manager.Manager) error {
	// The manager already has its own client, but we can add additional setup here
	return nil
}

// IsNamespaced returns true if the client is configured for a specific namespace
func (k *K8sClient) IsNamespaced() bool {
	// This would be determined by the configuration
	return false
}

// GetNamespace returns the namespace the client is configured for
func (k *K8sClient) GetNamespace() string {
	// This would be determined by the configuration
	return ""
}
