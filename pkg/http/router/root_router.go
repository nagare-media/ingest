/*
Copyright 2022 The nagare media authors

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
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"unsafe"

	"github.com/gobwas/glob/compiler"
	"github.com/gobwas/glob/match"
	"github.com/gobwas/glob/syntax"
	"github.com/gobwas/glob/syntax/ast"
	"github.com/gofiber/fiber/v2"

	"github.com/nagare-media/ingest/pkg/http"
)

type RootRouter interface {
	Router

	Host(pattern string) Router

	FiberApp() *fiber.App
	Register()
}

type hostMap struct {
	toPrefix           map[string]string
	toExactMatching    map[string]bool
	toGlobMatching     map[string]bool
	toGlobMatcher      map[string]match.Matcher
	globSearchPriority []string
}

type rootRouter struct {
	app      *fiber.App
	prefix   string
	handlers []fiber.Handler
	hostMap  *hostMap
}

func New(app *fiber.App) *rootRouter {
	return &rootRouter{
		app:      app,
		prefix:   "/",
		handlers: make([]fiber.Handler, 0),
		hostMap: &hostMap{
			toPrefix:        make(map[string]string),
			toExactMatching: make(map[string]bool),
			toGlobMatching:  make(map[string]bool),
			toGlobMatcher:   make(map[string]match.Matcher),
		},
	}
}

func (r *rootRouter) Host(hostPattern string) Router {
	if strings.HasPrefix(r.prefix, HostRoutesPrefix) {
		panic("tried to register a new host on a host router")
	}

	hostPrefix, ok := r.hostMap.toPrefix[hostPattern]
	if !ok {
		// we have not seen this host pattern before => update host maps

		// use hash of host pattern in internal paths
		hash := sha256.Sum256([]byte(hostPattern))
		hostPrefix = absPath(HostRoutesPrefix, r.prefix, fmt.Sprintf("%x", hash))
		r.hostMap.toPrefix[hostPattern] = hostPrefix

		hostAST, err := syntax.Parse(hostPattern)
		if err != nil {
			panic("host pattern invalid")
		}

		if len(hostAST.Children) == 1 && hostAST.Children[0].Kind == ast.KindText {
			// no glob => use exact host matching
			r.hostMap.toExactMatching[hostPattern] = true
		} else {
			// glob => use glob host matching
			r.hostMap.toGlobMatching[hostPattern] = true

			matcher, err := compiler.Compile(hostAST, []rune{'.'})
			if err != nil {
				panic("host pattern could not be compiled")
			}
			r.hostMap.toGlobMatcher[hostPattern] = matcher

			// globSearchPriority should be sorted by descending pattern length
			i := sort.Search(len(r.hostMap.globSearchPriority), func(i int) bool {
				return len(r.hostMap.globSearchPriority[i]) < len(hostPattern)
			})
			r.hostMap.globSearchPriority = append(r.hostMap.globSearchPriority, "")
			copy(r.hostMap.globSearchPriority[i+1:], r.hostMap.globSearchPriority[i:])
			r.hostMap.globSearchPriority[i] = hostPattern
		}
	}

	preHostRouteHandler := func(c *fiber.Ctx) error {
		// this is a host route => hostPattern must be set => only allow internal redirects
		if str, ok := c.Locals(http.HostPatternKey).(string); !ok || str != hostPattern {
			return fiber.ErrNotFound
		}
		return c.Next()
	}

	return &rootRouter{
		app:      r.app,
		prefix:   hostPrefix,
		hostMap:  r.hostMap,
		handlers: append(r.handlers, preHostRouteHandler),
	}
}

func (r *rootRouter) Register() {
	r.app.Use(func(c *fiber.Ctx) error {
		var hostPattern, matchType string
		searchIdx := -1
		foundMatch := false

		hostname := c.Hostname()

		originalPath, ok := c.Locals(http.OriginalPathKey).(string)
		if !ok {
			// duplicate string as fiber can manipulate the underlying byte slice
			// TODO: replace with strings.Clone in Go 1.18
			fiberPath := c.Path()
			b := make([]byte, len(fiberPath))
			copy(b, fiberPath)
			originalPath = *(*string)(unsafe.Pointer(&b))
			c.Locals(http.OriginalPathKey, originalPath)
		}

		matchType, ok = c.Locals(http.HostMatchTypeKey).(string)
		if !ok {
			// no previous search for host match

			// search for exact matches
			if r.hostMap.toExactMatching[hostname] {
				// found exact match
				foundMatch = true
				hostPattern = hostname
				matchType = "exact"
				goto next
			}

			// no exact match
			// search for glob match
			matchType = "glob"
		}

		switch matchType {
		case "exact":
			// previous exact match had no handlers
			// search for glob match
			matchType = "glob"
		case "glob":
		default:
			// unknown match type
			return fiber.ErrInternalServerError
		}

		searchIdx, ok = c.Locals(http.HostGlobSearchIndexKey).(int)
		if !ok {
			searchIdx = -1
		}
		for searchIdx++; searchIdx < len(r.hostMap.globSearchPriority); searchIdx++ {
			hostPattern = r.hostMap.globSearchPriority[searchIdx]
			glob := r.hostMap.toGlobMatcher[hostPattern]
			if glob.Match(hostname) {
				// found glob match
				foundMatch = true
				goto next
			}
		}

	next:
		if !foundMatch {
			c.Locals(http.HostPatternKey, nil)
			c.Locals(http.HostMatchTypeKey, nil)
			c.Locals(http.HostGlobSearchIndexKey, nil)
			return c.Next()
		}

		c.Locals(http.HostPatternKey, hostPattern)
		c.Locals(http.HostMatchTypeKey, matchType)
		c.Locals(http.HostGlobSearchIndexKey, searchIdx)
		c.Locals(http.InInternalRedirectKey, true)
		// internal redirect
		c.Path(absPath(r.hostMap.toPrefix[hostPattern], originalPath))
		return c.RestartRouting()
	})
}

func (r *rootRouter) Use(args ...interface{}) Router {
	if len(args) == 0 {
		panic("missing handler")
	}

	handlerIdx := 0
	newArgs := make([]interface{}, len(r.handlers)+1)

	// add path
	if len(args) == 1 {
		// args[0] is a handler
		newArgs[0] = r.prefix
	} else if len(args) >= 2 {
		// args[0] is a prefix
		// args[1:] are handlers
		newArgs[0] = absPath(r.prefix, args[0].(string))
		handlerIdx = 1
	}

	// add router handlers
	for i := range r.handlers {
		newArgs[i+1] = r.handlers[i]
	}

	// add passed handlers
	newArgs = append(newArgs, args[handlerIdx:]...)

	r.app.Use(newArgs...)
	return r
}

func (r *rootRouter) Get(path string, handlers ...fiber.Handler) Router {
	r.app.Get(absPath(r.prefix, path), append(r.handlers, handlers...)...)
	return r
}

func (r *rootRouter) Head(path string, handlers ...fiber.Handler) Router {
	r.app.Head(absPath(r.prefix, path), append(r.handlers, handlers...)...)
	return r
}

func (r *rootRouter) Post(path string, handlers ...fiber.Handler) Router {
	r.app.Post(absPath(r.prefix, path), append(r.handlers, handlers...)...)
	return r
}

func (r *rootRouter) Put(path string, handlers ...fiber.Handler) Router {
	r.app.Put(absPath(r.prefix, path), append(r.handlers, handlers...)...)
	return r
}

func (r *rootRouter) Delete(path string, handlers ...fiber.Handler) Router {
	r.app.Delete(absPath(r.prefix, path), append(r.handlers, handlers...)...)
	return r
}

func (r *rootRouter) Connect(path string, handlers ...fiber.Handler) Router {
	r.app.Connect(absPath(r.prefix, path), append(r.handlers, handlers...)...)
	return r
}

func (r *rootRouter) Options(path string, handlers ...fiber.Handler) Router {
	r.app.Options(absPath(r.prefix, path), append(r.handlers, handlers...)...)
	return r
}

func (r *rootRouter) Trace(path string, handlers ...fiber.Handler) Router {
	r.app.Trace(absPath(r.prefix, path), append(r.handlers, handlers...)...)
	return r
}

func (r *rootRouter) Patch(path string, handlers ...fiber.Handler) Router {
	r.app.Patch(absPath(r.prefix, path), append(r.handlers, handlers...)...)
	return r
}

func (r *rootRouter) Add(method, path string, handlers ...fiber.Handler) Router {
	r.app.Add(method, absPath(r.prefix, path), append(r.handlers, handlers...)...)
	return r
}

func (r *rootRouter) All(path string, handlers ...fiber.Handler) Router {
	r.app.All(absPath(r.prefix, path), append(r.handlers, handlers...)...)
	return r
}

func (r *rootRouter) Group(prefix string, handlers ...fiber.Handler) Router {
	return &rootRouter{
		app:      r.app,
		prefix:   absPath(r.prefix, prefix),
		handlers: append(r.handlers, handlers...),
		hostMap:  r.hostMap,
	}
}

func (r *rootRouter) FiberApp() *fiber.App {
	return r.app
}
