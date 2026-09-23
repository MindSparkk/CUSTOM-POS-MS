package models

import "time"

type POSLog struct {
	Timestamp        time.Time              `json:"timestamp"`
	IncidentID       string                 `json:"incident_id,omitempty"`
	TraceID          string                 `json:"trace_id"`
	OrderNo          string                 `json:"order_no,omitempty"`
	Service          string                 `json:"service"`
	Operation        string                 `json:"operation,omitempty"`
	Level            string                 `json:"level"`
	EventType        string                 `json:"event_type,omitempty"`
	Category         string                 `json:"category,omitempty"` // BUSINESS vs INFRASTRUCTURE
	HTTPStatus       int                    `json:"http_status,omitempty"`
	ErrorCode        string                 `json:"error_code,omitempty"`
	Message          string                 `json:"message"`
	Dependency       string                 `json:"dependency,omitempty"`
	DependencyStatus string                 `json:"dependency_status,omitempty"`
	DurationMS       int64                  `json:"duration_ms,omitempty"`
	StoreID          string                 `json:"store_id,omitempty"`
	LaneID           string                 `json:"lane_id,omitempty"`
	LaneType         string                 `json:"lane_type,omitempty"`
	CashierID        string                 `json:"cashier_id,omitempty"`
	Environment      string                 `json:"environment,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
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
	StoreID   string      `json:"store_id,omitempty"`
	LaneID    string      `json:"lane_id,omitempty"`
	LaneType  string      `json:"lane_type,omitempty"`
	CashierID string      `json:"cashier_id,omitempty"`
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
	StoreID      string    `json:"store_id,omitempty"`
	LaneID       string    `json:"lane_id,omitempty"`
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
	Service   string    `json:"service"`
	Status    string    `json:"status"`
	Version   string    `json:"version"`
	Timestamp time.Time `json:"timestamp"`
}

type ReadinessResponse struct {
	Service      string            `json:"service"`
	Status       string            `json:"status"` // READY or NOT_READY
	Dependencies map[string]string `json:"dependencies"`
	Timestamp    time.Time         `json:"timestamp"`
}
