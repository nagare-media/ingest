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

package media

type FileType uint16

const (
	UnknownFileType               FileType = iota
	ManifestFileType                       // generic manifest (can be HLS or DASH)
	HLSManifestFileType                    // specific manifest for HLS
	DASHManifestFileType                   // specific manifest for DASH
	SegmentFileType                        // generic segment (can be media or initialization)
	InitializationSegmentFileType          // specific fmp4 segment containing header
	MediaSegmentFileType                   // specific fmp4 or ts segment containing media
	EncryptionKeyFileType
)

var (
	extToFileType = map[string]FileType{
		".m3u":    HLSManifestFileType,
		".m3u8":   HLSManifestFileType,
		".mpd":    DASHManifestFileType,
		".cmfv":   SegmentFileType,
		".cmfa":   SegmentFileType,
		".cmft":   SegmentFileType,
		".cmfm":   SegmentFileType,
		".mp4":    SegmentFileType,
		".m4v":    SegmentFileType,
		".m4a":    SegmentFileType,
		".m4s":    SegmentFileType,
		".init":   InitializationSegmentFileType,
		".header": InitializationSegmentFileType,
		".ts":     MediaSegmentFileType,
		".webm":   SegmentFileType,
		".key":    EncryptionKeyFileType,
	}
)

func FileTypeExt(ext string) FileType {
	if ft, ok := extToFileType[ext]; ok {
		return ft
	}
	return UnknownFileType
}

func IsManifest(ft FileType) bool {
	switch ft {
	case ManifestFileType, HLSManifestFileType, DASHManifestFileType:
		return true
	}
	return false
}

func IsSegment(ft FileType) bool {
	switch ft {
	case SegmentFileType, InitializationSegmentFileType, MediaSegmentFileType:
		return true
	}
	return false
}

func IsEncryptionKey(ft FileType) bool {
	return ft == EncryptionKeyFileType
}
