package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TaskSpec defines the desired state of Task
type TaskSpec struct {
	// Domain specifies the domain this task belongs to
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Domain string `json:"domain"`

	// RequiredCapabilities is a list of capabilities required to execute this task
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	RequiredCapabilities []string `json:"requiredCapabilities"`

	// Priority defines the task priority (1-10, higher is more urgent)
	// +kubebuilder:default=5
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	Priority int32 `json:"priority,omitempty"`

	// Payload contains the task-specific data and parameters
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Payload *runtime.RawExtension `json:"payload,omitempty"`

	// Deadline specifies when the task must be completed
	// +optional
	Deadline *metav1.Time `json:"deadline,omitempty"`

	// RetryPolicy defines how the task should be retried on failure
	// +optional
	RetryPolicy *TaskRetryPolicy `json:"retryPolicy,omitempty"`

	// Timeout specifies the maximum duration for task execution
	// +kubebuilder:default="300s"
	Timeout metav1.Duration `json:"timeout,omitempty"`

	// RequiredTags specifies agent tags that must be present for task assignment
	// +optional
	RequiredTags []string `json:"requiredTags,omitempty"`

	// PreferredTags specifies agent tags that are preferred but not required
	// +optional
	PreferredTags []string `json:"preferredTags,omitempty"`
}

// TaskRetryPolicy defines the retry behavior for failed tasks
type TaskRetryPolicy struct {
	// MaxRetries is the maximum number of retry attempts
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	MaxRetries int32 `json:"maxRetries,omitempty"`

	// BackoffStrategy defines the retry backoff strategy
	// +kubebuilder:default="exponential"
	// +kubebuilder:validation:Enum=linear;exponential;fixed
	BackoffStrategy BackoffStrategy `json:"backoffStrategy,omitempty"`

	// InitialDelay is the initial delay before the first retry
	// +kubebuilder:default="1s"
	InitialDelay metav1.Duration `json:"initialDelay,omitempty"`

	// MaxDelay is the maximum delay between retries
	// +kubebuilder:default="300s"
	MaxDelay metav1.Duration `json:"maxDelay,omitempty"`
}

// BackoffStrategy represents the retry backoff strategy
type BackoffStrategy string

const (
	// BackoffStrategyLinear increases delay linearly
	BackoffStrategyLinear BackoffStrategy = "linear"
	// BackoffStrategyExponential increases delay exponentially
	BackoffStrategyExponential BackoffStrategy = "exponential"
	// BackoffStrategyFixed uses a fixed delay
	BackoffStrategyFixed BackoffStrategy = "fixed"
)

// TaskStatus defines the observed state of Task
type TaskStatus struct {
	// Phase represents the current execution phase of the task
	// +kubebuilder:validation:Enum=Pending;Assigned;Running;WaitingForElicitation;Completed;Failed;Cancelled
	Phase TaskPhase `json:"phase,omitempty"`

	// AssignedAgent is the name of the agent assigned to this task
	// +optional
	AssignedAgent string `json:"assignedAgent,omitempty"`

	// StartTime is when the task execution started
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the task execution completed
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// Result contains the task execution results
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Result *runtime.RawExtension `json:"result,omitempty"`

	// Error contains error information if the task failed
	// +optional
	Error string `json:"error,omitempty"`

	// RetryCount is the number of times this task has been retried
	// +kubebuilder:default=0
	// +kubebuilder:validation:Minimum=0
	RetryCount int32 `json:"retryCount,omitempty"`

	// ElicitationRequest contains information about a pending elicitation request
	// +optional
	ElicitationRequest *TaskElicitationRequest `json:"elicitationRequest,omitempty"`

	// ElicitationResponse contains the response to an elicitation request
	// +optional
	ElicitationResponse *TaskElicitationResponse `json:"elicitationResponse,omitempty"`

	// Conditions represent the latest available observations of the task's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []TaskCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// ObservedGeneration reflects the generation of the most recently observed Task spec
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// TaskPhase represents the current phase of a task
type TaskPhase string

const (
	// TaskPhasePending indicates the task is waiting to be assigned
	TaskPhasePending TaskPhase = "Pending"
	// TaskPhaseAssigned indicates the task has been assigned to an agent
	TaskPhaseAssigned TaskPhase = "Assigned"
	// TaskPhaseRunning indicates the task is currently being executed
	TaskPhaseRunning TaskPhase = "Running"
	// TaskPhaseWaitingForElicitation indicates the task is waiting for user input
	TaskPhaseWaitingForElicitation TaskPhase = "WaitingForElicitation"
	// TaskPhaseCompleted indicates the task has completed successfully
	TaskPhaseCompleted TaskPhase = "Completed"
	// TaskPhaseFailed indicates the task has failed
	TaskPhaseFailed TaskPhase = "Failed"
	// TaskPhaseCancelled indicates the task has been cancelled
	TaskPhaseCancelled TaskPhase = "Cancelled"
)

// TaskCondition describes the state of a task at a certain point
type TaskCondition struct {
	// Type of task condition
	Type TaskConditionType `json:"type"`

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

// TaskConditionType represents the type of condition for a task
type TaskConditionType string

const (
	// TaskConditionAssigned indicates whether the task has been assigned to an agent
	TaskConditionAssigned TaskConditionType = "Assigned"
	// TaskConditionExecuting indicates whether the task is currently being executed
	TaskConditionExecuting TaskConditionType = "Executing"
	// TaskConditionCompleted indicates whether the task has completed
	TaskConditionCompleted TaskConditionType = "Completed"
)

// TaskElicitationRequest represents a request for user input during task execution
type TaskElicitationRequest struct {
	// ID is a unique identifier for this elicitation request
	// +kubebuilder:validation:Required
	ID string `json:"id"`

	// Prompt is the message to display to the user
	// +kubebuilder:validation:Required
	Prompt string `json:"prompt"`

	// PromptType specifies the type of input expected
	// +kubebuilder:validation:Enum=text;choice;confirmation;secret
	// +kubebuilder:default=text
	PromptType string `json:"promptType,omitempty"`

	// Choices provides options for choice-type prompts
	// +optional
	Choices []map[string]string `json:"choices,omitempty"`

	// DefaultValue provides a default value for the prompt
	// +optional
	DefaultValue string `json:"defaultValue,omitempty"`

	// Required indicates whether a response is mandatory
	// +kubebuilder:default=true
	Required bool `json:"required,omitempty"`

	// RequestTime is when the elicitation was requested
	// +optional
	RequestTime *metav1.Time `json:"requestTime,omitempty"`
}

// TaskElicitationResponse represents the user's response to an elicitation request
type TaskElicitationResponse struct {
	// ID matches the elicitation request ID
	// +kubebuilder:validation:Required
	ID string `json:"id"`

	// Response contains the user's response
	// +optional
	Response string `json:"response,omitempty"`

	// ResponseTime is when the user provided the response
	// +optional
	ResponseTime *metav1.Time `json:"responseTime,omitempty"`

	// Cancelled indicates if the elicitation was cancelled
	// +optional
	Cancelled bool `json:"cancelled,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=mcptask
// +kubebuilder:printcolumn:name="Domain",type="string",JSONPath=".spec.domain"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Agent",type="string",JSONPath=".status.assignedAgent"
// +kubebuilder:printcolumn:name="Priority",type="integer",JSONPath=".spec.priority"
// +kubebuilder:printcolumn:name="Retries",type="integer",JSONPath=".status.retryCount"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Task is the Schema for the tasks API
type Task struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TaskSpec   `json:"spec,omitempty"`
	Status TaskStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TaskList contains a list of Task
type TaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Task `json:"items"`
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TaskElicitationRequest) DeepCopyInto(out *TaskElicitationRequest) {
	*out = *in
	if in.Choices != nil {
		in, out := &in.Choices, &out.Choices
		*out = make([]map[string]string, len(*in))
		for i := range *in {
			if (*in)[i] != nil {
				in, out := &(*in)[i], &(*out)[i]
				*out = make(map[string]string, len(*in))
				for key, val := range *in {
					(*out)[key] = val
				}
			}
		}
	}
	if in.RequestTime != nil {
		in, out := &in.RequestTime, &out.RequestTime
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TaskElicitationRequest.
func (in *TaskElicitationRequest) DeepCopy() *TaskElicitationRequest {
	if in == nil {
		return nil
	}
	out := new(TaskElicitationRequest)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *TaskElicitationResponse) DeepCopyInto(out *TaskElicitationResponse) {
	*out = *in
	if in.ResponseTime != nil {
		in, out := &in.ResponseTime, &out.ResponseTime
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new TaskElicitationResponse.
func (in *TaskElicitationResponse) DeepCopy() *TaskElicitationResponse {
	if in == nil {
		return nil
	}
	out := new(TaskElicitationResponse)
	in.DeepCopyInto(out)
	return out
}

func init() {
	SchemeBuilder.Register(&Task{}, &TaskList{})
}
