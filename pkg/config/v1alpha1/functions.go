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
)

type Function struct {
	Name string `mapstructure:"name"`

	Cleanup    *CleanupFunction    `mapstructure:"cleanup,omitempty"`
	CloudEvent *CloudEventFunction `mapstructure:"cloudEvent,omitempty"`
	Copy       *CopyFunction       `mapstructure:"copy,omitempty"`
	Manifest   *ManifestFunction   `mapstructure:"manifest,omitempty"`
}

type CopyFunction struct {
	VolumeRef Reference `mapstructure:"volumeRef"`
}

type CleanupFunction struct {
	Schedule   string        `mapstructure:"schedule"`
	VolumeRefs []Reference   `mapstructure:"volumeRefs"`
	Files      []string      `mapstructure:"files,omitempty"`
	Age        time.Duration `mapstructure:"age,omitempty"`
}

type ManifestFunction struct {
	VolumeRef Reference `mapstructure:"volumeRef"`
}

type CloudEventFunction struct {
	URL string `mapstructure:"url"`
}
