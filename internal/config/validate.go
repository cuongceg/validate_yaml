package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

func ValidateConfig(cfg *UserConfig) error {
	v := validator.New()
	if err := v.Struct(cfg); err != nil {
		return err
	}

	var allErrs []error

	if err := validateConnectors(cfg); err != nil {
		allErrs = append(allErrs, err)
	}

	if err := validateGroupReceivers(cfg); err != nil {
		allErrs = append(allErrs, err)
	}

	// 4) Kiểm tra route: from/to hoặc to_group có tồn tại trong connectors
	if err := validateRoutesEndpoints(cfg); err != nil {
		allErrs = append(allErrs, err)
	}

	// 5) Kiểm tra filter & projection được khai báo
	if err := validateFiltersAndProjections(cfg); err != nil {
		allErrs = append(allErrs, err)
	}

	return joinErrors(allErrs)
}

func validateConnectors(cfg *UserConfig) error {
	var errs []error

	connectorNames := make(map[string]struct{}, len(cfg.Connectors))
	for i, c := range cfg.Connectors {
		if strings.TrimSpace(c.Name) == "" {
			errs = append(errs, fmt.Errorf("connectors[%d]: name is required", i))
			continue
		}
		if _, ok := connectorNames[c.Name]; ok {
			errs = append(errs, fmt.Errorf("connectors[%d]: duplicate connector name %q", i, c.Name))
		}
		connectorNames[c.Name] = struct{}{}

		switch strings.ToLower(c.Type) {
		case "kafka":
			// yêu cầu: params.brokers (slice non-empty)
			if err := requireKafkaParams(i, &c); err != nil {
				errs = append(errs, err)
			}
		case "rabbitmq":
			// yêu cầu: params.url (string non-empty)
			if err := requireStringParam(i, &c, "url"); err != nil {
				errs = append(errs, err)
			}
		case "nats":
			// yêu cầu: params.url (string non-empty)
			if err := requireStringParam(i, &c, "url"); err != nil {
				errs = append(errs, err)
			}
		default:
			errs = append(errs, fmt.Errorf("connectors[%d]: unsupported type %q (expect: kafka|rabbitmq|nats)", i, c.Type))
		}

		// TLS: nếu enabled và verify=true (tức là !insecure_skip_verify) thì cần CAFile;
		// nếu có CertFile hoặc KeyFile thì bắt buộc phải có cả hai.
		if c.TLS != nil && c.TLS.Enabled {
			verify := !c.TLS.InsecureSkipVerify
			if verify {
				if strings.TrimSpace(c.TLS.CAFile) == "" {
					errs = append(errs, fmt.Errorf("connectors[%d]: tls.enabled=true & verify=true requires ca_file", i))
				}
			}
			hasCert := strings.TrimSpace(c.TLS.CertFile) != ""
			hasKey := strings.TrimSpace(c.TLS.KeyFile) != ""
			if hasCert != hasKey {
				errs = append(errs, fmt.Errorf("connectors[%d]: tls cert_file and key_file must be provided together", i))
			}
		}

		// Ingress/Egress: validate đặt tên nguồn/đích để tham chiếu trong routes
		ingressNames := make(map[string]struct{})
		for j, in := range c.Ingress {
			if strings.TrimSpace(in.SourceName) == "" {
				errs = append(errs, fmt.Errorf("connectors[%d].ingress[%d]: source_name is required", i, j))
			} else {
				if _, ok := ingressNames[in.SourceName]; ok {
					errs = append(errs, fmt.Errorf("connectors[%d].ingress[%d]: duplicated source_name %q", i, j, in.SourceName))
				}
				ingressNames[in.SourceName] = struct{}{}
			}
			// Ít nhất phải có một trong topic/queue/subject
			if in.Topic == "" && in.Queue == "" && in.Subject == "" {
				errs = append(errs, fmt.Errorf("connectors[%d].ingress[%d]: require one of topic|queue|subject", i, j))
			}
		}

		egressNames := make(map[string]struct{})
		for j, eg := range c.Egress {
			// BẮT BUỘC có Name để Route tham chiếu
			getName := strings.TrimSpace(egName(&eg))
			if getName == "" {
				errs = append(errs, fmt.Errorf("connectors[%d].egress[%d]: name is required (add field 'name' to egress)", i, j))
			} else {
				if _, ok := egressNames[getName]; ok {
					errs = append(errs, fmt.Errorf("connectors[%d].egress[%d]: duplicated egress name %q", i, j, getName))
				}
				egressNames[getName] = struct{}{}
			}
			// Ít nhất phải có một đích hợp lệ theo type
			switch strings.ToLower(eg.Type) {
			case "topic":
				if strings.TrimSpace(eg.TopicTemplate) == "" {
					errs = append(errs, fmt.Errorf("connectors[%d].egress[%d]: type=topic requires topic_template", i, j))
				}
			case "subject":
				if strings.TrimSpace(eg.SubjectTemplate) == "" {
					errs = append(errs, fmt.Errorf("connectors[%d].egress[%d]: type=subject requires subject_template", i, j))
				}
			case "exchange":
				if strings.TrimSpace(eg.Exchange) == "" || strings.TrimSpace(eg.RoutingKeyTemplate) == "" {
					errs = append(errs, fmt.Errorf("connectors[%d].egress[%d]: type=exchange requires exchange and routing_key_template", i, j))
				}
			case "":
				errs = append(errs, fmt.Errorf("connectors[%d].egress[%d]: type is required (topic|subject|exchange)", i, j))
			default:
				errs = append(errs, fmt.Errorf("connectors[%d].egress[%d]: unsupported egress type %q", i, j, eg.Type))
			}
		}
	}

	return joinErrors(errs)
}

func validateGroupReceivers(cfg *UserConfig) error {
	var errs []error

	type egressKey struct{ conn, name string }
	connectorNames := make(map[string]struct{}, len(cfg.Connectors))
	egressIdx := map[egressKey]bool{}
	for _, c := range cfg.Connectors {
		connectorNames[c.Name] = struct{}{}

		for _, eg := range c.Egress {
			egressIdx[egressKey{c.Name, egName(&eg)}] = true
		}
	}

	for i, g := range cfg.GroupReceivers {

		if _, clash := connectorNames[g.Name]; clash {
			errs = append(errs, fmt.Errorf("group_receivers[%d]: name %q conflicts with existing connector name", i, g.Name))
		}

		for j, t := range g.Targets {
			if strings.TrimSpace(t.Connector) == "" {
				errs = append(errs, fmt.Errorf("group_receivers[%d].targets[%d]: connector is required", i, j))
				continue
			}

			if _, ok := connectorNames[t.Connector]; !ok {
				errs = append(errs, fmt.Errorf("group_receivers[%d].targets[%d]: connector %q not found", i, j, t.Connector))
			}

			if !egressIdx[egressKey{t.Connector, t.Target}] {
				errs = append(errs, fmt.Errorf("group_receivers[%d].targets[%d]: target %q not found in connector %q", i, j, t.Target, t.Connector))
			}
		}
	}

	return joinErrors(errs)
}

func validateRoutesEndpoints(cfg *UserConfig) error {
	var errs []error

	connectorMap := map[string]*Connector{}
	for i := range cfg.Connectors {
		c := &cfg.Connectors[i]
		connectorMap[c.Name] = c
	}

	type ingressKey struct{ conn, src string }
	type egressKey struct{ conn, name string }
	ingressIdx := map[ingressKey]bool{}
	egressIdx := map[egressKey]bool{}

	for _, c := range cfg.Connectors {
		for _, in := range c.Ingress {
			ingressIdx[ingressKey{c.Name, in.SourceName}] = true
		}
		for _, eg := range c.Egress {
			egressIdx[egressKey{c.Name, egName(&eg)}] = true
		}
	}

	groupSet := map[string]bool{}
	for _, g := range cfg.GroupReceivers {
		groupSet[g.Name] = true
	}

	for i, r := range cfg.Routes {
		if strings.TrimSpace(r.From.Connector) == "" {
			errs = append(errs, fmt.Errorf("routes[%d].from.connector is required", i))
			continue
		}
		if _, ok := connectorMap[r.From.Connector]; !ok {
			errs = append(errs, fmt.Errorf("routes[%d].from.connector %q not found", i, r.From.Connector))
			continue
		}
		if strings.TrimSpace(r.From.Source) == "" {
			errs = append(errs, fmt.Errorf("routes[%d].from requires either source", i))
		}
		if r.From.Source != "" {
			if !ingressIdx[ingressKey{r.From.Connector, r.From.Source}] {
				errs = append(errs, fmt.Errorf("routes[%d].from: source %q not found in connector %q", i, r.From.Source, r.From.Connector))
			}
		}

		// to hoặc to_group
		hasTo := r.To != nil && strings.TrimSpace(r.To.Connector) != ""
		hasGroup := strings.TrimSpace(r.ToGroup) != ""
		if !hasTo && !hasGroup {
			errs = append(errs, fmt.Errorf("routes[%d]: either 'to' or 'to_group' must be provided", i))
			continue
		}
		if hasTo && hasGroup {
			errs = append(errs, fmt.Errorf("routes[%d]: cannot set both 'to' and 'to_group'", i))
		}
		if hasTo {
			if _, ok := connectorMap[r.To.Connector]; !ok {
				errs = append(errs, fmt.Errorf("routes[%d].to.connector %q not found", i, r.To.Connector))
			} else {
				if strings.TrimSpace(r.To.Target) == "" {
					errs = append(errs, fmt.Errorf("routes[%d].to.target is required (egress name)", i))
				} else {
					if !egressIdx[egressKey{r.To.Connector, r.To.Target}] {
						errs = append(errs, fmt.Errorf("routes[%d].to: target %q not found in connector %q", i, r.To.Target, r.To.Connector))
					}
				}
			}
		}
		if hasGroup {
			if !groupSet[r.ToGroup] {
				errs = append(errs, fmt.Errorf("routes[%d].to_group %q not found", i, r.ToGroup))
			}
		}

		// mode
		switch strings.ToLower(r.Mode.Type) {
		case "persistent":
			// ok
		case "drop":
			route := &cfg.Routes[i]
			if route.Mode.TTLms <= 0 {
				route.Mode.TTLms = 180000 // 3 minutes default
			}

			if route.Mode.TTLms <= 0 {
				route.Mode.TTLms = 3 // default 3 attempts
			}
		default:
			errs = append(errs, fmt.Errorf("routes[%d].mode.type must be 'persistent' or 'drop'", i))
		}
	}

	return joinErrors(errs)
}

func validateFiltersAndProjections(cfg *UserConfig) error {
	var errs []error

	filterSet := map[string]bool{}
	for _, f := range cfg.Filters {
		filterSet[f.Name] = true
	}
	projSet := map[string]bool{}
	for _, p := range cfg.Projections {
		projSet[p.Name] = true
	}

	for i, r := range cfg.Routes {
		for _, fname := range r.Filters {
			if !filterSet[fname] {
				errs = append(errs, fmt.Errorf("routes[%d]: filter %q not declared", i, fname))
			}
		}
		if r.Projection != "" && !projSet[r.Projection] {
			errs = append(errs, fmt.Errorf("routes[%d]: projection %q not declared", i, r.Projection))
		}
	}

	return joinErrors(errs)
}

// ------- Helpers

func requireStringParam(idx int, c *Connector, key string) error {
	v, ok := c.Params[key]
	if !ok {
		return fmt.Errorf("connectors[%d] %q: params.%s is required", idx, c.Name, key)
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return fmt.Errorf("connectors[%d] %q: params.%s must be non-empty string", idx, c.Name, key)
	}
	return nil
}

func requireKafkaParams(idx int, c *Connector) error {
	v, ok := c.Params["brokers"]
	if !ok {
		return fmt.Errorf("connectors[%d] %q: params.brokers is required", idx, c.Name)
	}
	switch vv := v.(type) {
	case []interface{}:
		if len(vv) == 0 {
			return fmt.Errorf("connectors[%d] %q: params.brokers must be non-empty", idx, c.Name)
		}
		// xác nhận tất cả là string
		for k, item := range vv {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("connectors[%d] %q: params.brokers[%d] must be string", idx, c.Name, k)
			}
		}
	case []string:
		if len(vv) == 0 {
			return fmt.Errorf("connectors[%d] %q: params.brokers must be non-empty", idx, c.Name)
		}
	default:
		return fmt.Errorf("connectors[%d] %q: params.brokers must be []string", idx, c.Name)
	}
	return nil
}

func validateTargetConfig(gi, tj int, t *TargetConfig) error {
	switch strings.ToLower(t.Type) {
	case "topic_template":
		if strings.TrimSpace(t.Value) == "" {
			return fmt.Errorf("group_receivers[%d].targets[%d]: type=topic_template requires value", gi, tj)
		}
	case "subject_template":
		if strings.TrimSpace(t.Value) == "" {
			return fmt.Errorf("group_receivers[%d].targets[%d]: type=subject_template requires value", gi, tj)
		}
	case "exchange":
		if strings.TrimSpace(t.Exchange) == "" || strings.TrimSpace(t.RoutingKeyTemplate) == "" {
			return fmt.Errorf("group_receivers[%d].targets[%d]: type=exchange requires exchange and routing_key_template", gi, tj)
		}
	default:
		return fmt.Errorf("group_receivers[%d].targets[%d]: unsupported target type %q", gi, tj, t.Type)
	}
	return nil
}

func egName(e *Egress) string {
	if e.Name != "" {
		return e.Name
	}
	return "" // sẽ bị bắt lỗi ở validateConnectors
}

func joinErrors(errs []error) error {
	var filtered []string
	for _, e := range errs {
		if e != nil {
			filtered = append(filtered, e.Error())
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return errors.New(strings.Join(filtered, "\n"))
}
