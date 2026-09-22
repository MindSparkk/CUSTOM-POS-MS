package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
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
	logFilePath string
)

func getLogFilePath() string {
	logDir := os.Getenv("LOG_DIR")
	if logDir == "" {
		if _, err := os.Stat("/app"); err == nil {
			logDir = "/app/logs"
		} else {
			logDir = "./logs"
		}
	}
	os.MkdirAll(logDir, 0755)
	return filepath.Join(logDir, "pos_telemetry.jsonl")
}

func loadPersistedLogs() {
	file, err := os.Open(logFilePath)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var loaded []models.POSLog
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var posLog models.POSLog
		if err := json.Unmarshal(line, &posLog); err == nil {
			loaded = append(loaded, posLog)
		}
	}

	if len(loaded) > 5000 {
		loaded = loaded[len(loaded)-5000:]
	}

	logsMutex.Lock()
	logsBuffer = loaded
	logsMutex.Unlock()
	slog.Info("Loaded persisted telemetry logs from file", "count", len(loaded), "path", logFilePath)
}

func appendLogToFile(posLog models.POSLog) {
	file, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		slog.Error("Failed to open log file for appending", "error", err.Error())
		return
	}
	defer file.Close()

	data, err := json.Marshal(posLog)
	if err != nil {
		return
	}
	file.Write(append(data, '\n'))
}

func main() {
	logger.InitLogger("telemetry-service")
	slog.Info("Starting telemetry service on port 8085")

	logFilePath = getLogFilePath()
	loadPersistedLogs()

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
		WriteTimeout: 0, // Disable WriteTimeout for SSE stream
		ReadTimeout:  15 * time.Second,
	}

	slog.Error("Server stopped", "error", srv.ListenAndServe())
}

func ingestHandler(w http.ResponseWriter, r *http.Request) {
	var posLog models.POSLog
	if err := json.NewDecoder(r.Body).Decode(&posLog); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Persist log entry to file
	go appendLogToFile(posLog)

	// Store log in memory (keep last 5000 for simulation/API response)
	logsMutex.Lock()
	logsBuffer = append(logsBuffer, posLog)
	if len(logsBuffer) > 5000 {
		logsBuffer = logsBuffer[1:]
	}
	logsMutex.Unlock()

	// Broadcast to SSE clients
	clientsMu.Lock()
	for clientChan := range clients {
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
