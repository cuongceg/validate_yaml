package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cuongceg/validate_yaml/internal/config"
	"github.com/cuongceg/validate_yaml/internal/router"
	util "github.com/cuongceg/validate_yaml/internal/util"
	"github.com/cuongceg/validate_yaml/proto/pb"
)

func main() {
	var path string
	var showExample bool
	flag.StringVar(&path, "config", "configs/config.example.yaml", "Đường dẫn file cấu hình YAML")
	flag.BoolVar(&showExample, "example", false, "Hiển thị cấu hình mẫu và thoát (không kiểm tra)")
	flag.Parse()

	if showExample {
		fmt.Println("Cấu hình mẫu:")
		fmt.Println(sampleConfig())
		os.Exit(0)
	}

	userCfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	connectors, err := util.MapConnectors(ctx, userCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
	defer func() {
		for _, c := range connectors {
			_ = c.Close()
		}
	}()

	fmt.Println("✅ Valid configuration & connectors created.")

	buses := make(map[string]router.Bus, len(connectors))
	for name, c := range connectors {
		b, err := router.NewBusFromConnector(c) // dùng interface Connector/Ingress/Egress bạn đã định nghĩa
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ build bus for %s: %v\n", name, err)
			os.Exit(1)
		}
		buses[name] = b
	}

	codecs := map[string]router.PayloadCodec{
		"kafka.app.input": router.NewProtoCodec[*pb.Envelope](),
		// "rabbit.orders.inbox": router.NewProtoCodec[*pb.OrderV2](),
		// "nats.bridge.in":      router.NewProtoCodec[*pb.Whatever](),
	}

	eng := &router.Engine{
		Buses:          buses,
		CodecsBySource: codecs,
		Filters:        router.BuiltinFilters(),     // có sẵn size_le_1mb, recent_1h, user_basic
		Projections:    router.BuiltinProjections(), // có sẵn keep_name_age_* (best_effort/strict)
	}

	stop, err := eng.StartRoutes(ctx, userCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ start routes: %v\n", err)
		os.Exit(1)
	}
	defer stop()

	fmt.Println("🚚 Routes running… Press Ctrl+C to stop.")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigCh:
		log.Println("signal received, shutting down…")
	case <-ctx.Done():
		log.Println("context canceled, shutting down…")
	}
}

func sampleConfig() string {
	return `
	connectors:
	- name: kafka_01
	  type: kafka
	  params:
		brokers: ["kafka-1:9093","kafka-2:9093"]
	  tls:
		enabled: true
		ca_file: /etc/ssl/certs/ca.pem
		# cert_file: /etc/ssl/certs/client.pem
		# key_file: /etc/ssl/private/client.key
		insecure_skip_verify: false
	  ingress:
		- topic: app.input
		  source_name: kafka.app.input
	  egress:
		- name: synced_all
		  type: topic
		  topic_template: "bridge.synced.{source_name}"
  
	- name: nats_core
	  type: nats
	  params:
		url: "nats://nats:4222"
	  tls:
		enabled: false
	  ingress:
		- subject: "bridge.in.>"
		  source_name: nats.bridge.in
	  egress:
		- name: bridge_out
		  type: subject
		  subject_template: "bridge.out.{source_name}"
  
	- name: rabbit_main
	  type: rabbitmq
	  params:
		url: "amqps://user:pass@rabbitmq:5671/"
	  tls:
		enabled: true
		ca_file: /etc/ssl/certs/ca.pem
		# cert_file: /etc/ssl/certs/client.pem
		# key_file: /etc/ssl/private/client.key
		insecure_skip_verify: false
	  ingress:
		- queue: orders.inbox
		  source_name: rabbit.orders.inbox
	  egress:
		- name: orders_synced
		  type: exchange
		  exchange: orders
		  kind: topic
		  routing_key_template: "orders.synced.{source_name}"
  
  # ========== 2) FILTER RULES (CEL) ==========
  # drop is the default action if a filter fiekd is missing
  filters:
	- name: user_basic
	  expr: 'has(payload.name) && (payload.age > 16 || payload.name.contains("A"))'
	  on_missing_field: drop   # drop | skip | false
  
	- name: size_le_1mb
	  expr: 'meta.size <= 1048576'
  
	- name: recent_1h
	  expr: 'meta.createdAtMs >= nowMs() - 3600 * 1000'
  
  # ========== 3) PROJECTIONS (Minimum Payload) ==========
  # - best_effort: true => "có trường nào thì lấy trường đó" (không drop nếu thiếu)
  # - on_missing_field: drop|skip|false (áp dụng khi best_effort=false)
  projections:
	- name: keep_name_age_best_effort
	  include:
		- payload.name
		- payload.age
	  best_effort: true
  
	- name: keep_name_age_strict
	  include:
		- payload.name
		- payload.age
	  best_effort: false
	  on_missing_field: drop
  
  # ========== 4) GROUP RECEIVERS ==========
  group_receivers:
	- name: grp_sync_all
	  targets:
		- connector: kafka_01
		  target: synced_all
		- connector: nats_core
		  target: bridge_out
		- connector: rabbit_main
		  target: orders_synced
  
	- name: grp_kafka_nats_hotpath
	  targets:
		- kafka_01
		  target: synced_all
  
  # ========== 5) ROUTES ==========
  routes:
	- name: rabbit_orders_to_kafka
	  from:
		connector: rabbit_main
		source: rabbit.orders.inbox    
	  to:
		connector: kafka_01
		target: synced_all
	  mode:
		type: persistent                 
	  filters: [user_basic, size_le_1mb, recent_1h]
	  projection: keep_name_age_best_effort
  
	- name: kafka_app_to_group_all
	  from:
		connector: kafka_01
		source: kafka.app.input
	  to_group: grp_sync_all             
	  mode:
		type: drop
		ttl_ms: 600000                   
		max_attempts: 3                  
	  filters: [user_basic]
	  projection: keep_name_age_strict
  
	- name: nats_bridge_in_to_kafka_hotpath
	  from:
		connector: nats_core
		source: nats.bridge.in
	  to_group: grp_kafka_nats_hotpath
	  mode:
		type: persistent
	  filters: [size_le_1mb, recent_1h]
	  projection: keep_name_age_best_effort
  `
}
