/*
Copyright 2022-2024 The nagare media authors

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

package version

import (
	"html/template"
	"io"
)

var printTmpl = template.Must(template.New("print").Parse(`{{.Name}}:
  Version:        {{.Version}}
  Git Commit:     {{.GitCommit}}
  Git Tree State: {{.GitTreeState}}
  Build Date:     {{.BuildDate}}
  Go Version:     {{.GoVersion}}
  Compiler:       {{.Compiler}}
  Platform:       {{.Platform}}
`))

// Info holds version and build information. The fields are largely the same as in the `k8s.io/kubernetes/pkg/version`
// package of the Kubernetes project.
type Info struct {
	// Name of the versioned object.
	Name string

	// Version number.
	Version string

	// GitCommit SHA.
	GitCommit string

	// GitTreeState is either "clean" or "dirty".
	GitTreeState string

	// BuildDate of the binary.
	BuildDate string

	// GoVersion of the binary.
	GoVersion string

	// Compiler used for the binary.
	Compiler string

	// Platform the binary is compiled for.
	Platform string
}

// String returns a formated version string.
func (i Info) String() string {
	return i.Version
}

// Write the full version
func (i Info) Write(w io.Writer) error {
	return printTmpl.Execute(w, i)
}
