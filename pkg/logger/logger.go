package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"retail-pos/pkg/models"
)

type TelemetryHandler struct {
	serviceName  string
	telemetryURL string
}

func NewTelemetryHandler(serviceName string) *TelemetryHandler {
	url := os.Getenv("RETAIL_AI_BACKEND_URL")
	if url == "" {
		// Use a local default telemetry service URL for the POS system to forward to itself first
		url = "http://telemetry-service:8085/ingest"
	}
	return &TelemetryHandler{
		serviceName:  serviceName,
		telemetryURL: url,
	}
}

func (h *TelemetryHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return true
}

func (h *TelemetryHandler) Handle(ctx context.Context, r slog.Record) error {
	// Parse attributes
	var traceID, orderNo, errorCode, operation, eventType, category, dependency, dependencyStatus, storeID, laneID, laneType, cashierID, incidentID, environment string
	var httpStatus int
	var durationMS int64
	metadata := make(map[string]interface{})

	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "trace_id":
			traceID = a.Value.String()
		case "order_no":
			orderNo = a.Value.String()
		case "error_code":
			errorCode = a.Value.String()
		case "operation":
			operation = a.Value.String()
		case "event_type":
			eventType = a.Value.String()
		case "category":
			category = a.Value.String()
		case "dependency":
			dependency = a.Value.String()
		case "dependency_status":
			dependencyStatus = a.Value.String()
		case "store_id":
			storeID = a.Value.String()
		case "lane_id":
			laneID = a.Value.String()
		case "lane_type":
			laneType = a.Value.String()
		case "cashier_id":
			cashierID = a.Value.String()
		case "incident_id":
			incidentID = a.Value.String()
		case "environment":
			environment = a.Value.String()
		case "duration_ms":
			if val, ok := a.Value.Any().(int64); ok {
				durationMS = val
			} else if valInt, ok := a.Value.Any().(int); ok {
				durationMS = int64(valInt)
			} else {
				durationMS = a.Value.Int64()
			}
		case "http_status":
			if val, ok := a.Value.Any().(int); ok {
				httpStatus = val
			} else if val64, ok := a.Value.Any().(int64); ok {
				httpStatus = int(val64)
			} else {
				httpStatus = int(a.Value.Int64())
			}
		default:
			metadata[a.Key] = a.Value.Any()
		}
		return true
	})

	if storeID == "" {
		storeID = os.Getenv("STORE_ID")
		if storeID == "" {
			storeID = "STORE-104"
		}
	}
	if environment == "" {
		environment = os.Getenv("ENV")
		if environment == "" {
			environment = "dev"
		}
	}

	logEntry := models.POSLog{
		Timestamp:        r.Time,
		IncidentID:       incidentID,
		TraceID:          traceID,
		OrderNo:          orderNo,
		Service:          h.serviceName,
		Operation:        operation,
		Level:            r.Level.String(),
		EventType:        eventType,
		Category:         category,
		HTTPStatus:       httpStatus,
		ErrorCode:        errorCode,
		Message:          r.Message,
		Dependency:       dependency,
		DependencyStatus: dependencyStatus,
		DurationMS:       durationMS,
		StoreID:          storeID,
		LaneID:           laneID,
		LaneType:         laneType,
		CashierID:        cashierID,
		Environment:      environment,
		Metadata:         metadata,
	}

	// Also print locally as JSON
	localBytes, _ := json.Marshal(logEntry)
	os.Stdout.Write(localBytes)
	os.Stdout.Write([]byte("\n"))

	// Forward to telemetry asynchronously
	if h.telemetryURL != "" && h.serviceName != "telemetry-service" {
		go func(entry models.POSLog) {
			payload, err := json.Marshal(entry)
			if err != nil {
				return
			}
			req, err := http.NewRequest("POST", h.telemetryURL, bytes.NewBuffer(payload))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			client := &http.Client{Timeout: 2 * time.Second}
			client.Do(req)
			// Ignore errors as per requirements
		}(logEntry)
	}

	return nil
}

func (h *TelemetryHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h // Not fully implemented for simplicity
}

func (h *TelemetryHandler) WithGroup(name string) slog.Handler {
	return h // Not fully implemented for simplicity
}

func InitLogger(serviceName string) {
	handler := NewTelemetryHandler(serviceName)
	logger := slog.New(handler)
	slog.SetDefault(logger)
}
