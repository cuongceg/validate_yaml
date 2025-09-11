# How to start
## Install Kafka on Ubuntu(host machine)
- Install Java and check java version
- Download Kafka
```
sudo mkdir /home/kafka
cd /home/kafka
curl "https://downloads.apache.org/kafka/2.8.2/kafka_2.13-2.8.2.tgz" -o /home/kafka.tgz
sudo tar -xvzf /home/kafka.tgz --strip 1
```
- Create service in Ubuntu system
```
sudo nano /etc/systemd/system/kafka.service

[Service]

Type=simple

Environment="JAVA_HOME=/usr/lib/jvm/java-17-openjdk-amd64"

#JMX
Environment="JMX_PORT=9999"
Environment="KAFKA_OPTS=-Dcom.sun.management.jmxremote.local.only=false -Dcom.sun.management.jmxremote.rmi.port=9999 -Dcom.sun.management.jmxremote.port=9999 -Dcom.sun.management.jmxremote.authenticate=false -Dcom.sun.management.jmxremote.ssl=false -Djava.rmi.server.hostname=127.0.0.1"

ExecStart=/home/domanhcuong/Downloads/bin/kafka-server-start.sh /home/domanhcuong/Downloads/config/server.properties

ExecStop=/home/domanhcuong/Downloads/bin/kafka-server-stop.sh

Restart=on-abnormal

[Install]

WantedBy=multi-user.target
```
## Install RabbitMQ on Ubuntu(virtual machine)
```
sudo apt update
sudo apt install rabbitmq-server -y
sudo systemctl enable rabbitmq-server
sudo systemctl start rabbitmq-server
#enable the web UI on port 15672
sudo rabbitmq-plugins enable rabbitmq_management
# add users to sign in to web and grant admin permission to them
rabbitmqctl add_user "username" "password"
rabbitmqctl set_permissions -p "custom-vhost" "username" ".*" ".*" ".*"
rabbitmqctl set_user_tags "username" administrator
```
View web UI on virtual_machine_ip:15672
You can enable Grafana and Prometheus to view metrics from Kafka and RabbitMQ from [here](./GRAFANA.md)
## Example config files
```
connectors:
  - name: kafka_01
    type: kafka
    params:
      brokers: ["localhost:9092"]
    tls:
      enabled: true
      ca_file: /etc/ssl/certs/ca.pem
      # cert_file: /etc/ssl/certs/client.pem
      # key_file: /etc/ssl/private/client.key
      insecure_skip_verify: true
    ingress:
      - topic: topic-kafka1
        group_id: "kafka-group"
        source_name: kafka.app.input
    egress:
      - name: topic-kafka
        type: topic
        topic_template: topic-kafka1
  # - name: nats_core
  #   type: nats
  #   params:
  #     url: "nats://localhost:4222"
  #   tls:
  #     enabled: false
  #   ingress:
  #     - subject: "bridge.in.>"
  #       source_name: nats.bridge.in
  #   egress:
  #     - name: bridge_out
  #       type: subject
  #       subject_template: "bridge.out.{source_name}"

  - name: rabbit_main
    type: rabbitmq
    params:
      url: "amqp://username:password@127.0.0.1:5672/"
    tls:
      enabled: false
      ca_file: /etc/ssl/certs/ca.pem
      # cert_file: /etc/ssl/certs/client.pem
      # key_file: /etc/ssl/private/client.key
      insecure_skip_verify: true
    ingress:
      - queue: orders.payment
        source_name: rabbit.orders.payment
    egress:
      - name: orders_synced
        type: exchange
        exchange: orders
        kind: topic
        routing_key_template: "orders.synced.test"

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
  # - name: grp_sync_all
  #   targets:
  #     - connector: kafka_01
  #       target: synced_all
  #     - connector: nats_core
  #       target: bridge_out
  #     - connector: rabbit_main
  #       target: orders_synced

  # - name: grp_kafka_nats_hotpath
  #   targets:
  #     - connector: kafka_01
  #       target: synced_all

# ========== 5) ROUTES ==========
routes:
  - name: rabbit_orders_to_kafka
    from:
      connector: kafka_01
      source: kafka.app.input   
    to:
      connector: rabbit_main
      target: orders_synced
    mode:
      type: persistent
      ttl_ms: 60000                   # message expires after 60s if not delivered
      max_attempts: 10                  # max redelivery attempts                
    filters: [user_basic]
    projection: keep_name_age_best_effort  
  # - name: rabbit_orders_to_nats
  #   from:
  #     connector: rabbit_main
  #     source: rabbit.orders.payment
  #   to:
  #     connector: kafka_01
  #     target: topic-kafka
  #   mode:
  #     type: drop
  #     ttl_ms: 2000                   # message expires after 2s if not delivered
  #     max_attempts: 5                  # max redelivery attempts                
  #   filters: [user_basic]
  #   projection: keep_name_age_best_effort

  # - name: kafka_app_to_group_all
  #   from:
  #     connector: kafka_01
  #     source: kafka.app.input
  #   to:
  #     connector: kafka_01
  #     target: kafka.app.input
  #   mode:
  #     type: drop
  #     ttl_ms: -12                   
  #     max_attempts: 3                  
  #   filters: [user_basic]
  #   projection: keep_name_age_strict

  # - name: nats_bridge_in_to_kafka_hotpath
  #   from:
  #     connector: nats_core
  #     source: nats.bridge.in
  #   to_group: grp_kafka_nats_hotpath
  #   mode:
  #     type: persistent
  #   filters: [size_le_1mb, recent_1h]
  #   projection: keep_name_age_best_effort
```