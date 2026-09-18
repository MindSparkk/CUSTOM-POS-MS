package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
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
	logger.InitLogger("order-service")
	slog.Info("Starting order service on port 8081")

	initDB()

	r := mux.NewRouter()
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(models.HealthResponse{Service: "order-service", Status: "UP"})
	}).Methods("GET")

	r.HandleFunc("/orders", createOrderHandler).Methods("POST")
	r.HandleFunc("/orders/{orderNo}", getOrderHandler).Methods("GET")
	r.HandleFunc("/orders/{orderNo}/items", addItemHandler).Methods("POST")
	r.HandleFunc("/orders/{orderNo}/items/{sku}", voidItemHandler).Methods("DELETE")
	r.HandleFunc("/orders/{orderNo}/calculate", calculateOrderHandler).Methods("POST")

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

func generateID(prefix string) string {
	return fmt.Sprintf("%s-%d%04d", prefix, time.Now().Unix(), rand.Intn(10000))
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

func createOrderHandler(w http.ResponseWriter, r *http.Request) {
	orderNo := generateID("ORD")
	traceID := generateID("TRC")

	order := models.Order{
		OrderNo:   orderNo,
		TraceID:   traceID,
		Status:    "OPEN",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := db.QueryRow(
		"INSERT INTO orders (order_no, trace_id, status, subtotal, discount, tax, total) VALUES ($1, $2, $3, 0, 0, 0, 0) RETURNING id",
		order.OrderNo, order.TraceID, order.Status,
	).Scan(&order.ID)

	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Failed to create order"), traceID, orderNo)
		return
	}

	slog.Info("Order created", "trace_id", traceID, "order_no", orderNo)
	writeJSON(w, http.StatusCreated, order)
}

func getOrderHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orderNo := vars["orderNo"]

	var order models.Order
	err := db.QueryRow(
		"SELECT id, order_no, trace_id, status, subtotal, discount, tax, total, created_at, updated_at FROM orders WHERE order_no = $1",
		orderNo,
	).Scan(&order.ID, &order.OrderNo, &order.TraceID, &order.Status, &order.Subtotal, &order.Discount, &order.Tax, &order.Total, &order.CreatedAt, &order.UpdatedAt)

	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrOrderNotFound, "Order not found"), "", orderNo)
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Database error"), "", orderNo)
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
	vars := mux.Vars(r)
	orderNo := vars["orderNo"]

	var req models.AddItemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrInvalidRequest, "Invalid request body"), "", orderNo)
		return
	}

	var orderID int
	var traceID, status string
	err := db.QueryRow("SELECT id, trace_id, status FROM orders WHERE order_no = $1", orderNo).Scan(&orderID, &traceID, &status)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrOrderNotFound, "Order not found"), "", orderNo)
		return
	}
	if status != "OPEN" {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrInvalidRequest, "Order is not OPEN"), traceID, orderNo)
		return
	}

	var item models.Item
	var promoCode sql.NullString
	var discType sql.NullString
	var discVal sql.NullFloat64

	err = db.QueryRow("SELECT id, sku, name, category, price, tax_rate, promo_code, discount_type, discount_value FROM items WHERE sku = $1", req.SKU).
		Scan(&item.ID, &item.SKU, &item.Name, &item.Category, &item.Price, &item.TaxRate, &promoCode, &discType, &discVal)
	if err == sql.ErrNoRows {
		slog.Error("Item lookup failed: Not found in catalog", 
			"trace_id", traceID, 
			"order_no", orderNo, 
			"sku", req.SKU, 
			"requested_name", req.Name, 
			"requested_category", req.Category, 
			"requested_price", req.UnitPrice)
		writeError(w, http.StatusNotFound, errors.New(errors.ErrItemNotFound, "Item not found"), traceID, orderNo)
		return
	}

	unitPrice := item.Price
	quantity := float64(req.Quantity)
	
	// Calculate discount
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
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Failed to add item"), traceID, orderNo)
		return
	}

	slog.Info("Item successfully added to cart", 
		"trace_id", traceID, 
		"order_no", orderNo, 
		"sku", item.SKU, 
		"name", item.Name, 
		"quantity", req.Quantity, 
		"unit_price", unitPrice,
		"tax_rate", item.TaxRate,
		"discount_applied", discount,
		"line_total", total)
	
	// Also trigger recalculation of order total
	calculateOrderTotals(orderID, traceID, orderNo)

	writeJSON(w, http.StatusOK, map[string]string{"status": "Item added successfully"})
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

// Let's rewrite calculateOrderTotals and calculateOrderHandler.
func calculateOrderHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orderNo := vars["orderNo"]

	var orderID int
	var traceID, status string
	err := db.QueryRow("SELECT id, trace_id, status FROM orders WHERE order_no = $1", orderNo).Scan(&orderID, &traceID, &status)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrOrderNotFound, "Order not found"), "", orderNo)
		return
	}

	rows, err := db.Query("SELECT quantity, unit_price, discount, tax, total FROM order_items WHERE order_id = $1", orderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Failed to fetch items"), traceID, orderNo)
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

	_, err = db.Exec(
		"UPDATE orders SET subtotal = $1, discount = $2, tax = $3, total = $4, updated_at = $5 WHERE id = $6",
		totalSub, totalDisc, totalTax, grandTotal, time.Now(), orderID,
	)
	
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Failed to update order"), traceID, orderNo)
		return
	}

	slog.Info("Order total calculated", "trace_id", traceID, "order_no", orderNo, "subtotal", totalSub, "discount", totalDisc, "tax", totalTax, "grand_total", grandTotal)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"order_no": orderNo,
		"trace_id": traceID,
		"subtotal": totalSub,
		"discount": totalDisc,
		"tax":      totalTax,
		"total":    grandTotal,
	})
}

func voidItemHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	orderNo := vars["orderNo"]
	sku := vars["sku"]

	var orderID int
	var traceID, status string
	err := db.QueryRow("SELECT id, trace_id, status FROM orders WHERE order_no = $1", orderNo).Scan(&orderID, &traceID, &status)
	if err == sql.ErrNoRows {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrOrderNotFound, "Order not found"), "", orderNo)
		return
	}
	if status != "OPEN" {
		writeError(w, http.StatusBadRequest, errors.New(errors.ErrInvalidRequest, "Cannot void items unless order is OPEN"), traceID, orderNo)
		return
	}

	// Delete all instances of this SKU from the cart
	res, err := db.Exec("DELETE FROM order_items WHERE order_id = $1 AND sku = $2", orderID, sku)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New(errors.ErrInternalError, "Failed to void item"), traceID, orderNo)
		return
	}

	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		writeError(w, http.StatusNotFound, errors.New(errors.ErrItemNotFound, "Item not found in cart"), traceID, orderNo)
		return
	}

	slog.Info("Item voided from cart", "trace_id", traceID, "order_no", orderNo, "sku", sku)

	// Recalculate totals
	calculateOrderTotals(orderID, traceID, orderNo)

	writeJSON(w, http.StatusOK, map[string]string{"status": "Item voided successfully"})
}
