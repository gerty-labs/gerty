package agent

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
)

// Reporter serves the /report endpoint with waste-per-pod JSON.
type Reporter struct {
	nodeName string
	store    *Store
}

// NewReporter creates a Reporter.
func NewReporter(nodeName string, store *Store) *Reporter {
	return &Reporter{
		nodeName: nodeName,
		store:    store,
	}
}

// HandleReport is the HTTP handler for GET /report.
func (r *Reporter) HandleReport(w http.ResponseWriter, req *http.Request) {
	report := models.NodeReport{
		NodeName:   r.nodeName,
		ReportTime: time.Now(),
		Pods:       []models.PodWaste{},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}
