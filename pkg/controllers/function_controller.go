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
	"errors"
	"fmt"

	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
	"github.com/nagare-media/ingest/pkg/function"
	"github.com/nagare-media/ingest/pkg/function/cleanup"
	"github.com/nagare-media/ingest/pkg/function/cloudevent"
	"github.com/nagare-media/ingest/pkg/function/copy"
	"github.com/nagare-media/ingest/pkg/function/manifest"
)

type functionController struct {
	function function.Function
}

func NewFunctionController(cfg v1alpha1.Function) (*functionController, error) {
	// create function
	function, err := newFunction(cfg)
	if err != nil {
		return nil, err
	}

	return &functionController{
		function: function,
	}, nil
}

func (c *functionController) Function() function.Function {
	return c.function
}

func (c *functionController) Exec(ctx context.Context, execCtx *ExecCtx) error {
	log := execCtx.Logger().
		Named(c.function.Config().Name).
		With("function", c.function.Config().Name)
	execCtx = execCtx.
		WithFunctionCtrl(c).
		WithLogger(log)

	log.Info("start function controller")
	err := c.function.Exec(ctx, execCtx)

	log.Info("shutdown function controller")
	return err
}

func newFunction(cfg v1alpha1.Function) (function.Function, error) {
	configuredTypes := make([]string, 0, 1)
	var createFunc func(cfg v1alpha1.Function) (function.Function, error)
	if cfg.Copy != nil {
		configuredTypes = append(configuredTypes, "copy")
		createFunc = copy.New
	}
	if cfg.Cleanup != nil {
		configuredTypes = append(configuredTypes, "cleanup")
		createFunc = cleanup.New
	}
	if cfg.Manifest != nil {
		configuredTypes = append(configuredTypes, "manifest")
		createFunc = manifest.New
	}
	if cfg.CloudEvent != nil {
		configuredTypes = append(configuredTypes, "cloudEvent")
		createFunc = cloudevent.New
	}
	if len(configuredTypes) == 0 {
		return nil, errors.New("newFunction: no function type configured")
	} else if len(configuredTypes) > 1 {
		return nil, fmt.Errorf("newFunction: multiple function types configured: %s", configuredTypes)
	}

	return createFunc(cfg)
}
