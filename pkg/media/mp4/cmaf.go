/*
Copyright 2022 The nagare media authors

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
	mp4ff "github.com/edgeware/mp4ff/mp4"
)

func CheckMoovCMAF(moov *mp4ff.MoovBox) error {
	// TODO: This only implements some quick checks. Should we further check for CMAF compliance or trust the source?
	// TODO: nil checks should also check len == 1
	if moov.Mvhd == nil {
		return ErrNotACMAFHeader
	}
	if len(moov.Traks) != 1 {
		return ErrNotACMAFHeader
	}
	if moov.Mvex == nil {
		return ErrNotACMAFHeader
	}
	return nil
}

func CheckMoofCMAF(moof *mp4ff.MoofBox) error {
	// TODO: This only implements some quick checks. Should we further check for CMAF compliance or trust the source?
	if len(moof.Children) != 2 {
		return ErrNotACMAFChunk
	}
	if moof.Mfhd == nil {
		return ErrNotACMAFChunk
	}
	if len(moof.Trafs) != 1 {
		return ErrNotACMAFChunk
	}
	return CheckTrafCMAF(moof.Traf)
}

func CheckTrafCMAF(traf *mp4ff.TrafBox) error {
	// TODO: This only implements some quick checks. Should we further check for CMAF compliance or trust the source?
	if traf.Tfhd == nil {
		return ErrNotACMAFChunk
	}
	if traf.Tfdt == nil {
		return ErrNotACMAFChunk
	}
	if traf.Trun == nil {
		return ErrNotACMAFChunk
	}
	return nil
}

type Chunk struct {
	Styp *mp4ff.StypBox
	Emsg []*mp4ff.EmsgBox
	Prft *mp4ff.PrftBox
	Moof *mp4ff.MoofBox
	// we don't hold Mdat
}

func (c *Chunk) Duration() uint64 {
	total := uint64(0)

	if traf := c.Moof.Traf; traf != nil {
		if trun := traf.Trun; trun != nil {
			if tfhd := traf.Tfhd; tfhd != nil {

				// default sample duration
				if tfhd.HasDefaultSampleDuration() && !trun.HasSampleDuration() {
					return uint64(tfhd.DefaultSampleDuration) * uint64(trun.SampleCount())
				}

				// add duration of all samples
				for _, s := range trun.Samples {
					total += uint64(s.Dur)
				}
			}
		}
	}

	return total
}

type Fragment struct {
	Chunks []*Chunk
}

func (f *Fragment) Duration() uint64 {
	total := uint64(0)
	for _, c := range f.Chunks {
		total += c.Duration()
	}
	return total
}
