package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cuongceg/validate_yaml/internal/config"
	"github.com/cuongceg/validate_yaml/internal/router"
	util "github.com/cuongceg/validate_yaml/internal/util"
	"github.com/cuongceg/validate_yaml/proto/pb"
	"github.com/redis/go-redis/v9"
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

	util.Init()

	userCfg, err := config.Load(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}

	connectors, err := util.MapConnectors(userCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
	defer func() {
		for _, c := range connectors {
			err = c.Close()
			if err != nil {
				fmt.Fprintf(os.Stderr, "❌ close connector %s: %v\n", c.Name(), err)
			}
		}
	}()

	util.App.Println("✅ Valid configuration & connectors created.")

	buses := make(map[string]router.Bus, len(connectors))
	for name, c := range connectors {
		b, err := router.NewBusFromConnector(c)
		if err != nil {
			fmt.Fprintf(os.Stderr, "❌ build bus for %s: %v\n", name, err)
			os.Exit(1)
		}
		buses[name] = b
	}

	codecs := router.NewProtoCodec[*pb.Envelope]()

	eng := &router.Engine{
		Buses:          buses,
		CodecsBySource: codecs,
		Filters:        router.BuiltinFilters(),
		Projections:    router.BuiltinProjections(),
		LanesPerTarget: 16,
		LaneBuffer:     20000,
	}

	ctx, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	// Connect to Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:6379",
		Password:     "", // nếu có requirepass thì đặt ở đây
		DB:           0,
		MinIdleConns: 4,
		PoolSize:     32,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		panic(err)
	}
	util.App.Println("Connected to Redis")

	stop, err := eng.StartRoutes(ctx, userCfg, rdb)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ start routes: %v\n", err)
		os.Exit(1)
	}
	defer stop()

	fmt.Println("🚚 Routes running… Press Ctrl+C to stop.")

	// ✅ Chờ context bị hủy bởi tín hiệu
	<-ctx.Done()
	fmt.Println("signal received, shutting down…")
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
