package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ElicitationSpec defines the desired state of Elicitation
type ElicitationSpec struct {
	// TaskName is the name of the task that requested this elicitation
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	TaskName string `json:"taskName"`

	// TaskNamespace is the namespace of the task that requested this elicitation
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	TaskNamespace string `json:"taskNamespace"`

	// Prompt is the message to display to the user
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Prompt string `json:"prompt"`

	// PromptType specifies the type of input expected from the user
	// +kubebuilder:validation:Enum=text;choice;confirmation;secret
	// +kubebuilder:default=text
	PromptType ElicitationPromptType `json:"promptType,omitempty"`

	// Choices provides options for choice-type prompts
	// +optional
	Choices []ElicitationChoice `json:"choices,omitempty"`

	// DefaultValue provides a default value for the prompt
	// +optional
	DefaultValue string `json:"defaultValue,omitempty"`

	// Required indicates whether a response is mandatory
	// +kubebuilder:default=true
	Required bool `json:"required,omitempty"`

	// Timeout specifies how long to wait for user response
	// +kubebuilder:default="300s"
	Timeout metav1.Duration `json:"timeout,omitempty"`

	// Context provides additional context for the prompt
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Context *runtime.RawExtension `json:"context,omitempty"`
}

// ElicitationChoice represents a choice option for choice-type prompts
type ElicitationChoice struct {
	// Value is the actual value to return if this choice is selected
	// +kubebuilder:validation:Required
	Value string `json:"value"`

	// Label is the human-readable label for this choice
	// +kubebuilder:validation:Required
	Label string `json:"label"`

	// Description provides additional context for this choice
	// +optional
	Description string `json:"description,omitempty"`
}

// ElicitationPromptType defines the type of elicitation prompt
// +kubebuilder:validation:Enum=text;choice;confirmation;secret
type ElicitationPromptType string

const (
	// ElicitationPromptTypeText requests free-form text input
	ElicitationPromptTypeText ElicitationPromptType = "text"
	// ElicitationPromptTypeChoice requests selection from predefined options
	ElicitationPromptTypeChoice ElicitationPromptType = "choice"
	// ElicitationPromptTypeConfirmation requests yes/no confirmation
	ElicitationPromptTypeConfirmation ElicitationPromptType = "confirmation"
	// ElicitationPromptTypeSecret requests sensitive input (passwords, tokens)
	ElicitationPromptTypeSecret ElicitationPromptType = "secret"
)

// ElicitationPhase represents the current phase of the elicitation
// +kubebuilder:validation:Enum=Pending;Waiting;Responded;Timeout;Cancelled
type ElicitationPhase string

const (
	// ElicitationPhasePending indicates the elicitation is being prepared
	ElicitationPhasePending ElicitationPhase = "Pending"
	// ElicitationPhaseWaiting indicates the elicitation is waiting for user response
	ElicitationPhaseWaiting ElicitationPhase = "Waiting"
	// ElicitationPhaseResponded indicates the user has provided a response
	ElicitationPhaseResponded ElicitationPhase = "Responded"
	// ElicitationPhaseTimeout indicates the elicitation timed out
	ElicitationPhaseTimeout ElicitationPhase = "Timeout"
	// ElicitationPhaseCancelled indicates the elicitation was cancelled
	ElicitationPhaseCancelled ElicitationPhase = "Cancelled"
)

// ElicitationStatus defines the observed state of Elicitation
type ElicitationStatus struct {
	// Phase represents the current phase of the elicitation
	// +kubebuilder:validation:Enum=Pending;Waiting;Responded;Timeout;Cancelled
	Phase ElicitationPhase `json:"phase,omitempty"`

	// CreatedTime is when the elicitation was created
	// +optional
	CreatedTime *metav1.Time `json:"createdTime,omitempty"`

	// ResponseTime is when the user provided a response
	// +optional
	ResponseTime *metav1.Time `json:"responseTime,omitempty"`

	// Response contains the user's response
	// +optional
	Response string `json:"response,omitempty"`

	// Error contains error information if the elicitation failed
	// +optional
	Error string `json:"error,omitempty"`

	// Conditions represent the latest available observations of the elicitation's state
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []ElicitationCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// ObservedGeneration reflects the generation of the most recently observed Elicitation
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// ElicitationCondition describes the state of an elicitation at a certain point
type ElicitationCondition struct {
	// Type of elicitation condition
	Type ElicitationConditionType `json:"type"`

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

// ElicitationConditionType represents the type of elicitation condition
type ElicitationConditionType string

const (
	// ElicitationConditionReady indicates whether the elicitation is ready for user response
	ElicitationConditionReady ElicitationConditionType = "Ready"
	// ElicitationConditionCompleted indicates whether the elicitation has been completed
	ElicitationConditionCompleted ElicitationConditionType = "Completed"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Task",type="string",JSONPath=".spec.taskName"
// +kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.promptType"
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Elicitation is the Schema for the elicitations API
type Elicitation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ElicitationSpec   `json:"spec,omitempty"`
	Status ElicitationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ElicitationList contains a list of Elicitation
type ElicitationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Elicitation `json:"items"`
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElicitationSpec) DeepCopyInto(out *ElicitationSpec) {
	*out = *in
	out.Timeout = in.Timeout
	if in.Choices != nil {
		in, out := &in.Choices, &out.Choices
		*out = make([]ElicitationChoice, len(*in))
		copy(*out, *in)
	}
	if in.Context != nil {
		in, out := &in.Context, &out.Context
		*out = new(runtime.RawExtension)
		(*in).DeepCopyInto(*out)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElicitationSpec.
func (in *ElicitationSpec) DeepCopy() *ElicitationSpec {
	if in == nil {
		return nil
	}
	out := new(ElicitationSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElicitationChoice) DeepCopyInto(out *ElicitationChoice) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElicitationChoice.
func (in *ElicitationChoice) DeepCopy() *ElicitationChoice {
	if in == nil {
		return nil
	}
	out := new(ElicitationChoice)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElicitationStatus) DeepCopyInto(out *ElicitationStatus) {
	*out = *in
	if in.CreatedTime != nil {
		in, out := &in.CreatedTime, &out.CreatedTime
		*out = (*in).DeepCopy()
	}
	if in.ResponseTime != nil {
		in, out := &in.ResponseTime, &out.ResponseTime
		*out = (*in).DeepCopy()
	}
	if in.Conditions != nil {
		in, out := &in.Conditions, &out.Conditions
		*out = make([]ElicitationCondition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElicitationStatus.
func (in *ElicitationStatus) DeepCopy() *ElicitationStatus {
	if in == nil {
		return nil
	}
	out := new(ElicitationStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElicitationCondition) DeepCopyInto(out *ElicitationCondition) {
	*out = *in
	in.LastTransitionTime.DeepCopyInto(&out.LastTransitionTime)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElicitationCondition.
func (in *ElicitationCondition) DeepCopy() *ElicitationCondition {
	if in == nil {
		return nil
	}
	out := new(ElicitationCondition)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Elicitation) DeepCopyInto(out *Elicitation) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Elicitation.
func (in *Elicitation) DeepCopy() *Elicitation {
	if in == nil {
		return nil
	}
	out := new(Elicitation)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Elicitation) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ElicitationList) DeepCopyInto(out *ElicitationList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Elicitation, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ElicitationList.
func (in *ElicitationList) DeepCopy() *ElicitationList {
	if in == nil {
		return nil
	}
	out := new(ElicitationList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ElicitationList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func init() {
	SchemeBuilder.Register(&Elicitation{}, &ElicitationList{})
}
