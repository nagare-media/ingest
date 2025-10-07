/*
Copyright 2022-2025 The nagare media authors

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

package cleanup

import (
	"container/list"
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/gobwas/glob/compiler"
	"github.com/gobwas/glob/match"
	"github.com/gobwas/glob/syntax"

	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
	"github.com/nagare-media/ingest/pkg/event"
	"github.com/nagare-media/ingest/pkg/function"
	"github.com/nagare-media/ingest/pkg/volume"
)

type cleanup struct {
	cfg         v1alpha1.Function
	fileMatcher match.Matchers

	mtx          sync.Mutex
	pendingFiles *list.List // protected by mtx
}

func New(cfg v1alpha1.Function) (function.Function, error) {
	if err := function.CheckAndSetDefaults(&cfg); err != nil {
		return nil, err
	}

	fn := &cleanup{
		cfg:          cfg,
		fileMatcher:  make(match.Matchers, 0, len(cfg.Cleanup.Files)),
		pendingFiles: list.New(),
	}

	for _, fp := range cfg.Cleanup.Files {
		ast, err := syntax.Parse(fp)
		if err != nil {
			return nil, err
		}
		matcher, err := compiler.Compile(ast, []rune{'/'})
		if err != nil {
			return nil, err
		}
		fn.fileMatcher = append(fn.fileMatcher, matcher)
	}

	return fn, nil
}

func (fn *cleanup) Config() v1alpha1.Function {
	return fn.cfg
}

func (fn *cleanup) Exec(ctx context.Context, execCtx function.ExecCtx) error {
	eventStream := execCtx.App().EventStream().Sub()
	defer execCtx.App().EventStream().Desub(eventStream)

	go fn.runGC(ctx, execCtx)

	for {
		select {
		case <-ctx.Done():
			return nil

		case es := <-eventStream:
			switch e := es.(type) {
			case *event.FileEvent:
				if e.Type == event.FileCommittedEvent {
					// does file name match pattern
					fileName := e.File.Name()
					for _, matcher := range fn.fileMatcher {
						if matcher.Match(fileName) {
							// yes -> append to pendingFiles
							fn.mtx.Lock()
							fn.pendingFiles.PushBack(e.File)
							fn.mtx.Unlock()
							break
						}
					}
				}
			}
		}
	}
}

func (fn *cleanup) runGC(ctx context.Context, execCtx function.ExecCtx) {
	log := execCtx.Logger()
	eventStream := execCtx.App().EventStream()

	t := time.NewTimer(fn.wakeAfter())
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case now := <-t.C:
			for {
				fn.mtx.Lock()
				pendingFile := fn.pendingFiles.Front()
				fn.mtx.Unlock()

				modTime, err := getFileElementModTime(pendingFile)
				if err != nil {
					log.With("error", err).Error("failed to get file modified time")
					continue
				}

				// are we finished with the current set of files?
				if modTime.Add(fn.cfg.Cleanup.Age).After(now) {
					break
				}

				// delete file
				file, ok := pendingFile.Value.(volume.File)
				if !ok {
					panic("BUG: list contains not a file")
				}

				removed := true
				for _, vref := range fn.cfg.Cleanup.VolumeRefs {
					vol, ok := execCtx.VolumeRegistry().Get(vref.Name)
					if !ok {
						log.Errorf("failed to resolve volume reference %s", vref.Name)
						continue
					}

					if err := vol.Delete(file.Name()); err != nil {
						log.With("error", err).Error("failed to delete file")
						removed = false
					}

					e := event.NewFileEvent(event.FileDeletedEvent, file)
					eventStream.Pub(e)
				}

				// remove from pending list
				if removed {
					fn.mtx.Lock()
					fn.pendingFiles.Remove(pendingFile)
					fn.mtx.Unlock()
				}
			}
		}

		// determine when to wake up again
		t.Reset(fn.wakeAfter())
	}
}

func (fn *cleanup) wakeAfter() time.Duration {
	// TODO: make configurable
	defaultTime := 10 * time.Second

	for {
		fn.mtx.Lock()
		oldestFile := fn.pendingFiles.Front()
		fn.mtx.Unlock()

		if oldestFile == nil {
			// wait a default amount of time
			return defaultTime
		}

		modTime, err := getFileElementModTime(oldestFile)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// file no longer exists -> remove from list and go to next oldest file
				fn.mtx.Lock()
				fn.pendingFiles.Remove(oldestFile)
				fn.mtx.Unlock()
				continue
			}

			// unknown error -> wait a default amount of time
			return defaultTime
		}

		return time.Until(modTime.Add(fn.cfg.Cleanup.Age))
	}
}

func getFileElementModTime(e *list.Element) (time.Time, error) {
	f, ok := e.Value.(volume.File)
	if !ok {
		panic("BUG: list contains not a file")
	}
	r, err := f.AcquireReader()
	if err != nil {
		return time.Time{}, err
	}
	defer r.Close()
	return r.ModTime(), nil
}
