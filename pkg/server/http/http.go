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

package http

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/basicauth"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"go.uber.org/zap"

	"github.com/nagare-media/ingest/internal/uuid"
	"github.com/nagare-media/ingest/pkg/app"
	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
	"github.com/nagare-media/ingest/pkg/http"
	"github.com/nagare-media/ingest/pkg/http/router"
	"github.com/nagare-media/ingest/pkg/server"
)

const (
	DefaultHTTPNetwork     = "tcp"
	DefaultHTTPIdleTimeout = 75 * time.Second
)

type httpSrv struct {
	cfg     v1alpha1.Server
	router  router.RootRouter
	execCtx server.ExecCtx
}

type HTTPRegistrable interface {
	RegisterHTTPRoutes(router router.Router, handleOptions bool) error
	HTTPConfig() *v1alpha1.HTTPApp
}

func New(cfg v1alpha1.Server) (server.Server, error) {
	if err := server.CheckAndSetDefaults(&cfg); err != nil {
		return nil, err
	}
	if cfg.Network == "" {
		cfg.Network = DefaultHTTPNetwork
	}
	if cfg.HTTP.IdleTimeout <= 0 {
		cfg.HTTP.IdleTimeout = DefaultHTTPIdleTimeout
	}

	// create HTTP server
	fiberApp := fiber.New(fiber.Config{
		ServerHeader:          "nagare media ingest",
		AppName:               cfg.Name,
		DisableStartupMessage: true,
		Network:               cfg.Network,
		IdleTimeout:           cfg.HTTP.IdleTimeout,
		StreamRequestBody:     true,
	})
	router := router.New(fiberApp)
	httpSrv := &httpSrv{
		cfg:    cfg,
		router: router,
	}

	// add global HTTP middlewares

	// telemetry middleware
	router.Use(func(c *fiber.Ctx) error {
		if http.NextIfInInternalRedirect(c) {
			return c.Next()
		}

		responseStart := time.Now()
		err := c.Next()
		responseTime := time.Since(responseStart)

		// logging
		if httpSrv.execCtx.Logger().Desugar().Core().Enabled(zap.DebugLevel) {
			// request
			remoteAddr := c.Context().RemoteAddr()
			hostname := c.Hostname()
			method := c.Method()
			path, ok := c.Locals(http.OriginalPathKey).(string)
			if !ok {
				path = c.Path()
			}
			referer := c.Request().Header.Referer()
			userAgent := string(c.Request().Header.UserAgent())

			// response
			status := c.Response().StatusCode()
			if err != nil {
				if e, ok := err.(*fiber.Error); ok {
					status = e.Code
				}
			}

			// both
			requestID, _ := c.Locals(http.RequestIDKey).(string)

			httpSrv.execCtx.Logger().Debugw(
				fmt.Sprintf("%d %s %s%s", status, method, hostname, path),

				// request
				"remoteAddr", remoteAddr,
				"hostname", hostname,
				"method", method,
				"path", path,
				"referer", referer,
				"userAgent", userAgent,

				// response
				"status", status,
				"responseTime", responseTime,

				// both
				"requestID", requestID,
			)
		}
		// TODO: add HTTP metrics
		// TODO: add HTTP tracing
		return err
	})

	// request ID
	router.Use(requestid.New(
		requestid.Config{
			Next:       http.NextIfInInternalRedirect,
			ContextKey: http.RequestIDKey,
			Generator:  uuid.UUIDv4,
		},
	))

	return httpSrv, nil
}

func (s *httpSrv) Config() v1alpha1.Server {
	return s.cfg
}

func (s *httpSrv) Register(execCtx server.ExecCtx, app app.App) error {
	log := execCtx.Logger()

	// check app
	httpApp, ok := app.(HTTPRegistrable)
	if !ok {
		return errors.New("http.Register: cannot register non HTTP app")
	}

	// check config and set defaults
	cfg := httpApp.HTTPConfig()
	if cfg.Host == "" {
		log.Warn("HTTP Host not set; using '*'")
		cfg.Host = "*"
	}

	if cfg.Path == "" {
		log.Warn("HTTP Path not set; using '/'")
		cfg.Host = "/"
	}

	// add middlewares
	middlewares := make([]fiber.Handler, 0)

	// auth middleware
	if cfg.Auth != nil {
		if cfg.Auth.Basic != nil {
			users := make(map[string]string)
			for _, u := range cfg.Auth.Basic.Users {
				users[u.Name] = u.Password
			}
			middlewares = append(middlewares, basicauth.New(basicauth.Config{
				Users: users,
			}))
		}
	}

	// TODO: add Auth middleware for digest, tls

	// CORS middleware
	if cfg.CORS != nil {
		middlewares = append(middlewares, cors.New(cors.Config{
			AllowOrigins:     cfg.CORS.AllowOrigins,
			AllowMethods:     cfg.CORS.AllowMethods,
			AllowHeaders:     cfg.CORS.AllowHeaders,
			AllowCredentials: cfg.CORS.AllowCredentials,
			ExposeHeaders:    cfg.CORS.ExposeHeaders,
			MaxAge:           cfg.CORS.MaxAge,
		}))
	}

	// add routes
	router := s.router.Host(cfg.Host).Group(cfg.Path, middlewares...)
	return httpApp.RegisterHTTPRoutes(router, cfg.CORS != nil)
}

func (s *httpSrv) Listen(ctx context.Context, execCtx server.ExecCtx) error {
	s.execCtx = execCtx
	log := execCtx.Logger()

	s.router.Register()
	s.router.Use(func(c *fiber.Ctx) error {
		// catch-all handler
		switch c.Method() {
		// GET, HEAD, OPTIONS => 404 Not Found
		case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions:
			return fiber.ErrNotFound

		// POST, PUT, PATCH, DELETE, CONNECT, TRACE => 403 Forbidden
		default:
			return fiber.ErrForbidden
		}
	})

	var err error
	listenDone := make(chan struct{})
	go func() {
		log.Infof("start listening on %s %s", s.cfg.Network, s.cfg.Address)
		// TODO: add support for TLS
		err = s.router.FiberApp().Listen(s.cfg.Address)
		close(listenDone)
	}()

	select {
	case <-listenDone:
		// stopped listening, but ctx is still running
		log.Errorw(fmt.Sprintf("unexpectedly stopped listening on %s %s:", s.cfg.Network, s.cfg.Address), "error", err)
		return err
	case <-ctx.Done():
		// graceful shutdown
		log.Infof("stopped listening on %s %s", s.cfg.Network, s.cfg.Address)
		return s.router.FiberApp().Shutdown()
	}
}
