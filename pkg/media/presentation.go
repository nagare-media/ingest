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

package media

import (
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	mp4ff "github.com/edgeware/mp4ff/mp4"
)

var (
	StreamNameRegex       = regexp.MustCompile(`^[a-zA-Z0-9-_]+$`)
	SwitchingSetNameRegex = regexp.MustCompile(`^[a-zA-Z0-9-_]+$`)
	TrackNameRegex        = regexp.MustCompile("^[^,;:?'\"[\\]{}@*\\\\&#%`^+<=>|~$\\x00-\\x1F\\x7F\\t\\n\\f\\r ]+$")
)

type Presentation struct {
	sync.RWMutex

	Name           string
	LastModifiedAt time.Time
	// CMAF defines selection sets as the next hierarchy for grouping switching sets. A mapping to AdaptationSet@group in
	// MPEG-DASH is proposed. The DASH-IF ingest specification ignores selection sets (may be used transparently in
	// interface 2) so we do the same for now.
	SwitchingSets map[string]*SwitchingSet
}

func (s *Presentation) AddSwitchingSet(set *SwitchingSet) {
	if s.SwitchingSets == nil {
		s.SwitchingSets = make(map[string]*SwitchingSet)
	}
	set.Presentation = s
	s.SwitchingSets[set.Name] = set
}

type SwitchingSet struct {
	Presentation *Presentation
	Name         string
	Tracks       map[string]*Track
}

func (set *SwitchingSet) RLock()   { set.Presentation.RLock() }
func (set *SwitchingSet) RUnlock() { set.Presentation.RUnlock() }
func (set *SwitchingSet) Lock()    { set.Presentation.Lock() }
func (set *SwitchingSet) Unlock()  { set.Presentation.Unlock() }

func (set *SwitchingSet) AddTrack(t *Track) {
	if set.Tracks == nil {
		set.Tracks = make(map[string]*Track)
	}
	t.SwitchingSet = set
	set.Tracks[t.Name] = t
}

type Track struct {
	SwitchingSet       *SwitchingSet
	Name               string
	CMAFHeader         *mp4ff.InitSegment
	Format             Format
	Initialized        bool
	LastSequenceNumber uint32
}

func (t *Track) RLock()   { t.SwitchingSet.RLock() }
func (t *Track) RUnlock() { t.SwitchingSet.RUnlock() }
func (t *Track) Lock()    { t.SwitchingSet.Lock() }
func (t *Track) Unlock()  { t.SwitchingSet.Unlock() }

func (t *Track) Timescale() (uint32, error) {
	if moov := t.CMAFHeader.Moov; moov != nil {
		if trak := moov.Trak; trak != nil {
			if mdia := trak.Mdia; mdia != nil {
				if mdhd := mdia.Mdhd; mdhd != nil {
					return mdhd.Timescale, nil
				}
			}
		}
	}

	return 0, errors.New("could not find mdhd box")
}

func (t *Track) Resolution() (string, error) {
	if moov := t.CMAFHeader.Moov; moov != nil {
		if trak := moov.Trak; trak != nil {
			if tkhd := trak.Tkhd; tkhd != nil {
				width := tkhd.Width >> 16
				height := tkhd.Height >> 16
				return fmt.Sprintf("%dx%d", width, height), nil
			}
		}
	}

	return "", errors.New("could not find tkhd box")
}
