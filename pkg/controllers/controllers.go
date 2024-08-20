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

package controllers

import (
	"context"
	"path"

	"go.uber.org/zap"

	"github.com/nagare-media/ingest/pkg/app"
	"github.com/nagare-media/ingest/pkg/function"
	"github.com/nagare-media/ingest/pkg/server"
	"github.com/nagare-media/ingest/pkg/volume"
)

type Controller interface {
	Exec(ctx context.Context, execCtx *ExecCtx) error
}

type ControllerFunc func(ctx context.Context, execCtx *ExecCtx) error

func (f ControllerFunc) Exec(ctx context.Context, execCtx *ExecCtx) error {
	return f(ctx, execCtx)
}

type ExecCtx struct {
	log          *zap.SugaredLogger
	ingestCtrl   *ingestController
	serverCtrl   *serverController
	appCtrl      *appController
	functionCtrl *functionController
}

func (c *ExecCtx) Logger() *zap.SugaredLogger        { return c.log }
func (c *ExecCtx) IngestCtrl() *ingestController     { return c.ingestCtrl }
func (c *ExecCtx) ServerCtrl() *serverController     { return c.serverCtrl }
func (c *ExecCtx) AppCtrl() *appController           { return c.appCtrl }
func (c *ExecCtx) FunctionCtrl() *functionController { return c.functionCtrl }

func (c *ExecCtx) Server() server.Server {
	if c.serverCtrl != nil {
		return c.serverCtrl.Server()
	}
	return nil
}

func (c *ExecCtx) App() app.App {
	if c.appCtrl != nil {
		return c.appCtrl.App()
	}
	return nil
}

func (c *ExecCtx) Function() function.Function {
	if c.functionCtrl != nil {
		return c.functionCtrl.Function()
	}
	return nil
}

func (c *ExecCtx) PathPrefix(override ...string) string {
	seg := make([]string, 0, 2)

	if c.serverCtrl != nil {
		seg = append(seg, c.serverCtrl.server.Config().Name)
	}
	if c.appCtrl != nil {
		seg = append(seg, c.appCtrl.app.Config().Name)
	}
	if len(override) > 0 {
		if len(seg) < len(override) {
			seg = override
		} else {
			copy(seg[len(seg)-len(override):], override)
		}
	}
	return path.Clean("/" + path.Join(seg...))
}

func (c *ExecCtx) VolumeRegistry() volume.Registry {
	return &volumeRegistry{
		ingestCtrl: c.ingestCtrl,
	}
}

func (c *ExecCtx) WithLogger(l *zap.SugaredLogger) *ExecCtx {
	copy := *c
	copy.log = l
	return &copy
}

func (c *ExecCtx) WithIngestCtrl(ingestCtrl *ingestController) *ExecCtx {
	copy := *c
	copy.ingestCtrl = ingestCtrl
	return &copy
}

func (c *ExecCtx) WithServerCtrl(serverCtrl *serverController) *ExecCtx {
	copy := *c
	copy.serverCtrl = serverCtrl
	return &copy
}

func (c *ExecCtx) WithAppCtrl(appCtrl *appController) *ExecCtx {
	copy := *c
	copy.appCtrl = appCtrl
	return &copy
}

func (c *ExecCtx) WithFunctionCtrl(functionCtrl *functionController) *ExecCtx {
	copy := *c
	copy.functionCtrl = functionCtrl
	return &copy
}

type volumeRegistry struct {
	ingestCtrl *ingestController
}

func (reg *volumeRegistry) Get(name string) (volume.Volume, bool) {
	if reg.ingestCtrl == nil {
		return nil, false
	}
	vol, ok := reg.ingestCtrl.volumes[name]
	return vol, ok
}
