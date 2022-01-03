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

package mime

func Normalize(t string) string {
	if nt, ok := normalized[t]; ok {
		return nt
	}
	return t
}

func NormalizeExt(t string, ext string) string {
	if nt, ok := normalized[t]; ok {
		return nt
	}
	if nts, ok := extToTypes[ext]; ok {
		return nts[0]
	}
	return t
}

func TypesExt(ext string) []string {
	return extToTypes[ext]
}

func PreferredTypeExt(ext string) string {
	if ts, ok := extToTypes[ext]; ok {
		return ts[0]
	}
	return ""
}

func MatchExt(t string, ext string) bool {
	if ts, ok := extToTypes[ext]; ok {
		for _, match := range ts {
			if t == match {
				return true
			}
		}
	}
	return false
}

const (
	ApplicationDASH_XML        = "application/dash+xml"
	ApplicationMP4             = "application/mp4"
	ApplicationOctetStream     = "application/octet-stream"
	ApplicationVndAppleMPEGURL = "application/vnd.apple.mpegurl"
	AudioMP4                   = "audio/mp4"
	AudioWebM                  = "audio/webm"
	VideoISOSegment            = "video/iso.segment"
	VideoMP2T                  = "video/MP2T"
	VideoMP4                   = "video/mp4"
	VideoWebM                  = "video/webm"
)

var (
	normalized = map[string]string{
		// DASH-IF Ingest only allows "application/x-mpegurl" and "application/vnd.apple.mpegurl"
		// RFC 8216 only allows "audio/mpegurl" and "application/vnd.apple.mpegurl"
		// Other MIME types are used according to https://en.wikipedia.org/wiki/M3U
		"application/mpegurl":                 ApplicationVndAppleMPEGURL,
		"application/vnd.apple.mpegurl.audio": ApplicationVndAppleMPEGURL,
		"application/x-mpegurl":               ApplicationVndAppleMPEGURL,
		"audio/mpegurl":                       ApplicationVndAppleMPEGURL,
		"audio/x-mpegurl":                     ApplicationVndAppleMPEGURL,
	}

	extToTypes = map[string][]string{
		".cmfa":   {AudioMP4},
		".cmfm":   {ApplicationMP4},
		".cmft":   {ApplicationMP4},
		".cmfv":   {VideoMP4},
		".header": {VideoMP4},
		".init":   {VideoMP4},
		".key":    {ApplicationOctetStream},
		".m3u":    {ApplicationVndAppleMPEGURL},
		".m3u8":   {ApplicationVndAppleMPEGURL},
		".m4a":    {AudioMP4},
		".m4s":    {VideoISOSegment},
		".m4v":    {VideoMP4},
		".mp4":    {VideoMP4, ApplicationMP4},
		".mpd":    {ApplicationDASH_XML},
		".ts":     {VideoMP2T},
		".weba":   {AudioWebM},
		".webm":   {VideoWebM},
	}
)
