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

package event

import (
	"context"
	"sync"
)

const (
	// TODO: make this configurable in the application
	DefaultBufferLen = 32
)

type Stream interface {
	Start(ctx context.Context)
	Pub(e Event)
	Sub() <-chan Event
	SubBuf(n int) <-chan Event
	Desub(ch chan Event)
}

type channelStream struct {
	mtx sync.RWMutex

	recvCh chan Event
	subs   []chan Event
}

func NewStream() Stream {
	return NewStreamBuf(DefaultBufferLen)
}

func NewStreamBuf(n int) Stream {
	return &channelStream{
		recvCh: make(chan Event, n),
		subs:   make([]chan Event, 0),
	}
}

func (cs *channelStream) Pub(e Event) {
	cs.recvCh <- e
}

func (cs *channelStream) Sub() <-chan Event {
	return cs.SubBuf(DefaultBufferLen)
}

func (cs *channelStream) SubBuf(n int) <-chan Event {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	ch := make(chan Event, n)
	cs.subs = append(cs.subs, ch)
	return ch
}

func (cs *channelStream) Desub(ch chan Event) {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	defer close(ch)

	for i, subCh := range cs.subs {
		if ch == subCh {
			if i == len(cs.subs)-1 {
				cs.subs[i] = nil
				cs.subs = cs.subs[:i]
			} else {
				cs.subs[i] = cs.subs[len(cs.subs)-1]
				cs.subs[len(cs.subs)-1] = nil
				cs.subs = cs.subs[:len(cs.subs)-1]
			}
			return
		}
	}
}

func (cs *channelStream) Start(ctx context.Context) {
	go cs.start(ctx)
}

func (cs *channelStream) start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			cs.clean()
			return
		case e := <-cs.recvCh:
			go cs.emit(e)
		}
	}
}

func (cs *channelStream) emit(e Event) {
	cs.mtx.RLock()
	defer cs.mtx.RUnlock()
	for _, ch := range cs.subs {
		ch <- e
	}
}

func (cs *channelStream) clean() {
	cs.mtx.Lock()
	defer cs.mtx.Unlock()
	for _, ch := range cs.subs {
		close(ch)
	}
	close(cs.recvCh)
}
