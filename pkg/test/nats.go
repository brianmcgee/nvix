package test

import (
	"os"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats-server/v2/test"
	"github.com/nats-io/nats.go"
)

const (
	_EMPTY_ = ""
)

type TestingT interface {
	Helper()
	Fatalf(msg string, args ...any)
	Errorf(msg string, args ...any)
	TempDir() string
}

type LogAdapter struct{}

func (l *LogAdapter) Noticef(format string, v ...interface{}) {
	log.Infof(format, v...)
}

func (l *LogAdapter) Warnf(format string, v ...interface{}) {
	log.Warnf(format, v...)
}

func (l *LogAdapter) Fatalf(format string, v ...interface{}) {
	log.Fatalf(format, v...)
}

func (l *LogAdapter) Errorf(format string, v ...interface{}) {
	log.Errorf(format, v...)
}

func (l *LogAdapter) Debugf(format string, v ...interface{}) {
	log.Debugf(format, v...)
}

func (l *LogAdapter) Tracef(format string, v ...interface{}) {
	log.Debugf(format, v...)
}

func RunBasicServer(t TestingT) *server.Server {
	t.Helper()
	opts := test.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = false
	opts.Debug = false
	opts.MaxPayload = 8 * 1024 * 1024
	srv := test.RunServer(&opts)
	srv.SetLoggerV2(&LogAdapter{}, opts.Debug, opts.Trace, false)
	return srv
}

func RunBasicJetStreamServer(t TestingT) *server.Server {
	t.Helper()
	opts := test.DefaultTestOptions
	opts.Port = -1
	opts.JetStream = true
	opts.StoreDir = t.TempDir()
	opts.Debug = false
	opts.MaxPayload = 8 * 1024 * 1024
	srv := test.RunServer(&opts)
	srv.SetLoggerV2(&LogAdapter{}, opts.Debug, opts.Trace, false)
	return srv
}

func NatsConn(t TestingT, s *server.Server, opts ...nats.Option) *nats.Conn {
	t.Helper()
	nc, err := nats.Connect(s.ClientURL(), opts...)
	if err != nil {
		t.Fatalf("Unexpected error creating Nats connection: %v", err)
	}
	return nc
}

func JsClient(t TestingT, s *server.Server, opts ...nats.Option) (*nats.Conn, nats.JetStreamContext) {
	t.Helper()
	nc := NatsConn(t, s, opts...)
	js, err := nc.JetStream(nats.MaxWait(10 * time.Second))
	if err != nil {
		t.Fatalf("Unexpected error getting JetStream context: %v", err)
	}
	return nc, js
}

func ShutdownServer(t TestingT, s *server.Server) {
	t.Helper()
	s.Shutdown()
	s.WaitForShutdown()
}

func ShutdownJSServerAndRemoveStorage(t TestingT, s *server.Server) {
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
