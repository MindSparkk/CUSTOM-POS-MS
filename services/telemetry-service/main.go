package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"retail-pos/pkg/logger"
	"retail-pos/pkg/models"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

var (
	logsBuffer []models.POSLog
	logsMutex  sync.RWMutex
	clients    = make(map[chan models.POSLog]bool)
	clientsMu  sync.Mutex
)

func main() {
	logger.InitLogger("telemetry-service")
	slog.Info("Starting telemetry service on port 8085")

	r := mux.NewRouter()
	r.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(models.HealthResponse{Service: "telemetry-service", Status: "UP"})
	}).Methods("GET")
	r.HandleFunc("/ingest", ingestHandler).Methods("POST")
	r.HandleFunc("/logs", getLogsHandler).Methods("GET") // For initial load
	r.HandleFunc("/stream", streamHandler).Methods("GET") // For SSE real-time stream

	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "DELETE", "PUT", "OPTIONS"},
		AllowedHeaders: []string{"*"},
	})

	srv := &http.Server{
		Handler:      c.Handler(r),
		Addr:         ":8085",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}
	// For SSE to work properly without timeouts, we might need to bypass WriteTimeout for /stream,
	// but standard http.Server limits apply to all. We'll rely on the browser reconnecting or we can remove WriteTimeout.
	srv.WriteTimeout = 0 // Disable WriteTimeout for SSE stream

	slog.Error("Server stopped", "error", srv.ListenAndServe())
}

func ingestHandler(w http.ResponseWriter, r *http.Request) {
	var posLog models.POSLog
	if err := json.NewDecoder(r.Body).Decode(&posLog); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Store log in memory (keep last 5000 for simulation purposes)
	logsMutex.Lock()
	logsBuffer = append(logsBuffer, posLog)
	if len(logsBuffer) > 5000 {
		logsBuffer = logsBuffer[1:]
	}
	logsMutex.Unlock()

	// Broadcast to SSE clients
	clientsMu.Lock()
	for clientChan := range clients {
		// Non-blocking send
		select {
		case clientChan <- posLog:
		default:
		}
	}
	clientsMu.Unlock()

	slog.Info("Telemetry received", "trace_id", posLog.TraceID, "order_no", posLog.OrderNo, "service", posLog.Service, "log_message", posLog.Message)

	backendURL := os.Getenv("RETAIL_AI_BACKEND_URL")
	if backendURL != "" {
		payload, _ := json.Marshal(posLog)
		req, err := http.NewRequest("POST", backendURL, bytes.NewBuffer(payload))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				slog.Error("AI backend forwarding failed", "trace_id", posLog.TraceID, "error", err.Error())
			} else {
				defer resp.Body.Close()
			}
		}
	}

	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintln(w, `{"status":"accepted"}`)
}

func getLogsHandler(w http.ResponseWriter, r *http.Request) {
	logsMutex.RLock()
	defer logsMutex.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logsBuffer)
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	messageChan := make(chan models.POSLog, 100)
	clientsMu.Lock()
	clients[messageChan] = true
	clientsMu.Unlock()

	defer func() {
		clientsMu.Lock()
		delete(clients, messageChan)
		clientsMu.Unlock()
		close(messageChan)
	}()

	notify := r.Context().Done()

	for {
		select {
		case <-notify:
			return
		case posLog := <-messageChan:
			data, _ := json.Marshal(posLog)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
