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

package null

import (
	"io"
	"os"
	"time"

	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
	"github.com/nagare-media/ingest/pkg/volume"
)

type null struct {
	cfg v1alpha1.Volume
}

func New(cfg v1alpha1.Volume) (volume.Volume, error) {
	if err := volume.CheckAndSetDefaults(&cfg); err != nil {
		return nil, err
	}
	return &null{cfg: cfg}, nil
}

func (v *null) Config() v1alpha1.Volume {
	return v.cfg
}

func (v *null) Init(execCtx volume.ExecCtx) error {
	log := execCtx.Logger()
	log.Info("initialize null volume")
	log.Info("null volume initialized")
	return nil
}

func (v *null) Finalize(execCtx volume.ExecCtx) error {
	log := execCtx.Logger()
	log.Info("finalizing null volume")
	log.Info("null volume finalized")
	return nil
}

func (v *null) Open(path string) (volume.File, error) {
	return nil, os.ErrNotExist
}

func (v *null) OpenCreate(path string) (volume.File, error) {
	return nullFile, nil
}

func (v *null) Delete(path string) error {
	return nil
}

type file struct{}

var nullFile = &file{}

func (f *file) Name() string {
	return "null"
}

func (f *file) AcquireReader() (volume.FileReader, error) {
	return nullFileReader, nil
}

func (f *file) AcquireWriter(inPlace bool) (volume.FileWriter, error) {
	return nullFileWriter, nil
}

type fileReader struct{}

var (
	nullFileReader = &fileReader{}
	closedCh       = make(chan struct{})
)

func (fr *fileReader) ModTime() time.Time {
	return volume.UnixEpoch
}

func (fr *fileReader) Read(p []byte) (n int, err error) {
	return 0, io.EOF
}

func (fr *fileReader) Close() error {
	return nil
}

func (fr *fileReader) Seek(offset int64, whence int) (int64, error) {
	return 0, nil
}

func (fr *fileReader) Size() int64 {
	return 0
}

func (fr *fileReader) WriteDone() <-chan struct{} {
	return closedCh
}

type fileWriter struct{}

var nullFileWriter = &fileWriter{}

func (fw *fileWriter) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (fw *fileWriter) Commit() error {
	return nil
}

func (fw *fileWriter) Abort() error {
	return nil
}

func init() {
	close(closedCh)
}
