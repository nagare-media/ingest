# nagare media ingest

Implementation of various HTTP based media ingest protocols.

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

## License

Apache 2.0 (c) nagare media authors
