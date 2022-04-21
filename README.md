# nagare media ingest

[![Go Report Card](https://goreportcard.com/badge/github.com/nagare-media/ingest?style=flat-square)](https://goreportcard.com/report/github.com/nagare-media/ingest)
![Go Version](https://img.shields.io/badge/go%20version-%3E=1.18-61CFDD.svg?style=flat-square)
[![PkgGoDev](https://pkg.go.dev/badge/mod/github.com/nagare-media/ingest)](https://pkg.go.dev/mod/github.com/nagare-media/ingest)

Implementation of various (HTTP based) media ingest protocols.

Original repo: <https://github.com/nagare-media/ingest>

## Quickstart

We provide three examples for the DASH-IF Ingest protocol:

### Example 1: HLS ingest using DASH-IF Ingest Interface-1

```sh
# using Docker
$ docker-compose -f examples/docker-compose.hls-fmp4-ffmpeg.yaml up --build

# run locally
$ make clean build; bin/ingest-dev-* --dev -c configs/example.yaml
$ make run-hls-fmp4-ffmpeg
```

The ingest starts after 10s and can be watched using [hls.js](https://hls-js.netlify.app/demo/?src=http%3A%2F%2Flocalhost%3A8080%2Fhls%2Fexample.str%2Fmaster.m3u8)

### Example 2: low latency DASH ingest using DASH-IF Ingest Interface-1

```sh
# using Docker
$ docker-compose -f examples/docker-compose.ll-dash-ffmpeg.yaml up --build

# run locally
$ make clean build; bin/ingest-dev-* --dev -c configs/example.yaml
$ make run-ll-dash-ffmpeg
```

The ingest starts after 10s and can be watched using [dash.js](https://reference.dashif.org/dash.js/nightly/samples/dash-if-reference-player/index.html?mpd=http%3A%2F%2Flocalhost%3A8080%2Fdash%2Fexample.str%2Fmanifest.mpd)

### Example 3: CMAF ingest with long running HTTP CTE request using DASH-IF Ingest Interface-2

```sh
# using Docker
$ docker-compose -f examples/docker-compose.cmaf-long-upload-ffmpeg.yaml up --build

# run locally
$ make clean build; bin/ingest-dev-* --dev -c configs/example.yaml
$ make run-cmaf-long-upload-ffmpeg
```

The ingest starts after 10s and can be watched using [hls.js](https://hls-js.netlify.app/demo/?src=http%3A%2F%2Flocalhost%3A8080%2Fcmaf%2Fexample.str%2Fmaster.m3u8)

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
