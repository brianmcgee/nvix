package store

import (
	"github.com/charmbracelet/log"
	"os"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
)

const (
	_EMPTY_ = ""
)

type logAdapter struct {
}

func (l *logAdapter) Noticef(format string, v ...interface{}) {
	log.Infof(format, v...)
}

func (l *logAdapter) Warnf(format string, v ...interface{}) {
	log.Warnf(format, v...)
}

func (l *logAdapter) Fatalf(format string, v ...interface{}) {
	log.Fatalf(format, v...)
}

func (l *logAdapter) Errorf(format string, v ...interface{}) {
	log.Errorf(format, v...)
}

func (l *logAdapter) Debugf(format string, v ...interface{}) {
	log.Debugf(format, v...)
}

func (l *logAdapter) Tracef(format string, v ...interface{}) {
	log.Debugf(format, v...)
}

func runBasicJetStreamServer(t *testing.T) *server.Server {
	t.Helper()
	opts := test.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	opts.Debug = true
	opts.MaxPayload = 8 * 1024 * 1024
	srv := test.RunServer(&opts)
	srv.SetLoggerV2(&logAdapter{}, opts.Debug, opts.Trace, false)
	return srv
}

func natsConn(t *testing.T, s *server.Server, opts ...nats.Option) *nats.Conn {
	t.Helper()
	nc, err := nats.Connect(s.ClientURL(), opts...)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	return nc
}

func jsClient(t *testing.T, s *server.Server, opts ...nats.Option) (*nats.Conn, nats.JetStreamContext) {
	t.Helper()
	nc := natsConn(t, s, opts...)
	js, err := nc.JetStream(nats.MaxWait(10 * time.Second))
	if err != nil {
		t.Fatalf("Unexpected error getting JetStream context: %v", err)
	}
	return nc, js
}

func shutdownJSServerAndRemoveStorage(t *testing.T, s *server.Server) {
	t.Helper()
	var sd string
	if config := s.JetStreamConfig(); config != nil {
		sd = config.StoreDir
	}
	s.Shutdown()
	if sd != _EMPTY_ {
		if err := os.RemoveAll(sd); err != nil {
			t.Fatalf("Unable to remove storage %q: %v", sd, err)
		}
	}
	s.WaitForShutdown()
}
