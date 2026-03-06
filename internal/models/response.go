package models

import "time"

// APIResponse is the standard envelope for all REST API responses.
type APIResponse struct {
	Status    string      `json:"status"`              // "ok" or "error"
	Data      interface{} `json:"data,omitempty"`       // response payload
	Error     string      `json:"error,omitempty"`      // error message (when status == "error")
	Timestamp string      `json:"timestamp"`            // RFC3339 UTC
}

// NewOKResponse creates a success envelope wrapping the given data.
func NewOKResponse(data interface{}) APIResponse {
	return APIResponse{
		Status:    "ok",
		Data:      data,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// NewErrorResponse creates an error envelope with the given message.
func NewErrorResponse(msg string) APIResponse {
	return APIResponse{
		Status:    "error",
		Error:     msg,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}
