package cli

import (
	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
)

type NatsOptions struct {
	NatsUrl         string `short:"n" env:"NVIX_STORE_NATS_URL" default:"nats://localhost:4222"`
	NatsCredentials string `short:"c" env:"NVIX_STORE_NATS_CREDENTIALS_FILE" required:"" type:"path"`
}

func (no *NatsOptions) Connect() *nats.Conn {
	conn, err := nats.Connect(no.NatsUrl, nats.UserCredentials(no.NatsCredentials))
	if err != nil {
		log.Fatalf("failed to connect to nats: %v", err)
	}
	return conn
}
