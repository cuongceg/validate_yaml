package config

func (c *Config) ApplyDefaults() {
	if c.SchemaVersion == 0 {
		c.SchemaVersion = 1
	}
	if c.App.LogLevel == "" {
		c.App.LogLevel = "info"
	}
	if c.App.MetricsAddr == "" {
		c.App.MetricsAddr = ":9090"
	}
	if c.Compression == "" {
		c.Compression = "none"
	}
	if c.Persistence.Type == "" {
		c.Persistence.Type = "badger"
	}
	if c.Persistence.Path == "" {
		c.Persistence.Path = "./data"
	}

	for i := range c.Routes {
		rt := &c.Routes[i]
		if rt.Direction == "" {
			rt.Direction = Forward
		}
		// QoS
		if rt.QoS.Acks == "" {
			rt.QoS.Acks = AtLeastOnce
		}
		if rt.QoS.MaxInflight == 0 {
			rt.QoS.MaxInflight = 1000
		}
		// TTL
		if rt.TTL.Timeout.Duration == 0 {
			rt.TTL.Timeout.Duration = 30_000_000_000 // 30s
		}
		if rt.TTL.MaxHops == 0 {
			rt.TTL.MaxHops = 3
		}
	}
}
