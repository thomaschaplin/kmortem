package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TerminationCause describes why a node was terminated.
type TerminationCause string

const (
	TerminationCauseSpotInterruption         TerminationCause = "SpotInterruption"
	TerminationCauseClusterAutoscalerScaleDown TerminationCause = "ClusterAutoscalerScaleDown"
	TerminationCauseManualDrain              TerminationCause = "ManualDrain"
	TerminationCauseNodeConditionFailure     TerminationCause = "NodeConditionFailure"
	TerminationCauseCloudMaintenanceEvent    TerminationCause = "CloudMaintenanceEvent"
	TerminationCauseUnknown                 TerminationCause = "Unknown"
)

// Lifecycle describes the AWS instance purchasing option.
type Lifecycle string

const (
	LifecycleSpot     Lifecycle = "spot"
	LifecycleOnDemand Lifecycle = "on-demand"
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
	StatusPhaseCollecting     StatusPhase = "Collecting"
	StatusPhaseComplete       StatusPhase = "Complete"
	StatusPhasePendingDeletion StatusPhase = "PendingDeletion"
	StatusPhaseArchived       StatusPhase = "Archived"
	StatusPhaseArchiveFailed  StatusPhase = "ArchiveFailed"
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
	// +kubebuilder:validation:Enum=spot;on-demand
	Lifecycle Lifecycle `json:"lifecycle,omitempty"`
	// Region is the AWS region.
	Region string `json:"region,omitempty"`
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

// ArchivalRecord holds information about where the NodeReport was archived.
type ArchivalRecord struct {
	// Enabled indicates whether archival is enabled.
	Enabled bool `json:"enabled,omitempty"`
	// Bucket is the S3 bucket name.
	Bucket string `json:"bucket,omitempty"`
	// Key is the S3 object key.
	Key string `json:"key,omitempty"`
	// ArchivedAt is when the archival completed.
	ArchivedAt *metav1.Time `json:"archivedAt,omitempty"`
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
	// +kubebuilder:validation:Enum=SpotInterruption;ClusterAutoscalerScaleDown;ManualDrain;NodeConditionFailure;CloudMaintenanceEvent;Unknown
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
	// Archival holds archival information.
	Archival ArchivalRecord `json:"archival,omitempty"`
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
	// +kubebuilder:validation:Enum=Collecting;Complete;PendingDeletion;Archived;ArchiveFailed
	Phase StatusPhase `json:"phase,omitempty"`
	// Message is a human-readable status message.
	Message string `json:"message,omitempty"`
	// CollectionCompletedAt is when evidence collection finished.
	CollectionCompletedAt *metav1.Time `json:"collectionCompletedAt,omitempty"`
	// Conditions are detailed status conditions.
	Conditions []NodeReportStatusCondition `json:"conditions,omitempty"`
	// ArchiveFailureCount tracks how many times archival has been attempted.
	ArchiveFailureCount int `json:"archiveFailureCount,omitempty"`
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
