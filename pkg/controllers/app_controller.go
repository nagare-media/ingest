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

	"github.com/nagare-media/ingest/pkg/app"
	"github.com/nagare-media/ingest/pkg/app/cmafingest"
	"github.com/nagare-media/ingest/pkg/app/dashandhlsingest"
	"github.com/nagare-media/ingest/pkg/app/genericserve"
	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
)

type AppController interface {
	Controller
	App() app.App
}

type appController struct {
	app                 app.App
	functionControllers []FunctionController
}

var _ AppController = &appController{}

func NewAppController(cfg v1alpha1.App) (AppController, error) {
	// create app
	app, err := newApp(cfg)
	if err != nil {
		return nil, err
	}

	// create function controllers
	functionCtrl := make([]FunctionController, len(cfg.Functions))
	functionNameExists := make(map[string]bool)
	for i, functionCfg := range cfg.Functions {
		name := functionCfg.Name
		if functionNameExists[name] {
			return nil, fmt.Errorf("NewAppController: multiple functions with the same name '%s' configured", name)
		}

		ctrl, err := NewFunctionController(functionCfg)
		if err != nil {
			return nil, err
		}
		functionCtrl[i] = ctrl
		functionNameExists[name] = true
	}

	return &appController{
		app:                 app,
		functionControllers: functionCtrl,
	}, nil
}

func (c *appController) App() app.App {
	return c.app
}

func (c *appController) Exec(ctx context.Context, execCtx *ExecCtx) error {
	log := execCtx.Logger().
		Named(c.app.Config().Name).
		With("app", c.app.Config().Name)
	execCtx = execCtx.
		WithAppCtrl(c).
		WithLogger(log)

	log.Info("start app controller")
	c.app.SetCtx(ctx)
	c.app.SetExecCtx(execCtx)
	if len(c.functionControllers) == 0 {
		log.Warn("no functions configured; nothing to do")
		return nil
	}

	// create new context to allow signaling to the app that no requests should be handled once functions fail
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c.app.SetCtx(ctx)

	log.Info("start sub-controllers")
	opts := GroupControllerOpts{
		// functions could depend upon each other => stop all if one dies
		StopAllOnError: true,
	}
	subControllerGroup := NewGroupController(opts, c.functionControllers...)
	err := subControllerGroup.Exec(ctx, execCtx)

	log.Info("shutdown app controller")
	return err
}

func newApp(cfg v1alpha1.App) (app.App, error) {
	configuredTypes := make([]string, 0, 1)
	var createFunc func(cfg v1alpha1.App) (app.App, error)

	if cfg.GenericServe != nil {
		configuredTypes = append(configuredTypes, "genericServe")
		createFunc = genericserve.New
	}
	if cfg.CMAFIngest != nil {
		configuredTypes = append(configuredTypes, "cmafIngest")
		createFunc = cmafingest.New
	}
	if cfg.DASHAndHLSIngest != nil {
		configuredTypes = append(configuredTypes, "dashAndHlsIngest")
		createFunc = dashandhlsingest.New
	}
	if len(configuredTypes) == 0 {
		return nil, errors.New("newApp: no app type configured")
	} else if len(configuredTypes) > 1 {
		return nil, fmt.Errorf("newApp: multiple app types configured: %s", configuredTypes)
	}

	return createFunc(cfg)
}
