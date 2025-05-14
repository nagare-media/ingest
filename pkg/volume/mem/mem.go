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

package mem

import (
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/inhies/go-bytesize"
	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
	"github.com/nagare-media/ingest/pkg/volume"
)

const (
	maxConsecutiveEmptyWrites = 10
)

var (
	DefaultConfig = v1alpha1.MemoryVolume{
		BlockSize:                4 * bytesize.KB,
		GarbageCollectionPeriode: 10 * time.Second,
	}
)

type mem struct {
	cfg       v1alpha1.Volume
	blockPool sync.Pool

	filesMtx      sync.RWMutex
	files         map[string]*file
	stopGC        chan struct{}
	pendingGCList []*superBlock
}

// New creates a new memory volume.
func New(cfg v1alpha1.Volume) (volume.Volume, error) {
	if err := volume.CheckAndSetDefaults(&cfg); err != nil {
		return nil, err
	}
	if cfg.Memory.BlockSize <= 0 {
		cfg.Memory.BlockSize = DefaultConfig.BlockSize
	}
	if cfg.Memory.GarbageCollectionPeriode <= 0 {
		cfg.Memory.GarbageCollectionPeriode = DefaultConfig.GarbageCollectionPeriode
	}

	m := &mem{
		cfg:    cfg,
		files:  make(map[string]*file),
		stopGC: make(chan struct{}),
		blockPool: sync.Pool{
			New: func() any {
				return &block{
					data: make([]byte, 0, cfg.Memory.BlockSize),
				}
			},
		},
	}

	return m, nil
}

// Config returns the configuration of this volume.
func (m *mem) Config() v1alpha1.Volume {
	return m.cfg
}

// Init volume.
//
// The volume must not be used beforehand.
func (m *mem) Init(execCtx volume.ExecCtx) error {
	log := execCtx.Logger()
	log.Info("initialize mem volume")

	go func() {
		t := time.NewTicker(m.cfg.Memory.GarbageCollectionPeriode)
		defer t.Stop()
		for {
			select {
			case <-m.stopGC:
				// m.Deinit was called
				m.files = nil
				m.pendingGCList = nil
				return
			case <-t.C:
				m.RunGC(execCtx)
			}
		}
	}()

	log.Info("mem volume initialized")
	return nil
}

// Deinit volume.
//
// The volume must not be used afterwards.
func (m *mem) Deinit(execCtx volume.ExecCtx) error {
	log := execCtx.Logger()
	log.Info("deinitialize mem volume")

	close(m.stopGC)
	// references cleared when returning from GC

	log.Info("mem volume deinitialized")
	return nil
}

// RunGC releases blocks that are no longer referenced to a sync.Pool instance for blocks. Actual freeing of memory is
// implemented by sync.Pool and Go's GC.
func (m *mem) RunGC(execCtx volume.ExecCtx) {
	m.filesMtx.Lock()
	defer m.filesMtx.Unlock()

	var pendingGCList []*superBlock
	for _, sb := range m.pendingGCList {
		if atomic.LoadUint32(&sb.openCount) == 0 {
			blk := sb.blk
			for blk != nil {
				nextBlk := blk.nextBlk
				blk.reset()
				m.blockPool.Put(blk)
				blk = nextBlk
			}
		} else {
			// there are still readers or a writer
			pendingGCList = append(pendingGCList, sb)
		}
	}

	m.pendingGCList = pendingGCList
}

// Open file on given path. If file does not exist, os.ErrNotExist will be returned.
func (m *mem) Open(path string) (volume.File, error) {
	m.filesMtx.RLock()
	defer m.filesMtx.RUnlock()
	if f, ok := m.files[path]; ok {
		return f, nil
	}
	return nil, os.ErrNotExist
}

// Open file on given path. If file does not exist a new file will be created.
func (m *mem) OpenCreate(path string) (volume.File, error) {
	m.filesMtx.Lock()
	defer m.filesMtx.Unlock()
	f, ok := m.files[path]
	if !ok {
		f = &file{
			path: path,
			fs:   m,
		}
		m.files[path] = f
	}
	return f, nil
}

// Delete file if it exists.
func (m *mem) Delete(path string) error {
	m.filesMtx.Lock()
	defer m.filesMtx.Unlock()

	if f, ok := m.files[path]; ok {
		delete(m.files, path)
		if f.superBlk != nil {
			m.pendingGCList = append(m.pendingGCList, f.superBlk)
		}
	}

	return nil
}

type file struct {
	path     string
	fs       *mem
	writeMtx sync.Mutex
	readMtx  sync.RWMutex
	superBlk *superBlock // can only be changed if lock on writeMtx and readMtx
}

func (f *file) Name() string {
	return f.path
}

func (f *file) AcquireReader() (volume.FileReader, error) {
	f.readMtx.RLock()
	defer f.readMtx.RUnlock()
	if f.superBlk == nil {
		return nil, os.ErrNotExist
	}

	fr := &fileReader{
		file:     f,
		superBlk: f.superBlk,
	}
	atomic.AddUint32(&fr.superBlk.openCount, 1)

	return fr, nil
}

func (f *file) AcquireWriter(inPlace bool) (volume.FileWriter, error) {
	f.writeMtx.Lock()
	// Unlock called by writer's commit or abort

	blk := f.fs.blockPool.Get().(*block)
	sb := &superBlock{
		openCount:       1,
		modTime:         volume.UnixEpoch,
		notifyWrite:     make(chan struct{}),
		notifyWriteDone: make(chan struct{}),
		blk:             blk,
	}

	fw := &fileWriter{
		file:     f,
		lastBlk:  blk,
		superBlk: sb,
	}

	if inPlace {
		f.readMtx.Lock()
		defer f.readMtx.Unlock()
		if f.superBlk != nil {
			f.fs.filesMtx.Lock()
			defer f.fs.filesMtx.Unlock()
			f.fs.pendingGCList = append(f.fs.pendingGCList, f.superBlk)
		}
		f.superBlk = sb
	}

	return fw, nil
}

type fileReader struct {
	file     *file
	superBlk *superBlock // superBlock at the start of opening file
	pos      int64
	curBlk   *block
	moveNext bool
}

func (fr *fileReader) ModTime() time.Time {
	fr.superBlk.mtx.RLock()
	defer fr.superBlk.mtx.RUnlock()
	return fr.superBlk.modTime
}

func (fr *fileReader) Size() int64 {
	fr.superBlk.mtx.RLock()
	defer fr.superBlk.mtx.RUnlock()
	return fr.superBlk.size - fr.pos
}

// TODO: add WriteTo

func (fr *fileReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	// initialize current block if empty
	if fr.curBlk == nil {
		if fr.pos != 0 {
			panic("BUG: no current block while read position is not at the beginning")
		}
		fr.curBlk = fr.superBlk.blk
	}

	// check if there are bytes remaining
	fr.superBlk.mtx.RLock()
	remaining := fr.superBlk.size - fr.pos
	for i := maxConsecutiveEmptyWrites; i > 0 && remaining <= 0; i-- {
		if remaining < 0 {
			panic("BUG: read position is larger than file size")
		}

		if !fr.superBlk.hasWriter() {
			// read all bytes and there is no active writer => EOF
			return 0, io.EOF
		}

		// wait for writer
		wait := fr.superBlk.notifyWrite
		fr.superBlk.mtx.RUnlock()
		<-wait
		fr.superBlk.mtx.RLock()

		// update remaining after writer returned
		remaining = fr.superBlk.size - fr.pos
	}
	// writes are append only so we can release early after we determined remaining bytes
	fr.superBlk.mtx.RUnlock()

	if remaining <= 0 {
		// maxConsecutiveEmptyWrites was reached; return with 0 bytes instead of waiting longer
		return 0, nil
	}

	// assumption: all blocks have same size
	blkSize := cap(fr.superBlk.blk.data)
	blkStart := int(fr.pos % int64(blkSize))
	blkEnd := blkSize

	// copy into p until p is full or we run out of remaining bytes
	for remaining > 0 && len(p) > 0 {
		if fr.moveNext {
			fr.moveNext = false
			fr.curBlk = fr.curBlk.nextBlk
		}
		if remaining < int64(blkSize-blkStart) {
			// we are in the last block that is open for reading
			blkEnd = blkStart + int(remaining)
		}

		n1 := copy(p, fr.curBlk.data[blkStart:blkEnd])
		n += n1
		blkStart += n1
		fr.pos += int64(n1)
		remaining -= int64(n1)
		p = p[n1:]

		if blkStart == blkSize {
			fr.moveNext = true
			blkStart = 0
		}
	}

	return n, nil
}

// Close file reader.
//
// The file reader must not be used afterwards.
func (fr *fileReader) Close() error {
	// decrement openCount
	atomic.AddUint32(&fr.superBlk.openCount, ^uint32(0))

	// clear references
	fr.file = nil
	fr.superBlk = nil
	return nil
}

func (fr *fileReader) Seek(offset int64, whence int) (int64, error) {
	fr.superBlk.mtx.RLock()
	defer fr.superBlk.mtx.RUnlock()

	switch whence {
	default:
		return 0, errors.New("Seek: invalid offset")
	case io.SeekStart:
	case io.SeekCurrent:
		offset += fr.pos
	case io.SeekEnd:
		offset += fr.superBlk.size
	}
	if offset < 0 {
		return 0, errors.New("Seek: invalid offset")
	}
	if offset > fr.superBlk.size {
		// TODO: seek while inPlace writing?
		// TODO: is this correct Seek behavior?
		offset = fr.superBlk.size
	}

	// assumption: all blocks have same size
	blkSize := cap(fr.superBlk.blk.data)
	fr.pos = offset
	fr.moveNext = false

	// TODO: be more clever when seeking forward from current position
	fr.curBlk = fr.superBlk.blk
	for i := offset / int64(blkSize); i > 0; i-- {
		fr.curBlk = fr.curBlk.nextBlk
	}

	return offset, nil
}

func (fr *fileReader) WriteDone() <-chan struct{} {
	return fr.superBlk.notifyWriteDone
}

type fileWriter struct {
	file     *file
	lastBlk  *block
	superBlk *superBlock // new superBlock of file
}

func (fw *fileWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	if n == 0 {
		return 0, nil
	}

	for {
		p = fw.lastBlk.write(p)
		if len(p) == 0 {
			break
		}
		blk := fw.file.fs.blockPool.Get().(*block)
		fw.lastBlk.nextBlk = blk
		fw.lastBlk = blk
	}

	fw.superBlk.mtx.Lock()
	defer fw.superBlk.mtx.Unlock()

	// increase file size
	fw.superBlk.size += int64(n)

	// notify all waiting readers
	close(fw.superBlk.notifyWrite)
	fw.superBlk.notifyWrite = make(chan struct{})

	return n, nil
}

// Commit written bytes to file and close file writer. Note that the file writer may have written in place and the
// changes can already be seen by readers.
//
// The file writer must not be used afterwards.
func (fw *fileWriter) Commit() error {
	fw.superBlk.mtx.Lock()
	fw.file.readMtx.Lock()

	// fs changes
	if fw.file.superBlk != nil && fw.file.superBlk != fw.superBlk {
		fw.file.fs.filesMtx.Lock()
		fw.file.fs.pendingGCList = append(fw.file.fs.pendingGCList, fw.file.superBlk)
		fw.file.fs.filesMtx.Unlock()
	}

	// file changes
	// overwrite file superBlock
	fw.file.superBlk = fw.superBlk
	fw.file.readMtx.Unlock()

	// superBlock changes
	fw.superBlk.modTime = time.Now()
	// decrement openCount
	atomic.AddUint32(&fw.superBlk.openCount, ^uint32(0))
	// notify all waiting readers
	close(fw.superBlk.notifyWrite)
	close(fw.superBlk.notifyWriteDone)
	fw.superBlk.mtx.Unlock()

	// writeMtx.Lock was acquire in AcquireWriter
	fw.file.writeMtx.Unlock()

	// clear references
	fw.file = nil
	fw.lastBlk = nil
	fw.superBlk = nil
	return nil
}

// Abort discards all written bytes and closes file writer. Note that the file writer may have written in place and the
// changes can already be seen by readers. Abort will return ErrAlreadyCommitted in this case.
//
// The file writer must not be used afterwards.
func (fw *fileWriter) Abort() error {
	// TODO: store old superBlk to support aborting in-place writes
	if fw.file.superBlk == fw.superBlk {
		// already written in place
		_ = fw.Commit()
		return volume.ErrAlreadyCommitted
	}

	fw.superBlk.mtx.Lock()
	fw.file.readMtx.Lock()

	// fs changes
	fw.file.fs.filesMtx.Lock()
	fw.file.fs.pendingGCList = append(fw.file.fs.pendingGCList, fw.superBlk)
	if fw.file.superBlk == nil {
		// remove file from fs as first writer aborted write
		delete(fw.file.fs.files, fw.file.path)
	}
	fw.file.readMtx.Unlock()
	fw.file.fs.filesMtx.Unlock()

	// superBlock changes
	// decrement openCount
	atomic.AddUint32(&fw.superBlk.openCount, ^uint32(0))
	// notify all waiting readers
	close(fw.superBlk.notifyWrite)
	close(fw.superBlk.notifyWriteDone)
	fw.superBlk.mtx.Unlock()

	// writeMtx.Lock was acquire in AcquireWriter
	fw.file.writeMtx.Unlock()

	// clear references
	fw.file = nil
	fw.lastBlk = nil
	fw.superBlk = nil
	return nil
}

// superBlock is the first of a file that also contains management information.
type superBlock struct {
	// Count open file reader and writer. Must be in-/decreased using atomic operations.
	//
	// If superBlock is in pending GC list:
	//   openCount == 0  =>  all blocks can be released
	//   openCount > 0   =>  there are still open reader and/or a writer, but superBlock
	//                       is no longer part of a file and openCount cannot increase.
	openCount uint32

	mtx             sync.RWMutex
	notifyWrite     chan struct{}
	notifyWriteDone chan struct{}

	modTime time.Time
	size    int64
	blk     *block
}

func (sb *superBlock) hasWriter() bool {
	select {
	case <-sb.notifyWriteDone:
		// notifyWriteDone is closed => no writer
		return false
	default:
		// notifyWriteDone is open => writer
		return true
	}
}

type block struct {
	nextBlk *block
	data    []byte
}

func (blk *block) reset() {
	blk.nextBlk = nil
	blk.data = blk.data[:0]
}

func (blk *block) write(p []byte) []byte {
	l := len(blk.data)
	n := copy(blk.data[l:cap(blk.data)], p)
	blk.data = blk.data[0 : l+n]
	return p[n:]
}
