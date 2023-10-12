package store

import (
	"context"

	"github.com/brianmcgee/nvix/pkg/blob"
	"github.com/brianmcgee/nvix/pkg/cli"
	"github.com/brianmcgee/nvix/pkg/directory"
	"github.com/brianmcgee/nvix/pkg/pathinfo"
	"github.com/charmbracelet/log"
)

type Init struct {
	Log  cli.LogOptions  `embed:""`
	Nats cli.NatsOptions `embed:""`
}

func (i *Init) Run() error {
	i.Log.ConfigureLogger()
	conn := i.Nats.Connect()

	ctx := context.Background()

	log.Info("initialising stores")

	log.Info("initialising blob chunk store")
	if err := blob.NewChunkStore(conn).Init(ctx); err != nil {
		log.Errorf("failed to initialise blob chunk store: %v", err)
		return err
	}

	log.Info("initialising blob meta store")
	if err := blob.NewMetaStore(conn).Init(ctx); err != nil {
		log.Errorf("failed to initialise blob meta store: %v", err)
		return err
	}

	log.Info("initialising directory store")
	if err := directory.NewDirectoryStore(conn).Init(ctx); err != nil {
		log.Errorf("failed to initialise directory store: %v", err)
		return err
	}

	log.Info("initialising path info store")
	if err := pathinfo.NewPathInfoStore(conn).Init(ctx); err != nil {
		log.Errorf("failed to initialise path info store: %v", err)
		return err
	}

	log.Info("initialising path info out idx store")
	if err := pathinfo.NewPathInfoOutIdxStore(conn).Init(ctx); err != nil {
		log.Errorf("failed to initialise path info out idx store: %v", err)
		return err
	}

	log.Info("initialising stores complete")

	return nil
}
