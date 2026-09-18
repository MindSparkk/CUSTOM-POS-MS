package models

import "time"

type POSLog struct {
	Timestamp time.Time              `json:"timestamp"`
	TraceID   string                 `json:"trace_id"`
	OrderNo   string                 `json:"order_no"`
	Service   string                 `json:"service"`
	Level     string                 `json:"level"`
	ErrorCode string                 `json:"error_code,omitempty"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type Item struct {
	ID            int     `json:"id"`
	SKU           string  `json:"sku"`
	Name          string  `json:"name"`
	Category      string  `json:"category"`
	Price         float64 `json:"price"`
	TaxRate       float64 `json:"tax_rate"`
	StockQuantity int     `json:"stock_quantity"`
	PromoCode     string  `json:"promo_code,omitempty"`
	DiscountType  string  `json:"discount_type,omitempty"`
	DiscountValue float64 `json:"discount_value,omitempty"`
}

type Order struct {
	ID        int         `json:"id,omitempty"`
	OrderNo   string      `json:"order_no"`
	TraceID   string      `json:"trace_id"`
	Status    string      `json:"status"`
	Subtotal  float64     `json:"subtotal"`
	Discount  float64     `json:"discount"`
	Tax       float64     `json:"tax"`
	Total     float64     `json:"total"`
	Items     []OrderItem `json:"items,omitempty"`
	CreatedAt time.Time   `json:"created_at,omitempty"`
	UpdatedAt time.Time   `json:"updated_at,omitempty"`
}

type OrderItem struct {
	ID        int     `json:"id,omitempty"`
	OrderID   int     `json:"order_id,omitempty"`
	SKU       string  `json:"sku"`
	Name      string  `json:"name"`
	Category  string  `json:"category"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
	Discount  float64 `json:"discount"`
	Tax       float64 `json:"tax"`
	Total     float64 `json:"total"`
}

type Payment struct {
	ID           int       `json:"id,omitempty"`
	OrderNo      string    `json:"order_no"`
	TraceID      string    `json:"trace_id"`
	Amount       float64   `json:"amount"`
	CashReceived float64   `json:"cash_received"`
	ChangeAmount float64   `json:"change_amount"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
}

type FinalizeRequest struct {
	OrderNo string `json:"order_no"`
	TraceID string `json:"trace_id"`
}

type FinalizeResponse struct {
	OrderNo string `json:"order_no"`
	TraceID string `json:"trace_id"`
	Status  string `json:"status"`
}

type AddItemRequest struct {
	SKU       string  `json:"sku"`
	Name      string  `json:"name,omitempty"`
	Category  string  `json:"category,omitempty"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price,omitempty"`
}

type HealthResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}
