package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodReportSpec defines the desired state of PodReport.
type PodReportSpec struct {
	// NodeReportName is the name of the parent NodeReport.
	NodeReportName string `json:"nodeReportName,omitempty"`
	// NodeReportUID is the UID of the parent NodeReport, for cross-reference integrity.
	NodeReportUID string `json:"nodeReportUID,omitempty"`
	// NodeName is the name of the node the pod was scheduled on.
	NodeName string `json:"nodeName,omitempty"`
	// TerminationCause is the node termination cause, copied from the NodeReport.
	// +kubebuilder:validation:Enum=SpotInterruption;ClusterAutoscalerScaleDown;ManualDrain;NodeConditionFailure;Unknown
	TerminationCause TerminationCause `json:"terminationCause,omitempty"`
	// PodName is the name of the pod.
	PodName string `json:"podName,omitempty"`
	// PodUID is the UID of the pod, used for deduplication.
	PodUID string `json:"podUID,omitempty"`
	// Namespace is the namespace the pod belonged to.
	Namespace string `json:"namespace,omitempty"`
	// OwnerKind is the kind of the owning workload.
	// +kubebuilder:validation:Enum=Deployment;StatefulSet;DaemonSet;Job;CronJob;None
	OwnerKind OwnerKind `json:"ownerKind,omitempty"`
	// OwnerName is the name of the owning workload.
	OwnerName string `json:"ownerName,omitempty"`
	// StartTime is when the pod started.
	StartTime *metav1.Time `json:"startTime,omitempty"`
	// EndTime is when the pod terminated.
	EndTime *metav1.Time `json:"endTime,omitempty"`
	// ExitReason describes how the pod exited.
	// +kubebuilder:validation:Enum=Evicted;Completed;OOMKilled;Error;Unknown
	ExitReason PodExitReason `json:"exitReason,omitempty"`
	// Phase is the last known pod phase.
	Phase string `json:"phase,omitempty"`
	// CPURequest is the total CPU requested across all containers.
	CPURequest string `json:"cpuRequest,omitempty"`
	// MemoryRequest is the total memory requested across all containers.
	MemoryRequest string `json:"memoryRequest,omitempty"`
	// CPULimit is the total CPU limit across all containers.
	CPULimit string `json:"cpuLimit,omitempty"`
	// MemoryLimit is the total memory limit across all containers.
	MemoryLimit string `json:"memoryLimit,omitempty"`
	// RestartCount is the total container restarts across all containers.
	RestartCount int32 `json:"restartCount,omitempty"`
	// Containers lists the containers and their images.
	Containers []ContainerInfo `json:"containers,omitempty"`
	// OOMKills is the list of OOMKill events for this pod.
	OOMKills []OOMKillRecord `json:"oomKills,omitempty"`
}

// PodReportStatus defines the observed state of PodReport.
type PodReportStatus struct {
	// Phase is the current lifecycle phase of the PodReport.
	// +kubebuilder:validation:Enum=Collecting;Complete
	Phase StatusPhase `json:"phase,omitempty"`
	// Message is a human-readable status message.
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=pr
// +kubebuilder:printcolumn:name="Pod",type=string,JSONPath=".spec.podName"
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=".spec.nodeName"
// +kubebuilder:printcolumn:name="Exit",type=string,JSONPath=".spec.exitReason"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// PodReport is the Schema for the podreports API.
type PodReport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PodReportSpec   `json:"spec,omitempty"`
	Status PodReportStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PodReportList contains a list of PodReport.
type PodReportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodReport `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodReport{}, &PodReportList{})
}
