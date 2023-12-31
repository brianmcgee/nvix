package store

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/brianmcgee/nvix/pkg/util"

	"github.com/juju/errors"
	"github.com/nats-io/nats.go"
)

type NatsStore struct {
	Conn          *nats.Conn
	StreamConfig  *nats.StreamConfig
	SubjectPrefix string
}

func (n *NatsStore) Init(ctx context.Context) error {
	js, err := n.js(ctx)
	if err != nil {
		return err
	}
	_, err = js.AddStream(n.StreamConfig)
	return err
}

func (n *NatsStore) Stat(key string, ctx context.Context) (ok bool, err error) {
	var reader io.ReadCloser
	reader, err = n.Get(key, ctx)
	if err != nil {
		return
	}
	defer func() {
		_ = reader.Close()
	}()

	// try to read, forcing an error if the entry doesn't exist
	b := make([]byte, 0)
	_, err = reader.Read(b)
	ok = err == nil

	return
}

func (n *NatsStore) subject(key string) string {
	return n.SubjectPrefix + "." + key
}

func (n *NatsStore) Get(key string, ctx context.Context) (io.ReadCloser, error) {
	js, err := n.js(ctx)
	if err != nil {
		return nil, err
	}

	reader := natsMsgReader{
		js:      js,
		stream:  n.StreamConfig.Name,
		subject: n.subject(key),
	}

	return &reader, nil
}

func (n *NatsStore) Put(key string, reader io.ReadCloser, ctx context.Context) error {
	future, err := n.PutAsync(key, reader, ctx)
	if err != nil {
		return err
	}

	select {
	case <-future.Ok():
		return nil
	case err := <-future.Err():
		return err
	}
}

func (n *NatsStore) PutAsync(key string, reader io.ReadCloser, ctx context.Context) (nats.PubAckFuture, error) {
	js, err := n.js(ctx)
	if err != nil {
		return nil, err
	}

	msg := nats.NewMsg(n.subject(key))
	msg.Data, err = io.ReadAll(reader)
	if err != nil {
		return nil, err
	} else if err = reader.Close(); err != nil {
		return nil, err
	}

	// overwrite the last msg for this subject
	msg.Header.Set(nats.MsgRollup, nats.MsgRollupSubject)

	return js.PublishMsgAsync(msg)
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

func (n *NatsStore) List(ctx context.Context) (util.Iterator[io.ReadCloser], error) {
	js, err := n.js(ctx)
	if err != nil {
		return nil, err
	}
	sub, err := js.SubscribeSync(n.subject("*"), nats.DeliverAll())
	if err != nil {
		return nil, err
	}
	return &natsIterator{
		ctx:          ctx,
		sub:          sub,
		fetchTimeout: 5 * time.Second,
		numPending:   1,
	}, nil
}

func (n *NatsStore) js(_ context.Context) (nats.JetStreamContext, error) {
	// todo potentially extract js from ctx
	js, err := n.Conn.JetStream(nats.DirectGet())
	if err != nil {
		err = errors.Annotate(err, "failed to create js context")
	}
	return js, err
}

type natsIterator struct {
	ctx          context.Context
	sub          *nats.Subscription
	fetchTimeout time.Duration

	numPending uint64
}

func (n *natsIterator) Next() (io.ReadCloser, error) {
	if n.numPending == 0 {
		// we have caught up
		return nil, io.EOF
	}

	select {
	case <-n.ctx.Done():
		return nil, n.ctx.Err()
	default:
		ctx, cancel := context.WithTimeout(n.ctx, n.fetchTimeout)
		defer cancel()

		msg, err := n.sub.NextMsgWithContext(ctx)
		if err != nil {
			return nil, err
		}

		meta, err := msg.Metadata()
		if err != nil {
			return nil, errors.Annotate(err, "failed to get msg metadata")
		}

		n.numPending = meta.NumPending

		return &natsMsgReader{
			reader: bytes.NewReader(msg.Data),
		}, nil
	}
}

func (n *natsIterator) Close() error {
	return n.sub.Unsubscribe()
}

type natsMsgReader struct {
	js      nats.JetStreamContext
	stream  string
	subject string

	msg    *nats.RawStreamMsg
	reader io.Reader
}

func (r *natsMsgReader) Read(p []byte) (n int, err error) {
	if r.reader == nil && r.msg == nil {
		r.msg, err = r.js.GetLastMsg(r.stream, r.subject)
		if err == nats.ErrMsgNotFound {
			return 0, ErrKeyNotFound
		} else if err != nil {
			return
		}
		r.reader = bytes.NewReader(r.msg.Data)
	}

	return r.reader.Read(p)
}

func (r *natsMsgReader) Close() error {
	return nil
}
