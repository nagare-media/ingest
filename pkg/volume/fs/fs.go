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

package fs

import (
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/nagare-media/ingest/internal/pool"
	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
	"github.com/nagare-media/ingest/pkg/volume"
)

const (
	maxConsecutiveEmptyWrites = 10
)

var (
	DefaultConfig = v1alpha1.FileSystemVolume{
		GarbageCollectionPeriod: 10 * time.Second,
	}
)

var closedCh = make(chan struct{})

func init() {
	close(closedCh)
}

type fs struct {
	cfg v1alpha1.Volume

	filesMtx sync.RWMutex
	files    map[string]*file

	stopGC chan struct{}
}

func New(cfg v1alpha1.Volume) (volume.Volume, error) {
	var err error
	if err = volume.CheckAndSetDefaults(&cfg); err != nil {
		return nil, err
	}
	if cfg.FileSystem.Path == "" {
		return nil, errors.New("fs.New: Path not set")
	}

	if cfg.FileSystem.GarbageCollectionPeriod <= 0 {
		cfg.FileSystem.GarbageCollectionPeriod = DefaultConfig.GarbageCollectionPeriod
	}

	fs := &fs{
		cfg:    cfg,
		files:  make(map[string]*file),
		stopGC: make(chan struct{}),
	}

	cfg.FileSystem.Path, err = filepath.Abs(cfg.FileSystem.Path)
	if err != nil {
		return nil, err
	}

	return fs, nil
}

func (fs *fs) Config() v1alpha1.Volume {
	return fs.cfg
}

func (fs *fs) Init(execCtx volume.ExecCtx) error {
	log := execCtx.Logger()

	log.Info("initialize fs volume")

	err := os.MkdirAll(fs.cfg.FileSystem.Path, 0755)
	if err != nil {
		return err
	}

	go func() {
		t := time.NewTicker(fs.cfg.FileSystem.GarbageCollectionPeriod)
		defer t.Stop()
		for {
			select {
			case <-fs.stopGC:
				// fs.Finalize was called
				fs.files = nil
				return
			case <-t.C:
				fs.RunGC(execCtx)
			}
		}
	}()

	log.Info("fs volume initialized")
	return nil
}

func (fs *fs) Finalize(execCtx volume.ExecCtx) error {
	log := execCtx.Logger()
	log.Info("finalizing fs volume")

	close(fs.stopGC)
	// references cleared when returning from GC

	log.Info("fs volume finalized")
	return nil
}

func (fs *fs) RunGC(execCtx volume.ExecCtx) {
	// TODO: locking filesMtx prevents opening files potentially for some time
	fs.filesMtx.Lock()
	defer fs.filesMtx.Unlock()

	for absPath, f := range fs.files {
		// TODO: we should better use ref counter
		f.writeMtx.Lock()
		if f.fw == nil {
			delete(fs.files, absPath)
		}
		f.writeMtx.Unlock()
	}
}

func (fs *fs) Open(name string) (volume.File, error) {
	absPath := path.Join(fs.cfg.FileSystem.Path, name)

	fs.filesMtx.RLock()
	if f, ok := fs.files[absPath]; ok {
		defer fs.filesMtx.RUnlock()
		return f, nil
	}

	// upgrade to lock
	fs.filesMtx.RUnlock()
	fs.filesMtx.Lock()
	defer fs.filesMtx.Unlock()

	// race?
	if f, ok := fs.files[absPath]; ok {
		return f, nil
	}

	// create file
	f := &file{
		absPath: absPath,
		name:    path.Clean("/" + name),
		fs:      fs,
	}
	fs.files[absPath] = f

	return f, nil
}

// Open file on given path. If file does not exist a new file will be created.
func (fs *fs) OpenCreate(name string) (volume.File, error) {
	return fs.Open(name)
}

// Delete file if it exists.
func (fs *fs) Delete(name string) error {
	absPath := path.Join(fs.cfg.FileSystem.Path, name)

	fs.filesMtx.Lock()
	defer fs.filesMtx.Unlock()

	delete(fs.files, absPath)
	return os.Remove(absPath)
}

type file struct {
	absPath  string
	name     string
	fs       *fs
	readMtx  sync.RWMutex
	writeMtx sync.Mutex
	fw       *fileWriter
}

func (f *file) Name() string {
	return f.name
}

func (f *file) AcquireReader() (volume.FileReader, error) {
	f.readMtx.RLock()
	defer f.readMtx.RUnlock()

	// TODO: we should cache file descriptors for some time to handle bursts
	fd, err := os.Open(f.absPath)
	if err != nil {
		return nil, err
	}

	fr := &fileReader{
		file: f,
		fd:   fd,
		fw:   f.fw,
	}

	return fr, nil
}

func (f *file) AcquireWriter(inPlace bool) (volume.FileWriter, error) {
	f.writeMtx.Lock()
	// Unlock called by close

	f.readMtx.Lock()
	defer f.readMtx.Unlock()

	// check if file exists
	overwrite := true
	s, err := os.Lstat(f.absPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		overwrite = false
	}

	// check existing file
	if err == nil {
		if s.IsDir() {
			return nil, errors.New("cannot acquire writer for directory")
		}
	}

	// create directory structure
	base := path.Dir(f.absPath)
	err = os.MkdirAll(base, 0755)
	if err != nil {
		return nil, err
	}

	// create file
	var fd *os.File
	var oldFilePath string

	if inPlace {
		// move old file
		if overwrite {
			fdOld, err := os.CreateTemp(base, ".old-*.tmp")
			if err != nil {
				return nil, err
			}
			oldFilePath = fdOld.Name()

			// TODO: only generate tmp path as string
			err = fdOld.Close()
			if err != nil {
				return nil, err
			}

			err = os.Rename(f.absPath, oldFilePath)
			if err != nil {
				return nil, err
			}
		}

		fd, err = os.Create(f.absPath)
	} else {
		fd, err = os.CreateTemp(base, ".ingest-*.tmp")
	}
	if err != nil {
		return nil, err
	}

	// create file writer
	fw := &fileWriter{
		file:            f,
		fd:              fd,
		oldFilePath:     oldFilePath,
		inPlace:         inPlace,
		notifyWrite:     make(chan struct{}),
		notifyWriteDone: make(chan struct{}),
	}
	f.fw = fw

	return fw, nil
}

type fileReader struct {
	file *file
	fd   *os.File
	fw   *fileWriter
	pos  int64
}

func (fr *fileReader) Stat() (os.FileInfo, error) {
	return fr.fd.Stat()
}

func (fr *fileReader) ModTime() time.Time {
	s, err := fr.Stat()
	if err != nil {
		return volume.UnixEpoch
	}
	return s.ModTime()
}

func (fr *fileReader) Size() int64 {
	s, err := fr.Stat()
	if err != nil {
		return -2
	}
	return s.Size()
}

func (fr *fileReader) WriteTo(w io.Writer) (int64, error) {
	// fast pass
	// When writing to TCP connection and reader is a file, sendfile system call will be used.
	// This is also true if w is bufio.Writer (as used by fasthttp), but only if the buffer is empty.
	if rf, ok := w.(io.ReaderFrom); ok {
		// handle: no writer
		if fr.fw == nil {
			return rf.ReadFrom(fr.fd)
		}

		// handle: done writer
		select {
		case <-fr.fw.notifyWriteDone:
			return rf.ReadFrom(fr.fd)
		default:
		}
	}

	// handle: active writer => slow path
	copyBuf := pool.CopyBuf.Get().([]byte)
	defer pool.CopyBuf.Put(copyBuf) // nolint
	return io.CopyBuffer(w, &withoutWriteTo{fr}, copyBuf)
}

func (fr *fileReader) Read(p []byte) (n int, err error) {
	// handle: no writer
	if fr.fw == nil {
		return fr.fd.Read(p)
	}

	// handle: done writer
	select {
	case <-fr.fw.notifyWriteDone:
		return fr.fd.Read(p)
	default:
	}

	// handle: active writer => we need to track position
	fr.fw.mtx.RLock()
	remaining := fr.fw.size - fr.pos
	for i := maxConsecutiveEmptyWrites; i > 0 && remaining <= 0; i-- {
		if remaining < 0 {
			panic("BUG: read position is larger than file size")
		}

		// is writer done?
		select {
		case <-fr.fw.notifyWriteDone:
			// read all bytes and there is no active writer => EOF
			return 0, io.EOF
		default:
		}

		// wait for writer
		wait := fr.fw.notifyWrite
		fr.fw.mtx.RUnlock()
		<-wait
		fr.fw.mtx.RLock()

		// update remaining after writer returned
		remaining = fr.fw.size - fr.pos
	}
	fr.fw.mtx.RUnlock()

	// check how much we can read to avoid EOF
	if int64(len(p)) > remaining {
		p = p[:int(remaining)]
	}

	// read and track position
	n, err = fr.fd.Read(p)
	fr.pos += int64(n)
	return n, err
}

func (fr *fileReader) Close() error {
	err := fr.fd.Close()
	fr.file = nil
	fr.fd = nil
	fr.fw = nil
	return err
}

func (fr *fileReader) Seek(offset int64, whence int) (int64, error) {
	// handle: no writer
	if fr.fw == nil {
		return fr.fd.Seek(offset, whence)
	}

	// handle: done writer
	select {
	case <-fr.fw.notifyWriteDone:
		return fr.fd.Seek(offset, whence)
	default:
	}

	// handle: active writer => we need to track position

	// calculate new position
	fr.fw.mtx.RLock()
	switch whence {
	default:
		return 0, errors.New("Seek: invalid offset")
	case io.SeekStart:
	case io.SeekCurrent:
		offset += fr.pos
	case io.SeekEnd:
		offset += fr.fw.size
	}
	if offset < 0 {
		return 0, errors.New("Seek: invalid offset")
	}
	if offset > fr.fw.size {
		// TODO: is this corret Seek behavior?
		offset = fr.fw.size
	}
	fr.fw.mtx.RUnlock()

	// seek to new position
	fr.pos = offset
	return fr.fd.Seek(fr.pos, io.SeekStart)
}

func (fr *fileReader) WriteDone() <-chan struct{} {
	if fr.fw != nil {
		return fr.fw.notifyWriteDone
	}
	return closedCh
}

type fileWriter struct {
	file *file
	fd   *os.File
	size int64

	mtx             sync.RWMutex
	notifyWrite     chan struct{}
	notifyWriteDone chan struct{}

	oldFilePath string
	inPlace     bool
}

func (fw *fileWriter) Write(p []byte) (n int, err error) {
	fw.mtx.Lock()
	defer fw.mtx.Unlock()

	n, err = fw.fd.Write(p)
	fw.size += int64(n)

	if n > 0 {
		// notify all waiting readers
		close(fw.notifyWrite)
		fw.notifyWrite = make(chan struct{})
	}

	return n, err
}

func (fw *fileWriter) Commit() error {
	fw.mtx.Lock()
	fw.file.readMtx.Lock()
	defer fw.file.readMtx.Unlock()
	defer fw.mtx.Unlock()

	newFilePath := fw.fd.Name()

	// set permissions
	err := fw.fd.Chmod(0644)
	if err != nil {
		return err
	}

	// close file
	err = fw.fd.Close()
	if err != nil {
		return err
	}

	// move tmp file to correct location
	if !fw.inPlace {
		err = os.Rename(newFilePath, fw.file.absPath)
		if err != nil {
			return err
		}
	}

	// delete old file
	if fw.oldFilePath != "" {
		err = os.Remove(fw.oldFilePath)
		if err != nil {
			return err
		}
	}

	// notify all waiting readers
	close(fw.notifyWrite)
	close(fw.notifyWriteDone)

	// writeMtx.Lock was acquire in AcquireWriter
	fw.file.writeMtx.Unlock()

	// clear references
	fw.file.fw = nil
	fw.file = nil
	fw.fd = nil
	return nil
}

func (fw *fileWriter) Abort() error {
	fw.mtx.Lock()
	fw.file.readMtx.Lock()
	defer fw.file.readMtx.Unlock()
	defer fw.mtx.Unlock()

	// close file
	err := fw.fd.Close()
	if err != nil {
		return err
	}

	// delete or replace by old file
	if fw.oldFilePath == "" {
		newFilePath := fw.fd.Name()
		err = os.Remove(newFilePath)
		if err != nil {
			return err
		}
	} else {
		err = os.Rename(fw.oldFilePath, fw.file.absPath)
		if err != nil {
			return err
		}
	}

	// notify all waiting readers
	close(fw.notifyWrite)
	close(fw.notifyWriteDone)

	// writeMtx.Lock was acquire in AcquireWriter
	fw.file.writeMtx.Unlock()

	// clear references
	fw.file.fw = nil
	fw.file = nil
	fw.fd = nil
	return nil
}

// withoutWriteTo implements io.Reader without the WriteTo method.
type withoutWriteTo struct {
	r io.Reader
}

var _ io.Reader = &withoutWriteTo{}

func (r *withoutWriteTo) Read(p []byte) (n int, err error) {
	return r.r.Read(p)
}
