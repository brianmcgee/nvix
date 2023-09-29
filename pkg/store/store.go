package store

import (
	"bytes"
	"context"
	"io"

	"github.com/juju/errors"
	"github.com/nats-io/nats.go"
)

type NatsStore struct {
	Conn          *nats.Conn
	StreamConfig  *nats.StreamConfig
	SubjectPrefix string
}

func (n *NatsStore) subject(key string) string {
	return n.SubjectPrefix + "." + key
}

func (n *NatsStore) Get(key string, ctx context.Context) (io.ReadCloser, error) {
	js, err := n.js(ctx)
	if err != nil {
		return nil, err
	}

	subj := n.subject(key)
	msg, err := js.GetLastMsg(n.StreamConfig.Name, subj)
	if err != nil {
		if err == nats.ErrMsgNotFound {
			return nil, ErrKeyNotFound
		} else {
			return nil, errors.Annotate(err, "failed to retrieve last msg from stream")
		}
	}

	return io.NopCloser(bytes.NewReader(msg.Data)), nil
}

func (n *NatsStore) Put(key string, reader io.ReadCloser, ctx context.Context) error {
	js, err := n.js(ctx)
	if err != nil {
		return err
	}

	msg := nats.NewMsg(n.subject(key))
	msg.Data, err = io.ReadAll(reader)
	if err != nil {
		return err
	} else if err = reader.Close(); err != nil {
		return err
	}

	// overwrite the last msg for this subject
	msg.Header.Set(nats.MsgRollup, nats.MsgRollupSubject)

	_, err = js.PublishMsg(msg)
	return err
}

func (n *NatsStore) Delete(key string, ctx context.Context) error {
	js, err := n.js(ctx)
	if err != nil {
		return err
	}
	return js.PurgeStream(n.StreamConfig.Name, &nats.StreamPurgeRequest{
		Subject: n.subject(key),
	})
}

func (n *NatsStore) js(_ context.Context) (nats.JetStreamContext, error) {
	// todo potentially extract js from ctx
	js, err := n.Conn.JetStream(nats.DirectGet())
	if err != nil {
		err = errors.Annotate(err, "failed to create js context")
	}
	return js, err
}
