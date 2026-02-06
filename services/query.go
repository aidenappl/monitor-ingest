package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/aidenappl/monitor-core/db"
	"github.com/aidenappl/monitor-core/structs"
)

type QueryParams struct {
	Service     string
	Env         string
	JobID       string
	RequestID   string
	TraceID     string
	Name        string
	Level       string
	From        time.Time
	To          time.Time
	DataFilters map[string]string
	Limit       int
	Offset      int
}

type QueryResult struct {
	Events []*structs.Event `json:"events"`
	Total  int              `json:"total"`
}

type LabelValuesResult struct {
	Values []string `json:"values"`
}

type DataKeysResult struct {
	Keys []string `json:"keys"`
}

func eventsTable() string {
	return fmt.Sprintf("%s.events", db.Database)
}

func applyFilters(builder sq.SelectBuilder, params QueryParams) sq.SelectBuilder {
	if params.Service != "" {
		builder = builder.Where(sq.Eq{"service": params.Service})
	}
	if params.Env != "" {
		builder = builder.Where(sq.Eq{"env": params.Env})
	}
	if params.JobID != "" {
		builder = builder.Where(sq.Eq{"job_id": params.JobID})
	}
	if params.RequestID != "" {
		builder = builder.Where(sq.Eq{"request_id": params.RequestID})
	}
	if params.TraceID != "" {
		builder = builder.Where(sq.Eq{"trace_id": params.TraceID})
	}
	if params.Name != "" {
		builder = builder.Where(sq.Eq{"name": params.Name})
	}
	if params.Level != "" {
		builder = builder.Where(sq.Eq{"level": params.Level})
	}
	if !params.From.IsZero() {
		builder = builder.Where(sq.GtOrEq{"timestamp": params.From})
	}
	if !params.To.IsZero() {
		builder = builder.Where(sq.LtOrEq{"timestamp": params.To})
	}
	for key, value := range params.DataFilters {
		builder = builder.Where("JSONExtractString(data, ?) = ?", key, value)
	}
	return builder
}

func QueryEvents(ctx context.Context, params QueryParams) (*QueryResult, error) {
	if params.Limit <= 0 {
		params.Limit = 100
	}
	if params.Limit > 1000 {
		params.Limit = 1000
	}

	// Count query
	countBuilder := sq.Select("count()").
		From(eventsTable()).
		PlaceholderFormat(sq.Question)
	countBuilder = applyFilters(countBuilder, params)

	countSQL, countArgs, err := countBuilder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build count query: %w", err)
	}

	var total uint64
	if err := db.Conn.QueryRow(ctx, countSQL, countArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count query failed: %w", err)
	}

	// Data query
	queryBuilder := sq.Select("timestamp", "service", "env", "job_id", "request_id", "trace_id", "name", "level", "data").
		From(eventsTable()).
		OrderBy("timestamp DESC").
		Limit(uint64(params.Limit)).
		Offset(uint64(params.Offset)).
		PlaceholderFormat(sq.Question)
	queryBuilder = applyFilters(queryBuilder, params)

	querySQL, queryArgs, err := queryBuilder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := db.Conn.Query(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var events []*structs.Event
	for rows.Next() {
		var e structs.Event
		var dataStr string
		if err := rows.Scan(&e.Timestamp, &e.Service, &e.Env, &e.JobID, &e.RequestID, &e.TraceID, &e.Name, &e.Level, &dataStr); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		if dataStr != "" && dataStr != "{}" {
			json.Unmarshal([]byte(dataStr), &e.Data)
		}
		events = append(events, &e)
	}

	if events == nil {
		events = []*structs.Event{}
	}

	return &QueryResult{
		Events: events,
		Total:  int(total),
	}, nil
}

var validLabels = map[string]string{
	"service": "service",
	"env":     "env",
	"name":    "name",
	"level":   "level",
}

func GetLabelValues(ctx context.Context, label string, params QueryParams) (*LabelValuesResult, error) {
	column, ok := validLabels[label]
	if !ok {
		return nil, fmt.Errorf("invalid label: %s", label)
	}

	builder := sq.Select(fmt.Sprintf("DISTINCT %s", column)).
		From(eventsTable()).
		OrderBy(column).
		Limit(1000).
		PlaceholderFormat(sq.Question)

	// Apply filters except the one we're getting values for
	if params.Service != "" && column != "service" {
		builder = builder.Where(sq.Eq{"service": params.Service})
	}
	if params.Env != "" && column != "env" {
		builder = builder.Where(sq.Eq{"env": params.Env})
	}
	if params.Name != "" && column != "name" {
		builder = builder.Where(sq.Eq{"name": params.Name})
	}
	if params.Level != "" && column != "level" {
		builder = builder.Where(sq.Eq{"level": params.Level})
	}
	if !params.From.IsZero() {
		builder = builder.Where(sq.GtOrEq{"timestamp": params.From})
	}
	if !params.To.IsZero() {
		builder = builder.Where(sq.LtOrEq{"timestamp": params.To})
	}

	querySQL, queryArgs, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := db.Conn.Query(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		if v != "" {
			values = append(values, v)
		}
	}

	if values == nil {
		values = []string{}
	}

	return &LabelValuesResult{Values: values}, nil
}

func GetDataKeys(ctx context.Context, params QueryParams) (*DataKeysResult, error) {
	builder := sq.Select("DISTINCT arrayJoin(JSONExtractKeys(data)) AS key").
		From(eventsTable()).
		OrderBy("key").
		Limit(1000).
		PlaceholderFormat(sq.Question)

	if params.Service != "" {
		builder = builder.Where(sq.Eq{"service": params.Service})
	}
	if params.Env != "" {
		builder = builder.Where(sq.Eq{"env": params.Env})
	}
	if params.Name != "" {
		builder = builder.Where(sq.Eq{"name": params.Name})
	}
	if params.Level != "" {
		builder = builder.Where(sq.Eq{"level": params.Level})
	}
	if !params.From.IsZero() {
		builder = builder.Where(sq.GtOrEq{"timestamp": params.From})
	}
	if !params.To.IsZero() {
		builder = builder.Where(sq.LtOrEq{"timestamp": params.To})
	}

	querySQL, queryArgs, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := db.Conn.Query(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		keys = append(keys, k)
	}

	if keys == nil {
		keys = []string{}
	}

	return &DataKeysResult{Keys: keys}, nil
}

func GetDataValues(ctx context.Context, key string, params QueryParams) (*LabelValuesResult, error) {
	if key == "" {
		return nil, fmt.Errorf("key is required")
	}

	builder := sq.Select("DISTINCT JSONExtractString(data, ?) AS value").
		From(eventsTable()).
		OrderBy("value").
		Limit(1000).
		PlaceholderFormat(sq.Question)

	if params.Service != "" {
		builder = builder.Where(sq.Eq{"service": params.Service})
	}
	if params.Env != "" {
		builder = builder.Where(sq.Eq{"env": params.Env})
	}
	if params.Name != "" {
		builder = builder.Where(sq.Eq{"name": params.Name})
	}
	if params.Level != "" {
		builder = builder.Where(sq.Eq{"level": params.Level})
	}
	if !params.From.IsZero() {
		builder = builder.Where(sq.GtOrEq{"timestamp": params.From})
	}
	if !params.To.IsZero() {
		builder = builder.Where(sq.LtOrEq{"timestamp": params.To})
	}

	// Add HAVING clause manually since squirrel doesn't support it well
	builder = builder.Suffix("HAVING value != ''")

	querySQL, queryArgs, err := builder.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	// Prepend the key argument for JSONExtractString
	queryArgs = append([]interface{}{key}, queryArgs...)

	rows, err := db.Conn.Query(ctx, querySQL, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		values = append(values, v)
	}

	if values == nil {
		values = []string{}
	}

	return &LabelValuesResult{Values: values}, nil
}
