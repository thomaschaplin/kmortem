package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TerminationCause describes why a node was terminated.
type TerminationCause string

const (
	TerminationCauseSpotInterruption           TerminationCause = "SpotInterruption"
	TerminationCauseClusterAutoscalerScaleDown TerminationCause = "ClusterAutoscalerScaleDown"
	TerminationCauseManualDrain                TerminationCause = "ManualDrain"
	TerminationCauseNodeConditionFailure       TerminationCause = "NodeConditionFailure"
	TerminationCauseUnknown TerminationCause = "Unknown"
)

// Lifecycle describes the AWS instance purchasing option.
type Lifecycle string

const (
	LifecycleSpot     Lifecycle = "spot"
	LifecycleOnDemand Lifecycle = "on-demand"
	LifecycleUnknown  Lifecycle = "unknown"
)

// PodExitReason describes how a pod exited.
type PodExitReason string

const (
	PodExitReasonEvicted   PodExitReason = "Evicted"
	PodExitReasonCompleted PodExitReason = "Completed"
	PodExitReasonOOMKilled PodExitReason = "OOMKilled"
	PodExitReasonError     PodExitReason = "Error"
	PodExitReasonUnknown   PodExitReason = "Unknown"
)

// OwnerKind describes the owning workload type of a pod.
type OwnerKind string

const (
	OwnerKindDeployment  OwnerKind = "Deployment"
	OwnerKindStatefulSet OwnerKind = "StatefulSet"
	OwnerKindDaemonSet   OwnerKind = "DaemonSet"
	OwnerKindJob         OwnerKind = "Job"
	OwnerKindCronJob     OwnerKind = "CronJob"
	OwnerKindNone        OwnerKind = "None"
)

// StatusPhase describes the current phase of a NodeReport.
type StatusPhase string

const (
	StatusPhaseCollecting StatusPhase = "Collecting"
	StatusPhaseComplete   StatusPhase = "Complete"
)

// InstanceMetadata holds AWS instance information.
type InstanceMetadata struct {
	// InstanceID is the AWS EC2 instance ID.
	InstanceID string `json:"instanceID,omitempty"`
	// InstanceType is the EC2 instance type (e.g. m5.large).
	InstanceType string `json:"instanceType,omitempty"`
	// AvailabilityZone is the AZ the instance was in.
	AvailabilityZone string `json:"availabilityZone,omitempty"`
	// Lifecycle indicates spot or on-demand.
	// +kubebuilder:validation:Enum=spot;on-demand;unknown
	Lifecycle Lifecycle `json:"lifecycle,omitempty"`
	// Region is the AWS region.
	Region string `json:"region,omitempty"`
}

// TaintRecord holds a node taint.
type TaintRecord struct {
	// Key is the taint key.
	Key string `json:"key"`
	// Value is the taint value.
	Value string `json:"value,omitempty"`
	// Effect is the taint effect (NoSchedule, PreferNoSchedule, NoExecute).
	Effect string `json:"effect"`
}

// NodeMetadata holds Kubernetes node system information.
type NodeMetadata struct {
	// KernelVersion is the kernel version reported by the node.
	KernelVersion string `json:"kernelVersion,omitempty"`
	// OSImage is the OS image reported by the node.
	OSImage string `json:"osImage,omitempty"`
	// ContainerRuntime is the container runtime version.
	ContainerRuntime string `json:"containerRuntime,omitempty"`
	// KubeletVersion is the kubelet version.
	KubeletVersion string `json:"kubeletVersion,omitempty"`
	// AllocatableCPU is the allocatable CPU on the node.
	AllocatableCPU string `json:"allocatableCPU,omitempty"`
	// AllocatableMemory is the allocatable memory on the node.
	AllocatableMemory string `json:"allocatableMemory,omitempty"`
	// CapacityCPU is the total CPU capacity of the node.
	CapacityCPU string `json:"capacityCPU,omitempty"`
	// CapacityMemory is the total memory capacity of the node.
	CapacityMemory string `json:"capacityMemory,omitempty"`
	// CreatedAt is when the node was created.
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`
	// Labels are the node's labels.
	Labels map[string]string `json:"labels,omitempty"`
	// Taints are the node's taints at the time of termination.
	Taints []TaintRecord `json:"taints,omitempty"`
}

// ConditionRecord holds a snapshot of a node condition.
type ConditionRecord struct {
	// Type is the condition type (e.g. Ready, MemoryPressure).
	Type string `json:"type,omitempty"`
	// Status is the condition status (True, False, Unknown).
	Status string `json:"status,omitempty"`
	// Reason is a machine-readable reason string.
	Reason string `json:"reason,omitempty"`
	// Message is a human-readable message.
	Message string `json:"message,omitempty"`
	// LastTransitionTime is when the condition last changed.
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
}

// ContainerInfo holds the name and image of a container.
type ContainerInfo struct {
	// Name is the container name.
	Name string `json:"name"`
	// Image is the container image.
	Image string `json:"image"`
}

// PodRecord holds information about a pod that was on the node.
type PodRecord struct {
	// Name is the pod name.
	Name string `json:"name,omitempty"`
	// Namespace is the pod namespace.
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
	// CPURequest is the total CPU requested by all containers in the pod.
	CPURequest string `json:"cpuRequest,omitempty"`
	// MemoryRequest is the total memory requested by all containers in the pod.
	MemoryRequest string `json:"memoryRequest,omitempty"`
	// CPULimit is the total CPU limit across all containers in the pod.
	CPULimit string `json:"cpuLimit,omitempty"`
	// MemoryLimit is the total memory limit across all containers in the pod.
	MemoryLimit string `json:"memoryLimit,omitempty"`
	// RestartCount is the total number of container restarts across all containers.
	RestartCount int32 `json:"restartCount,omitempty"`
	// Containers lists the containers in the pod with their images.
	Containers []ContainerInfo `json:"containers,omitempty"`
}

// OOMKillRecord holds information about an OOMKill event.
type OOMKillRecord struct {
	// PodName is the name of the affected pod.
	PodName string `json:"podName,omitempty"`
	// Namespace is the namespace of the affected pod.
	Namespace string `json:"namespace,omitempty"`
	// ContainerName is the name of the OOMKilled container.
	ContainerName string `json:"containerName,omitempty"`
	// MemoryLimit is the memory limit that was exceeded.
	MemoryLimit string `json:"memoryLimit,omitempty"`
	// Timestamp is when the OOMKill occurred.
	Timestamp *metav1.Time `json:"timestamp,omitempty"`
}

// PDBViolationRecord holds information about a PodDisruptionBudget violation.
type PDBViolationRecord struct {
	// PDBName is the name of the PodDisruptionBudget.
	PDBName string `json:"pdbName,omitempty"`
	// Namespace is the namespace of the PDB.
	Namespace string `json:"namespace,omitempty"`
	// Reason explains why the PDB was violated.
	Reason string `json:"reason,omitempty"`
}

// NodeReportSpec defines the desired state of NodeReport.
type NodeReportSpec struct {
	// NodeName is the name of the terminated node.
	NodeName string `json:"nodeName,omitempty"`
	// NodeUID is the UID of the node, used for deduplication.
	NodeUID string `json:"nodeUID,omitempty"`
	// TerminationTime is when the node termination was detected.
	TerminationTime *metav1.Time `json:"terminationTime,omitempty"`
	// TerminationCause classifies why the node was terminated.
	// +kubebuilder:validation:Enum=SpotInterruption;ClusterAutoscalerScaleDown;ManualDrain;NodeConditionFailure;Unknown
	TerminationCause TerminationCause `json:"terminationCause,omitempty"`
	// InitiatedBy describes who or what triggered the drain.
	InitiatedBy string `json:"initiatedBy,omitempty"`
	// InstanceMetadata holds AWS instance information.
	InstanceMetadata InstanceMetadata `json:"instanceMetadata,omitempty"`
	// NodeMetadata holds Kubernetes node system information.
	NodeMetadata NodeMetadata `json:"nodeMetadata,omitempty"`
	// ConditionHistory is the history of node conditions.
	ConditionHistory []ConditionRecord `json:"conditionHistory,omitempty"`
	// Pods is the list of pods that were on the node.
	Pods []PodRecord `json:"pods,omitempty"`
	// OOMKills is the list of OOMKill events on the node.
	OOMKills []OOMKillRecord `json:"oomKills,omitempty"`
	// PDBViolations is the list of PDB violations during drain.
	PDBViolations []PDBViolationRecord `json:"pdbViolations,omitempty"`
}

// NodeReportStatusCondition is a condition on a NodeReport.
type NodeReportStatusCondition struct {
	// Type is the condition type.
	Type string `json:"type,omitempty"`
	// Status is the condition status (True, False, Unknown).
	Status string `json:"status,omitempty"`
	// Reason is a machine-readable reason string.
	Reason string `json:"reason,omitempty"`
	// Message is a human-readable message.
	Message string `json:"message,omitempty"`
	// LastTransitionTime is when the condition last changed.
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`
}

// NodeReportStatus defines the observed state of NodeReport.
type NodeReportStatus struct {
	// Phase is the current lifecycle phase of the NodeReport.
	// +kubebuilder:validation:Enum=Collecting;Complete
	Phase StatusPhase `json:"phase,omitempty"`
	// Message is a human-readable status message.
	Message string `json:"message,omitempty"`
	// CollectionCompletedAt is when evidence collection finished.
	CollectionCompletedAt *metav1.Time `json:"collectionCompletedAt,omitempty"`
	// Conditions are detailed status conditions.
	Conditions []NodeReportStatusCondition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=nr
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=".spec.nodeName"
// +kubebuilder:printcolumn:name="Cause",type=string,JSONPath=".spec.terminationCause"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"

// NodeReport is the Schema for the nodereports API.
type NodeReport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeReportSpec   `json:"spec,omitempty"`
	Status NodeReportStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeReportList contains a list of NodeReport.
type NodeReportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeReport `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeReport{}, &NodeReportList{})
}
