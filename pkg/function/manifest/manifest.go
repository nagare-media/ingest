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

package manifest

import (
	"context"
	"fmt"
	"path"
	"sync"

	"go.uber.org/zap"

	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
	"github.com/nagare-media/ingest/pkg/event"
	"github.com/nagare-media/ingest/pkg/function"
	"github.com/nagare-media/ingest/pkg/media"
	"github.com/nagare-media/ingest/pkg/volume"
)

type manifest struct {
	cfg v1alpha1.Function
	vol volume.Volume
	log *zap.SugaredLogger

	appPathPrefix string

	mtx sync.RWMutex
	hls map[string]*hlsManifest
}

type hlsManifest struct {
	pathPrefix string

	audio hlsGroup
	video hlsGroup
}

type hlsGroup struct {
	tracks map[string]*hlsTrack
}

type hlsTrack struct {
	trk         *media.Track
	initSegment *hlsSegment
	segments    []*hlsSegment
}

type hlsSegment struct {
	path     string
	duration float64
}

func New(cfg v1alpha1.Function) (function.Function, error) {
	if err := function.CheckAndSetDefaults(&cfg); err != nil {
		return nil, err
	}

	fn := &manifest{
		cfg: cfg,
		hls: make(map[string]*hlsManifest),
	}

	return fn, nil
}

func (fn *manifest) Config() v1alpha1.Function {
	return fn.cfg
}

func (fn *manifest) Exec(ctx context.Context, execCtx function.ExecCtx) error {
	fn.log = execCtx.Logger()

	vol, ok := execCtx.VolumeRegistry().Get(fn.cfg.Manifest.VolumeRef.Name)
	if !ok {
		return fmt.Errorf("volume '%s' not found", fn.cfg.Manifest.VolumeRef.Name)
	}
	fn.vol = vol

	fn.appPathPrefix = execCtx.PathPrefix()

	eventStream := execCtx.App().EventStream().Sub()
	defer execCtx.App().EventStream().Desub(eventStream)

	for {
		select {
		case <-ctx.Done():
			// TODO: cleanup
			return nil

		case es := <-eventStream:
			switch e := es.(type) {
			case *event.InitSegmentEvent:
				switch e.Type {
				case event.InitSegmentCommittedEvent:
					fn.handleInitSegmentCommitted(e)
				}

			case *event.FragmentEvent:
				switch e.Type {
				case event.FragmentCommittedEvent:
					fn.handleFragmentCommitted(e)
				}
			}
		}
	}
}

func (fn *manifest) handleInitSegmentCommitted(e *event.InitSegmentEvent) {
	fn.mtx.Lock()
	e.Track.RLock()
	defer e.Track.RUnlock()
	defer fn.mtx.Unlock()

	strName := e.Track.SwitchingSet.Presentation.Name
	hls, ok := fn.hls[strName]
	if !ok {
		hls = &hlsManifest{
			pathPrefix: e.Track.SwitchingSet.Presentation.Name,
		}
		hls.audio.tracks = make(map[string]*hlsTrack)
		hls.video.tracks = make(map[string]*hlsTrack)
		fn.hls[strName] = hls
	}

	hlsTrk := &hlsTrack{
		trk: e.Track,
		initSegment: &hlsSegment{
			path: path.Base(e.File.Name()),
		},
	}

	trackName := path.Join(e.Track.SwitchingSet.Name, e.Track.Name)
	switch e.Track.CMAFHeader.Moov.Trak.Mdia.Hdlr.HandlerType {
	case "vide":
		hls.video.tracks[trackName] = hlsTrk
	case "soun":
		hls.audio.tracks[trackName] = hlsTrk
	}

	fn.updateHLSMasterPlaylist(hls)
}

func (fn *manifest) handleFragmentCommitted(e *event.FragmentEvent) {
	fn.mtx.RLock()
	e.Track.RLock()

	strName := e.Track.SwitchingSet.Presentation.Name
	hls, ok := fn.hls[strName]
	if !ok {
		fn.log.Error("failed to find stream for new fragment")
		return
	}
	fn.mtx.RUnlock()

	// check audio
	trackName := path.Join(e.Track.SwitchingSet.Name, e.Track.Name)
	hlsTrk, ok := hls.audio.tracks[trackName]
	if !ok {
		// check video
		hlsTrk, ok = hls.video.tracks[trackName]
	}
	if !ok {
		fn.log.Error("failed to find track for new fragment")
		return
	}
	e.Track.RUnlock()

	ts, err := hlsTrk.trk.Timescale()
	if err != nil {
		fn.log.With("error", err).Error("failed to find track timescale")
		return
	}

	seg := &hlsSegment{
		path:     path.Base(e.File.Name()),
		duration: float64(e.Fragment.Duration()) / float64(ts),
	}
	hlsTrk.segments = append(hlsTrk.segments, seg)

	fn.updateMediaPlaylist(hls, hlsTrk)
}

func (fn *manifest) updateHLSMasterPlaylist(hls *hlsManifest) {
	filePath := path.Join(fn.appPathPrefix, hls.pathPrefix, "master.m3u8")

	file, err := fn.vol.OpenCreate(filePath)
	if err != nil {
		fn.log.With("error", err).Errorf("failed to open HLS master playlist: '%s'", filePath)
		return
	}

	fw, err := file.AcquireWriter(false)
	if err != nil {
		fn.log.With("error", err).Errorf("failed to acquire writer for HLS master playlist: '%s'", filePath)
		return
	}

	// write header
	_, err = fmt.Fprintf(fw, "#EXTM3U\n#EXT-X-VERSION:7\n")
	if err != nil {
		fn.log.With("error", err).Errorf("failed to write header for HLS master playlist: '%s'", filePath)
		return
	}

	// write audio tracks
	i := -1
	for _, hlsTrk := range hls.audio.tracks {
		i++

		uri := path.Join("/", hlsTrk.trk.SwitchingSet.Name, hlsTrk.trk.Name, "index.m3u8")
		uri = uri[1:]

		def := "NO"
		if i == 0 {
			def = "YES"
		}

		_, err = fmt.Fprintf(fw, "#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"audio\",NAME=\"audio_%d\",DEFAULT=%s,URI=\"%s\"\n", i, def, uri)
		if err != nil {
			fn.log.With("error", err).Errorf("failed to write audio tracks for HLS master playlist: '%s'", filePath)
			return
		}
	}

	// write video tracks
	for _, trk := range hls.video.tracks {
		uri := path.Join("/", trk.trk.SwitchingSet.Name, trk.trk.Name, "index.m3u8")
		uri = uri[1:]

		resStr := ""
		res, err := trk.trk.Resolution()
		if err == nil {
			resStr = fmt.Sprintf("RESOLUTION=%s,", res)
		}

		_, err = fmt.Fprintf(fw, "#EXT-X-STREAM-INF:%sAUDIO=\"audio\"\n%s\n", resStr, uri)
		if err != nil {
			fn.log.With("error", err).Errorf("failed to write video tracks for HLS master playlist: '%s'", filePath)
			return
		}
	}

	// commit
	err = fw.Commit()
	if err != nil {
		fn.log.With("error", err).Errorf("failed to commit HLS master playlist: '%s'", filePath)
		return
	}

	// TODO: abort
}

func (fn *manifest) updateMediaPlaylist(hls *hlsManifest, hlsTrk *hlsTrack) {
	filePath := path.Join("/", fn.appPathPrefix, hls.pathPrefix, hlsTrk.trk.SwitchingSet.Name, hlsTrk.trk.Name, "index.m3u8")

	file, err := fn.vol.OpenCreate(filePath)
	if err != nil {
		fn.log.With("error", err).Errorf("failed to open HLS media playlist: '%s'", filePath)
		return
	}

	fw, err := file.AcquireWriter(false)
	if err != nil {
		fn.log.With("error", err).Errorf("failed to acquire writer for HLS media playlist: '%s'", filePath)
		return
	}

	// set target duration
	// assumption: all segments are of roughly equal duration
	targetDur := float64(1)
	if len(hlsTrk.segments) > 0 {
		targetDur = hlsTrk.segments[0].duration
	}

	// write header
	_, err = fmt.Fprintf(fw, "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:%f\n", targetDur)
	if err != nil {
		fn.log.With("error", err).Errorf("failed to write header for HLS media playlist: '%s'", filePath)
		return
	}

	// write init segment
	_, err = fmt.Fprintf(fw, "#EXT-X-MAP:URI=\"%s\"\n", hlsTrk.initSegment.path)
	if err != nil {
		fn.log.With("error", err).Errorf("failed to write header for HLS media playlist: '%s'", filePath)
		return
	}

	// write video tracks
	for _, seg := range hlsTrk.segments {
		_, err = fmt.Fprintf(fw, "#EXTINF:%f,\n%s\n", seg.duration, seg.path)
		if err != nil {
			fn.log.With("error", err).Errorf("failed to write segments for HLS media playlist: '%s'", filePath)
			return
		}
	}

	// commit
	err = fw.Commit()
	if err != nil {
		fn.log.With("error", err).Errorf("failed to commit HLS media playlist: '%s'", filePath)
		return
	}

	// TODO: abort
}
