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

package genericserve

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/nagare-media/ingest/pkg/app"
	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
	"github.com/nagare-media/ingest/pkg/event"
	"github.com/nagare-media/ingest/pkg/http"
	"github.com/nagare-media/ingest/pkg/http/router"
	"github.com/nagare-media/ingest/pkg/mime"
)

type genericServe struct {
	cfg         v1alpha1.App
	eventStream event.Stream
	ctx         context.Context
	execCtx     app.ExecCtx
}

var (
	DefaultConfig = v1alpha1.GenericServe{
		DefaultMIMEType:    mime.ApplicationOctetStream,
		UseXAccelHeader:    false,
		UseXSendfileHeader: false,
	}
)

func New(cfg v1alpha1.App) (app.App, error) {
	if err := app.CheckAndSetDefaults(&cfg); err != nil {
		return nil, err
	}
	if cfg.GenericServe.AppRef.Name == "" {
		return nil, errors.New("genericserve.New: appRef is missing")
	}
	if len(cfg.GenericServe.VolumesRef) == 0 {
		return nil, errors.New("genericserve.New: volumesRef is missing")
	}
	for _, vr := range cfg.GenericServe.VolumesRef {
		if vr.Name == "" {
			return nil, errors.New("genericserve.New: some volumesRef have no name")
		}
	}

	return &genericServe{
		cfg:         cfg,
		eventStream: event.NewStream(),
	}, nil
}

func (a *genericServe) Config() v1alpha1.App {
	return a.cfg
}

func (a *genericServe) HTTPConfig() *v1alpha1.HTTPApp {
	return a.cfg.HTTP
}

func (a *genericServe) EventStream() event.Stream {
	return a.eventStream
}

func (a *genericServe) SetCtx(ctx context.Context) {
	a.ctx = ctx
	a.eventStream.Start(ctx)
}

func (a *genericServe) SetExecCtx(execCtx app.ExecCtx) {
	a.execCtx = execCtx
}

func (a *genericServe) RegisterHTTPRoutes(router router.Router, handleOptions bool) error {
	router.Get("/+", a.handleGet)

	if handleOptions { // preflight requests
		router.Options("/+", http.NoContentHandler)
	}

	return nil
}

func (a *genericServe) handleGet(c *fiber.Ctx) error {
	log := a.execCtx.Logger()

	// determine file path
	httpPath := path.Join("/", c.Params("+")) // Join will also clean path
	prefixPath := path.Join(a.execCtx.PathPrefix(a.cfg.GenericServe.AppRef.Name), c.Hostname())
	prefixPath = string(http.PathIllegalCharsRegex.ReplaceAll([]byte(prefixPath), http.PathIllegalReplaceChar))
	filePath := path.Join(prefixPath, httpPath)
	fileExt := path.Ext(filePath)

	for _, vRef := range a.cfg.GenericServe.VolumesRef {
		vol, ok := a.execCtx.VolumeRegistry().Get(vRef.Name)
		if !ok {
			log.Errorf("volume named '%s' not found", vRef.Name)
			return fiber.ErrInternalServerError
		}

		file, err := vol.Open(filePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			log.With("error", err).Errorf("failed to open file '%s'", filePath)
			return fiber.ErrInternalServerError
		}

		fileReader, err := file.AcquireReader()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			log.With("error", err).Errorf("failed to acquire reader for file '%s'", filePath)
			return fiber.ErrInternalServerError
		}

		// found file; now look at the request more closely
		status := fiber.StatusOK              // set below
		bodyStreamer := io.Reader(fileReader) // set below

		// Last-Modified
		lastModified := fileReader.ModTime()
		c.Response().Header.SetLastModified(lastModified)

		// Content-Length
		var contentLength int
		select {
		case <-fileReader.WriteDone():
			s := fileReader.Size()
			// awkward overflow check for 32-bit systems
			// s. https://github.com/valyala/fasthttp/issues/1222
			contentLength = int(s)
			if s != int64(contentLength) {
				contentLength = -2
			}
		default:
			contentLength = -1
		}
		// header set below with SetBodyStream

		// ETag
		etag := ""
		if contentLength > 0 {
			etag = fmt.Sprintf("%x-%x", lastModified.UnixMilli(), contentLength)
			c.Response().Header.Set(fiber.HeaderETag, etag)
		}

		// Content-Range
		if contentLength >= 0 {
			reqRange := c.Get(fiber.HeaderRange)
			if reqRange != "" {
				startPos, endPos, err := fasthttp.ParseByteRange([]byte(reqRange), contentLength)
				if err != nil {
					return fiber.ErrRequestedRangeNotSatisfiable
				}

				seekPos, err := fileReader.Seek(int64(startPos), io.SeekStart)
				if int64(startPos) != seekPos {
					return fiber.ErrRequestedRangeNotSatisfiable
				}
				if err != nil {
					return fiber.ErrInternalServerError
				}

				contentLength = endPos - startPos + 1
				bodyStreamer = io.LimitReader(bodyStreamer, int64(contentLength))
				c.Response().Header.SetContentRange(startPos, endPos, contentLength)
				status = fiber.StatusPartialContent
			}
		}

		// Content-Type
		contentType := mime.PreferredTypeExt(fileExt)
		if contentType == "" {
			contentType = a.cfg.GenericServe.DefaultMIMEType
		}
		c.Response().Header.SetContentType(contentType)

		// Accept-Ranges
		if contentLength >= 0 {
			c.Response().Header.Set(fiber.HeaderAcceptRanges, "bytes")
		}

		// X-Content-Type-Options
		c.Response().Header.Set(fiber.HeaderXContentTypeOptions, "nosniff")

		// write body

		// let upstream proxy handle delivery of files
		if a.cfg.GenericServe.UseXAccelHeader || a.cfg.GenericServe.UseXSendfileHeader {
			_ = fileReader.Close()

			if a.cfg.GenericServe.UseXAccelHeader {
				c.Response().Header.Set("X-Accel-Redirect", file.Name())
			}
			if a.cfg.GenericServe.UseXSendfileHeader {
				c.Response().Header.Set("X-Sendfile", file.Name())
			}

			c.Status(status)
			return nil
		}

		// TODO: implement conditional requests
		//         * If-Match
		//         * If-None-Match
		//         * If-Modified-Since
		//         * If-Unmodified-Since
		//         * If-Range

		// TODO: add user defined headers

		// flush header when doing chunk transfere encoding
		c.Response().ImmediateHeaderFlush = (contentLength < 0)
		c.Response().SetBodyStream(bodyStreamer, contentLength)
		c.Status(status)

		// TODO: fileReader might return os.ErrNotExist. Maybe implement custom write loop?
		return nil
	}

	return fiber.ErrNotFound
}
