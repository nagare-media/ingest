/*
Copyright 2022-2024 The nagare media authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package copy

import (
	"context"
	"fmt"
	"io"

	"github.com/nagare-media/ingest/internal/pool"
	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
	"github.com/nagare-media/ingest/pkg/event"
	"github.com/nagare-media/ingest/pkg/function"
	"github.com/nagare-media/ingest/pkg/volume"
	"go.uber.org/zap"
)

type copy struct {
	cfg v1alpha1.Function
	vol volume.Volume
	log *zap.SugaredLogger
}

func New(cfg v1alpha1.Function) (function.Function, error) {
	if err := function.CheckAndSetDefaults(&cfg); err != nil {
		return nil, err
	}

	return &copy{cfg: cfg}, nil
}

func (fn *copy) Config() v1alpha1.Function {
	return fn.cfg
}

func (fn *copy) Exec(ctx context.Context, execCtx function.ExecCtx) error {
	fn.log = execCtx.Logger()

	vol, ok := execCtx.VolumeRegistry().Get(fn.cfg.Copy.VolumeRef.Name)
	if !ok {
		return fmt.Errorf("volume '%s' not found", fn.cfg.Copy.VolumeRef.Name)
	}
	fn.vol = vol

	eventStream := execCtx.App().EventStream().Sub()

	for {
		select {
		case <-ctx.Done():
			// TODO: cleanup
			return nil

		case es := <-eventStream:
			switch e := es.(type) {
			case *event.FileEvent:
				if e.Type == event.FileStartEvent {
					go fn.handleCopy(e.File)
				}

			case *event.InitSegmentEvent:
				if e.Type == event.InitSegmentStartEvent {
					go fn.handleCopy(e.File)
				}

			case *event.FragmentEvent:
				if e.Type == event.FragmentStartEvent {
					go fn.handleCopy(e.File)
				}
			}
		}
	}
}

func (fn *copy) handleCopy(file volume.File) {
	fr, err := file.AcquireReader()
	if err != nil {
		fn.log.With("error", err).Errorf("failed to acquire file reader: '%s'", file.Name())
		return
	}
	defer fr.Close()

	newFile, err := fn.vol.OpenCreate(file.Name())
	if err != nil {
		fn.log.With("error", err).Errorf("failed to open file: '%s'", file.Name())
		return
	}

	fw, err := newFile.AcquireWriter(true)
	if err != nil {
		fn.log.With("error", err).Errorf("failed to acquire file writer: '%s'", file.Name())
		return
	}

	copyBuf := pool.CopyBuf.Get().([]byte)
	defer pool.CopyBuf.Put(copyBuf) // nolint

	_, err = io.CopyBuffer(fw, fr, copyBuf)
	if err != nil {
		fn.log.With("error", err).Errorf("failed to copy file: '%s'", file.Name())

		err = fw.Abort()
		if err != nil {
			fn.log.With("error", err).Errorf("failed to abort file: '%s'", file.Name())
		}

		return
	}

	err = fw.Commit()
	if err != nil {
		fn.log.With("error", err).Errorf("failed to commit file: '%s'", file.Name())
	}
}
