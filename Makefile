# Copyright 2022-2024 The nagare media authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Options

OS             ?= $(HOST_OS)
ARCH           ?= $(HOST_ARCH)
VERSION        ?= dev

GIT_COMMIT     ?= $(shell git rev-parse --short HEAD || echo "unknown")
GIT_TREE_STATE ?= $(shell sh -c 'if test -z "$$(git status --porcelain 2>/dev/null)"; then echo clean; else echo dirty; fi')
BUILD_DATE     ?= $(shell date -u +"%Y-%m-%dT%TZ")

IMAGE_REGISTRY ?= $(shell cat build/package/image/IMAGE_REGISTRY)
IMAGE_TAG      ?= $(VERSION)

# Do not change
HOST_OS   = $(shell which go >/dev/null 2>&1 && go env GOOS)
HOST_ARCH = $(shell which go >/dev/null 2>&1 && go env GOARCH)
GOVERSION = $(shell awk '/^go/ { print $$2 }' go.mod)
PKG       = $(shell awk '/^module/ { print $$2 }' go.mod)
CMDS      = $(shell find ./cmd/ -maxdepth 1 -mindepth 1 -type d -exec basename {} \;)
IMAGES    = $(shell find ./build/package/image -maxdepth 1 -mindepth 1 -type d -exec basename {} \;)

# Targets

.DEFAULT_GOAL:=help

.PHONY: build
build: $(addprefix build-, $(CMDS))

build-%:
	@	CMD="$*" \
		PKG="$(PKG)" \
		OS="$(OS)" \
		ARCH="$(ARCH)" \
		VERSION="$(VERSION)" \
		GIT_COMMIT="$(GIT_COMMIT)" \
		GIT_TREE_STATE="$(GIT_TREE_STATE)" \
		BUILD_DATE="$(BUILD_DATE)" \
	scripts/exec-local build

.PHONY: clean
clean:
	@scripts/exec-local clean

.PHONY: image
image: $(addprefix image-, $(IMAGES))

image-%:
	@	IMAGE="$*" \
		IMAGE_REGISTRY="$(IMAGE_REGISTRY)" \
		IMAGE_TAG="$(IMAGE_TAG)" \
		GOVERSION="$(GOVERSION)" \
		OS="$(OS)" \
		ARCH="$(ARCH)" \
		VERSION="$(VERSION)" \
		GIT_COMMIT="$(GIT_COMMIT)" \
		GIT_TREE_STATE="$(GIT_TREE_STATE)" \
		BUILD_DATE="$(BUILD_DATE)" \
	scripts/exec-local image

run-%:
	@scripts/exec-local "run-$*"

help:
	@printf "Usage:\n"
	@printf "  make \033[36m<task>\033[0m\n"
	@printf "\n"
	@printf "\033[1m%s\033[0m\n"                     "General"
	@printf "  \033[36m%-15s\033[0m %s\n"              "help"                         "Print this help"
	@printf "  \033[36m%-15s\033[0m %s\n"              "info"                         "Print options"
	@printf "\n"
	@printf "\033[1m%s\033[0m\n"                     "Build"
	@printf "  \033[36m%-15s\033[0m %s\n"              "build"                        "Build all commands"
	@printf "  \033[36m%-15s\033[0m %s\n"              "build-[cmd]"                  "Build command [cmd] (possible values: $(CMDS))"
	@printf "  \033[36m%-15s\033[0m %s\n"              "clean"                        "Delete build output"
	@printf "\n"
	@printf "\033[1m%s\033[0m\n"                     "Run"
	@printf "  \033[36m%-30s\033[0m %s\n"              "run-hls-fmp4-ffmpeg"          "Run example HLS fMP4 ingest"
	@printf "  \033[36m%-30s\033[0m %s\n"              "run-ll-dash-ffmpeg"           "Run example LL-DASH ingest"
	@printf "  \033[36m%-30s\033[0m %s\n"              "run-cmaf-long-upload-ffmpeg"  "Run example CMAF ingest (long chunked transfer encoding requests)"
	@printf "\n"
	@printf "\033[1m%s\033[0m\n"                     "Container Image"
	@printf "  \033[36m%-15s\033[0m %s\n"              "image"                        "Build all container images"
	@printf "  \033[36m%-15s\033[0m %s\n"              "image-[img]"                  "Build container image [img] (possible values: $(IMAGES))"

info:
	@printf "\n"
	@printf "\033[1m%s\033[0m\n"                     "Build"
	@printf "  \033[36m%-15s\033[0m %s\n"              "OS"             "$(OS)"
	@printf "  \033[36m%-15s\033[0m %s\n"              "ARCH"           "$(ARCH)"
	@printf "\n"
	@printf "\033[1m%s\033[0m\n"                     "Version Info"
	@printf "  \033[36m%-15s\033[0m %s\n"              "VERSION"        "$(VERSION)"
	@printf "  \033[36m%-15s\033[0m %s\n"              "GIT_COMMIT"     "$(GIT_COMMIT)"
	@printf "  \033[36m%-15s\033[0m %s\n"              "GIT_TREE_STATE" "$(GIT_TREE_STATE)"
	@printf "  \033[36m%-15s\033[0m %s\n"              "BUILD_DATE"     "$(BUILD_DATE)"
	@printf "\n"
	@printf "\033[1m%s\033[0m\n"                     "Container Image"
	@printf "  \033[36m%-15s\033[0m %s\n"              "IMAGE_REGISTRY" "$(IMAGE_REGISTRY)"
	@printf "  \033[36m%-15s\033[0m %s\n"              "IMAGE_TAG"      "$(IMAGE_TAG)"
