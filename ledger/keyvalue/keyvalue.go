/*
Copyright 2022 @mcandeia
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

package keyvalue

import "context"

// Value wraps a database value with its content and its version
type Value[T any] struct {
	Content T
	Version string
}

// Versioned is a simple data base that stores a content by its key and retrieve by its value,
// it also supports versioned and optimistic lock using the version.
type Versioned[T any] interface {
	// BulkGet returns a value and its associated version for a bulk of keys.
	BulkGet(ctx context.Context, keys []string) (map[string]Value[T], error)
	// BulkSet sets the value and optionally check for the version if it was set.
	BulkSet(ctx context.Context, values map[string]Value[T]) error
}
