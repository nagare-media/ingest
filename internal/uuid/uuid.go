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

package uuid

import (
	"github.com/gofiber/fiber/v2/utils"
	"github.com/google/uuid"
)

func init() {
	// assumption: our UUIDs are not sensitive information and can live in RAM for some time
	uuid.EnableRandPool()
}

func UUIDv4() string {
	token, err := uuid.NewRandom()
	if err != nil {
		return utils.UUID()
	}
	return token.String()
}
