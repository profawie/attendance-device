// Package iclock implements the server side of the iclock "push" protocol
// used by FingerTec/ZKTeco terminals (example: Face ID 5).
//
// Short flow:
//
//  1. Device boots  -> GET  /iclock/cdata?SN=..&options=all   (ask for settings)
//  2. Device pushes -> POST /iclock/cdata?SN=..&table=ATTLOG  (attendance records)
//  3. Device polls  -> GET  /iclock/getrequest?SN=..          (any command?)
//  4. Device reports-> POST /iclock/devicecmd?SN=..           (command result)
//
// The device always opens the connection; the server only answers.
// That is why commands from the system must wait in a queue until the device asks.
package iclock

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"smc.com/att/internal/store"
)

type handler struct {
	st  *store.Store
	log *slog.Logger
}

func Register(mux *http.ServeMux, st *store.Store, logger *slog.Logger) {
	h := &handler{st: st, log: logger}
	mux.HandleFunc("/iclock/cdata", h.cdata)
	mux.HandleFunc("/iclock/getrequest", h.getRequest)
	mux.HandleFunc("/iclock/devicecmd", h.deviceCmd)
	// Other paths (fdata, ping, ...): answer OK so the device does not retry.
	mux.HandleFunc("/iclock/", func(w http.ResponseWriter, r *http.Request) { text(w, "OK") })
}

// guard checks the SN and updates "last seen". false = stop here.
func (h *handler) guard(w http.ResponseWriter, r *http.Request) (string, bool) {
	sn := r.URL.Query().Get("SN")
	if !h.st.Allowed(sn) {
		h.log.Warn("unknown device", "sn", sn, "remote", r.RemoteAddr)
		// Still answer OK so the device does not keep retrying.
		text(w, "OK")
		return "", false
	}
	h.st.Touch(sn, clientIP(r))
	return sn, true
}

// GET  -> device asks for settings (handshake)
// POST -> device pushes data (ATTLOG, OPERLOG, ...)
func (h *handler) cdata(w http.ResponseWriter, r *http.Request) {
	sn, ok := h.guard(w, r)
	if !ok {
		return
	}

	if r.Method == http.MethodGet {
		// Basic settings the device reads when it first connects.
		// Delay = how many seconds between getrequest polls.
		// Realtime=1 = push ATTLOG right after each face/finger scan.
		lines := []string{
			"GET OPTION FROM: " + sn,
			"ATTLOGStamp=None",
			"OPERLOGStamp=None",
			"ATTPHOTOStamp=None",
			"ErrorDelay=30",
			"Delay=10",
			"TransTimes=00:00;12:00",
			"TransInterval=1",
			"TransFlag=TransData AttLog OpLog",
			"TimeZone=8",
			"Realtime=1",
			"Encrypt=None",
		}
		text(w, strings.Join(lines, "\n"))
		return
	}

	body, _ := io.ReadAll(r.Body)
	table := strings.ToUpper(r.URL.Query().Get("table"))

	switch table {
	case "ATTLOG":
		rows := parseAttLog(sn, string(body))
		h.st.AddAttendance(rows)
		h.log.Info("ATTLOG received", "sn", sn, "rows", len(rows))
		text(w, fmt.Sprintf("OK: %d", len(rows)))
	case "OPERLOG":
		// Operation log (user added, admin login, ...). Demo: log only.
		h.log.Info("OPERLOG received", "sn", sn, "lines", countLines(string(body)))
		text(w, "OK")
	default:
		h.log.Info("unhandled table", "sn", sn, "table", table)
		text(w, "OK")
	}
}

// Device asks "any work for me?". Answer one command or "OK".
func (h *handler) getRequest(w http.ResponseWriter, r *http.Request) {
	sn, ok := h.guard(w, r)
	if !ok {
		return
	}
	if c, found := h.st.NextCommand(sn); found {
		h.log.Info("sending command to device", "sn", sn, "id", c.ID, "command", c.Text)
		text(w, c.Text)
		return
	}
	text(w, "OK")
}

// Device reports a command result. Example body:
//
//	ID=12&Return=0&CMD=REBOOT
//
// For INFO, the following lines hold device details.
func (h *handler) deviceCmd(w http.ResponseWriter, r *http.Request) {
	sn, ok := h.guard(w, r)
	if !ok {
		return
	}
	body, _ := io.ReadAll(r.Body)
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")

	var lastID int64
	var extra []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "ID=") {
			extra = append(extra, line) // extra data (e.g. INFO result)
			continue
		}
		v, _ := url.ParseQuery(line)
		id, err := strconv.ParseInt(v.Get("ID"), 10, 64)
		if err != nil {
			continue
		}
		lastID = id
		if c, found := h.st.Complete(id, v.Get("Return")); found {
			h.log.Info("command result", "sn", sn, "id", id, "command", c.Text, "return", c.Return, "status", c.Status)
		} else {
			h.log.Warn("result for unknown command", "sn", sn, "id", id)
		}
	}
	if lastID != 0 && len(extra) > 0 {
		h.st.SetInfo(sn, strings.Join(extra, "; "))
	}
	text(w, "OK")
}

// One ATTLOG line (TAB separated):
//
//	PIN  Time(YYYY-MM-DD HH:MM:SS)  Status  Verify  WorkCode ...
func parseAttLog(sn, body string) []store.Attendance {
	var rows []store.Attendance
	for _, line := range strings.Split(body, "\n") {
		f := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(f) < 2 || strings.TrimSpace(f[0]) == "" {
			continue
		}
		a := store.Attendance{SN: sn, PIN: strings.TrimSpace(f[0]), Time: strings.TrimSpace(f[1])}
		if len(f) > 2 {
			a.Status = f[2]
		}
		if len(f) > 3 {
			a.Verify = f[3]
		}
		rows = append(rows, a)
	}
	return rows
}

func countLines(s string) int {
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func text(w http.ResponseWriter, s string) {
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(w, s)
}
