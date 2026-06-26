package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (s *MCPServer) handleMCPPost(w http.ResponseWriter, r *http.Request) {
	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Error:   &JSONRPCError{Code: -32700, Message: "Parse error", Data: err.Error()},
		})
		return
	}

	result, rpcErr := s.dispatchSync(&req)

	if req.ID == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if rpcErr != nil {
		json.NewEncoder(w).Encode(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   rpcErr,
		})
	} else {
		json.NewEncoder(w).Encode(JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  result,
		})
	}
}

func (s *MCPServer) handleMCPGet(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	sessionID := generateID()
	session := &Session{
		id:     sessionID,
		sendCh: make(chan []byte, 64),
		done:   make(chan struct{}),
	}
	s.sessions.Store(sessionID, session)
	defer s.sessions.Delete(sessionID)
	defer close(session.done)

	// Send endpoint event so clients using the old HTTP+SSE pattern
	// (GET for SSE, POST to endpoint) can find the POST URL.
	_, _ = fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", s.messagesURL(r, sessionID))
	flusher.Flush()

	for {
		select {
		case msg, ok := <-session.sendCh:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *MCPServer) handleMCPDelete(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
