package main

import (
	"database/sql"
	"encoding/json"
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
	logger.InitLogger("payment-service")
	slog.Info("Starting payment service on port 8083")

	initDB()

	r := mux.NewRouter()
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(models.HealthResponse{Service: "payment-service", Status: "UP"})
	}).Methods("GET")

	r.HandleFunc("/payments", paymentHandler).Methods("POST")

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

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, posErr *errors.POSError, traceID, orderNo string) {
	slog.Error("Request failed", "error_code", posErr.Code, "message", posErr.Message, "trace_id", traceID, "order_no", orderNo)
	writeJSON(w, status, posErr)
}

func paymentHandler(w http.ResponseWriter, r *http.Request) {
	var req models.Payment
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrInvalidRequest, "Invalid request body"), "", "")
		return
	}

	slog.Info("Payment initiated", "trace_id", req.TraceID, "order_no", req.OrderNo, "amount", req.Amount, "cash_received", req.CashReceived)

	var status string
	var total float64
	err := db.QueryRow("SELECT status, total FROM orders WHERE order_no = $1", req.OrderNo).Scan(&status, &total)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrOrderNotFound, "Order not found"), req.TraceID, req.OrderNo)
		return
	}
	if status != "OPEN" {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrInvalidRequest, "Order is not OPEN for payment"), req.TraceID, req.OrderNo)
		return
	}
	if req.Amount != total {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrInvalidRequest, "Payment amount does not match order total"), req.TraceID, req.OrderNo)
		return
	}

	if req.CashReceived < req.Amount {
		slog.Error("Payment rejected: Insufficient cash received", 
			"trace_id", req.TraceID, 
			"order_no", req.OrderNo,
			"amount_due", req.Amount,
			"cash_received", req.CashReceived,
			"shortfall", req.Amount - req.CashReceived)
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrInsufficientCash, "Insufficient cash received"), req.TraceID, req.OrderNo)
		return
	}

	slog.Info("Cash received", "trace_id", req.TraceID, "order_no", req.OrderNo)

	change := req.CashReceived - req.Amount
	paymentStatus := "PAID"

	tx, err := db.Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Database error"), req.TraceID, req.OrderNo)
		return
	}

	_, err = tx.Exec(
		"INSERT INTO payments (order_no, trace_id, amount, cash_received, change_amount, status) VALUES ($1, $2, $3, $4, $5, $6)",
		req.OrderNo, req.TraceID, req.Amount, req.CashReceived, change, paymentStatus,
	)
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Failed to record payment"), req.TraceID, req.OrderNo)
		return
	}

	_, err = tx.Exec("UPDATE orders SET status = $1, updated_at = $2 WHERE order_no = $3", paymentStatus, time.Now(), req.OrderNo)
	if err != nil {
		tx.Rollback()
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Failed to update order status"), req.TraceID, req.OrderNo)
		return
	}

	tx.Commit()

	slog.Info("Payment successful", "trace_id", req.TraceID, "order_no", req.OrderNo, "change", change)

	resp := models.Payment{
		OrderNo:      req.OrderNo,
		TraceID:      req.TraceID,
		Amount:       req.Amount,
		CashReceived: req.CashReceived,
		ChangeAmount: change,
		Status:       paymentStatus,
	}

	writeJSON(w, http.StatusOK, resp)
}
