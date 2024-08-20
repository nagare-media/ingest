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
	"github.com/nagare-media/ingest/pkg/volume"
	"github.com/nagare-media/ingest/pkg/volume/fs"
	"github.com/nagare-media/ingest/pkg/volume/mem"
	"github.com/nagare-media/ingest/pkg/volume/null"
)

type ingestController struct {
	volumes           map[string]volume.Volume
	serverControllers []Controller
}

func NewIngestController(cfg v1alpha1.Config) (*ingestController, error) {
	// create volumes
	volumes := make(map[string]volume.Volume)
	for _, volCfg := range cfg.Volumes {
		name := volCfg.Name
		if _, ok := volumes[name]; ok {
			return nil, fmt.Errorf("NewIngestController: multiple volumes with the same name '%s' configured", name)
		}

		vol, err := newVolume(volCfg)
		if err != nil {
			return nil, err
		}
		volumes[name] = vol
	}

	// create server controllers
	serverCtrl := make([]Controller, len(cfg.Servers))
	serverNameExists := make(map[string]bool)
	for i, srvCfg := range cfg.Servers {
		name := srvCfg.Name
		if serverNameExists[name] {
			return nil, fmt.Errorf("NewIngestController: multiple server with the same name '%s' configured", name)
		}

		ctrl, err := NewServerController(srvCfg)
		if err != nil {
			return nil, err
		}
		serverCtrl[i] = ctrl
		serverNameExists[name] = true
	}

	return &ingestController{
		volumes:           volumes,
		serverControllers: serverCtrl,
	}, nil
}

func (c *ingestController) Exec(ctx context.Context, execCtx *ExecCtx) error {
	log := execCtx.Logger().Named("ingest")
	execCtx = execCtx.
		WithIngestCtrl(c).
		WithLogger(log)

	log.Info("start ingest controller")
	if len(c.serverControllers) == 0 {
		log.Warn("no server configured; nothing to do")
		return nil
	}

	log.Info("initialize volumes")
	for _, vol := range c.volumes {
		err := vol.Init(execCtx)
		if err != nil {
			log.Errorw("ingestController: initializing volume failed", "error", err)
			return err
		}
	}

	log.Info("start sub-controllers")
	subControllerGroup := NewGroupController(GroupControllerOpts{}, c.serverControllers...)
	err := subControllerGroup.Exec(ctx, execCtx)

	log.Info("deinitialize volumes")
	for _, vol := range c.volumes {
		errvol := vol.Deinit(execCtx)
		if errvol != nil {
			log.Errorw("ingestController: deinitializing volume failed", "error", errvol)
			err = errvol
		}
	}

	log.Info("shutdown ingest controller")
	return err
}

func newVolume(cfg v1alpha1.Volume) (volume.Volume, error) {
	configuredTypes := make([]string, 0, 1)
	var createFunc func(cfg v1alpha1.Volume) (volume.Volume, error)

	if cfg.Null != nil {
		configuredTypes = append(configuredTypes, "null")
		createFunc = null.New
	}
	if cfg.Memory != nil {
		configuredTypes = append(configuredTypes, "mem")
		createFunc = mem.New
	}
	if cfg.FileSystem != nil {
		configuredTypes = append(configuredTypes, "fs")
		createFunc = fs.New
	}
	if len(configuredTypes) == 0 {
		return nil, errors.New("newVolume: no volume type configured")
	} else if len(configuredTypes) > 1 {
		return nil, fmt.Errorf("newVolume: multiple volume types configured: %s", configuredTypes)
	}

	return createFunc(cfg)
}
