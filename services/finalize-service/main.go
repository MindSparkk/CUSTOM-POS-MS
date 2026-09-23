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
	db                *sql.DB
	simFailureMutex   sync.RWMutex
	activeSimFailures = make(map[string]bool)
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
	logger.InitLogger("finalize-service")
	slog.Info("Starting finalize service on port 8084")

	initDB()

	r := mux.NewRouter()

	// Health & Readiness
	r.HandleFunc("/health", healthHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/ready", readinessHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/simulation/failures", failureInjectionHandler).Methods("POST", "OPTIONS")

	// Finalization & Receipts
	r.HandleFunc("/finalize", finalizeHandler).Methods("POST", "OPTIONS")
	r.HandleFunc("/orders/{orderNo}/receipt", receiptHandler).Methods("GET", "OPTIONS")

	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "DELETE", "PUT", "OPTIONS"},
		AllowedHeaders: []string{"*"},
	})

	srv := &http.Server{
		Handler:      c.Handler(r),
		Addr:         ":8084",
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

	slog.Error("Finalize request failed",
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
		Service:   "finalize-service",
		Status:    "UP",
		Version:   "1.0.0",
		Timestamp: time.Now(),
	})
}

func readinessHandler(w http.ResponseWriter, r *http.Request) {
	dbStatus := "UP"
	status := "READY"
	httpCode := http.StatusOK

	if db == nil || db.Ping() != nil || checkSimFailure("DATABASE_UNAVAILABLE") {
		dbStatus = "DOWN"
		status = "NOT_READY"
		httpCode = http.StatusServiceUnavailable
	}

	writeJSON(w, httpCode, models.ReadinessResponse{
		Service:   "finalize-service",
		Status:    status,
		Timestamp: time.Now(),
		Dependencies: map[string]string{
			"postgresql": dbStatus,
		},
	})
}

func failureInjectionHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type    string `json:"type"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	simFailureMutex.Lock()
	activeSimFailures[req.Type] = req.Enabled
	simFailureMutex.Unlock()

	slog.Warn("Finalize simulation failure state updated", "type", req.Type, "enabled", req.Enabled)
	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "updated", "type": req.Type, "enabled": req.Enabled})
}

func finalizeHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	storeID, laneID, laneType, cashierID := getLaneContext(r)

	var req models.FinalizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrInvalidRequest, "Invalid request body"), "", "", r, startTime, "FINALIZE_FAILED")
		return
	}

	if checkSimFailure("DATABASE_UNAVAILABLE") {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrDatabaseUnavailable, "Database connection failed during finalization"), req.TraceID, req.OrderNo, r, startTime, "DATABASE_CONNECTION_FAILED")
		return
	}

	slog.Info("Order finalization started",
		"trace_id", req.TraceID,
		"order_no", req.OrderNo,
		"event_type", "FINALIZE_STARTED",
		"store_id", storeID,
		"lane_id", laneID,
		"lane_type", laneType,
		"cashier_id", cashierID,
	)

	var orderID int
	var status string
	err := db.QueryRow("SELECT id, status FROM orders WHERE order_no = $1", req.OrderNo).Scan(&orderID, &status)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrOrderNotFound, "Order not found"), req.TraceID, req.OrderNo, r, startTime, "FINALIZE_FAILED")
		return
	}

	if status != "PAID" {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrOrderNotPaid, "Order must be PAID before finalization"), req.TraceID, req.OrderNo, r, startTime, "FINALIZE_FAILED")
		return
	}

	tx, err := db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Database error"), req.TraceID, req.OrderNo, r, startTime, "FINALIZE_FAILED")
		return
	}

	rows, err := tx.Query("SELECT sku, quantity FROM order_items WHERE order_id = $1", orderID)
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Failed to fetch order items"), req.TraceID, req.OrderNo, r, startTime, "FINALIZE_FAILED")
		return
	}

	type itemStock struct {
		sku      string
		quantity int
	}
	var items []itemStock
	for rows.Next() {
		var i itemStock
		if err := rows.Scan(&i.sku, &i.quantity); err == nil {
			items = append(items, i)
		}
	}
	rows.Close()

	for _, item := range items {
		res, err := tx.Exec("UPDATE items SET stock_quantity = stock_quantity - $1 WHERE sku = $2", item.quantity, item.sku)
		if err != nil {
			tx.Rollback()
			writeError(w, http.StatusInternalServerError, errors.New(errors.ErrFinalizeFailed, "Failed to update stock quantity"), req.TraceID, req.OrderNo, r, startTime, "FINALIZE_FAILED")
			return
		}
		rowsAffected, _ := res.RowsAffected()
		if rowsAffected == 0 {
			tx.Rollback()
			writeError(w, http.StatusInternalServerError, errors.New(errors.ErrFinalizeFailed, fmt.Sprintf("Stock update failed for SKU %s", item.sku)), req.TraceID, req.OrderNo, r, startTime, "FINALIZE_FAILED")
			return
		}
	}

	// Mark order as COMPLETED
	_, err = tx.Exec("UPDATE orders SET status = 'COMPLETED', updated_at = $1 WHERE id = $2", time.Now(), orderID)
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrFinalizeFailed, "Failed to update order status to COMPLETED"), req.TraceID, req.OrderNo, r, startTime, "FINALIZE_FAILED")
		return
	}

	tx.Commit()
	duration := time.Since(startTime).Milliseconds()

	slog.Info("Order completed successfully",
		"trace_id", req.TraceID,
		"order_no", req.OrderNo,
		"event_type", "FINALIZE_COMPLETED",
		"store_id", storeID,
		"lane_id", laneID,
		"lane_type", laneType,
		"cashier_id", cashierID,
		"duration_ms", duration,
		"dependency", "postgresql",
		"dependency_status", "UP",
	)

	resp := models.FinalizeResponse{
		OrderNo: req.OrderNo,
		TraceID: req.TraceID,
		Status:  "COMPLETED",
	}

	writeJSON(w, http.StatusOK, resp)
}

func receiptHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orderNo := vars["orderNo"]
	storeID, laneID, laneType, cashierID := getLaneContext(r)

	var orderID int
	var traceID, status string
	var subtotal, discount, tax, total float64
	var updatedAt time.Time

	err := db.QueryRow("SELECT id, trace_id, status, subtotal, discount, tax, total, updated_at FROM orders WHERE order_no = $1", orderNo).
		Scan(&orderID, &traceID, &status, &subtotal, &discount, &tax, &total, &updatedAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrOrderNotFound, "Order not found"), "", orderNo, r, time.Now(), "RECEIPT_FAILED")
		return
	}

	if status != "COMPLETED" {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrInvalidRequest, "Order must be COMPLETED to generate receipt"), traceID, orderNo, r, time.Now(), "RECEIPT_FAILED")
		return
	}

	rows, err := db.Query("SELECT name, quantity, total FROM order_items WHERE order_id = $1", orderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Failed to load order items"), traceID, orderNo, r, time.Now(), "RECEIPT_FAILED")
		return
	}
	defer rows.Close()

	receipt := fmt.Sprintf("================================\n")
	receipt += fmt.Sprintf("     RETAIL POS - %s      \n", storeID)
	receipt += fmt.Sprintf("     LANE: %s (%s)\n", laneID, laneType)
	receipt += fmt.Sprintf("     CASHIER: %s\n", cashierID)
	receipt += fmt.Sprintf("================================\n")
	receipt += fmt.Sprintf("Order No: %s\n", orderNo)
	receipt += fmt.Sprintf("Date: %s\n", updatedAt.Format("2006-01-02 15:04:05"))
	receipt += fmt.Sprintf("--------------------------------\n")

	for rows.Next() {
		var name string
		var quantity int
		var itemTotal float64
		rows.Scan(&name, &quantity, &itemTotal)
		receipt += fmt.Sprintf("%dx %-20s $ %6.2f\n", quantity, name, itemTotal)
	}

	receipt += fmt.Sprintf("--------------------------------\n")
	receipt += fmt.Sprintf("Subtotal:             $ %6.2f\n", subtotal)
	receipt += fmt.Sprintf("Discount:            -$ %6.2f\n", discount)
	receipt += fmt.Sprintf("Tax:                  $ %6.2f\n", tax)
	receipt += fmt.Sprintf("--------------------------------\n")
	receipt += fmt.Sprintf("GRAND TOTAL:          $ %6.2f\n", total)
	receipt += fmt.Sprintf("================================\n")

	writeJSON(w, http.StatusOK, map[string]string{
		"order_no":     orderNo,
		"receipt_text": receipt,
	})
}
