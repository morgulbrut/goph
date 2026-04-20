package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const traceTimeout = 2 * time.Minute

type traceRequest struct {
	Word     string `json:"word"`
	Language string `json:"language"`
	Target   string `json:"target"`
}

type traceResponse struct {
	Steps []string `json:"steps"`
	Found bool     `json:"found"`
	Error string   `json:"error,omitempty"`
}

func handleTrace(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req traceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	req.Word = strings.TrimSpace(req.Word)
	if req.Word == "" {
		http.Error(w, "word is required", http.StatusBadRequest)
		return
	}
	if req.Language == "" {
		req.Language = "en"
	}

	ctx, cancel := context.WithTimeout(r.Context(), traceTimeout)
	defer cancel()

	steps, found, err := traceToPhilosophy(ctx, req.Word, req.Language, req.Target)
	resp := traceResponse{
		Steps: steps,
		Found: found,
	}
	if err != nil {
		resp.Error = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	if encErr := json.NewEncoder(w).Encode(resp); encErr != nil {
		log.Printf("error encoding response: %v", encErr)
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("static")))
	mux.HandleFunc("/api/trace", handleTrace)

	addr := ":" + port
	log.Printf("Starting server on http://localhost%s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
