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
	"fmt"
	"sync"
	"sync/atomic"
)

type groupController[T Controller] struct {
	controllers []T
	opts        GroupControllerOpts
}

type GroupControllerOpts struct {
	StopAllOnError bool
}

func NewGroupController[T Controller](opts GroupControllerOpts, ctrl ...T) *groupController[T] {
	if ctrl == nil {
		ctrl = make([]T, 0)
	}

	return &groupController[T]{
		controllers: ctrl,
		opts:        opts,
	}
}

func (c *groupController[T]) IsZero() bool {
	return c.IsEmpty()
}

func (c *groupController[T]) IsEmpty() bool {
	return len(c.controllers) == 0
}

func (c *groupController[T]) Add(ctrl T) {
	c.controllers = append(c.controllers, ctrl)
}

func (c *groupController[T]) Exec(ctx context.Context, execCtx *ExecCtx) error {
	log := execCtx.Logger()
	errCount := int32(0)
	wg := sync.WaitGroup{}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, ctrl := range c.controllers {
		ctrl := ctrl
		wg.Go(func() {
			err := ctrl.Exec(ctx, execCtx)
			if err != nil {
				atomic.AddInt32(&errCount, 1)
				if c.opts.StopAllOnError {
					log.Errorw("sub-controller failed unexpectedly; signal all to stop", "error", err)
					cancel()
				} else {
					log.Errorw("sub-controller failed unexpectedly; keep others running", "error", err)
				}
			}
		})
	}

	wg.Wait()

	if errCount > 0 {
		return fmt.Errorf("groupController.Exec: %d controller(s) failed", errCount)
	}

	return nil
}
