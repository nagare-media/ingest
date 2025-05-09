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

package mp4

import (
	"errors"

	mp4ff "github.com/Eyevinn/mp4ff/mp4"

	"github.com/nagare-media/ingest/pkg/media"
)

func FirstSampleFlags(track *media.Track, moof *mp4ff.MoofBox) (uint32, error) {
	// look at fragment header
	if traf := moof.Traf; traf != nil {
		if trun := traf.Trun; trun != nil {
			if flags, ok := trun.FirstSampleFlags(); ok {
				return flags, nil
			}
			if trun.HasSampleFlags() {
				if len(trun.Samples) == 0 {
					return 0, errors.New("trun with no samples")
				}
				return trun.Samples[0].Flags, nil
			}
		}

		if tfhd := traf.Tfhd; tfhd != nil {
			if tfhd.HasDefaultSampleFlags() {
				return tfhd.DefaultSampleFlags, nil
			}
		}
	}

	// fallback to movie header
	track.RLock()
	defer track.RUnlock()
	if track.Initialized && track.CMAFHeader != nil {
		if moov := track.CMAFHeader.Moov; moov != nil {
			if mvex := moov.Mvex; mvex != nil {
				if trex := mvex.Trex; trex != nil {
					// TODO: check if DefaultSampleFlags is set
					// return trex.DefaultSampleFlags
					_ = trex
				}
			}
		}
	}

	return 0, errors.New("could not read segment flags")
}
