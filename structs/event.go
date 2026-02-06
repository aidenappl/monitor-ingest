package structs

import (
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

// uuidRegex matches standard UUID format (with or without hyphens)
var uuidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Event represents a single monitoring event
type Event struct {
	Timestamp time.Time              `json:"timestamp"`
	Service   string                 `json:"service"`
	Env       string                 `json:"env"`
	JobID     string                 `json:"job_id"`
	RequestID string                 `json:"request_id"`
	TraceID   string                 `json:"trace_id"`
	Name      string                 `json:"name"`
	Level     string                 `json:"level"`
	Data      map[string]interface{} `json:"data"`
}

// Validate checks that all required fields are present and IDs are valid UUIDs
func (e *Event) Validate() error {
	if e.Timestamp.IsZero() {
		return errors.New("timestamp is required")
	}
	if e.Service == "" {
		return errors.New("service is required")
	}
	if e.Name == "" {
		return errors.New("name is required")
	}
	if e.JobID != "" && !uuidRegex.MatchString(e.JobID) {
		return errors.New("job_id must be a valid UUID")
	}
	if e.RequestID != "" && !uuidRegex.MatchString(e.RequestID) {
		return errors.New("request_id must be a valid UUID")
	}
	if e.TraceID != "" && !uuidRegex.MatchString(e.TraceID) {
		return errors.New("trace_id must be a valid UUID")
	}
	return nil
}

// DataJSON returns the data field as a JSON string
func (e *Event) DataJSON() string {
	if e.Data == nil {
		return "{}"
	}
	b, err := json.Marshal(e.Data)
	if err != nil {
		return "{}"
	}
	return string(b)
}
