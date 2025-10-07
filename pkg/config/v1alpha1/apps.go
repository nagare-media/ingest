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

type App struct {
	Name      string     `mapstructure:"name"`
	Functions []Function `mapstructure:"functions,omitempty"`

	HTTP *HTTPApp `mapstructure:"http,omitempty"`

	GenericServe *GenericServe `mapstructure:"genericServe,omitempty"`

	CMAFIngest       *CMAFIngest       `mapstructure:"cmafIngest,omitempty"`
	DASHAndHLSIngest *DASHAndHLSIngest `mapstructure:"dashAndHlsIngest,omitempty"`
}

type HTTPApp struct {
	Host string `mapstructure:"host,omitempty"`
	Path string `mapstructure:"path,omitempty"`
	Auth *Auth  `mapstructure:"auth,omitempty"`
	CORS *CORS  `mapstructure:"cors,omitempty"`
}

type Auth struct {
	Basic  *BasicAuth  `mapstructure:"basic,omitempty"`
	Digest *DigestAuth `mapstructure:"digest,omitempty"`
	Bearer *BearerAuth `mapstructure:"bearer,omitempty"`
}

type BasicAuth struct {
	Users []User `mapstructure:"users"`
}

type DigestAuth struct {
	Realm string `mapstructure:"realm"`
	Users []User `mapstructure:"users"`
}

type User struct {
	Name     string `mapstructure:"name"`
	Password string `mapstructure:"password"`
}

type BearerAuth struct {
	Tokens []Token `mapstructure:"tokens"`
}

type Token struct {
	Name string `mapstructure:"name"`
}

type CORS struct {
	AllowOrigins     string `mapstructure:"AllowOrigins,omitempty"`
	AllowMethods     string `mapstructure:"AllowMethods,omitempty"`
	AllowHeaders     string `mapstructure:"AllowHeaders,omitempty"`
	AllowCredentials bool   `mapstructure:"AllowCredentials,omitempty"`
	ExposeHeaders    string `mapstructure:"ExposeHeaders,omitempty"`
	MaxAge           int    `mapstructure:"MaxAge,omitempty"`
}

type GenericServe struct {
	AppRef      Reference   `mapstructure:"appRef"`
	VolumesRefs []Reference `mapstructure:"volumeRefs"`

	DefaultContentType string `mapstructure:"defaultContentType,omitempty"`
	UseXAccelHeader    bool   `mapstructure:"useXAccelHeader,omitempty"`
	UseXSendfileHeader bool   `mapstructure:"useXSendfileHeader,omitempty"`
}

type CMAFIngest struct {
	VolumeRef Reference `mapstructure:"volumeRef"`

	StreamTimeout      time.Duration     `mapstructure:"streamTimeout,omitempty"`
	MaxManifestSize    bytesize.ByteSize `mapstructure:"maxManifestSize,omitempty"`
	MaxHeaderSize      bytesize.ByteSize `mapstructure:"maxHeaderSize,omitempty"`
	MaxChunkHeaderSize bytesize.ByteSize `mapstructure:"maxChunkHeaderSize,omitempty"`
	MaxChunkMdatSize   bytesize.ByteSize `mapstructure:"maxChunkDataSize,omitempty"`
}

type DASHAndHLSIngest struct {
	VolumeRef Reference `mapstructure:"volumeRef"`

	StreamTimeout           time.Duration     `mapstructure:"streamTimeout,omitempty"`
	ReleaseCompleteSegments bool              `mapstructure:"releaseCompleteSegments,omitempty"` // needs to be false for low latency mode
	RequestBodyBufferSize   bytesize.ByteSize `mapstructure:"requestBodyBufferSize,omitempty"`
	MaxManifestSize         bytesize.ByteSize `mapstructure:"maxManifestSize,omitempty"`
	MaxSegmentSize          bytesize.ByteSize `mapstructure:"maxSegmentSize,omitempty"`
	MaxEncryptionKeySize    bytesize.ByteSize `mapstructure:"maxEncryptionKeySize,omitempty"`
}
