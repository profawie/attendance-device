// Package api is the interface for your SYSTEM (not the device):
//
//	GET  /status            - status of all devices
//	GET  /status?sn=XXX     - one device + latest attendance records
//	GET  /command?sn=XXX    - command history
//	POST /command           - queue a command for a device
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"smc.com/att/internal/store"
)

type api struct{ st *store.Store }

func Register(mux *http.ServeMux, st *store.Store) {
	a := &api{st: st}
	mux.HandleFunc("GET /status", a.status)
	mux.HandleFunc("GET /command", a.listCommands)
	mux.HandleFunc("POST /command", a.addCommand)
}

func (a *api) status(w http.ResponseWriter, r *http.Request) {
	sn := r.URL.Query().Get("sn")
	if sn == "" {
		writeJSON(w, http.StatusOK, map[string]any{"devices": a.st.Devices()})
		return
	}
	d, ok := a.st.Device(sn)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "device has never connected: " + sn})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device":            d,
		"recent_attendance": a.st.RecentAttendance(sn, 20),
	})
}

func (a *api) listCommands(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"commands": a.st.Commands(r.URL.Query().Get("sn"))})
}

// JSON body, examples:
//
//	{"sn":"ABC123","action":"reboot"}
//	{"sn":"ABC123","action":"add_user","pin":"1001","name":"Ali"}
//	{"sn":"ABC123","action":"query_attlog","from":"2026-11-01 00:00:00","to":"2026-11-02 23:59:59"}
type commandReq struct {
	SN     string `json:"sn"`
	Action string `json:"action"`
	PIN    string `json:"pin,omitempty"`
	Name   string `json:"name,omitempty"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
}

var (
	pinRe  = regexp.MustCompile(`^\d{1,9}$`)
	timeRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`)
)

func (a *api) addCommand(w http.ResponseWriter, r *http.Request) {
	var req commandReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON: " + err.Error()})
		return
	}
	if req.SN == "" || !a.st.Allowed(req.SN) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "sn is empty or not allowed"})
		return
	}

	body, err := build(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	c := a.st.Enqueue(req.SN, body)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"message": "command queued; the device will pick it up on its next getrequest",
		"command": c,
	})
}

// build turns an "action" into an iclock protocol command.
func build(req commandReq) (string, error) {
	switch strings.ToLower(req.Action) {
	case "reboot":
		return "REBOOT", nil
	case "info":
		return "INFO", nil
	case "check":
		// Ask the device to check and re-send data it has not pushed yet.
		return "CHECK", nil
	case "query_users":
		return "DATA QUERY USERINFO", nil
	case "query_attlog":
		if !timeRe.MatchString(req.From) || !timeRe.MatchString(req.To) {
			return "", fmt.Errorf("from/to are required, format YYYY-MM-DD HH:MM:SS")
		}
		return fmt.Sprintf("DATA QUERY ATTLOG StartTime=%s\tEndTime=%s", req.From, req.To), nil
	case "add_user":
		if !pinRe.MatchString(req.PIN) {
			return "", fmt.Errorf("pin must be numeric")
		}
		name := strings.Map(func(r rune) rune { // strip TAB/newlines from the name
			if r == '\t' || r == '\n' || r == '\r' {
				return ' '
			}
			return r
		}, req.Name)
		return fmt.Sprintf("DATA UPDATE USERINFO PIN=%s\tName=%s\tPri=0", req.PIN, name), nil
	case "delete_user":
		// "*" (delete all) is blocked on purpose in this demo.
		if !pinRe.MatchString(req.PIN) {
			return "", fmt.Errorf("pin must be numeric (delete all is not allowed)")
		}
		return "DATA DELETE USERINFO PIN=" + req.PIN, nil
	default:
		return "", fmt.Errorf("unknown action: %q (reboot, info, check, query_users, query_attlog, add_user, delete_user)", req.Action)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
