- Create docker-compose.yml
```
version: "3.9"

services:
  # JMX Exporter (standalone http server) kết nối tới JMX của Kafka
  jmx_exporter:
    image: bitnami/jmx-exporter:0.20.0
    container_name: jmx_exporter
    network_mode: "host"                # để container truy cập 127.0.0.1:9999 của host
    volumes:
      - ./kafka-jmx.yml:/config/kafka-jmx.yml:ro
    command: [ "9404", "/config/kafka-jmx.yml" ]
    restart: unless-stopped

  prometheus:
    image: prom/prometheus:v2.55.1
    container_name: prometheus
    network_mode: "host"                # expose trực tiếp :9090
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prom_data:/prometheus
    restart: unless-stopped

  grafana:
    image: grafana/grafana:11.2.0
    container_name: grafana
    network_mode: "host"                # expose trực tiếp :3000
    environment:
      - GF_SECURITY_ADMIN_USER=admin
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana_data:/var/lib/grafana
    restart: unless-stopped

volumes:
  prom_data:
  grafana_data:
```
- Create kafka-jmx.yml
```
startDelaySeconds: 0
jmxUrl: service:jmx:rmi:///jndi/rmi://127.0.0.1:9999/jmxrmi
lowercaseOutputName: true
lowercaseOutputLabelNames: true

# Bạn có thể thêm/đổi rules theo nhu cầu. Một số metric Kafka phổ biến:
rules:
  # Bytes In/Out per topic/broker
  - pattern: 'kafka.server<type=BrokerTopicMetrics, name=BytesInPerSec(?:, topic=(.+))?><>OneMinuteRate'
    name: kafka_brokertopicmetrics_bytesin_total
    labels:
      topic: "$1"
    type: GAUGE

  - pattern: 'kafka.server<type=BrokerTopicMetrics, name=BytesOutPerSec(?:, topic=(.+))?><>OneMinuteRate'
    name: kafka_brokertopicmetrics_bytesout_total
    labels:
      topic: "$1"
    type: GAUGE

  # Messages In per topic/broker
  - pattern: 'kafka.server<type=BrokerTopicMetrics, name=MessagesInPerSec(?:, topic=(.+))?><>OneMinuteRate'
    name: kafka_brokertopicmetrics_messagesin_total
    labels:
      topic: "$1"
    type: GAUGE

  # Network Request/Response (tùy chọn)
  - pattern: 'kafka.network<type=RequestMetrics, name=RequestBytesPerSec, request=(.+)><>OneMinuteRate'
    name: kafka_network_requestbytes_total
    labels:
      request: "$1"
    type: GAUGE

  - pattern: 'kafka.network<type=RequestMetrics, name=ResponseBytesPerSec, request=(.+)><>OneMinuteRate'
    name: kafka_network_responsebytes_total
    labels:
      request: "$1"
    type: GAUGE
```
- Create prometheus.yml
```
global:
  scrape_interval: 5s
  evaluation_interval: 5s

scrape_configs:
  # Kafka qua JMX Exporter (trên host)
  - job_name: "kafka-jmx"
    static_configs:
      - targets: ["localhost:9404"]

  # RabbitMQ trên VM (Prometheus plugin)
  - job_name: "rabbitmq"
    static_configs:
      - targets: ["virtual_machine_ip:15692"]
```
- Run docker-compose:
```
sudo docker-compose up -d
```
- Check status of data source is up on web [Prometheus](http://localhost:9090/)
- Add source data from Prometheus on web [Grafana](http://localhost:3000/)