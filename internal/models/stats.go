package models

import (
	"net/netip"
)

// HTMLData is a list of HTML fields to be rendered.
// It is exported so that the HTML template engine can render it.
type Stats struct {
	Stats []Stat `json:"rows"`
}

// Stat contains fields to be rendered
// It is exported so that the  template engine can render it.
type Stat struct {
	Domain      string       `json:"domain"`
	Owner       string       `json:"owner"`
	Provider    string       `json:"provider"`
	IPVersion   string       `json:"ip_version"`
	Status      string       `json:"status"`
	CurrentIP   string       `json:"current_ip"`
	PreviousIPs []netip.Addr `json:"previous_ips"`
}
