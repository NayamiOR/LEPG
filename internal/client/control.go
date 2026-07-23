package client

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"LEPG/internal/model"
)

type commandRequest struct {
	Device string       `json:"device"`
	Writes []writeEntry `json:"writes"`
}

type writeEntry struct {
	Point string  `json:"point"`
	Value float64 `json:"value"`
}

type writeResult struct {
	Point   string `json:"point"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type commandResponse struct {
	Device  string        `json:"device"`
	Results []writeResult `json:"results"`
}

// ControlServer is the HTTP API for reverse-control commands.
type ControlServer struct {
	addr   string
	server *http.Server
	pub    *ClientPublisher
}

// NewControlServer creates a new control HTTP server.
func NewControlServer(addr string, pub *ClientPublisher) *ControlServer {
	return &ControlServer{addr: addr, pub: pub}
}

// Start begins listening and blocks the caller only long enough to confirm the
// listener is up. It spawns a background goroutine for Serve and another that
// shuts down on ctx.Done.
func (cs *ControlServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /command", cs.handleCommand)

	cs.server = &http.Server{Addr: cs.addr, Handler: mux}

	ln, err := net.Listen("tcp", cs.addr)
	if err != nil {
		return fmt.Errorf("control server listen: %w", err)
	}

	go func() {
		slog.Info("control http server started", "addr", cs.addr)
		if err := cs.server.Serve(ln); err != http.ErrServerClosed {
			slog.Error("control server error", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		cs.server.Shutdown(context.Background())
	}()

	return nil
}

func (cs *ControlServer) handleCommand(w http.ResponseWriter, r *http.Request) {
	var req commandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}

	if req.Device == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device is required"})
		return
	}
	if len(req.Writes) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "writes is required"})
		return
	}

	// Try Modbus device registry first.
	if rt, ok := GetModbusRuntime(req.Device); ok {
		cs.handleModbusCommand(w, req.Device, req.Writes, rt)
		return
	}

	// Unrecognized device — treat as MQTT and publish command payload.
	cs.handleMqttCommand(w, req)
}

func (cs *ControlServer) handleModbusCommand(w http.ResponseWriter, device string, writes []writeEntry, rt *ModbusRuntime) {
	resp := commandResponse{Device: device}
	for _, wc := range writes {
		pt := rt.findPoint(wc.Point)
		if pt == nil {
			resp.Results = append(resp.Results, writeResult{Point: wc.Point, Success: false, Error: "point not found"})
			continue
		}
		if pt.Access == model.AccessReadOnly {
			resp.Results = append(resp.Results, writeResult{Point: wc.Point, Success: false, Error: "point is read-only"})
			continue
		}
		if err := rt.Write(wc.Point, wc.Value); err != nil {
			resp.Results = append(resp.Results, writeResult{Point: wc.Point, Success: false, Error: err.Error()})
			continue
		}
		resp.Results = append(resp.Results, writeResult{Point: wc.Point, Success: true})
	}
	writeJSON(w, http.StatusOK, resp)
}

func (cs *ControlServer) handleMqttCommand(w http.ResponseWriter, req commandRequest) {
	if cs.pub == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "no mqtt publisher available"})
		return
	}

	payload, err := json.Marshal(req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "marshal: " + err.Error()})
		return
	}

	topic := fmt.Sprintf("device/%s/command", req.Device)
	if err := cs.pub.PublishRaw(topic, payload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "publish: " + err.Error()})
		return
	}

	slog.Info("command published to mqtt device", "device", req.Device, "writes", len(req.Writes))

	resp := commandResponse{Device: req.Device}
	for _, wc := range req.Writes {
		resp.Results = append(resp.Results, writeResult{Point: wc.Point, Success: true})
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
