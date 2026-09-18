package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"retail-pos/pkg/errors"
	"retail-pos/pkg/logger"
	"retail-pos/pkg/models"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	_ "github.com/lib/pq"
)

var db *sql.DB

func initDB() {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://postgres:postgres@localhost:5432/posdb?sslmode=disable"
	}
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		slog.Error("Failed to connect to database", "error", err.Error())
		os.Exit(1)
	}
	if err := db.Ping(); err != nil {
		slog.Error("Database unreachable", "error", err.Error())
		os.Exit(1)
	}
	slog.Info("Connected to database")
}

func main() {
	logger.InitLogger("finalize-service")
	slog.Info("Starting finalize service on port 8084")

	initDB()

	r := mux.NewRouter()
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(models.HealthResponse{Service: "finalize-service", Status: "UP"})
	}).Methods("GET")

	r.HandleFunc("/finalize", finalizeHandler).Methods("POST")
	r.HandleFunc("/orders/{orderNo}/receipt", receiptHandler).Methods("GET")

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

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, posErr *errors.POSError, traceID, orderNo string) {
	slog.Error("Request failed", "error_code", posErr.Code, "message", posErr.Message, "trace_id", traceID, "order_no", orderNo)
	writeJSON(w, status, posErr)
}

func finalizeHandler(w http.ResponseWriter, r *http.Request) {
	var req models.FinalizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrInvalidRequest, "Invalid request body"), "", "")
		return
	}

	slog.Info("Finalization started", "trace_id", req.TraceID, "order_no", req.OrderNo)

	var orderID int
	var status string
	err := db.QueryRow("SELECT id, status FROM orders WHERE order_no = $1", req.OrderNo).Scan(&orderID, &status)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrOrderNotFound, "Order not found"), req.TraceID, req.OrderNo)
		return
	}
	
	if status != "PAID" {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrOrderNotPaid, "Order must be PAID to finalize"), req.TraceID, req.OrderNo)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Database error"), req.TraceID, req.OrderNo)
		return
	}

	// Update stock quantities
	rows, err := tx.Query("SELECT sku, quantity FROM order_items WHERE order_id = $1", orderID)
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Failed to fetch order items"), req.TraceID, req.OrderNo)
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
		_, err = tx.Exec("UPDATE items SET stock_quantity = stock_quantity - $1 WHERE sku = $2", item.quantity, item.sku)
		if err != nil {
			tx.Rollback()
			writeError(w, http.StatusInternalServerError, errors.New(errors.ErrFinalizeFailed, "Failed to update stock for item"), req.TraceID, req.OrderNo)
			return
		}
		slog.Info("Stock updated", "trace_id", req.TraceID, "order_no", req.OrderNo, "sku", item.sku, "quantity_decreased", item.quantity)
	}

	// Mark order as COMPLETED
	_, err = tx.Exec("UPDATE orders SET status = 'COMPLETED', updated_at = $1 WHERE id = $2", time.Now(), orderID)
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrFinalizeFailed, "Failed to update order status"), req.TraceID, req.OrderNo)
		return
	}

	tx.Commit()

	slog.Info("Order completed", "trace_id", req.TraceID, "order_no", req.OrderNo)

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

	var orderID int
	var traceID, status string
	var subtotal, discount, tax, total float64
	var updatedAt time.Time

	err := db.QueryRow("SELECT id, trace_id, status, subtotal, discount, tax, total, updated_at FROM orders WHERE order_no = $1", orderNo).
		Scan(&orderID, &traceID, &status, &subtotal, &discount, &tax, &total, &updatedAt)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrOrderNotFound, "Order not found"), "", orderNo)
		return
	}

	if status != "COMPLETED" {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrInvalidRequest, "Order must be COMPLETED to generate receipt"), traceID, orderNo)
		return
	}

	// Fetch items
	rows, err := db.Query("SELECT name, quantity, total FROM order_items WHERE order_id = $1", orderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Failed to load order items"), traceID, orderNo)
		return
	}
	defer rows.Close()

	// Build receipt string
	receipt := fmt.Sprintf("================================\n")
	receipt += fmt.Sprintf("           RETAIL POS           \n")
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

	slog.Info("Receipt generated", "trace_id", traceID, "order_no", orderNo)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"order_no":     orderNo,
		"receipt_text": receipt,
	})
}
