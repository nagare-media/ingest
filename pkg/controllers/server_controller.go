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

package controllers

import (
	"context"
	"errors"
	"fmt"

	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
	"github.com/nagare-media/ingest/pkg/server"
	"github.com/nagare-media/ingest/pkg/server/http"
)

type serverController struct {
	server         server.Server
	appControllers []*appController
}

func NewServerController(cfg v1alpha1.Server) (*serverController, error) {
	// create server
	srv, err := newServer(cfg)
	if err != nil {
		return nil, err
	}

	// create app controllers
	appCtrl := make([]*appController, len(cfg.Apps))
	appNameExists := make(map[string]bool)
	for i, appCfg := range cfg.Apps {
		name := appCfg.Name
		if appNameExists[name] {
			return nil, fmt.Errorf("NewServerController: multiple apps with the same name '%s' configured", name)
		}

		ctrl, err := NewAppController(appCfg)
		if err != nil {
			return nil, err
		}
		appCtrl[i] = ctrl
		appNameExists[name] = true
	}

	return &serverController{
		server:         srv,
		appControllers: appCtrl,
	}, nil
}

func (c *serverController) Server() server.Server {
	return c.server
}

func (c *serverController) Exec(ctx context.Context, execCtx *ExecCtx) error {
	log := execCtx.Logger().
		Named(c.server.Config().Name).
		With("server", c.server.Config().Name)
	execCtx = execCtx.
		WithServerCtrl(c).
		WithLogger(log)

	log.Info("start server controller")
	if len(c.appControllers) == 0 {
		log.Warn("no apps configured; nothing to do")
		return nil
	}

	log.Info("register apps")
	for _, appCtrl := range c.appControllers {
		err := c.server.Register(execCtx, appCtrl.App())
		if err != nil {
			return err
		}
	}

	log.Info("start sub-controllers")
	ctx, cancel := context.WithCancel(ctx)
	subControllerGroup := NewGroupController(GroupControllerOpts{})
	for _, appCtrl := range c.appControllers {
		subControllerGroup.Add(appCtrl)
	}
	subControllerGroup.Add(ControllerFunc(func(ctx context.Context, execCtx *ExecCtx) error {
		// if server returns cancel context for all apps
		defer cancel()

		err := c.server.Listen(ctx, execCtx)
		if err != nil {
			log.Errorw("server failed unexpectedly; stop all apps", "error", err)
		}

		return err
	}))
	err := subControllerGroup.Exec(ctx, execCtx)

	log.Info("shutdown server controller")
	return err
}

func newServer(cfg v1alpha1.Server) (server.Server, error) {
	configuredTypes := make([]string, 0, 1)
	var createFunc func(cfg v1alpha1.Server) (server.Server, error)

	if cfg.HTTP != nil {
		configuredTypes = append(configuredTypes, "http")
		createFunc = http.New
	}
	if len(configuredTypes) == 0 {
		return nil, errors.New("newServer: no server type configured")
	} else if len(configuredTypes) > 1 {
		return nil, fmt.Errorf("newServer: multiple server types configured: %s", configuredTypes)
	}

	return createFunc(cfg)
}
