// Package store keeps the demo state in memory:
// devices, command queues and attendance records.
package store

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Device is a short summary of one attendance terminal.
type Device struct {
	SN         string    `json:"sn"`
	IP         string    `json:"ip"`
	FirstSeen  time.Time `json:"first_seen"`
	LastSeen   time.Time `json:"last_seen"`
	Online     bool      `json:"online"`
	Pending    int       `json:"pending_commands"`
	AttLogs    int       `json:"attendance_count"`
	Info       string    `json:"info,omitempty"` // details from the INFO command
	pendingCmd []*Command
}

// Command is one command sent to a device.
type Command struct {
	ID        int64      `json:"id"`
	SN        string     `json:"sn"`
	Text      string     `json:"text"`   // e.g. C:12:REBOOT
	Status    string     `json:"status"` // queued -> sent -> done/failed
	Return    string     `json:"return,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	SentAt    *time.Time `json:"sent_at,omitempty"`
	DoneAt    *time.Time `json:"done_at,omitempty"`
}

// Attendance is one ATTLOG line from a device.
type Attendance struct {
	SN     string `json:"sn"`
	PIN    string `json:"pin"`
	Time   string `json:"time"`
	Status string `json:"status"` // 0=check in, 1=check out, ... (depends on device setup)
	Verify string `json:"verify"` // verify mode: finger, face, card...
}

type Store struct {
	mu         sync.Mutex
	allowed    map[string]bool // empty = accept all
	onlineFor  time.Duration
	devices    map[string]*Device
	commands   map[int64]*Command
	attendance []Attendance
	nextID     int64
}

func New(allowedSN []string, onlineFor time.Duration) *Store {
	s := &Store{
		allowed:   map[string]bool{},
		onlineFor: onlineFor,
		devices:   map[string]*Device{},
		commands:  map[int64]*Command{},
		nextID:    1,
	}
	for _, sn := range allowedSN {
		if sn = strings.TrimSpace(sn); sn != "" {
			s.allowed[sn] = true
		}
	}
	return s
}

// Allowed reports whether this SN may talk to the server.
func (s *Store) Allowed(sn string) bool {
	if sn == "" {
		return false
	}
	return len(s.allowed) == 0 || s.allowed[sn]
}

// Touch is called every time a device contacts the server (heartbeat).
func (s *Store) Touch(sn, ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.device(sn)
	d.LastSeen = time.Now()
	d.IP = ip
}

func (s *Store) device(sn string) *Device {
	d, ok := s.devices[sn]
	if !ok {
		d = &Device{SN: sn, FirstSeen: time.Now()}
		s.devices[sn] = d
	}
	return d
}

// Devices returns a copy of all devices with their online status.
func (s *Store) Devices() []Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Device, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, s.snapshot(d))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SN < out[j].SN })
	return out
}

func (s *Store) Device(sn string) (Device, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[sn]
	if !ok {
		return Device{}, false
	}
	return s.snapshot(d), true
}

func (s *Store) snapshot(d *Device) Device {
	c := *d
	c.Online = time.Since(d.LastSeen) <= s.onlineFor
	c.Pending = len(d.pendingCmd)
	c.pendingCmd = nil
	return c
}

// Enqueue queues a command. Protocol format: C:<id>:<command>
func (s *Store) Enqueue(sn, body string) *Command {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID
	s.nextID++
	c := &Command{
		ID:        id,
		SN:        sn,
		Text:      fmt.Sprintf("C:%d:%s", id, body),
		Status:    "queued",
		CreatedAt: time.Now(),
	}
	s.commands[id] = c
	d := s.device(sn)
	d.pendingCmd = append(d.pendingCmd, c)
	return c
}

// NextCommand pops the next command (FIFO) for a device.
func (s *Store) NextCommand(sn string) (*Command, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.devices[sn]
	if !ok || len(d.pendingCmd) == 0 {
		return nil, false
	}
	c := d.pendingCmd[0]
	d.pendingCmd = d.pendingCmd[1:]
	c.Status = "sent"
	now := time.Now()
	c.SentAt = &now
	return c, true
}

// Complete records a command result. Return=0 means success.
func (s *Store) Complete(id int64, ret string) (*Command, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.commands[id]
	if !ok {
		return nil, false
	}
	c.Return = ret
	now := time.Now()
	c.DoneAt = &now
	if ret == "0" {
		c.Status = "done"
	} else {
		c.Status = "failed"
	}
	return c, true
}

// Commands returns the command history (optionally filtered by SN).
func (s *Store) Commands(sn string) []Command {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Command{}
	for _, c := range s.commands {
		if sn == "" || c.SN == sn {
			out = append(out, *c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out
}

func (s *Store) AddAttendance(rows []Attendance) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attendance = append(s.attendance, rows...)
	for _, r := range rows {
		s.device(r.SN).AttLogs++
	}
}

// RecentAttendance returns the n latest records (optionally filtered by SN).
func (s *Store) RecentAttendance(sn string, n int) []Attendance {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Attendance{}
	for i := len(s.attendance) - 1; i >= 0 && len(out) < n; i-- {
		if sn == "" || s.attendance[i].SN == sn {
			out = append(out, s.attendance[i])
		}
	}
	return out
}

func (s *Store) SetInfo(sn, info string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.device(sn).Info = info
}
