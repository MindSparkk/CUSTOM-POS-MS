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
	var traceID, orderNo, errorCode, operation string
	var httpStatus int
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

	logEntry := models.POSLog{
		Timestamp:  r.Time,
		TraceID:    traceID,
		OrderNo:    orderNo,
		Service:    h.serviceName,
		Operation:  operation,
		Level:      r.Level.String(),
		HTTPStatus: httpStatus,
		ErrorCode:  errorCode,
		Message:    r.Message,
		Metadata:   metadata,
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
