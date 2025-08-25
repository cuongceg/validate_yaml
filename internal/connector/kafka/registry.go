package kafka

import "github.com/cuongceg/validate_yaml/internal/core"

func init() {
	core.RegisterConnector("kafka", NewKafkaConnector)
}
