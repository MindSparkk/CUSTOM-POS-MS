package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"
)

const (
	OrderServiceURL    = "http://localhost:8081"
	PaymentServiceURL  = "http://localhost:8083"
	FinalizeServiceURL = "http://localhost:8084"
)

type OrderResponse struct {
	OrderNo string `json:"order_no"`
	TraceID string `json:"trace_id"`
}

func main() {
	fmt.Println("Starting POS Chaos Simulator...")
	fmt.Println("Press Ctrl+C to stop.")

	// Start various simulation workers
	go staleCatalogSimulation()
	go abandonedCartSimulation()
	go insufficientCashSimulation()
	go prematureFinalizeSimulation()

	// Keep main goroutine alive
	select {}
}

func createOrder() (string, string, error) {
	resp, err := http.Post(OrderServiceURL+"/orders", "application/json", nil)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	var order OrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&order); err != nil {
		return "", "", err
	}
	return order.OrderNo, order.TraceID, nil
}

// Simulation 1: Stale Catalog (Attempts to add invalid SKUs)
func staleCatalogSimulation() {
	for {
		time.Sleep(time.Duration(rand.Intn(10)+5) * time.Second)
		orderNo, _, err := createOrder()
		if err != nil {
			continue
		}

		// Try to add a fake SKU
		payload := []byte(`{"sku": "999999", "name": "Discontinued Item", "quantity": 1, "unit_price": 100}`)
		http.Post(fmt.Sprintf("%s/orders/%s/items", OrderServiceURL, orderNo), "application/json", bytes.NewBuffer(payload))
		fmt.Printf("[Sim] Stale Catalog Error triggered for %s\n", orderNo)
	}
}

// Simulation 2: Abandoned Carts (Creates orders, adds valid items, never pays)
func abandonedCartSimulation() {
	for {
		time.Sleep(time.Duration(rand.Intn(15)+10) * time.Second)
		orderNo, _, err := createOrder()
		if err != nil {
			continue
		}

		// Add valid item (Bread - 100003)
		payload := []byte(`{"sku": "100003", "quantity": 1}`)
		http.Post(fmt.Sprintf("%s/orders/%s/items", OrderServiceURL, orderNo), "application/json", bytes.NewBuffer(payload))
		
		// Calculate total
		http.Post(fmt.Sprintf("%s/orders/%s/calculate", OrderServiceURL, orderNo), "application/json", nil)
		
		fmt.Printf("[Sim] Cart Abandoned for %s\n", orderNo)
		// Stops here, leaving order OPEN
	}
}

// Simulation 3: Cashier Error - Insufficient Cash
func insufficientCashSimulation() {
	for {
		time.Sleep(time.Duration(rand.Intn(20)+10) * time.Second)
		orderNo, traceID, err := createOrder()
		if err != nil {
			continue
		}

		// Add valid item (Milk - 100004)
		payload := []byte(`{"sku": "100004", "quantity": 1}`)
		http.Post(fmt.Sprintf("%s/orders/%s/items", OrderServiceURL, orderNo), "application/json", bytes.NewBuffer(payload))
		
		// Calculate total
		calcResp, _ := http.Post(fmt.Sprintf("%s/orders/%s/calculate", OrderServiceURL, orderNo), "application/json", nil)
		var calc map[string]interface{}
		json.NewDecoder(calcResp.Body).Decode(&calc)
		calcResp.Body.Close()

		total := calc["total"].(float64)

		// Pay with less cash than required (shortfall)
		shortfallPayment := fmt.Sprintf(`{"order_no": "%s", "trace_id": "%s", "amount": %.2f, "cash_received": %.2f}`, 
			orderNo, traceID, total, total-10.0)
		
		http.Post(PaymentServiceURL+"/payments", "application/json", bytes.NewBuffer([]byte(shortfallPayment)))
		fmt.Printf("[Sim] Insufficient Cash Error triggered for %s\n", orderNo)
	}
}

// Simulation 4: Premature Finalize (Trying to finalize an unpaid order)
func prematureFinalizeSimulation() {
	for {
		time.Sleep(time.Duration(rand.Intn(25)+15) * time.Second)
		orderNo, traceID, err := createOrder()
		if err != nil {
			continue
		}

		// Add valid item
		payload := []byte(`{"sku": "100001", "quantity": 1}`)
		http.Post(fmt.Sprintf("%s/orders/%s/items", OrderServiceURL, orderNo), "application/json", bytes.NewBuffer(payload))

		// Try to finalize immediately without paying
		finalizePayload := []byte(fmt.Sprintf(`{"order_no": "%s", "trace_id": "%s"}`, orderNo, traceID))
		resp, _ := http.Post(FinalizeServiceURL+"/finalize", "application/json", bytes.NewBuffer(finalizePayload))
		if resp != nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		
		fmt.Printf("[Sim] Premature Finalize Error triggered for %s\n", orderNo)
	}
}
