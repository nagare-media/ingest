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

package dashandhlsingest

import (
	"context"
	"errors"
	"io"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/inhies/go-bytesize"

	"github.com/nagare-media/ingest/pkg/app"
	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
	"github.com/nagare-media/ingest/pkg/event"
	"github.com/nagare-media/ingest/pkg/http"
	"github.com/nagare-media/ingest/pkg/http/router"
	"github.com/nagare-media/ingest/pkg/media"
	"github.com/nagare-media/ingest/pkg/mime"
	"github.com/nagare-media/ingest/pkg/volume"
)

const (
	DASH_IFVersionHeader  = "DASH-IF-Ingest"
	DASH_IFDefaultVersion = DASH_IFVersion1_1
	DASH_IFVersion1_0     = "1.0"
	DASH_IFVersion1_1     = "1.1"
)

var (
	DefaultConfig = v1alpha1.DASHAndHLSIngest{
		StreamTimeout:           2 * time.Minute,
		ReleaseCompleteSegments: false,
		RequestBodyBufferSize:   32 * bytesize.KB,
		MaxManifestSize:         5 * bytesize.MB,
		MaxSegmentSize:          15 * bytesize.MB,
		MaxEncryptionKeySize:    2 * bytesize.MB,
	}
)

type dashAndHLSIngest struct {
	cfg         v1alpha1.App
	eventStream event.Stream
	ctx         context.Context
	execCtx     app.ExecCtx

	bufferPool sync.Pool

	streamsMtx sync.RWMutex
	streams    map[string]*media.Presentation
}

func New(cfg v1alpha1.App) (app.App, error) {
	if err := app.CheckAndSetDefaults(&cfg); err != nil {
		return nil, err
	}
	if cfg.DASHAndHLSIngest.VolumeRef.Name == "" {
		return nil, errors.New("dashandhlsingest.New: volumeRef is missing")
	}
	if cfg.DASHAndHLSIngest.StreamTimeout <= 0 {
		cfg.DASHAndHLSIngest.StreamTimeout = DefaultConfig.StreamTimeout
	}
	if cfg.DASHAndHLSIngest.RequestBodyBufferSize <= 0 {
		cfg.DASHAndHLSIngest.RequestBodyBufferSize = DefaultConfig.RequestBodyBufferSize
	}
	if cfg.DASHAndHLSIngest.MaxManifestSize <= 0 {
		cfg.DASHAndHLSIngest.MaxManifestSize = DefaultConfig.MaxManifestSize
	}
	if cfg.DASHAndHLSIngest.MaxSegmentSize <= 0 {
		cfg.DASHAndHLSIngest.MaxSegmentSize = DefaultConfig.MaxSegmentSize
	}
	if cfg.DASHAndHLSIngest.MaxEncryptionKeySize <= 0 {
		cfg.DASHAndHLSIngest.MaxEncryptionKeySize = DefaultConfig.MaxEncryptionKeySize
	}

	app := &dashAndHLSIngest{
		cfg:         cfg,
		eventStream: event.NewStream(),
		streams:     make(map[string]*media.Presentation),
		bufferPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, cfg.DASHAndHLSIngest.RequestBodyBufferSize)
			},
		},
	}
	return app, nil
}

func (a *dashAndHLSIngest) Config() v1alpha1.App {
	return a.cfg
}

func (a *dashAndHLSIngest) HTTPConfig() *v1alpha1.HTTPApp {
	return a.cfg.HTTP
}

func (a *dashAndHLSIngest) EventStream() event.Stream {
	return a.eventStream
}

func (a *dashAndHLSIngest) SetCtx(ctx context.Context) {
	a.ctx = ctx
	a.eventStream.Start(ctx)
}

func (a *dashAndHLSIngest) SetExecCtx(execCtx app.ExecCtx) {
	a.execCtx = execCtx
}

func (a *dashAndHLSIngest) RegisterHTTPRoutes(router router.Router, handleOptions bool) error {
	router.
		// middleware
		Group("/", a.handleCheckDASHIFVersion).
		// routes
		Post("/:name.str/+", a.handleUpload).
		Put("/:name.str/+", a.handleUpload).
		Delete("/:name.str/+", a.handleDelete)

	if handleOptions { // preflight requests
		router.Options("/:name.str/+", http.NoContentHandler)
	}

	return nil
}

func (a *dashAndHLSIngest) handleCheckDASHIFVersion(c *fiber.Ctx) error {
	log := a.execCtx.Logger()

	// DASH-IF-Ingest is optional
	protoVer := c.Get(DASH_IFVersionHeader, DASH_IFDefaultVersion)
	if protoVer != DASH_IFVersion1_0 && protoVer != DASH_IFVersion1_1 {
		log.With("error", http.ErrUnsupportedProtocolVersion).Warnf("client connected with unsupported DASH-IF version '%s'", protoVer)
		return http.ErrUnsupportedProtocolVersion
	}

	return c.Next()
}

func (a *dashAndHLSIngest) handleDelete(c *fiber.Ctx) error {
	log := a.execCtx.Logger()

	streamName := c.Params("name")
	if !media.StreamNameRegex.Match([]byte(streamName)) {
		return http.ErrUnsupportedStreamName
	}
	streamName = path.Join("/", c.Hostname(), streamName+".str")
	streamName = string(http.PathIllegalCharsRegex.ReplaceAll([]byte(streamName), http.PathIllegalReplaceChar))
	log = log.With("stream", streamName)

	// determine file path
	uploadPath := path.Join("/", c.Params("+")) // Join will also clean path
	if !http.UploadPathRegex.Match([]byte(uploadPath)) {
		return http.ErrUnsupportedUploadPath
	}
	prefixPath := path.Join(a.execCtx.PathPrefix(), streamName)
	prefixPath = string(http.PathIllegalCharsRegex.ReplaceAll([]byte(prefixPath), http.PathIllegalReplaceChar))
	filePath := path.Join(prefixPath, uploadPath)

	// get volume
	vol, ok := a.getVolume()
	if !ok {
		return fiber.ErrInternalServerError
	}

	err := vol.Delete(filePath)
	if err != nil {
		log.With("error", err).Errorf("failed to delete file: %s", uploadPath)
		return fiber.ErrInternalServerError
	}

	// 200 according to DASH-IF
	c.Status(fiber.StatusOK)
	return nil
}

func (a *dashAndHLSIngest) handleUpload(c *fiber.Ctx) error {
	log := a.execCtx.Logger()

	streamName := c.Params("name")
	if !media.StreamNameRegex.Match([]byte(streamName)) {
		return http.ErrUnsupportedStreamName
	}
	streamName = path.Join("/", c.Hostname(), streamName+".str")
	streamName = string(http.PathIllegalCharsRegex.ReplaceAll([]byte(streamName), http.PathIllegalReplaceChar))
	log = log.With("stream", streamName)

	// determine file path
	uploadPath := path.Join("/", c.Params("+")) // Join will also clean path
	if !http.UploadPathRegex.Match([]byte(uploadPath)) {
		return http.ErrUnsupportedUploadPath
	}
	prefixPath := path.Join(a.execCtx.PathPrefix(), streamName)
	prefixPath = string(http.PathIllegalCharsRegex.ReplaceAll([]byte(prefixPath), http.PathIllegalReplaceChar))
	filePath := path.Join(prefixPath, uploadPath)

	// check file extension
	// DASH-IF requires the initialization segment to use the .init file extension. We allow other segment file extensions.
	fileExt := strings.ToLower(path.Ext(filePath))
	if !supportedFileExt(fileExt) {
		return http.ErrUnsupportedFileExtension
	}

	// check MIME type matches file extension
	originalMimeType := strings.ToLower(c.Get(fiber.HeaderContentType))
	// DASH-IF requires that a MIME type is set. We allow an empty MIME type and normalize it according to the file
	// extension. Otherwise both need to match.
	mimeType := mime.NormalizeExt(originalMimeType, fileExt)
	if !mime.MatchExt(mimeType, fileExt) {
		return fiber.ErrUnsupportedMediaType
	}

	// check file type
	fileType := media.FileTypeExt(fileExt)

	// configure upload
	var maxBytes int64
	writeInPlace := false

	switch {
	case media.IsManifest(fileType):
		maxBytes = int64(a.cfg.DASHAndHLSIngest.MaxManifestSize)

	case media.IsSegment(fileType):
		maxBytes = int64(a.cfg.DASHAndHLSIngest.MaxSegmentSize)
		writeInPlace = !a.cfg.DASHAndHLSIngest.ReleaseCompleteSegments // low latency mode

	case media.IsEncryptionKey(fileType):
		maxBytes = int64(a.cfg.DASHAndHLSIngest.MaxEncryptionKeySize)

	default:
		panic("BUG: missing file extension to media file type mapping")
	}

	// get volume
	vol, ok := a.getVolume()
	if !ok {
		return fiber.ErrInternalServerError
	}

	// get or create stream info
	stream := a.getOrCreateStream(streamName)

	// get reader
	if !c.Request().IsBodyStream() {
		return http.ErrNotAFileStream
	}
	reqReader := c.Context().RequestBodyStream()

	// get writer
	file, err := vol.OpenCreate(filePath)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	fileWriter, err := file.AcquireWriter(writeInPlace) // Closed below
	if err != nil {
		return fiber.ErrInternalServerError
	}
	e := event.NewFileEvent(event.FileStartEvent, file)
	a.eventStream.Pub(e)

	// copy stream
	buf := a.bufferPool.Get().([]byte)
	defer a.bufferPool.Put(buf) // nolint
	buf = buf[0:cap(buf)]       // reset

	var n int64
	for {
		if maxBytes-n < int64(len(buf)) {
			// we are approaching maxBytes; one last read
			buf = buf[0 : maxBytes-n]
		}
		nr, er := reqReader.Read(buf)
		if nr > 0 {
			nw, ew := fileWriter.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				panic("BUG: written bytes is negative or larger that buffer")
			}

			// track progress
			n += int64(nw)
			now := time.Now()
			stream.Lock()
			stream.LastModifiedAt = now
			stream.Unlock()

			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er != nil {
			err = er
			break
		}
		if n >= maxBytes {
			err = fiber.ErrRequestEntityTooLarge
			break
		}
	}

	if err != io.EOF {
		ea := fileWriter.Abort()
		e = event.NewFileEvent(event.FileAbortedEvent, file)
		a.eventStream.Pub(e)
		// ignore aborte error, e.g. if written in place; it's already done
		if err == fiber.ErrRequestEntityTooLarge {
			// TODO: DASH-IF strictly does not allow for 413; maybe switch to more general 400?
			return err
		}
		log.With("error", err).Error("failed to write upload")
		if ea != nil {
			log.With("error", ea).Warn("failed to abort upload")
		}
		return fiber.ErrInternalServerError
	}

	err = fileWriter.Commit()
	e = event.NewFileEvent(event.FileCommittedEvent, file)
	a.eventStream.Pub(e)
	if err != nil {
		log.With("error", err).Error("failed to commit upload")
		return fiber.ErrInternalServerError
	}

	// TODO: add support for gzip contenet encoding
	// TODO: check if manifest has declared ending => end stream early
	// TODO: check if media segment follows regular pattern and match it with track
	// TODO: check if fMP4 media segment had init segment => 412 Precondition Failed

	// DASH-IF does not specify a response code; use 201 Created
	c.Status(fiber.StatusCreated)
	// TODO: send body with transfert time, size, etc.
	// TODO: set headers like ETag, Last-Modified, Location?, caching, ...
	return nil
}

func (a *dashAndHLSIngest) getVolume() (volume.Volume, bool) {
	return a.execCtx.VolumeRegistry().Get(a.cfg.DASHAndHLSIngest.VolumeRef.Name)
}

func (a *dashAndHLSIngest) getOrCreateStream(name string) *media.Presentation {
	a.streamsMtx.RLock()
	if s, ok := a.streams[name]; ok {
		defer a.streamsMtx.RUnlock()
		return s
	}

	// upgrade to write lock
	a.streamsMtx.RUnlock()
	a.streamsMtx.Lock()
	defer a.streamsMtx.Unlock()

	// check if there was a race
	if s, ok := a.streams[name]; ok {
		return s
	}

	log := a.execCtx.Logger().With("stream", name)
	log.Info("start stream")

	s := &media.Presentation{
		Name:           name,
		LastModifiedAt: time.Now(),
	}
	a.streams[name] = s
	go a.gcStreamAfterTimeout(name)
	return s
}

func (a *dashAndHLSIngest) gcStreamAfterTimeout(name string) {
	log := a.execCtx.Logger().With("stream", name)

	a.streamsMtx.Lock()
	s, ok := a.streams[name]
	if !ok {
		return
	}
	a.streamsMtx.Unlock()

	gc := func() bool {
		s.Lock()
		defer s.Unlock()

		lastModifiedDuration := time.Since(s.LastModifiedAt)
		if lastModifiedDuration > a.cfg.DASHAndHLSIngest.StreamTimeout {
			a.streamsMtx.Lock()
			defer a.streamsMtx.Unlock()
			delete(a.streams, name)
			return true
		}

		return false
	}

	t := time.NewTicker(a.cfg.DASHAndHLSIngest.StreamTimeout)
	defer t.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-t.C:
			if gc() {
				log.Info("stop stream")
				return
			}
		}
	}
}

func supportedFileExt(ext string) bool {
	// TODO: DASH-IF ingest does not mention WebM support which is possible with DASH. Should we add it (as with MPEG-TS for HLS)?
	switch ext {
	case
		// Manifest
		// DASH-IF Ingest only allows .m3u8 and .mpd, but .m3u is allowed by RFC 8216
		".m3u", ".m3u8", ".mpd",
		// CMAF
		".cmfv", ".cmfa", ".cmft", ".cmfm",
		// MPEG-4
		".mp4", ".m4v", ".m4a", ".m4s",
		// CMAF/MPEG-4 header
		".init", ".header",
		// MPEG-2 Transport Stream
		".ts",
		// Encryption Key
		".key":
		return true
	}
	return false
}
