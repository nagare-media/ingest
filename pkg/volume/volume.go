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

package volume

import (
	"errors"
	"io"
	"regexp"
	"time"

	"go.uber.org/zap"

	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
)

var (
	NameRegex = regexp.MustCompile(`^[a-zA-Z0-9-_]+$`)
	UnixEpoch = time.Unix(0, 0)

	ErrAlreadyCommitted = errors.New("already committed")
)

type Volume interface {
	Config() v1alpha1.Volume
	Init(execCtx ExecCtx) error
	Deinit(execCtx ExecCtx) error

	Open(path string) (File, error)
	OpenCreate(path string) (File, error)
	Delete(path string) error
}

type File interface {
	Name() string
	AcquireReader() (FileReader, error)
	AcquireWriter(inPlace bool) (FileWriter, error)
}

// Committer is the interface that wraps the basic Commit method.
//
// Commit may return an error if the operation cannot be committed.
type Committer interface {
	Commit() error
}

// Aborter is the interface that wraps the basic Abort method.
//
// Abort may return an error if the operation cannot be aborted.
type Aborter interface {
	Abort() error
}

type FileReader interface {
	io.Reader
	io.Closer
	io.Seeker
	Size() int64
	WriteDone() <-chan struct{}
	ModTime() time.Time
}

type FileWriter interface {
	io.Writer
	Committer
	Aborter
}

type ExecCtx interface {
	Logger() *zap.SugaredLogger
}

type Registry interface {
	Get(name string) (Volume, bool)
}

func CheckAndSetDefaults(cfg *v1alpha1.Volume) error {
	if !NameRegex.Match([]byte(cfg.Name)) {
		return errors.New("volume: Name invalid")
	}
	return nil
}
