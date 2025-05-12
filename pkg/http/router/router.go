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

package router

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/utils"
)

const (
	SystemRoutesPrefix   = "/sys"
	InternalRoutesPrefix = SystemRoutesPrefix + "/internal"
	HostRoutesPrefix     = InternalRoutesPrefix + "/hosts"
)

type Router interface {
	Use(args ...any) Router

	Get(path string, handlers ...fiber.Handler) Router
	Head(path string, handlers ...fiber.Handler) Router
	Post(path string, handlers ...fiber.Handler) Router
	Put(path string, handlers ...fiber.Handler) Router
	Delete(path string, handlers ...fiber.Handler) Router
	Connect(path string, handlers ...fiber.Handler) Router
	Options(path string, handlers ...fiber.Handler) Router
	Trace(path string, handlers ...fiber.Handler) Router
	Patch(path string, handlers ...fiber.Handler) Router

	Add(method, path string, handlers ...fiber.Handler) Router
	All(path string, handlers ...fiber.Handler) Router

	Group(prefix string, handlers ...fiber.Handler) Router
}

func absPath(segments ...string) string {
	if len(segments) == 0 {
		return "/"
	}

	path := ""

	for _, seg := range segments {
		if len(seg) == 0 || seg == "/" {
			continue
		}

		if seg[0] != '/' {
			seg = "/" + seg
		}

		path = utils.TrimRight(path, '/') + seg
	}

	return path
}
