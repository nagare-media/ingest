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

package v1alpha1

import (
	"time"

	"github.com/inhies/go-bytesize"
)

type Volume struct {
	Name string `mapstructure:"name"`

	Null       *NullVolume       `mapstructure:"null,omitempty"`
	Memory     *MemoryVolume     `mapstructure:"mem,omitempty"`
	FileSystem *FileSystemVolume `mapstructure:"fs,omitempty"`
	S3         *S3Volume         `mapstructure:"s3,omitempty"`
}

type NullVolume struct{}

type MemoryVolume struct {
	BlockSize               bytesize.ByteSize `mapstructure:"blockSize,omitempty"`
	GarbageCollectionPeriod time.Duration     `mapstructure:"garbageCollectionPeriod,omitempty"`
}

type FileSystemVolume struct {
	Path                    string        `mapstructure:"path"`
	GarbageCollectionPeriod time.Duration `mapstructure:"garbageCollectionPeriod,omitempty"`
}

type S3Volume struct {
	Path string `mapstructure:"path"`
}
