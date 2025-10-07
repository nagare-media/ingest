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

package http

import (
	"regexp"

	"github.com/gofiber/fiber/v2"
)

const (
	OriginalPathKey        = "ingest.nagare.media/original-path"
	HostPatternKey         = "ingest.nagare.media/host-pattern"
	HostMatchTypeKey       = "ingest.nagare.media/host-match-type"
	HostGlobSearchIndexKey = "ingest.nagare.media/host-glob-search-index"
	InInternalRedirectKey  = "ingest.nagare.media/in-internal-redirect"
	RequestIDKey           = "ingest.nagare.media/request-id"
)

var (
	ErrNotAFileStream              = fiber.NewError(fiber.StatusBadRequest, "Not A File Stream")
	ErrUnsupportedFileExtension    = fiber.NewError(fiber.StatusUnsupportedMediaType, "Unsupported File Extension")
	ErrUnsupportedProtocolVersion  = fiber.NewError(fiber.StatusBadRequest, "Unsupported Protocol Version")
	ErrUnsupportedStreamName       = fiber.NewError(fiber.StatusBadRequest, "Unsupported Stream Name")
	ErrUnsupportedSwitchingSetName = fiber.NewError(fiber.StatusBadRequest, "Unsupported Switching Set Name")
	ErrUnsupportedTrackName        = fiber.NewError(fiber.StatusBadRequest, "Unsupported Track Name")
	ErrUnsupportedUploadPath       = fiber.NewError(fiber.StatusBadRequest, "Unsupported Upload Path")
)

var (
	UploadPathRegex       = regexp.MustCompile("^/[^,;:?'\"[\\]{}@*\\\\&#%`^+<=>|~$\\x00-\\x1F\\x7F\\t\\n\\f\\r ]+$")
	PathIllegalCharsRegex = regexp.MustCompile("[,;:?'\"[\\]{}@*\\\\&#%`^+<=>|~$\\x00-\\x1F\\x7F\\t\\n\\f\\r ]")
	PathIllegalReplaceStr = "_"
)

var (
	NextIfInInternalRedirect = func(c *fiber.Ctx) bool {
		inInternalRedirect, ok := c.Locals(InInternalRedirectKey).(bool)
		return ok && inInternalRedirect
	}

	NextHandler = func(c *fiber.Ctx) error {
		return c.Next()
	}

	NoContentHandler = func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	}
)
