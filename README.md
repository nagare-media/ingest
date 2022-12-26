# `nagare media ingest`

[![Go Report Card](https://goreportcard.com/badge/github.com/nagare-media/ingest?style=flat-square)](https://goreportcard.com/report/github.com/nagare-media/ingest)
![Go Version](https://img.shields.io/badge/go%20version-%3E=1.19-61CFDD.svg?style=flat-square)
[![PkgGoDev](https://pkg.go.dev/badge/mod/github.com/nagare-media/ingest)](https://pkg.go.dev/github.com/nagare-media/ingest)

Implementation of various (HTTP based) media ingest protocols.

Original repo: <https://github.com/nagare-media/ingest>

## Quickstart

```sh
# run locally
$ make clean build
$ bin/ingest-dev-* -c configs/example.yaml

# using Docker
$ docker run --rm \
    -v $PWD/configs/example.yaml:/etc/nagare-media/ingest/config.yaml:ro \
    -p "8080:8080" \
    ghcr.io/nagare-media/ingest:dev
```

## Building

`nagare media ingest` requires Go 1.18. The code does not depend on specific operating systems or architectures (apart for some optimization) and should compile to many of Go's compile targets. It is known to run well on `darwin/amd64` and `linux/amd64`. Binaries for various systems are provided for each release. By default, the local system architecture is used when compiling the code. You can cross-compile by passing `OS` and `ARCH` to `make`:

```sh
# build binaries for local system architecture
$ make build

# build for macOS amd64
$ make build OS=darwin ARCH=amd64
```

The build can be controlled through various additional options. You can list all available options using `make info`:

```sh
$ make info OS=linux VERSION=1.0.0

Build
  OS              linux
  ARCH            amd64

Version Info
  VERSION         1.0.0
  GIT_COMMIT      e6903fa
...
```

Instead of a binary, a container image can be built using `make image`. Note that currently only Linux image are supported and `OS=linux` has to be set explicitly on other systems. Docker BuildKit is used for building images and needs to be installed and configured on the build system. Cross-compiling to other architectures is likewise controlled via the `ARCH` option. Note that a corresponding builder needs to be configured in Docker BuildKit for this to work. Images for various architectures are provided for each release.

Running `make` or `make help` will output all available targets together with a description.

## Configuration

`nagare media ingest` is mainly configured through a YAML configuration file. By default, a file named `config.yaml` is search for in the current directory and in `/etc/nagare-media/ingest/`. The path can be overwritten with the `-c, --config` CLI flag. The `--dev` flag will activate a special developer mode (currently it only influences the logging output). The following additional flags are available:

```sh
$ ingest --help
Usage: ingest [options]
  -c, --config string      Path to the config file
      --dev                Run in developer mode
  -h, --help               Print this help message and exit
  -l, --log-level string   Log level ("debug", "info", "warn", "error", "panic", "fatal")
  -V, --version            Print the version number and exit
```

A usable example configuration is provided with [`configs/example.yaml`](configs/example.yaml). The [`configs/full.yaml`](configs/full.yaml) configuration file lists and documents all available options and is meant as a reference.

## Examples

We provide three examples for the [DASH-IF Ingest protocol](https://dashif-documents.azurewebsites.net/Ingest/master/DASH-IF-Ingest.html). To run these examples, either `docker-compose` or [FFmpeg](https://ffmpeg.org/) needs to be installed locally. The examples are known to work with FFmpeg 5.0.1 running on Linux and macOS.

All example scenarios encode, package and send media with FFmpeg using the `testsrc2` and `sine` filters to generate test patterns for the video and audio tracks, respectively. Additionally, the wall-clock time at the encoding is burned into the video. We encode the video in a 720p and 360p variant which is also burned into the video as text, respectively. The GOP length is set to 2 seconds with a constant frame rate of 25. Target bit rates are 2500 KBit/s and 600 KBit/s for the 720p and 360p video tracks, respectively. The audio track is encoded with constant 64 KBit/s.

The exact `ffmpeg` command for the three scenarios is available in the `scripts/tasks/run-*-ffmpeg` scripts and can be adjusted there.

### Example 1: HLS ingest using DASH-IF Ingest Interface-2

This example uses the `hls` FFmpeg muxer to output a DASH-IF Interface-2 compliant HLS stream to `nagare media ingest`. A sliding window of five segments is used; old segments are deleted with HTTP `DELETE` requests. This allows the usage of a `mem` volume. The ingest starts after 10s and can be watched using the [hls.js demo page](https://hls-js.netlify.app/demo/?src=http%3A%2F%2Flocalhost%3A8080%2Fhls%2Fexample.str%2Fmaster.m3u8).


```sh
# using Docker
$ docker-compose -f examples/docker-compose.hls-fmp4-ffmpeg.yaml up --build

# run locally (in two shells)
$ make clean build; bin/ingest-dev-* --dev -c configs/example.yaml
$ make run-hls-fmp4-ffmpeg
```

### Example 2: low latency DASH ingest using DASH-IF Ingest Interface-2

This example uses the `dash` FFmpeg muxer to output a DASH-IF Interface-2 compliant LL-DASH stream to `nagare media ingest` with a target latency of 3 seconds. The 8 second CMAF fragments are composed of 1 second chunks and are ingested using HTTP chunked transfer encoding. This allows for an early release from the encoder and immediate delivery to clients. The ingest starts after 10s and can be watched using the [dash.js demo page](https://reference.dashif.org/dash.js/nightly/samples/dash-if-reference-player/index.html?mpd=http%3A%2F%2Flocalhost%3A8080%2Fdash%2Fexample.str%2Fmanifest.mpd).

```sh
# using Docker
$ docker-compose -f examples/docker-compose.ll-dash-ffmpeg.yaml up --build

# run locally (in two shells)
$ make clean build; bin/ingest-dev-* --dev -c configs/example.yaml
$ make run-ll-dash-ffmpeg
```

### Example 3: CMAF ingest with long running HTTP CTE request using DASH-IF Ingest Interface-1

This example uses the normal `mp4` FFmpeg muxer to output a DASH-IF Interface-1 compliant CMAF stream to `nagare media ingest`. Each CMAF track is ingested using a long running HTTP `POST` request with HTTP chunked transfer encoding. The 1 second CMAF chunks are split on the server side and an HLS manifest is generated. The ingest starts after 10s and can be watched using the [hls.js demo page](https://hls-js.netlify.app/demo/?src=http%3A%2F%2Flocalhost%3A8080%2Fcmaf%2Fexample.str%2Fmaster.m3u8).

```sh
# using Docker
$ docker-compose -f examples/docker-compose.cmaf-long-upload-ffmpeg.yaml up --build

# run locally (in two shells)
$ make clean build; bin/ingest-dev-* --dev -c configs/example.yaml
$ make run-cmaf-long-upload-ffmpeg
```

## Publications

This software was presented at the "13th ACM Multimedia Systems Conference, 2022 (MMSys '22)". Please cite this project in your research projects as follows:

> Matthias Neugebauer. 2022. Nagare Media Ingest: A Server for Live CMAF Ingest Workflows. In *13th ACM Multimedia Systems Conference (MMSys ’22), June 14–17, 2022, Athlone, Ireland.* ACM, New York, NY, USA, 6 pages. <https://doi.org/10.1145/3524273.3532888>

```
@inproceedings{10.1145/3524273.3532888,
  author = {Neugebauer, Matthias},
  title = {Nagare Media Ingest: A Server for Live CMAF Ingest Workflows},
  year = {2022},
  isbn = {9781450392839},
  publisher = {Association for Computing Machinery},
  address = {New York, NY, USA},
  url = {https://doi.org/10.1145/3524273.3532888},
  doi = {10.1145/3524273.3532888},
  abstract = {New media ingest protocols have been presented recently. SRT and RIST compete with old protocols such as RTMP while the DASH-IF specified an HTTP-based ingest protocol for CMAF formatted media that lends itself towards delivery protocols such as DASH and HLS. Additionally, use cases of media ingest workflows can vary widely. This makes implementing generic and flexible tools for ingest workflows a hard challenge. A monolithic approach limits adoption if the implementation does not fit the use case completely. We propose a new design for ingest servers that splits responsibilities into multiple components running concurrently. This design enables flexible ingest deployments as is discussed for various use cases. We have implemented this design in the open source software Nagare Media Ingest for the new DASH-IF ingest protocol.},
  booktitle = {Proceedings of the 13th ACM Multimedia Systems Conference},
  numpages = {6},
  keywords = {video streaming, protocol, server, dash, cmaf},
  location = {Athlone, Ireland},
  series = {MMSys '22}
}
```

## License

Apache 2.0 (c) nagare media authors
