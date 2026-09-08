package docker

// buildQueue builds the AMQP broker for the selected variant. Both brokers
// expose AMQP on 5672 and the management UI on 15672 with guest/guest, so the
// shop's DSN never changes.
func buildQueue(e *Environment, svc service) composeService {
	variant, version := svc.selected(e)

	queue := composeService{
		Image:   variant.Image + ":" + version,
		Volumes: []string{"lavinmq-data:/var/lib/lavinmq:rw"},
	}

	if variant.Name == QueueRabbitMQ {
		queue.Volumes = []string{"rabbitmq-data:/var/lib/rabbitmq:rw"}
		// RabbitMQ restricts the guest user to loopback connections; lift
		// that so the web container can connect with the shared DSN.
		queue.Environment = yamlMap[string]{}.
			set("RABBITMQ_SERVER_ADDITIONAL_ERL_ARGS", "-rabbit loopback_users []")
		queue.Healthcheck = &composeHealthcheck{
			Test:          []string{"CMD", "rabbitmq-diagnostics", "-q", "ping"},
			StartPeriod:   "20s",
			StartInterval: "3s",
			Interval:      "10s",
			Timeout:       "5s",
			Retries:       10,
		}
	}

	return queue
}
