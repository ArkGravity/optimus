package resource

// DatabaseEndpoint retains the native endpoint type and distinguishes unknown ports.
type DatabaseEndpoint struct {
	Address string `json:"address"`
	Port    *int32 `json:"port"`
	Type    string `json:"type"`
}
