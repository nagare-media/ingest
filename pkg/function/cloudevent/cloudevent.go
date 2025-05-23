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
	"context"

	"github.com/nagare-media/ingest/pkg/config/v1alpha1"
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
	return nil
}
