package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"retail-pos/pkg/errors"
	"retail-pos/pkg/logger"
	"retail-pos/pkg/models"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"
	"github.com/rs/cors"
)

var (
	db                 *sql.DB
	simFailureMutex    sync.RWMutex
	activeSimFailures  = make(map[string]bool)
	simLatencyDelayMs  int64
)

func initDB() {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://postgres:postgres@localhost:5432/posdb?sslmode=disable"
	}
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		slog.Error("Failed to connect to database", "error", err.Error(), "dependency", "postgresql", "dependency_status", "DOWN")
		return
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
}

func main() {
	logger.InitLogger("payment-service")
	slog.Info("Starting payment service on port 8083")

	initDB()

	r := mux.NewRouter()

	// Health & Readiness
	r.HandleFunc("/health", healthHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/ready", readinessHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/simulation/failures", failureInjectionHandler).Methods("POST", "OPTIONS")
	r.HandleFunc("/admin/restart", adminRestartHandler).Methods("POST", "OPTIONS")

	// Payments
	r.HandleFunc("/payments", paymentHandler).Methods("POST", "OPTIONS")

	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "DELETE", "PUT", "OPTIONS"},
		AllowedHeaders: []string{"*"},
	})

	srv := &http.Server{
		Handler:      c.Handler(r),
		Addr:         ":8083",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}
	slog.Error("Server stopped", "error", srv.ListenAndServe())
}

func getLaneContext(r *http.Request) (string, string, string, string) {
	storeID := r.Header.Get("X-Store-ID")
	if storeID == "" {
		storeID = os.Getenv("STORE_ID")
		if storeID == "" {
			storeID = "STORE-104"
		}
	}
	laneID := r.Header.Get("X-Lane-ID")
	if laneID == "" {
		laneID = "LANE-01"
	}
	laneType := r.Header.Get("X-Lane-Type")
	if laneType == "" {
		laneType = "CASHIER_EXPRESS"
	}
	cashierID := r.Header.Get("X-Cashier-ID")
	if cashierID == "" {
		cashierID = "CASHIER-101"
	}
	return storeID, laneID, laneType, cashierID
}

func checkSimFailure(failureType string) bool {
	simFailureMutex.RLock()
	defer simFailureMutex.RUnlock()
	return activeSimFailures[failureType]
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, posErr *errors.POSError, traceID, orderNo string, r *http.Request, startTime time.Time, eventType string) {
	duration := time.Since(startTime).Milliseconds()
	storeID, laneID, laneType, cashierID := getLaneContext(r)

	slog.Error("Payment request failed",
		"error_code", posErr.Code,
		"category", posErr.Category,
		"message", posErr.Message,
		"trace_id", traceID,
		"order_no", orderNo,
		"event_type", eventType,
		"http_status", status,
		"duration_ms", duration,
		"store_id", storeID,
		"lane_id", laneID,
		"lane_type", laneType,
		"cashier_id", cashierID,
	)
	writeJSON(w, status, posErr)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, models.HealthResponse{
		Service:   "payment-service",
		Status:    "UP",
		Version:   "1.0.0",
		Timestamp: time.Now(),
	})
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	dbStatus := "UP"
	status := "READY"
	httpCode := http.StatusOK

	if db == nil || db.Ping() != nil || checkSimFailure("DATABASE_UNAVAILABLE") || checkSimFailure("PAYMENT_SERVICE_DOWN") {
		dbStatus = "DOWN"
		status = "NOT_READY"
		httpCode = http.StatusServiceUnavailable
	}

	writeJSON(w, httpCode, models.ReadinessResponse{
		Service:   "payment-service",
		Status:    status,
		Timestamp: time.Now(),
		Dependencies: map[string]string{
			"postgresql": dbStatus,
		},
	})
}

func failureInjectionHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type       string `json:"type"`
		Enabled    bool   `json:"enabled"`
		DurationMS int64  `json:"duration_ms,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	simFailureMutex.Lock()
	activeSimFailures[req.Type] = req.Enabled
	if req.Type == "PAYMENT_TIMEOUT" || req.Type == "LATENCY_SPIKE" {
		simLatencyDelayMs = req.DurationMS
	}
	simFailureMutex.Unlock()

	slog.Warn("Payment failure simulation state updated", "type", req.Type, "enabled", req.Enabled, "duration_ms", req.DurationMS)
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "updated", "type": req.Type, "enabled": req.Enabled})
}

func adminRestartHandler(w http.ResponseWriter, r *http.Request) {
	slog.Warn("Admin restart requested - service will restart in 1 second")
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarting", "message": "Service will restart shortly"})
	go func() {
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
}

func paymentHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	storeID, laneID, laneType, cashierID := getLaneContext(r)

	var req models.Payment
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrInvalidRequest, "Invalid request body"), "", "", r, startTime, "PAYMENT_FAILED")
		return
	}

	if req.LaneID == "" {
		req.LaneID = laneID
	}
	if req.StoreID == "" {
		req.StoreID = storeID
	}

	// Trigger simulation failure checks
	if checkSimFailure("PAYMENT_SERVICE_DOWN") {
		writeError(w, http.StatusServiceUnavailable, errors.New(errors.ErrPaymentFailed, "Payment gateway service unavailable"), req.TraceID, req.OrderNo, r, startTime, "PAYMENT_FAILED")
		return
	}

	if checkSimFailure("PAYMENT_TIMEOUT") {
		delay := simLatencyDelayMs
		if delay == 0 {
			delay = 5000
		}
		time.Sleep(time.Duration(delay) * time.Millisecond)
		writeError(w, http.StatusGatewayTimeout, errors.New(errors.ErrPaymentFailed, "Payment processing timed out upstream"), req.TraceID, req.OrderNo, r, startTime, "PAYMENT_FAILED")
		return
	}

	if checkSimFailure("DATABASE_UNAVAILABLE") {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrDatabaseUnavailable, "Database transaction unavailable"), req.TraceID, req.OrderNo, r, startTime, "DATABASE_CONNECTION_FAILED")
		return
	}

	slog.Info("Payment transaction initiated",
		"trace_id", req.TraceID,
		"order_no", req.OrderNo,
		"event_type", "PAYMENT_STARTED",
		"store_id", storeID,
		"lane_id", req.LaneID,
		"lane_type", laneType,
		"cashier_id", cashierID,
		"amount", req.Amount,
		"cash_received", req.CashReceived,
	)

	var status string
	var total float64
	err := db.QueryRow("SELECT status, total FROM orders WHERE order_no = $1", req.OrderNo).Scan(&status, &total)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrOrderNotFound, "Order not found"), req.TraceID, req.OrderNo, r, startTime, "PAYMENT_FAILED")
		return
	}
	if status != "OPEN" {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrInvalidRequest, "Order is not OPEN for payment"), req.TraceID, req.OrderNo, r, startTime, "PAYMENT_FAILED")
		return
	}

	if req.CashReceived < req.Amount {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrInsufficientCash, fmt.Sprintf("Insufficient cash received: Cash ($%.2f) is less than Amount Due ($%.2f)", req.CashReceived, req.Amount)), req.TraceID, req.OrderNo, r, startTime, "PAYMENT_FAILED")
		return
	}

	change := req.CashReceived - req.Amount
	paymentStatus := "PAID"

	tx, err := db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Database error starting payment transaction"), req.TraceID, req.OrderNo, r, startTime, "PAYMENT_FAILED")
		return
	}

	_, err = tx.Exec(
		"INSERT INTO payments (order_no, trace_id, lane_id, amount, cash_received, change_amount, status) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		req.OrderNo, req.TraceID, req.LaneID, req.Amount, req.CashReceived, change, paymentStatus,
	)
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Failed to record payment"), req.TraceID, req.OrderNo, r, startTime, "PAYMENT_FAILED")
		return
	}

	_, err = tx.Exec("UPDATE orders SET status = $1, updated_at = $2 WHERE order_no = $3", paymentStatus, time.Now(), req.OrderNo)
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Failed to update order status to PAID"), req.TraceID, req.OrderNo, r, startTime, "PAYMENT_FAILED")
		return
	}

	tx.Commit()
	duration := time.Since(startTime).Milliseconds()

	slog.Info("Payment completed successfully",
		"trace_id", req.TraceID,
		"order_no", req.OrderNo,
		"event_type", "PAYMENT_COMPLETED",
		"store_id", storeID,
		"lane_id", req.LaneID,
		"lane_type", laneType,
		"cashier_id", cashierID,
		"amount", req.Amount,
		"change_amount", change,
		"duration_ms", duration,
		"dependency", "postgresql",
		"dependency_status", "UP",
	)

	resp := models.Payment{
		OrderNo:      req.OrderNo,
		TraceID:      req.TraceID,
		StoreID:      storeID,
		LaneID:       req.LaneID,
		Amount:       req.Amount,
		CashReceived: req.CashReceived,
		ChangeAmount: change,
		Status:       paymentStatus,
	}

	writeJSON(w, http.StatusOK, resp)
}
