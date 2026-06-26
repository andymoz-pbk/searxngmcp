package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
)

type MCPServer struct {
	config   *Config
	searxng  *SearXNGClient
	sessions sync.Map
	tools    []Tool
}

type Session struct {
	id     string
	sendCh chan []byte
	done   chan struct{}
}

type JSONRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

var toolsList = []Tool{searchTool, fetchTool, datetimeTool, newsSearchTool, fetchManyTool, uuidTool, base64EncodeTool, base64DecodeTool, hashTool, randomStringTool, dnsLookupTool}

func NewMCPServer(cfg *Config) *MCPServer {
	return &MCPServer{
		config:  cfg,
		searxng: NewSearXNGClient(cfg.SearXNG.BaseURL, cfg.SearXNG.Timeout),
		tools:   toolsList,
	}
}

func (s *MCPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = "*"
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Vary", "Origin")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept, X-Requested-With, X-Request-Id, MCP-Protocol-Version, MCP-Session-Id")
	w.Header().Set("Access-Control-Expose-Headers", "MCP-Session-Id")
	w.Header().Set("Access-Control-Max-Age", "86400")
	if origin != "*" {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}

	if r.Method == http.MethodOptions {
		// Echo back exact request headers for maximum CORS compatibility.
		if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
			w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	path := strings.TrimRight(r.URL.Path, "/")

	switch {
	case path == "":
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("searxngmcp MCP server\n\nSSE endpoint: /sse\nMCP endpoint: /mcp\n"))
	case path == "/mcp":
		switch r.Method {
		case http.MethodPost:
			s.handleMCPPost(w, r)
		case http.MethodGet:
			s.handleMCPGet(w, r)
		case http.MethodDelete:
			s.handleMCPDelete(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	case path == "/sse":
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleSSE(w, r)
	case strings.HasPrefix(path, "/messages/"):
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.handleMessage(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *MCPServer) messagesURL(r *http.Request, sessionID string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/messages/%s", scheme, r.Host, sessionID)
}

func (s *MCPServer) handleSSE(w http.ResponseWriter, r *http.Request) {
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

func (s *MCPServer) handleMessage(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/messages/"), "/")
	sessionID := parts[0]

	v, ok := s.sessions.Load(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	session := v.(*Session)

	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		session.sendCh <- marshalError(nil, -32700, "Parse error", err.Error())
		w.WriteHeader(http.StatusAccepted)
		return
	}

	go s.dispatch(session, &req)

	w.WriteHeader(http.StatusAccepted)
}

func (s *MCPServer) dispatch(session *Session, req *JSONRPCRequest) {
	result, rpcErr := s.dispatchSync(req)
	if rpcErr != nil {
		s.sendError(session, req.ID, rpcErr.Code, rpcErr.Message, rpcErr.Data)
		return
	}
	if req.ID != nil {
		s.sendResult(session, req.ID, result)
	}
}

func (s *MCPServer) dispatchSync(req *JSONRPCRequest) (result any, rpcErr *JSONRPCError) {
	defer func() {
		if rec := recover(); rec != nil {
			rpcErr = &JSONRPCError{Code: -32603, Message: "Internal error", Data: fmt.Sprintf("%v", rec)}
		}
	}()

	switch req.Method {
	case "initialize":
		result = buildInitializeResult()
	case "ping":
		result = map[string]any{}
	case "tools/list":
		result = map[string]any{"tools": s.tools}
	case "tools/call":
		result, rpcErr = s.execTool(req.Params)
	case "notifications/initialized":
		// notification — no response
		return
	default:
		if req.ID != nil {
			rpcErr = &JSONRPCError{Code: -32601, Message: "Method not found"}
		}
	}
	return
}

func buildInitializeResult() map[string]any {
	return map[string]any{
		"protocolVersion": "2025-03-26",
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]any{
			"name":    "searxngmcp",
			"version": "1.0.0",
		},
	}
}

func (s *MCPServer) execTool(params any) (any, *JSONRPCError) {
	var callParams struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}

	p, ok := params.(map[string]any)
	if !ok {
		return nil, &JSONRPCError{Code: -32602, Message: "Invalid params", Data: "expected object"}
	}

	callParams.Name, _ = p["name"].(string)
	callParams.Arguments, _ = p["arguments"].(map[string]any)

	if callParams.Name == "" {
		return nil, &JSONRPCError{Code: -32602, Message: "Invalid params", Data: "name is required"}
	}

	if callParams.Arguments == nil {
		callParams.Arguments = map[string]any{}
	}

	return handleToolCall(callParams.Name, callParams.Arguments, s.config, s.searxng), nil
}

func (s *MCPServer) sendResult(session *Session, id any, result any) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(resp)
	select {
	case session.sendCh <- data:
	case <-session.done:
	}
}

func (s *MCPServer) sendError(session *Session, id any, code int, message string, data any) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	raw, _ := json.Marshal(resp)
	select {
	case session.sendCh <- raw:
	case <-session.done:
	}
}

func (s *MCPServer) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("PANIC: %v", rec)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func marshalError(id any, code int, message string, data any) []byte {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
	raw, _ := json.Marshal(resp)
	return raw
}

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}
