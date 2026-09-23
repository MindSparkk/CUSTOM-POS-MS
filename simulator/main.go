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
	StoreID            = "STORE-104"
)

type LaneConfig struct {
	ID      string
	Type    string
	Cashier string
}

var Lanes = []LaneConfig{
	{ID: "LANE-01", Type: "CASHIER_EXPRESS", Cashier: "CASHIER-101"},
	{ID: "LANE-02", Type: "CASHIER_BELT", Cashier: "CASHIER-102"},
	{ID: "LANE-03", Type: "SELF_CHECKOUT", Cashier: "CUSTOMER_SELF"},
	{ID: "LANE-04", Type: "SELF_CHECKOUT", Cashier: "CUSTOMER_SELF"},
}

type OrderResponse struct {
	OrderNo string `json:"order_no"`
	TraceID string `json:"trace_id"`
}

func main() {
	fmt.Println("Starting Multi-Lane POS Store Traffic & Chaos Simulator...")
	fmt.Println("Simulating concurrent transactions for Store: STORE-104 across Lanes 1..4")
	fmt.Println("Press Ctrl+C to stop.")

	// Start background simulation routines per lane
	go lane1ExpressTraffic()
	go lane2MainBeltTraffic()
	go lane3SelfCheckoutChaos()
	go lane4SelfCheckoutChaos()

	select {}
}

func doRequest(method, url string, lane LaneConfig, body []byte) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewBuffer(body)
	}
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Store-ID", StoreID)
	req.Header.Set("X-Lane-ID", lane.ID)
	req.Header.Set("X-Lane-Type", lane.Type)
	req.Header.Set("X-Cashier-ID", lane.Cashier)

	client := &http.Client{Timeout: 5 * time.Second}
	return client.Do(req)
}

func createOrder(lane LaneConfig) (string, string, error) {
	resp, err := doRequest("POST", OrderServiceURL+"/orders", lane, nil)
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

// Lane 1: Express Cashier Traffic (Fast successful orders)
func lane1ExpressTraffic() {
	lane := Lanes[0]
	for {
		time.Sleep(time.Duration(rand.Intn(6)+4) * time.Second)
		orderNo, traceID, err := createOrder(lane)
		if err != nil {
			continue
		}

		// Add Apples
		doRequest("POST", fmt.Sprintf("%s/orders/%s/items", OrderServiceURL, orderNo), lane, []byte(`{"sku": "100001", "quantity": 1}`))
		// Calculate
		calcResp, _ := doRequest("POST", fmt.Sprintf("%s/orders/%s/calculate", OrderServiceURL, orderNo), lane, nil)
		if calcResp == nil {
			continue
		}
		var calc map[string]interface{}
		json.NewDecoder(calcResp.Body).Decode(&calc)
		calcResp.Body.Close()

		total, ok := calc["total"].(float64)
		if !ok || total <= 0 {
			total = 5.41
		}

		// Pay & Finalize
		payPayload := []byte(fmt.Sprintf(`{"order_no": "%s", "trace_id": "%s", "amount": %.2f, "cash_received": %.2f}`, orderNo, traceID, total, total+5.0))
		payResp, err := doRequest("POST", PaymentServiceURL+"/payments", lane, payPayload)
		if err == nil && payResp != nil {
			payResp.Body.Close()
			finPayload := []byte(fmt.Sprintf(`{"order_no": "%s", "trace_id": "%s"}`, orderNo, traceID))
			finResp, _ := doRequest("POST", FinalizeServiceURL+"/finalize", lane, finPayload)
			if finResp != nil {
				finResp.Body.Close()
			}
			fmt.Printf("[Sim][%s] Order %s Completed Successfully\n", lane.ID, orderNo)
		}
	}
}

// Lane 2: Main Belt Traffic (Large orders)
func lane2MainBeltTraffic() {
	lane := Lanes[1]
	for {
		time.Sleep(time.Duration(rand.Intn(10)+8) * time.Second)
		orderNo, traceID, err := createOrder(lane)
		if err != nil {
			continue
		}

		doRequest("POST", fmt.Sprintf("%s/orders/%s/items", OrderServiceURL, orderNo), lane, []byte(`{"sku": "100021", "quantity": 2}`))
		doRequest("POST", fmt.Sprintf("%s/orders/%s/items", OrderServiceURL, orderNo), lane, []byte(`{"sku": "100037", "quantity": 1}`))
		calcResp, _ := doRequest("POST", fmt.Sprintf("%s/orders/%s/calculate", OrderServiceURL, orderNo), lane, nil)
		if calcResp == nil {
			continue
		}
		var calc map[string]interface{}
		json.NewDecoder(calcResp.Body).Decode(&calc)
		calcResp.Body.Close()

		total, ok := calc["total"].(float64)
		if !ok || total <= 0 {
			total = 17.80
		}

		payPayload := []byte(fmt.Sprintf(`{"order_no": "%s", "trace_id": "%s", "amount": %.2f, "cash_received": %.2f}`, orderNo, traceID, total, total))
		payResp, err := doRequest("POST", PaymentServiceURL+"/payments", lane, payPayload)
		if err == nil && payResp != nil {
			payResp.Body.Close()
			finPayload := []byte(fmt.Sprintf(`{"order_no": "%s", "trace_id": "%s"}`, orderNo, traceID))
			finResp, _ := doRequest("POST", FinalizeServiceURL+"/finalize", lane, finPayload)
			if finResp != nil {
				finResp.Body.Close()
			}
			fmt.Printf("[Sim][%s] Order %s Completed Successfully\n", lane.ID, orderNo)
		}
	}
}

// Lane 3: Self-Checkout 1 Chaos (Stale SKUs & Unpaid orders)
func lane3SelfCheckoutChaos() {
	lane := Lanes[2]
	for {
		time.Sleep(time.Duration(rand.Intn(12)+6) * time.Second)
		orderNo, _, err := createOrder(lane)
		if err != nil {
			continue
		}

		// Try to scan fake/discontinued SKU
		payload := []byte(`{"sku": "999999", "quantity": 1}`)
		doRequest("POST", fmt.Sprintf("%s/orders/%s/items", OrderServiceURL, orderNo), lane, payload)
		fmt.Printf("[Sim][%s] Stale SKU scanned on %s\n", lane.ID, orderNo)
	}
}

// Lane 4: Self-Checkout 2 Chaos (Insufficient cash / Payment rejection)
func lane4SelfCheckoutChaos() {
	lane := Lanes[3]
	for {
		time.Sleep(time.Duration(rand.Intn(15)+10) * time.Second)
		orderNo, traceID, err := createOrder(lane)
		if err != nil {
			continue
		}

		doRequest("POST", fmt.Sprintf("%s/orders/%s/items", OrderServiceURL, orderNo), lane, []byte(`{"sku": "100083", "quantity": 1}`))
		calcResp, _ := doRequest("POST", fmt.Sprintf("%s/orders/%s/calculate", OrderServiceURL, orderNo), lane, nil)
		if calcResp == nil {
			continue
		}
		var calc map[string]interface{}
		json.NewDecoder(calcResp.Body).Decode(&calc)
		calcResp.Body.Close()

		total, ok := calc["total"].(float64)
		if !ok || total <= 0 {
			total = 7.50
		}

		// Customer attempts to pay with insufficient cash (shortfall)
		shortfallPayload := []byte(fmt.Sprintf(`{"order_no": "%s", "trace_id": "%s", "amount": %.2f, "cash_received": %.2f}`, orderNo, traceID, total, total-2.0))
		doRequest("POST", PaymentServiceURL+"/payments", lane, shortfallPayload)
		fmt.Printf("[Sim][%s] Insufficient Cash payment attempt on %s\n", lane.ID, orderNo)
	}
}
