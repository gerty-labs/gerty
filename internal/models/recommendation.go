package models

import "time"

// WorkloadPattern classifies the resource usage pattern of a workload.
type WorkloadPattern string

const (
	PatternSteady    WorkloadPattern = "steady"
	PatternBurstable WorkloadPattern = "burstable"
	PatternBatch     WorkloadPattern = "batch"
	PatternIdle      WorkloadPattern = "idle"
)

// RiskLevel indicates the risk of applying a recommendation.
type RiskLevel string

const (
	RiskLow    RiskLevel = "LOW"
	RiskMedium RiskLevel = "MEDIUM"
	RiskHigh   RiskLevel = "HIGH"
)

// OwnerReference identifies the workload owner (Deployment, StatefulSet, etc).
type OwnerReference struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// Recommendation is a structured right-sizing recommendation.
type Recommendation struct {
	Target           OwnerReference  `json:"target"`
	Container        string          `json:"container"`
	Resource         string          `json:"resource"` // "cpu" or "memory"
	CurrentRequest   int64           `json:"currentRequest"`
	CurrentLimit     int64           `json:"currentLimit"`
	RecommendedReq   int64           `json:"recommendedRequest"`
	RecommendedLimit int64           `json:"recommendedLimit"`
	Pattern          WorkloadPattern `json:"pattern"`
	Confidence       float64         `json:"confidence"`
	Reasoning        string          `json:"reasoning"`
	EstSavings       int64           `json:"estimatedSavings"`
	Risk             RiskLevel       `json:"risk"`
	DataWindow       time.Duration   `json:"dataWindow"`
}
