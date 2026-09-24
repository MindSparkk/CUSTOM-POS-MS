package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"retail-pos/pkg/config"
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
	logger.InitLogger("order-service")
	slog.Info("Starting order service on port 8081")

	initDB()

	r := mux.NewRouter()

	// Health & Readiness
	r.HandleFunc("/health", healthHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/ready", readinessHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/simulation/failures", failureInjectionHandler).Methods("POST", "OPTIONS")
	r.HandleFunc("/admin/restart", adminRestartHandler).Methods("POST", "OPTIONS")

	// Order Operations
	r.HandleFunc("/orders", createOrderHandler).Methods("POST", "OPTIONS")
	r.HandleFunc("/orders/{orderNo}", getOrderHandler).Methods("GET", "OPTIONS")
	r.HandleFunc("/orders/{orderNo}/items", addItemHandler).Methods("POST", "OPTIONS")
	r.HandleFunc("/orders/{orderNo}/items/{sku}", voidItemHandler).Methods("DELETE", "OPTIONS")
	r.HandleFunc("/orders/{orderNo}/calculate", calculateOrderHandler).Methods("POST", "OPTIONS")

	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "DELETE", "PUT", "OPTIONS"},
		AllowedHeaders: []string{"*"},
	})

	srv := &http.Server{
		Handler:      c.Handler(r),
		Addr:         ":8081",
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

func generateID(prefix string) string {
	return fmt.Sprintf("%s-%d%04d", prefix, time.Now().Unix(), rand.Intn(10000))
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, posErr *errors.POSError, traceID, orderNo string, r *http.Request, startTime time.Time, eventType string) {
	duration := time.Since(startTime).Milliseconds()
	storeID, laneID, laneType, cashierID := getLaneContext(r)
	
	slog.Error("Request failed",
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
		Service:   "order-service",
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
		Service:   "order-service",
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
	if req.Type == "LATENCY_SPIKE" {
		simLatencyDelayMs = req.DurationMS
	}
	simFailureMutex.Unlock()

	slog.Warn("Simulation failure state updated", "type", req.Type, "enabled", req.Enabled, "duration_ms", req.DurationMS)
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

func createOrderHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	storeID, laneID, laneType, cashierID := getLaneContext(r)
	orderNo := generateID("ORD")
	traceID := generateID("TRC")

	if checkSimFailure("DATABASE_UNAVAILABLE") {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrDatabaseUnavailable, "Database connection unavailable"), traceID, orderNo, r, startTime, "DATABASE_CONNECTION_FAILED")
		return
	}

	order := models.Order{
		OrderNo:   orderNo,
		TraceID:   traceID,
		StoreID:   storeID,
		LaneID:    laneID,
		LaneType:  laneType,
		CashierID: cashierID,
		Status:    "OPEN",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := db.QueryRow(
		"INSERT INTO orders (order_no, trace_id, store_id, lane_id, lane_type, cashier_id, status, subtotal, discount, tax, total) VALUES ($1, $2, $3, $4, $5, $6, $7, 0, 0, 0, 0) RETURNING id",
		order.OrderNo, order.TraceID, order.StoreID, order.LaneID, order.LaneType, order.CashierID, order.Status,
	).Scan(&order.ID)

	duration := time.Since(startTime).Milliseconds()

	if err != nil {
		slog.Error("Failed to insert order into database", "raw_error", err.Error())
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Failed to create order"), traceID, orderNo, r, startTime, "ORDER_CREATE_FAILED")
		return
	}

	slog.Info("Order created successfully",
		"trace_id", traceID,
		"order_no", orderNo,
		"event_type", "ORDER_CREATED",
		"store_id", storeID,
		"lane_id", laneID,
		"lane_type", laneType,
		"cashier_id", cashierID,
		"duration_ms", duration,
		"dependency", "postgresql",
		"dependency_status", "UP",
	)

	writeJSON(w, http.StatusCreated, order)
}

func getOrderHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orderNo := vars["orderNo"]

	var order models.Order
	err := db.QueryRow(
		"SELECT id, order_no, trace_id, store_id, lane_id, lane_type, cashier_id, status, subtotal, discount, tax, total, created_at, updated_at FROM orders WHERE order_no = $1",
		orderNo,
	).Scan(&order.ID, &order.OrderNo, &order.TraceID, &order.StoreID, &order.LaneID, &order.LaneType, &order.CashierID, &order.Status, &order.Subtotal, &order.Discount, &order.Tax, &order.Total, &order.CreatedAt, &order.UpdatedAt)

	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrOrderNotFound, "Order not found"), "", orderNo, r, time.Now(), "ORDER_LOOKUP_FAILED")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Database query error"), "", orderNo, r, time.Now(), "DATABASE_ERROR")
		return
	}

	// Fetch items
	rows, err := db.Query("SELECT id, sku, name, category, quantity, unit_price, discount, tax, total FROM order_items WHERE order_id = $1", order.ID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var item models.OrderItem
			if err := rows.Scan(&item.ID, &item.SKU, &item.Name, &item.Category, &item.Quantity, &item.UnitPrice, &item.Discount, &item.Tax, &item.Total); err == nil {
				item.OrderID = order.ID
				order.Items = append(order.Items, item)
			}
		}
	}

	writeJSON(w, http.StatusOK, order)
}

func addItemHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	vars := mux.Vars(r)
	orderNo := vars["orderNo"]
	storeID, laneID, laneType, cashierID := getLaneContext(r)

	var req models.AddItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrInvalidRequest, "Invalid request body"), "", orderNo, r, startTime, "ITEM_ADD_FAILED")
		return
	}

	var orderID int
	var traceID, status string
	err := db.QueryRow("SELECT id, trace_id, status FROM orders WHERE order_no = $1", orderNo).Scan(&orderID, &traceID, &status)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrOrderNotFound, "Order not found"), "", orderNo, r, startTime, "ITEM_ADD_FAILED")
		return
	}
	if status != "OPEN" {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrInvalidRequest, "Order is not OPEN"), traceID, orderNo, r, startTime, "ITEM_ADD_FAILED")
		return
	}

	// Enforce max_items_limit from config.json for the active lane
	appConfig := config.GetConfig()
	var laneMaxItems int
	for _, l := range appConfig.Lanes {
		if l.LaneID == laneID {
			laneMaxItems = l.MaxItemsLimit
			break
		}
	}

	if laneMaxItems > 0 {
		var currentQtySum int
		db.QueryRow("SELECT COALESCE(SUM(quantity), 0) FROM order_items WHERE order_id = $1", orderID).Scan(&currentQtySum)
		if currentQtySum+req.Quantity > laneMaxItems {
			writeError(w, http.StatusBadRequest, errors.New(errors.ErrInvalidRequest, fmt.Sprintf("Lane %s item limit exceeded (%d max items allowed)", laneID, laneMaxItems)), traceID, orderNo, r, startTime, "ITEM_LIMIT_EXCEEDED")
			return
		}
	}

	var item models.Item
	var promoCode, discType sql.NullString
	var discVal sql.NullFloat64

	err = db.QueryRow("SELECT id, sku, name, category, price, tax_rate, promo_code, discount_type, discount_value FROM items WHERE sku = $1", req.SKU).
		Scan(&item.ID, &item.SKU, &item.Name, &item.Category, &item.Price, &item.TaxRate, &promoCode, &discType, &discVal)

	if err == sql.ErrNoRows || checkSimFailure("STALE_CATALOG") {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrItemNotFound, fmt.Sprintf("Item SKU %s not found in catalog", req.SKU)), traceID, orderNo, r, startTime, "ITEM_LOOKUP_FAILED")
		return
	}

	unitPrice := item.Price
	quantity := float64(req.Quantity)
	
	var discount float64
	if discType.Valid {
		if discType.String == "PERCENTAGE" {
			discount = (unitPrice * quantity) * (discVal.Float64 / 100)
		} else if discType.String == "FIXED" {
			discount = discVal.Float64 * quantity
		}
	}

	subtotal := unitPrice * quantity
	taxableAmount := subtotal - discount
	if taxableAmount < 0 {
		taxableAmount = 0
	}
	tax := taxableAmount * (item.TaxRate / 100)
	total := taxableAmount + tax

	orderItem := models.OrderItem{
		OrderID:   orderID,
		SKU:       item.SKU,
		Name:      item.Name,
		Category:  item.Category,
		Quantity:  req.Quantity,
		UnitPrice: unitPrice,
		Discount:  discount,
		Tax:       tax,
		Total:     total,
	}

	_, err = db.Exec(
		"INSERT INTO order_items (order_id, sku, name, category, quantity, unit_price, discount, tax, total) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)",
		orderItem.OrderID, orderItem.SKU, orderItem.Name, orderItem.Category, orderItem.Quantity, orderItem.UnitPrice, orderItem.Discount, orderItem.Tax, orderItem.Total,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Failed to add item to database"), traceID, orderNo, r, startTime, "ITEM_ADD_FAILED")
		return
	}

	calculateOrderTotals(orderID, traceID, orderNo)
	duration := time.Since(startTime).Milliseconds()

	slog.Info("Item added to order",
		"trace_id", traceID,
		"order_no", orderNo,
		"event_type", "ITEM_ADDED",
		"store_id", storeID,
		"lane_id", laneID,
		"lane_type", laneType,
		"cashier_id", cashierID,
		"sku", item.SKU,
		"item_name", item.Name,
		"quantity", req.Quantity,
		"line_total", total,
		"duration_ms", duration,
	)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "Item added successfully",
		"order_no": orderNo,
		"sku":      item.SKU,
		"name":     item.Name,
		"quantity": req.Quantity,
		"total":    total,
	})
}

func calculateOrderTotals(orderID int, traceID, orderNo string) {
	rows, err := db.Query("SELECT quantity, unit_price, discount, tax, total FROM order_items WHERE order_id = $1", orderID)
	if err != nil {
		return
	}
	defer rows.Close()

	var totalSub, totalDisc, totalTax, grandTotal float64
	for rows.Next() {
		var qty int
		var unitPrice, disc, tax, tot float64
		if err := rows.Scan(&qty, &unitPrice, &disc, &tax, &tot); err == nil {
			totalSub += unitPrice * float64(qty)
			totalDisc += disc
			totalTax += tax
			grandTotal += tot
		}
	}

	db.Exec(
		"UPDATE orders SET subtotal = $1, discount = $2, tax = $3, total = $4, updated_at = $5 WHERE id = $6",
		totalSub, totalDisc, totalTax, grandTotal, time.Now(), orderID,
	)
}

func calculateOrderHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	vars := mux.Vars(r)
	orderNo := vars["orderNo"]
	storeID, laneID, laneType, cashierID := getLaneContext(r)

	var orderID int
	var traceID, status string
	err := db.QueryRow("SELECT id, trace_id, status FROM orders WHERE order_no = $1", orderNo).Scan(&orderID, &traceID, &status)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrOrderNotFound, "Order not found"), "", orderNo, r, startTime, "CALCULATE_FAILED")
		return
	}

	calculateOrderTotals(orderID, traceID, orderNo)

	var subtotal, discount, tax, total float64
	db.QueryRow("SELECT subtotal, discount, tax, total FROM orders WHERE id = $1", orderID).Scan(&subtotal, &discount, &tax, &total)

	duration := time.Since(startTime).Milliseconds()

	slog.Info("Order total calculated",
		"trace_id", traceID,
		"order_no", orderNo,
		"event_type", "ORDER_CALCULATED",
		"store_id", storeID,
		"lane_id", laneID,
		"lane_type", laneType,
		"cashier_id", cashierID,
		"subtotal", subtotal,
		"discount", discount,
		"tax", tax,
		"grand_total", total,
		"duration_ms", duration,
	)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"order_no":    orderNo,
		"trace_id":    traceID,
		"subtotal":    subtotal,
		"discount":    discount,
		"tax":         tax,
		"grand_total": total,
	})
}

func voidItemHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	vars := mux.Vars(r)
	orderNo := vars["orderNo"]
	sku := vars["sku"]
	storeID, laneID, laneType, cashierID := getLaneContext(r)

	var orderID int
	var traceID, status string
	err := db.QueryRow("SELECT id, trace_id, status FROM orders WHERE order_no = $1", orderNo).Scan(&orderID, &traceID, &status)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrOrderNotFound, "Order not found"), "", orderNo, r, startTime, "VOID_FAILED")
		return
	}

	res, err := db.Exec("DELETE FROM order_items WHERE order_id = $1 AND sku = $2", orderID, sku)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Failed to void item"), traceID, orderNo, r, startTime, "VOID_FAILED")
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrItemNotFound, "Item not found in cart"), traceID, orderNo, r, startTime, "VOID_FAILED")
		return
	}

	calculateOrderTotals(orderID, traceID, orderNo)
	duration := time.Since(startTime).Milliseconds()

	slog.Info("Item voided from cart",
		"trace_id", traceID,
		"order_no", orderNo,
		"event_type", "ITEM_VOIDED",
		"store_id", storeID,
		"lane_id", laneID,
		"lane_type", laneType,
		"cashier_id", cashierID,
		"sku", sku,
		"duration_ms", duration,
	)

	writeJSON(w, http.StatusOK, map[string]string{"status": "Item voided successfully"})
}
