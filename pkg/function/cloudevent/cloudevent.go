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

package cloudevent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/nagare-media/ingest/internal/uuid"
	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
	"github.com/nagare-media/ingest/pkg/event"
	"github.com/nagare-media/ingest/pkg/function"
)

type cloudEvent struct {
	cfg v1alpha1.Function
}

func New(cfg v1alpha1.Function) (function.Function, error) {
	if err := function.CheckAndSetDefaults(&cfg); err != nil {
		return nil, err
	}

	return &cloudEvent{cfg: cfg}, nil
}

func (fn *cloudEvent) Config() v1alpha1.Function {
	return fn.cfg
}

func (fn *cloudEvent) Exec(ctx context.Context, execCtx function.ExecCtx) error {
	log := execCtx.Logger()

	if _, err := url.Parse(fn.cfg.CloudEvent.URL); err != nil {
		return fmt.Errorf("failed to parse cloud event URL: %w", err)
	}

	eventStream := execCtx.App().EventStream().Sub()
	defer execCtx.App().EventStream().Desub(eventStream)

	for {
		select {
		case <-ctx.Done():
			return nil

		case e := <-eventStream:
			if err := fn.sendEvent(ctx, e); err != nil {
				log.With("error", err).Error("sending cloud event failed")
			}
		}
	}
}

func (fn *cloudEvent) sendEvent(ctx context.Context, e event.Event) error {
	ce := cloudevents.NewEvent()
	ce.SetID(uuid.UUIDv4())
	ce.SetTime(time.Now())
	ce.SetSource("/ingest.nagare.media/function/cloudevent")
	ce.SetType(fmt.Sprintf("media.nagare.ingest.%T", e))
	// TODO: better support for cloud event subject
	err := ce.SetData("application/json", e)
	if err != nil {
		return err
	}

	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	if err := enc.Encode(ce); err != nil {
		return err
	}

	// TODO: make timeout configurable
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// TODO: implement retries with exp backoff?
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fn.cfg.CloudEvent.URL, buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/cloudevents+json")

	// TODO: allow to configure the client?
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode > 299 {
		return fmt.Errorf("unexpected HTTP status code in response: %d", resp.StatusCode)
	}

	return nil
}
