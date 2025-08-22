package config

type UserConfig struct {
	Connectors     []Connector      `yaml:"connectors"`
	Filters        []FilterRule     `yaml:"filters,omitempty"`
	Projections    []ProjectionRule `yaml:"projections,omitempty"`
	GroupReceivers []GroupReceiver  `yaml:"group_receivers,omitempty"`
	Routes         []Route          `yaml:"routes"`
}

type Connector struct {
	Name    string                 `yaml:"name"`
	Type    string                 `yaml:"type"` // kafka|nats|rabbitmq
	Params  map[string]interface{} `yaml:"params"`
	TLS     *TLSConfig             `yaml:"tls,omitempty"`
	Ingress []Ingress              `yaml:"ingress,omitempty"`
	Egress  []Egress               `yaml:"egress,omitempty"`
}

type TLSConfig struct {
	Enabled            bool   `yaml:"enabled"`
	CAFile             string `yaml:"ca_file,omitempty"`
	CertFile           string `yaml:"cert_file,omitempty"`
	KeyFile            string `yaml:"key_file,omitempty"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify,omitempty"`
}

type Ingress struct {
	Topic      string `yaml:"topic,omitempty"`
	Queue      string `yaml:"queue,omitempty"`
	Subject    string `yaml:"subject,omitempty"`
	GroupID    string `yaml:"group_id,omitempty"`
	SourceName string `yaml:"source_name"` // tên logic để map route
}

type Egress struct {
	Type               string `yaml:"type"` // topic|exchange|subject
	Name               string `yaml:"name"`
	TopicTemplate      string `yaml:"topic_template,omitempty"`
	SubjectTemplate    string `yaml:"subject_template,omitempty"`
	Exchange           string `yaml:"exchange,omitempty"`
	Kind               string `yaml:"kind,omitempty"`
	RoutingKeyTemplate string `yaml:"routing_key_template,omitempty"`
}

type FilterRule struct {
	Name           string `yaml:"name"`
	Expr           string `yaml:"expr"`
	OnMissingField string `yaml:"on_missing_field,omitempty"` // drop|skip|false
}

type ProjectionRule struct {
	Name           string   `yaml:"name"`
	Include        []string `yaml:"include"`
	BestEffort     bool     `yaml:"best_effort,omitempty"`
	OnMissingField string   `yaml:"on_missing_field,omitempty"` // drop|skip|false
}

type GroupReceiver struct {
	Name    string          `yaml:"name"`
	Targets []RouteEndpoint `yaml:"targets"`
}

type ReceiverTarget struct {
	Connector string       `yaml:"connector"`
	Target    TargetConfig `yaml:"target"`
}

type TargetConfig struct {
	Type               string `yaml:"type"` // topic_template|subject_template|exchange
	Value              string `yaml:"value,omitempty"`
	Exchange           string `yaml:"exchange,omitempty"`
	Kind               string `yaml:"kind,omitempty"`
	RoutingKeyTemplate string `yaml:"routing_key_template,omitempty"`
}

type Route struct {
	Name       string          `yaml:"name"`
	From       RouteStartpoint `yaml:"from"`
	To         *RouteEndpoint  `yaml:"to,omitempty"`
	ToGroup    string          `yaml:"to_group,omitempty"`
	Mode       RouteMode       `yaml:"mode"`
	Filters    []string        `yaml:"filters,omitempty"`    // tham chiếu theo tên filter
	Projection string          `yaml:"projection,omitempty"` // tham chiếu theo tên projection
}

type RouteStartpoint struct {
	Connector string `yaml:"connector"`
	Source    string `yaml:"source,omitempty"`
	Target    string `yaml:"target,omitempty"`
}

type RouteEndpoint struct {
	Connector string `yaml:"connector"`
	Target    string `yaml:"target,omitempty"`
}

type RouteMode struct {
	Type        string `yaml:"type"` // persistent|drop
	TTLms       int64  `yaml:"ttl_ms,omitempty"`
	MaxAttempts int    `yaml:"max_attempts,omitempty"`
}
