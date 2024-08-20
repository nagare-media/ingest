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

package function

import (
	"context"
	"errors"
	"regexp"

	"go.uber.org/zap"

	"github.com/nagare-media/ingest/pkg/app"
	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
	"github.com/nagare-media/ingest/pkg/volume"
)

var NameRegex = regexp.MustCompile(`^[a-zA-Z0-9-_]+$`)

type Function interface {
	Config() v1alpha1.Function
	Exec(ctx context.Context, execCtx ExecCtx) error
}

type ExecCtx interface {
	Logger() *zap.SugaredLogger
	PathPrefix(override ...string) string
	VolumeRegistry() volume.Registry
	App() app.App
}

func CheckAndSetDefaults(cfg *v1alpha1.Function) error {
	if !NameRegex.Match([]byte(cfg.Name)) {
		return errors.New("function: Name invalid")
	}
	return nil
}
