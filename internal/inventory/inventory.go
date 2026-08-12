// Package inventory manages the fleet of MikroTik devices the MCP server can
// talk to. A fleet is declared as a JSON array (inline via MIKROTIK_INVENTORY
// or in a file via MIKROTIK_INVENTORY_FILE); without either, the server falls
// back to a single device configured by the flat environment variables.
package inventory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Delnegend/mikrotik-mcp/internal/client"
)

// Device describes one MikroTik router.
type Device struct {
	Title                string
	Host                 string
	Port                 int
	Username             string
	Password             string
	APISSL               bool
	TLSVerify            bool
	Timeout              time.Duration
	TLSCAFiles           []string
	SSHPort              int
	SSHUsername          string
	SSHFingerprintSHA256 string
	Tags                 []string
	Region               string
}

// Client returns a RouterOS API client for this device. Connections are
// established lazily on first use.
func (d *Device) Client() *client.RouterOSClient {
	return client.NewRouterOSClient(d.Host, d.Username, d.Password,
		client.WithTLS(d.APISSL),
		client.WithTLSVerify(d.TLSVerify),
		client.WithPort(d.Port),
		client.WithTimeout(d.Timeout),
		client.WithTLSCAFiles(d.TLSCAFiles),
	)
}

// Registry holds a validated set of devices, keyed by case-insensitive title.
type Registry struct {
	devices []Device
	byTitle map[string]int
}

// Single returns a registry with one device (the flat-env fallback).
func Single(d Device) *Registry {
	return &Registry{devices: []Device{d}, byTitle: map[string]int{strings.ToLower(d.Title): 0}}
}

// Configured reports whether a fleet inventory is configured at all.
func Configured() bool {
	return os.Getenv("MIKROTIK_INVENTORY") != "" || os.Getenv("MIKROTIK_INVENTORY_FILE") != ""
}

// FromEnv parses the inventory from the environment (inline wins over file).
func FromEnv() (*Registry, error) {
	data, err := readInventoryEnv()
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

func readInventoryEnv() ([]byte, error) {
	if inline := os.Getenv("MIKROTIK_INVENTORY"); inline != "" {
		return []byte(inline), nil
	}
	path := os.Getenv("MIKROTIK_INVENTORY_FILE")
	if path == "" {
		return nil, errors.New("no inventory configured")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("MIKROTIK_INVENTORY_FILE %s: %v", path, err)
	}
	return data, nil
}

// Parse builds a registry from JSON. Validation errors never echo credentials.
func Parse(data []byte) (*Registry, error) {
	var raw []deviceJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid inventory JSON: %v", err)
	}
	if len(raw) == 0 {
		return nil, errors.New("inventory must contain at least one device")
	}
	r := &Registry{byTitle: map[string]int{}}
	for _, j := range raw {
		d, err := j.toDevice()
		if err != nil {
			return nil, err
		}
		key := strings.ToLower(d.Title)
		if _, dup := r.byTitle[key]; dup {
			return nil, fmt.Errorf("duplicate device title %q", d.Title)
		}
		r.byTitle[key] = len(r.devices)
		r.devices = append(r.devices, d)
	}
	return r, nil
}

func (r *Registry) Len() int { return len(r.devices) }
func (r *Registry) Titles() []string {
	titles := make([]string, len(r.devices))
	for i, d := range r.devices {
		titles[i] = d.Title
	}
	return titles
}

func (r *Registry) Devices() []Device { return r.devices }

// Default returns the first device (used for the single-device case).
func (r *Registry) Default() Device {
	if len(r.devices) == 0 {
		return Device{}
	}
	return r.devices[0]
}

// Get resolves a device by title (case-insensitive). An empty title selects
// the single device when there is exactly one; otherwise an error lists the
// available titles so the caller can correct itself.
func (r *Registry) Get(title string) (Device, error) {
	if len(r.devices) == 0 {
		return Device{}, errors.New("no devices configured")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		if len(r.devices) == 1 {
			return r.devices[0], nil
		}
		return Device{}, fmt.Errorf("device is required; available: %s", strings.Join(r.Titles(), ", "))
	}
	if i, ok := r.byTitle[strings.ToLower(title)]; ok {
		return r.devices[i], nil
	}
	return Device{}, fmt.Errorf("unknown device %q; available: %s", title, strings.Join(r.Titles(), ", "))
}

type deviceJSON struct {
	Title                string   `json:"title"`
	Host                 string   `json:"host"`
	Port                 int      `json:"port"`
	Username             string   `json:"username"`
	Password             string   `json:"password"`
	APISSL               *bool    `json:"api_ssl"`
	TLSVerify            *bool    `json:"tls_verify"`
	Timeout              float64  `json:"timeout"`
	SSHPort              int      `json:"ssh_port"`
	SSHUsername          string   `json:"ssh_username"`
	SSHFingerprintSHA256 string   `json:"ssh_fingerprint"`
	Tags                 []string `json:"tags"`
	Region               string   `json:"region"`
}

func (j *deviceJSON) toDevice() (Device, error) {
	d := Device{
		Title:                strings.TrimSpace(j.Title),
		Host:                 strings.TrimSpace(j.Host),
		Port:                 j.Port,
		Username:             strings.TrimSpace(j.Username),
		Password:             j.Password,
		APISSL:               true,
		TLSVerify:            true,
		Timeout:              10 * time.Second,
		SSHPort:              j.SSHPort,
		SSHUsername:          strings.TrimSpace(j.SSHUsername),
		SSHFingerprintSHA256: strings.TrimSpace(j.SSHFingerprintSHA256),
		Tags:                 j.Tags,
		Region:               strings.TrimSpace(j.Region),
	}
	if d.Port == 0 {
		d.Port = 8728
	}
	if d.Username == "" {
		d.Username = "admin"
	}
	if j.APISSL != nil {
		d.APISSL = *j.APISSL
	}
	if j.TLSVerify != nil {
		d.TLSVerify = *j.TLSVerify
	}
	if j.Timeout > 0 {
		d.Timeout = time.Duration(j.Timeout * float64(time.Second))
	}
	if d.SSHPort == 0 {
		d.SSHPort = 22
	}
	if d.SSHUsername == "" {
		d.SSHUsername = d.Username
	}
	if d.Title == "" {
		return Device{}, errors.New("inventory device: title is required")
	}
	if d.Host == "" {
		return Device{}, fmt.Errorf("inventory device %q: host is required", d.Title)
	}
	return d, nil
}
