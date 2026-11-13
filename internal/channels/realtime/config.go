package realtime

// Config contains process-level settings for the Realtime WebSocket server.
type Config struct {
	// Addr is the TCP listen address.
	Addr string
	// Path is the WebSocket endpoint path.
	Path string
}

// DefaultConfig returns the default Realtime server settings.
func DefaultConfig() *Config {
	return &Config{Addr: ":8090", Path: "/v1/realtime"}
}

func (c *Config) normalize() {
	if c.Addr == "" {
		c.Addr = ":8090"
	}
	if c.Path == "" {
		c.Path = "/v1/realtime"
	}
}
