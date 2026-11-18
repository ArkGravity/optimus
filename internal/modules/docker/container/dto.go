package container

import "time"

// Summary is one live container row. Ports are display strings such as
// "0.0.0.0:8080->80/tcp" or "443/tcp" for unpublished ports.
type Summary struct {
	ID      string    `json:"id"`
	ShortID string    `json:"short_id"`
	Name    string    `json:"name"`
	Image   string    `json:"image"`
	ImageID string    `json:"image_id"`
	Command string    `json:"command"`
	State   string    `json:"state"`
	Status  string    `json:"status"`
	Ports   []string  `json:"ports"`
	Created time.Time `json:"created"`
}

type ListResponse struct {
	Items []Summary `json:"items"`
	Total int       `json:"total"`
}

// Detail is the inspect summary shown in the container drawer.
type Detail struct {
	ID            string        `json:"id"`
	ShortID       string        `json:"short_id"`
	Name          string        `json:"name"`
	Image         string        `json:"image"`
	ImageID       string        `json:"image_id"`
	Created       *time.Time    `json:"created,omitempty"`
	State         State         `json:"state"`
	Command       []string      `json:"command"`
	RestartPolicy RestartPolicy `json:"restart_policy"`
	RestartCount  int           `json:"restart_count"`
	Ports         []string      `json:"ports"`
	Networks      []Network     `json:"networks"`
	Mounts        []Mount       `json:"mounts"`
	Env           []string      `json:"env"`
}

type State struct {
	Status     string     `json:"status"`
	Running    bool       `json:"running"`
	Paused     bool       `json:"paused"`
	Restarting bool       `json:"restarting"`
	OOMKilled  bool       `json:"oom_killed"`
	Dead       bool       `json:"dead"`
	ExitCode   int        `json:"exit_code"`
	Error      string     `json:"error,omitempty"`
	Health     string     `json:"health,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type RestartPolicy struct {
	Name              string `json:"name"`
	MaximumRetryCount int    `json:"maximum_retry_count"`
}

type Network struct {
	Name        string `json:"name"`
	IPAddress   string `json:"ip_address,omitempty"`
	IPv6Address string `json:"ipv6_address,omitempty"`
	Gateway     string `json:"gateway,omitempty"`
	MacAddress  string `json:"mac_address,omitempty"`
}

type Mount struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Mode        string `json:"mode,omitempty"`
	RW          bool   `json:"rw"`
}
