package nats

import (
	"crypto/tls"
	"time"
)

type TLSConfig struct {
	Enabled            bool   `yaml:"enabled" json:"enabled"`
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify" json:"insecureSkipVerify"`
	CAFile             string `yaml:"caFile" json:"caFile"`
	CertFile           string `yaml:"certFile" json:"certFile"`
	KeyFile            string `yaml:"keyFile" json:"keyFile"`
}

type AuthConfig struct {
	// Choose one of: User/Pass, Token, or NKey (seed/seedFile handled by caller)
	Username     string `yaml:"username" json:"username"`
	Password     string `yaml:"password" json:"password"`
	Token        string `yaml:"token" json:"token"`
	NKeySeed     string `yaml:"nkeySeed" json:"nkeySeed"`
	NKeySeedFile string `yaml:"nkeySeedFile" json:"nkeySeedFile"`
}

type IngressJetStream struct {
	Enabled       bool          `yaml:"enabled" json:"enabled"`
	Subject       string        `yaml:"subject" json:"subject"`
	Durable       string        `yaml:"durable" json:"durable"`
	Queue         string        `yaml:"queue" json:"queue"`
	AckWait       time.Duration `yaml:"ackWait" json:"ackWait"`
	MaxAckPending int           `yaml:"maxAckPending" json:"maxAckPending"`
	PullMode      bool          `yaml:"pullMode" json:"pullMode"`
	PullBatch     int           `yaml:"pullBatch" json:"pullBatch"`
	PullInterval  time.Duration `yaml:"pullInterval" json:"pullInterval"`
}

type IngressConfig struct {
	SourceName    string           `yaml:"sourceName" json:"sourceName"`
	Subjects      []string         `yaml:"subjects" json:"subjects"`           // core NATS subjects (no JS)
	HeadersToMeta []string         `yaml:"headersToMeta" json:"headersToMeta"` // copy selected headers into meta
	KeyFrom       string           `yaml:"keyFrom" json:"keyFrom"`             // payload.userId or header.X-Device
	JS            IngressJetStream `yaml:"jetstream" json:"jetstream"`
}

type EgressJetStream struct {
	Enabled     bool   `yaml:"enabled" json:"enabled"`
	Subject     string `yaml:"subject" json:"subject"`
	EnableDeDup bool   `yaml:"enableDeDup" json:"enableDeDup"` // set Nats-Msg-Id from meta["idempotencyKey"] or route_key
}

type EgressConfig struct {
	TargetName string            `yaml:"targetName" json:"targetName"`
	Subject    string            `yaml:"subject" json:"subject"` // may include ${route_key}
	Headers    map[string]string `yaml:"headers" json:"headers"`
	JS         EgressJetStream   `yaml:"jetstream" json:"jetstream"`
}

type ConnectorConfig struct {
	Name          string        `yaml:"name" json:"name"`
	Servers       []string      `yaml:"servers" json:"servers"`
	ClientName    string        `yaml:"clientName" json:"clientName"`
	ReconnectWait time.Duration `yaml:"reconnectWait" json:"reconnectWait"`
	MaxReconnects int           `yaml:"maxReconnects" json:"maxReconnects"`
	TLS           TLSConfig     `yaml:"tls" json:"tls"`
	Auth          AuthConfig    `yaml:"auth" json:"auth"`

	Ingresses []IngressConfig `yaml:"ingresses" json:"ingresses"`
	Egresses  []EgressConfig  `yaml:"egresses" json:"egresses"`
}

func (t TLSConfig) ToTLSConfig() *tls.Config {
	if !t.Enabled {
		return nil
	}
	return &tls.Config{InsecureSkipVerify: t.InsecureSkipVerify} // load CA/cert/key outside
}
