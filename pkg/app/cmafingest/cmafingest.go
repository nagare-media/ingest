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

package cmafingest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sync"
	"time"

	mp4ff "github.com/edgeware/mp4ff/mp4"
	"github.com/gofiber/fiber/v2"
	"github.com/inhies/go-bytesize"
	"go.uber.org/zap"

	"github.com/nagare-media/ingest/internal/pool"
	"github.com/nagare-media/ingest/pkg/app"
	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
	"github.com/nagare-media/ingest/pkg/event"
	"github.com/nagare-media/ingest/pkg/http"
	"github.com/nagare-media/ingest/pkg/http/router"
	"github.com/nagare-media/ingest/pkg/media"
	"github.com/nagare-media/ingest/pkg/media/mp4"
	"github.com/nagare-media/ingest/pkg/volume"
)

const (
	DASH_IFVersionHeader  = "DASH-IF-Ingest"
	DASH_IFDefaultVersion = DASH_IFVersion1_1
	DASH_IFVersion1_0     = "1.0"
	DASH_IFVersion1_1     = "1.1"
)

const (
	InitFileName         = "init.mp4"
	FragmentFileNameTmpl = "frag-%d.m4s"
)

var (
	DefaultConfig = v1alpha1.CMAFIngest{
		StreamTimeout:      2 * time.Minute,
		MaxManifestSize:    5 * bytesize.MB,
		MaxHeaderSize:      2 * bytesize.KB,
		MaxChunkHeaderSize: 2 * bytesize.KB,
		MaxChunkMdatSize:   5 * bytesize.MB,
	}
)

type cmafIngest struct {
	cfg         v1alpha1.App
	eventStream event.Stream
	ctx         context.Context
	execCtx     app.ExecCtx

	chunkHdrBufferPool sync.Pool

	streamsMtx sync.RWMutex
	streams    map[string]*media.Presentation
}

func New(cfg v1alpha1.App) (app.App, error) {
	if err := app.CheckAndSetDefaults(&cfg); err != nil {
		return nil, err
	}
	if cfg.CMAFIngest.StreamTimeout <= 0 {
		cfg.CMAFIngest.StreamTimeout = DefaultConfig.StreamTimeout
	}
	if cfg.CMAFIngest.MaxManifestSize <= 0 {
		cfg.CMAFIngest.MaxManifestSize = DefaultConfig.MaxManifestSize
	}
	if cfg.CMAFIngest.MaxHeaderSize <= 0 {
		cfg.CMAFIngest.MaxHeaderSize = DefaultConfig.MaxHeaderSize
	}
	if cfg.CMAFIngest.MaxChunkHeaderSize <= 0 {
		cfg.CMAFIngest.MaxChunkHeaderSize = DefaultConfig.MaxChunkHeaderSize
	}
	if cfg.CMAFIngest.MaxChunkMdatSize <= 0 {
		cfg.CMAFIngest.MaxChunkMdatSize = DefaultConfig.MaxChunkMdatSize
	}

	app := &cmafIngest{
		cfg:         cfg,
		eventStream: event.NewStream(),
		streams:     make(map[string]*media.Presentation),
		chunkHdrBufferPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, cfg.CMAFIngest.MaxChunkHeaderSize)
			},
		},
	}

	return app, nil
}

func (a *cmafIngest) Config() v1alpha1.App {
	return a.cfg
}

func (a *cmafIngest) HTTPConfig() *v1alpha1.HTTPApp {
	return a.cfg.HTTP
}

func (a *cmafIngest) EventStream() event.Stream {
	return a.eventStream
}

func (a *cmafIngest) SetCtx(ctx context.Context) {
	a.ctx = ctx
	a.eventStream.Start(ctx)
}

func (a *cmafIngest) SetExecCtx(execCtx app.ExecCtx) {
	a.execCtx = execCtx
}

func (a *cmafIngest) RegisterHTTPRoutes(router router.Router, handleOptions bool) error {
	router.
		// middleware
		Group("/", a.handleCheckDASHIFVersion).
		// routes
		Post("/:name.str/Switching(+)/Streams(+)", a.handleSwitchingSetTrackUpload).
		Put("/:name.str/Switching(+)/Streams(+)", a.handleSwitchingSetTrackUpload).
		Post("/:name.str/Streams(+)", a.handleTrackUpload).
		Put("/:name.str/Streams(+)", a.handleTrackUpload).
		Post("/:name.str/+", a.handleUpload).
		Put("/:name.str/+", a.handleUpload)

	if handleOptions { // preflight requests
		router.
			Options("/:name.str/Switching(+)/Streams(+)", http.NoContentHandler).
			Options("/:name.str/Streams(+)", http.NoContentHandler).
			Options("/:name.str/+", http.NoContentHandler)
	}

	return nil
}

func (a *cmafIngest) handleCheckDASHIFVersion(c *fiber.Ctx) error {
	log := a.execCtx.Logger()

	// DASH-IF-Ingest is optional
	protoVer := c.Get(DASH_IFVersionHeader, DASH_IFDefaultVersion)
	if protoVer != DASH_IFVersion1_0 && protoVer != DASH_IFVersion1_1 {
		log.With("error", http.ErrUnsupportedProtocolVersion).Warnf("client connected with unsupported DASH-IF version '%s'", protoVer)
		return http.ErrUnsupportedProtocolVersion
	}

	return c.Next()
}

func (a *cmafIngest) handleSwitchingSetTrackUpload(c *fiber.Ctx) error {
	// validate path segments
	streamName := c.Params("name")
	if !media.StreamNameRegex.Match([]byte(streamName)) {
		return http.ErrUnsupportedStreamName
	}
	streamName = path.Join("/", c.Hostname(), streamName+".str")
	streamName = string(http.PathIllegalCharsRegex.ReplaceAll([]byte(streamName), http.PathIllegalReplaceChar))

	switchingSetName := c.Params("+1")
	if !media.SwitchingSetNameRegex.Match([]byte(switchingSetName)) {
		return http.ErrUnsupportedSwitchingSetName
	}

	trackName := path.Join("/", c.Params("+2")) // Join will also clean path
	if !media.TrackNameRegex.Match([]byte(trackName)) {
		return http.ErrUnsupportedTrackName
	}

	// create state for stream
	stream := a.getOrCreateStream(streamName)
	switchingSet := a.getOrCreateSwitchingSet(stream, switchingSetName)
	track := a.getOrCreateTrack(switchingSet, trackName)
	log := a.getLoggerWithTrack(track)

	// start ingest: long upload using transfer chunked encoding
	// upload can only be a CMAF track (= CMAF header + CMAF fragments...)
	if !c.Request().IsBodyStream() {
		return http.ErrNotAFileStream
	}
	reqReader := c.Context().RequestBodyStream()

	// TODO: add support for gzip contenet encoding

	pathPrefix := a.trackPrefixPath(c, track)

	// TODO: ingestHeader can be skipped if track is already initialized
	err := convertIngestErr(a.ingestHeader(reqReader, track, pathPrefix))
	if err != nil {
		return err
	}

	err = convertIngestErr(a.ingestFragments(reqReader, track, pathPrefix))
	if err != nil {
		return err
	}

	log.Info("stop track")

	// DASH-IF does not specify a response code; use 201 Created
	c.Status(fiber.StatusCreated)
	// TODO: send body with transfert time, size, etc.
	// TODO: set headers like ETag, Last-Modified, Location?, caching, ...
	return nil
}

func (a *cmafIngest) handleTrackUpload(c *fiber.Ctx) error {
	log := a.execCtx.Logger()
	log.Warn("currently only /:name.str/Switching(+)/Streams(+) uploads allowed")
	return fiber.ErrInternalServerError

	// long upload using transfer chunked encoding
	// upload is CMAF track = CMAF header + CMAF fragments...

	// try to determine switching set
}

func (a *cmafIngest) handleUpload(c *fiber.Ctx) error {
	log := a.execCtx.Logger()
	log.Warn("currently only /:name.str/Switching(+)/Streams(+) uploads allowed")
	return fiber.ErrInternalServerError

	// upload is either
	//   1. MPEG DASH manifest
	//      must be identical
	//        SegmentTemplate@initialization $RepresentationID$
	//        SegmentTemplate@media          $RepresentationID$ + $Number$ / $Time$
	//      representation = CMAF track
	//      adoption set   = CMAF switching set
	//      singe period
	//   2. HLS manifest
	//   3. CMAF header corresponding to SegmentTemplate@initialization pattern
	//   4. CMAF fragment/segment corresponding to SegmentTemplate@media pattern
	//   5. an empty body to test the connection
}

func (a *cmafIngest) getOrCreateStream(name string) *media.Presentation {
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

	e := event.NewStreamEvent(event.StreamStartEvent, s)
	a.eventStream.Pub(e)

	go a.gcStreamAfterTimeout(name)
	return s
}

func (a *cmafIngest) gcStreamAfterTimeout(name string) {
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
		if lastModifiedDuration > a.cfg.CMAFIngest.StreamTimeout {
			a.streamsMtx.Lock()
			defer a.streamsMtx.Unlock()
			delete(a.streams, name)

			var e event.Event
			for _, set := range s.SwitchingSets {
				for _, t := range set.Tracks {
					e = event.NewTrackEvent(event.TrackStopEvent, t)
					a.eventStream.Pub(e)
				}
				e = event.NewSwitchingSetEvent(event.SwitchingSetStopEvent, set)
				a.eventStream.Pub(e)
			}
			e = event.NewStreamEvent(event.StreamStopEvent, s)
			a.eventStream.Pub(e)

			return true
		}

		return false
	}

	t := time.NewTicker(a.cfg.CMAFIngest.StreamTimeout)
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

func (a *cmafIngest) getOrCreateSwitchingSet(stream *media.Presentation, name string) *media.SwitchingSet {
	stream.RLock()
	if set, ok := stream.SwitchingSets[name]; ok {
		defer stream.RUnlock()
		return set
	}

	// upgrade to write lock
	stream.RUnlock()
	stream.Lock()
	defer stream.Unlock()

	// check if there was a race
	if set, ok := stream.SwitchingSets[name]; ok {
		return set
	}

	log := a.execCtx.Logger().With(
		"stream", stream.Name,
		"switchingSet", name,
	)
	log.Info("start switching set")

	set := &media.SwitchingSet{
		Name: name,
	}
	stream.AddSwitchingSet(set)

	e := event.NewSwitchingSetEvent(event.SwitchingSetStartEvent, set)
	a.eventStream.Pub(e)

	return set
}

func (a *cmafIngest) getOrCreateTrack(set *media.SwitchingSet, name string) *media.Track {
	set.RLock()
	if t, ok := set.Tracks[name]; ok {
		defer set.RUnlock()
		return t
	}

	// upgrade to write lock
	set.RUnlock()
	set.Lock()
	defer set.Unlock()

	// check if there was a race
	if t, ok := set.Tracks[name]; ok {
		return t
	}

	log := a.execCtx.Logger().With(
		"stream", set.Presentation.Name,
		"switchingSet", set.Name,
		"track", name,
	)
	log.Info("start track")

	t := &media.Track{
		Name:   name,
		Format: media.CMAF,
	}
	set.AddTrack(t)

	e := event.NewTrackEvent(event.TrackStartEvent, t)
	a.eventStream.Pub(e)

	return t
}

func (a *cmafIngest) trackPrefixPath(c *fiber.Ctx, track *media.Track) string {
	track.RLock()
	defer track.RUnlock()

	prefixPath := path.Join(
		a.execCtx.PathPrefix(),
		track.SwitchingSet.Presentation.Name,
		track.SwitchingSet.Name,
		track.Name,
	)
	return string(http.PathIllegalCharsRegex.ReplaceAll([]byte(prefixPath), http.PathIllegalReplaceChar))
}

// Ingest CMAF header.
//
// CMAF header boxes (ISO/IEC 23000-19:2020 7.3.1 Table 3)
//   ftyp 1
//   moov 1
//       mvhd 1
//       trak 1
//           tkhd 1
//           edts CR (edit list)
//               elst 1
//           mdia 1
//               mdhd 1
//               hdlr 1
//               elng 0/1
//               minf 1
//                   vmhd CR (video)
//                   smhd CR (audio)
//                   sthd CR (subtitles)
//                   dinf 1
//                       dref 1
//                   stbl 1
//                       stsd 1
//                           sinf CR (encryption)
//                               frma 1
//                               schm 1
//                               schi 1
//                               schi 1
//                                   tenc 1
//                       stts 1
//                       stsc 1
//                       stsz/stz2 1
//                       stco 1
//                       sgpd CR
//                       stss CR
//           udta 0/1
//               cprt *
//               kind *
//       mvex 1
//           mehd 0/1
//           trex 1
//       pssh * (encryption)
func (a *cmafIngest) ingestHeader(reader io.Reader, track *media.Track, pathPrefix string) error {
	log := a.getLoggerWithTrack(track)

	hdr := &mp4ff.BoxHeader{}
	cmafHdr := mp4ff.NewMP4Init()

	// limit header size as we keep the CMAF header in memory
	lr := io.LimitReader(reader, int64(a.cfg.CMAFIngest.MaxHeaderSize))

	// decode ftyp box
	err := mp4.DecodeHeader(hdr, lr)
	if err != nil {
		return err
	}
	if hdr.Name != mp4.FtypBoxStr {
		return mp4.ErrNotACMAFHeader
	}
	ftypBox, err := mp4ff.DecodeFtyp(*hdr, 0, lr) // we don't care about startPos
	if err != nil {
		return err
	}
	cmafHdr.AddChild(ftypBox)

	// decode moov box
	err = mp4.DecodeHeader(hdr, lr)
	if err != nil {
		return err
	}
	if hdr.Name != mp4.MoovBoxStr {
		return mp4.ErrNotACMAFHeader
	}
	moovBox, err := mp4ff.DecodeMoov(*hdr, 0, lr) // we don't care about startPos
	if err != nil {
		return err
	}
	cmafHdr.AddChild(moovBox)

	// check moov
	err = mp4.CheckMoovCMAF(cmafHdr.Moov)
	if err != nil {
		return err
	}

	// keep header in memory as functions might need it
	track.Lock()
	track.CMAFHeader = cmafHdr
	track.Initialized = true
	track.Unlock()

	// write CMAF header file
	filePath := path.Join(pathPrefix, InitFileName)
	file, fw, err := a.openFileWriter(filePath, false)
	if err != nil {
		return err
	}
	log.Debugf("ingest CMAF header: %s", InitFileName)
	e := event.NewInitSegmentEvent(event.InitSegmentStartEvent, file, track)
	a.eventStream.Pub(e)

	err = cmafHdr.Encode(fw)
	if err != nil {
		_ = fw.Abort()
		e = event.NewInitSegmentEvent(event.InitSegmentAbortedEvent, file, track)
		a.eventStream.Pub(e)
		return err
	}

	err = fw.Commit()
	if err != nil {
		return err
	}
	e = event.NewInitSegmentEvent(event.InitSegmentCommittedEvent, file, track)
	a.eventStream.Pub(e)

	return nil
}

// Ingest CMAF segments.
//
// CMAF chunk boxes (ISO/IEC 23000-19:2020 7.3.1 Table 5 and 7.3.2.3 Table 7)
//   styp 0/1
//   prft 0/1
//   emsg *
//   moof 1
//      mfhd 1
//      traf 1
//         tfhd 1
//         tfdt 1
//         trun 1
//         senc 0/1
//         saio CR
//         saiz CR
//         sbqp *
//         sgpd *
//         subs CR
//   mdat 1
func (a *cmafIngest) ingestFragments(reader io.Reader, track *media.Track, pathPrefix string) error {
	track.RLock()
	if !track.Initialized {
		return mp4.ErrTrackUninitialized
	}
	track.RUnlock()

	log := a.getLoggerWithTrack(track)

	var err error
	var e event.Event
	var frag *mp4.Fragment

	var n int
	var nMdat int64
	triedCommit := false
	var file volume.File
	var fw volume.FileWriter
	lr := &io.LimitedReader{
		R: reader,
		N: int64(a.cfg.CMAFIngest.MaxChunkHeaderSize),
	}

	hdr := &mp4ff.BoxHeader{}
	var bodyLen uint64

	chunkHdrSize := 0
	chunkHdrBuf := a.chunkHdrBufferPool.Get().([]byte)
	defer a.chunkHdrBufferPool.Put(chunkHdrBuf) // nolint

	copyBuf := pool.CopyBuf.Get().([]byte)
	defer pool.CopyBuf.Put(copyBuf) // nolint

	defer func() {
		if !triedCommit && fw != nil {
			_ = fw.Abort()
			e = event.NewFragmentEvent(event.FragmentAbortedEvent, file, track, frag)
			a.eventStream.Pub(e)
		}
	}()

	for {
		// decode box header
		err = mp4.DecodeHeader(hdr, lr)
		if err != nil {
			if (err == io.EOF || err == io.ErrUnexpectedEOF) && chunkHdrSize == 0 {
				// reader returned EOF and new CMAF chunk has not yet started => track finished
				if fw != nil {
					triedCommit = true
					err = fw.Commit()
					if err != nil {
						return err
					}
					e = event.NewFragmentEvent(event.FragmentCommittedEvent, file, track, frag)
					a.eventStream.Pub(e)
				}
				return nil
			}
			return err
		}

		// decode box
		switch hdr.Name {
		case mp4.StypBoxStr:
			// TODO: should we add a styp box if missing? maybe a config option?
			// TODO: should we react to slat and lmsg brands?
			fallthrough
		case mp4.PrftBoxStr:
			// TODO: should we measure and report latency? maybe as Prometheus metric?
			fallthrough
		case mp4.EmsgBoxStr: // TODO: should we emit events to functions?
			// we skip decoding these boxes further as we are not interested in the body

			// these boxes should not be large
			if hdr.Size >= 1<<32 {
				return mp4.ErrNotACMAFChunk
			}

			// write box header
			n, err = mp4.EncodeHeader(hdr, chunkHdrBuf[chunkHdrSize:cap(chunkHdrBuf)])
			if err != nil {
				return err
			}
			chunkHdrSize += n

			// write box body
			bodyLen = hdr.Size - uint64(hdr.Hdrlen)
			if cap(chunkHdrBuf) < chunkHdrSize+int(bodyLen) {
				return io.ErrShortBuffer
			}
			n, err = io.ReadAtLeast(lr, chunkHdrBuf[chunkHdrSize:chunkHdrSize+int(bodyLen)], int(bodyLen))
			if err != nil {
				return err
			}
			if uint64(n) != bodyLen {
				// we won't deal with that; writer should have returned less bytes
				return errors.New("large read")
			}
			chunkHdrSize += n

		case mp4.MoofBoxStr:
			// we must decode moof box to determine fragment boundaries

			// decode moof
			moofBox, err := mp4ff.DecodeMoof(*hdr, 0, lr) // we don't care about startPos
			if err != nil {
				return err
			}
			moof := moofBox.(*mp4ff.MoofBox)

			// check moof
			err = mp4.CheckMoofCMAF(moof)
			if err != nil {
				return err
			}

			// check if we missed a chunk
			// TODO: should we also check baseMediaDecodeTime?
			seqN := moof.Mfhd.SequenceNumber
			track.RLock()
			// SequenceNumber usually starts at 1; LastSequenceNumber is 0 initially so we don't think that we missed
			// something at the beginning
			missedChunk := (seqN != track.LastSequenceNumber+1)
			track.RUnlock()
			if missedChunk {
				// TODO: emit events
				_ = missedChunk
			}

			// determine output file
			smplFlags, err := mp4.FirstSampleFlags(track, moof)
			if err != nil {
				return err
			}
			if mp4ff.IsSyncSampleFlags(smplFlags) {
				// start a new fragment
				if fw != nil {
					triedCommit = true
					err = fw.Commit()
					if err != nil {
						return err
					}
					e = event.NewFragmentEvent(event.FragmentCommittedEvent, file, track, frag)
					a.eventStream.Pub(e)
				}
				fileName := fragmentFileName(moof)
				filePath := path.Join(pathPrefix, fileName)
				frag = &mp4.Fragment{}
				file, fw, err = a.openFileWriter(filePath, false)
				if err != nil {
					return err
				}
				log.Debugf("ingest CMAF fragment: %s", fileName)
			}
			if fw == nil {
				// => first chunk starts with a non sync segment
				return mp4.ErrNotACMAFFragment
			}

			// write buffered boxes
			_, err = fw.Write(chunkHdrBuf[:chunkHdrSize])
			if err != nil {
				return err
			}

			// write moof
			err = moof.Encode(fw)
			if err != nil {
				return err
			}

			// write mdat
			err = mp4.DecodeHeader(hdr, reader)
			if err != nil {
				return err
			}
			if hdr.Name != mp4.MdatBoxStr {
				return mp4.ErrNotACMAFChunk
			}
			bodyLen = hdr.Size - uint64(hdr.Hdrlen)
			lr.N = int64(bodyLen)
			if bodyLen != uint64(lr.N) || bodyLen > uint64(a.cfg.CMAFIngest.MaxChunkMdatSize) {
				return io.ErrUnexpectedEOF
			}

			chunkHdrSize, err = mp4.EncodeHeader(hdr, chunkHdrBuf) // reuse buffer for chunk header as we have already written the content
			if err != nil {
				return err
			}
			_, err = fw.Write(chunkHdrBuf[:chunkHdrSize])
			if err != nil {
				return err
			}

			nMdat, err = io.CopyBuffer(fw, lr, copyBuf)
			if err != nil {
				return err
			}
			if nMdat != int64(bodyLen) {
				// mdat body was shorter then advertised
				return io.ErrUnexpectedEOF
			}

			// track chunks
			chunk := &mp4.Chunk{
				Moof: moof,
			}
			frag.Chunks = append(frag.Chunks, chunk)

			// track progress
			now := time.Now()
			track.Lock()
			track.SwitchingSet.Presentation.LastModifiedAt = now
			track.LastSequenceNumber = seqN
			track.Unlock()

			// mdat is last box in chunk => prepare for next chunk
			moof = nil
			triedCommit = false
			chunkHdrSize = 0
			lr.N = int64(a.cfg.CMAFIngest.MaxChunkHeaderSize)

		default:
			return mp4.ErrNotACMAFChunk
		}
	}
}

func (a *cmafIngest) openFileWriter(path string, inPlace bool) (volume.File, volume.FileWriter, error) {
	if vol, ok := a.getVolume(); ok {
		f, err := vol.OpenCreate(path)
		if err != nil {
			return nil, nil, err
		}

		fw, err := f.AcquireWriter(inPlace)
		if err != nil {
			return nil, nil, err
		}

		return f, fw, nil
	}
	return nil, nil, fiber.ErrInternalServerError
}

func (a *cmafIngest) getVolume() (volume.Volume, bool) {
	return a.execCtx.VolumeRegistry().Get(a.cfg.CMAFIngest.VolumeRef.Name)
}

func (a *cmafIngest) getLoggerWithTrack(track *media.Track) *zap.SugaredLogger {
	track.RLock()
	defer track.RUnlock()
	log := a.execCtx.Logger().With(
		"stream", track.SwitchingSet.Presentation.Name,
		"switchingSet", track.SwitchingSet.Name,
		"track", track.Name,
	)
	return log
}

func fragmentFileName(moof *mp4ff.MoofBox) string {
	// TODO: use CMAF file extensions
	baseMediaDecodeTime := moof.Traf.Tfdt.BaseMediaDecodeTime
	return fmt.Sprintf(FragmentFileNameTmpl, baseMediaDecodeTime)
}

func convertIngestErr(err error) error {
	switch err {
	case nil, io.EOF:
		return nil

	case io.ErrUnexpectedEOF, io.ErrShortBuffer:
		return fiber.ErrRequestEntityTooLarge

	case mp4.ErrNotACMAFHeader, mp4.ErrNotACMAFChunk, mp4.ErrNotACMAFFragment:
		return fiber.ErrUnsupportedMediaType

	case mp4.ErrTrackUninitialized:
		return fiber.ErrPreconditionFailed

	default:
		return fiber.ErrInternalServerError
	}
}
