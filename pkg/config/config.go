package config

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

type StoreConfig struct {
	StoreID        string  `json:"store_id"`
	StoreName      string  `json:"store_name"`
	Currency       string  `json:"currency"`
	DefaultTaxRate float64 `json:"default_tax_rate"`
	Environment    string  `json:"environment"`
}

type LaneConfig struct {
	LaneID        string `json:"lane_id"`
	LaneName      string `json:"lane_name"`
	LaneType      string `json:"lane_type"`
	CashierID     string `json:"cashier_id"`
	Icon          string `json:"icon"`
	MaxItemsLimit int    `json:"max_items_limit"`
}

type TelemetryConfig struct {
	TelemetryServiceURL string `json:"telemetry_service_url"`
	AIBackendURL        string `json:"ai_backend_url"`
	LogLevel            string `json:"log_level"`
	BufferSize          int    `json:"buffer_size"`
}

type PoliciesConfig struct {
	AllowNegativeStock    bool `json:"allow_negative_stock"`
	RequirePaidToFinalize bool `json:"require_paid_to_finalize"`
	StrictSKULookup       bool `json:"strict_sku_lookup"`
	MaxPaymentRetries     int  `json:"max_payment_retries"`
}

type CashierConfig struct {
	CashierID string `json:"cashier_id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Avatar    string `json:"avatar"`
}

type AppConfig struct {
	Store     StoreConfig     `json:"store"`
	Cashiers  []CashierConfig `json:"cashiers"`
	Lanes     []LaneConfig    `json:"lanes"`
	Telemetry TelemetryConfig `json:"telemetry"`
	Policies  PoliciesConfig  `json:"policies"`
}

var (
	globalConfig AppConfig
	configOnce   sync.Once
)

func LoadConfig() AppConfig {
	configOnce.Do(func() {
		// Default config
		globalConfig = AppConfig{
			Store: StoreConfig{
				StoreID:        "STORE-104",
				StoreName:      "Metro Retail Store #104",
				Currency:       "USD",
				DefaultTaxRate: 8.5,
				Environment:    "dev",
			},
			Lanes: []LaneConfig{
				{LaneID: "LANE-01", LaneName: "Lane 1 (Express Cashier)", LaneType: "CASHIER_EXPRESS", CashierID: "CASHIER-101", Icon: "⚡", MaxItemsLimit: 10},
				{LaneID: "LANE-02", LaneName: "Lane 2 (Main Belt Cashier)", LaneType: "CASHIER_BELT", CashierID: "CASHIER-102", Icon: "🛒", MaxItemsLimit: 100},
				{LaneID: "LANE-03", LaneName: "Lane 3 (Self-Checkout 1)", LaneType: "SELF_CHECKOUT", CashierID: "CUSTOMER_SELF", Icon: "🤖", MaxItemsLimit: 25},
				{LaneID: "LANE-04", LaneName: "Lane 4 (Self-Checkout 2)", LaneType: "SELF_CHECKOUT", CashierID: "CUSTOMER_SELF", Icon: "🤖", MaxItemsLimit: 25},
			},
			Telemetry: TelemetryConfig{
				TelemetryServiceURL: "http://telemetry-service:8085/ingest",
				LogLevel:            "INFO",
				BufferSize:          5000,
			},
			Policies: PoliciesConfig{
				AllowNegativeStock:    false,
				RequirePaidToFinalize: true,
				StrictSKULookup:       true,
				MaxPaymentRetries:     3,
			},
		}

		// Try loading from file
		pathsToTry := []string{
			os.Getenv("CONFIG_PATH"),
			"./config.json",
			"../config.json",
			"../../config.json",
			"/app/config.json",
		}

		for _, p := range pathsToTry {
			if p == "" {
				continue
			}
			absPath, _ := filepath.Abs(p)
			data, err := os.ReadFile(absPath)
			if err == nil {
				var parsed AppConfig
				if err := json.Unmarshal(data, &parsed); err == nil {
					globalConfig = parsed
					slog.Info("Successfully loaded config file", "path", absPath, "store_id", globalConfig.Store.StoreID)
					break
				}
			}
		}

		// Override with ENV vars if present
		if envStoreID := os.Getenv("STORE_ID"); envStoreID != "" {
			globalConfig.Store.StoreID = envStoreID
		}
		if envAIURL := os.Getenv("RETAIL_AI_BACKEND_URL"); envAIURL != "" {
			globalConfig.Telemetry.AIBackendURL = envAIURL
		}
	})

	return globalConfig
}

func GetConfig() AppConfig {
	return LoadConfig()
}
