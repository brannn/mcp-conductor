package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AgentSpec defines the desired state of Agent
type AgentSpec struct {
	// Domain specifies the domain this agent belongs to (e.g., "data-processing", "infrastructure")
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Domain string `json:"domain"`

	// Capabilities is a list of capabilities this agent can handle
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Capabilities []string `json:"capabilities"`

	// Resources defines the resource allocation for this agent
	// +optional
	Resources AgentResources `json:"resources,omitempty"`

	// Tags are additional metadata tags for scheduling and filtering
	// +optional
	Tags []string `json:"tags,omitempty"`

	// MaxConcurrentTasks defines the maximum number of tasks this agent can handle simultaneously
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	MaxConcurrentTasks int32 `json:"maxConcurrentTasks,omitempty"`

	// HeartbeatInterval defines how often the agent should report its health status
	// +kubebuilder:default="30s"
	HeartbeatInterval metav1.Duration `json:"heartbeatInterval,omitempty"`
}

// AgentResources defines resource requirements and limits for an agent
type AgentResources struct {
	// CPU resource allocation (e.g., "1", "500m")
	// +optional
	CPU string `json:"cpu,omitempty"`

	// Memory resource allocation (e.g., "2Gi", "512Mi")
	// +optional
	Memory string `json:"memory,omitempty"`

	// GPU indicates if this agent has GPU access
	// +kubebuilder:default=false
	GPU bool `json:"gpu,omitempty"`

	// Storage resource allocation (e.g., "10Gi", "1Ti")
	// +optional
	Storage string `json:"storage,omitempty"`
}

// AgentStatus defines the observed state of Agent
type AgentStatus struct {
	// Phase represents the current operational status of the agent
	// +kubebuilder:validation:Enum=Ready;NotReady;Unknown;Terminating
	Phase AgentPhase `json:"phase,omitempty"`

	// LastHeartbeat is the timestamp of the last health check
	// +optional
	LastHeartbeat *metav1.Time `json:"lastHeartbeat,omitempty"`

	// CurrentTasks is the number of currently assigned tasks
	// +kubebuilder:default=0
	// +kubebuilder:validation:Minimum=0
	CurrentTasks int32 `json:"currentTasks,omitempty"`

	// CompletedTasks is the total number of completed tasks
	// +kubebuilder:default=0
	// +kubebuilder:validation:Minimum=0
	CompletedTasks int64 `json:"completedTasks,omitempty"`

	// FailedTasks is the total number of failed tasks
	// +kubebuilder:default=0
	// +kubebuilder:validation:Minimum=0
	FailedTasks int64 `json:"failedTasks,omitempty"`

	// Conditions represent the latest available observations of the agent's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []AgentCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// ObservedGeneration reflects the generation of the most recently observed Agent spec
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// AgentPhase represents the current phase of an agent
type AgentPhase string

const (
	// AgentPhaseReady indicates the agent is ready to accept tasks
	AgentPhaseReady AgentPhase = "Ready"
	// AgentPhaseNotReady indicates the agent is not ready to accept tasks
	AgentPhaseNotReady AgentPhase = "NotReady"
	// AgentPhaseUnknown indicates the agent status is unknown
	AgentPhaseUnknown AgentPhase = "Unknown"
	// AgentPhaseTerminating indicates the agent is being terminated
	AgentPhaseTerminating AgentPhase = "Terminating"
)

// AgentCondition describes the state of an agent at a certain point
type AgentCondition struct {
	// Type of agent condition
	Type AgentConditionType `json:"type"`

	// Status of the condition, one of True, False, Unknown
	Status metav1.ConditionStatus `json:"status"`

	// Last time the condition transitioned from one status to another
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// The reason for the condition's last transition
	// +optional
	Reason string `json:"reason,omitempty"`

	// A human readable message indicating details about the transition
	// +optional
	Message string `json:"message,omitempty"`
}

// AgentConditionType represents the type of condition for an agent
type AgentConditionType string

const (
	// AgentConditionReady indicates whether the agent is ready to accept tasks
	AgentConditionReady AgentConditionType = "Ready"
	// AgentConditionHealthy indicates whether the agent is healthy
	AgentConditionHealthy AgentConditionType = "Healthy"
	// AgentConditionCapable indicates whether the agent has registered its capabilities
	AgentConditionCapable AgentConditionType = "Capable"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=mcpagent
// +kubebuilder:printcolumn:name="Domain",type="string",JSONPath=".spec.domain"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Current Tasks",type="integer",JSONPath=".status.currentTasks"
// +kubebuilder:printcolumn:name="Completed",type="integer",JSONPath=".status.completedTasks"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Agent is the Schema for the agents API
type Agent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AgentSpec   `json:"spec,omitempty"`
	Status AgentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AgentList contains a list of Agent
type AgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Agent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Agent{}, &AgentList{})
}
