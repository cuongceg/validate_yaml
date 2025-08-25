package kafka

type Config struct {
	Name      string         `yaml:"name"`
	Brokers   []string       `yaml:"brokers"`
	ClientID  string         `yaml:"clientId"`
	SASL      *SASL          `yaml:"sasl,omitempty"`
	TLS       *TLS           `yaml:"tls,omitempty"`
	Ingresses []IngressCfg   `yaml:"ingress"`
	Egresses  []EgressCfg    `yaml:"egress"`
	Extra     map[string]any `yaml:"extra,omitempty"`
}

type SASL struct {
	Enable    bool   `yaml:"enable"`
	Mechanism string `yaml:"mechanism"`
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
}
type TLS struct {
	Enable   bool   `yaml:"enable"`
	Insecure bool   `yaml:"insecure"`
	CAFile   string `yaml:"caFile"`
	CertFile string `yaml:"certFile"`
	KeyFile  string `yaml:"keyFile"`
}

type IngressCfg struct {
	SourceName string `yaml:"sourceName"`
	Topic      string `yaml:"topics"`
	GroupID    string `yaml:"groupId"`
}

type EgressCfg struct {
	TargetName string `yaml:"targetName"`
	Topic      string `yaml:"topic"`
	KeyFrom    string `yaml:"keyFrom,omitempty"`
}
