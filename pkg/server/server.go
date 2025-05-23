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

package server

import (
	"context"
	"errors"
	"regexp"

	"go.uber.org/zap"

	"github.com/nagare-media/ingest/pkg/app"
	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
)

var NameRegex = regexp.MustCompile(`^[a-zA-Z0-9-_]+$`)

type Server interface {
	Config() v1alpha1.Server
	Register(execCtx ExecCtx, app app.App) error
	Listen(ctx context.Context, execCtx ExecCtx) error
}

type ExecCtx interface {
	Logger() *zap.SugaredLogger
}

func CheckAndSetDefaults(cfg *v1alpha1.Server) error {
	if !NameRegex.MatchString(cfg.Name) {
		return errors.New("server: Name invalid")
	}
	if cfg.Address == "" {
		return errors.New("server: Address not set")
	}
	return nil
}
