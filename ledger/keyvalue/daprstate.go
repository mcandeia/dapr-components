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

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	dapr "github.com/dapr/go-sdk/client"
	internal "github.com/mcandeia/dapr-components/internal"
)

const (
	daprKeySeparator      = "||"
	ledgerPrefixSeparator = "__"
)

type daprClient interface {
	// SaveBulkState saves multiple state item to store with specified options.
	SaveBulkState(ctx context.Context, storeName string, items ...*dapr.SetStateItem) error
	// GetBulkState retrieves state for multiple keys from specific store.
	GetBulkState(ctx context.Context, storeName string, keys []string, meta map[string]string, parallelism int32) ([]*dapr.BulkStateItem, error)
}

type daprState[T any] struct {
	store      string
	keyPrefix  string
	daprClient daprClient
}

func sanitize(str string) string {
	return strings.Replace(str, daprKeySeparator, ledgerPrefixSeparator, 1)
}
func (d *daprState[T]) addPrefix(key string) string {
	return sanitize(d.keyPrefix + "_" + key)
}

func (d *daprState[T]) removePrefix(key string) string {
	return strings.Replace(key[(len(d.keyPrefix)+1):], ledgerPrefixSeparator, daprKeySeparator, 1)
}

// Get returns a value and its associated version
func (d *daprState[T]) BulkGet(ctx context.Context, keys []string) (map[string]Value[T], error) {
	if len(keys) == 0 {
		return make(map[string]Value[T]), nil
	}
	prefixedKeys := make([]string, len(keys))
	for idx, key := range keys {
		prefixedKeys[idx] = d.addPrefix(key)
	}
	result := make(map[string]Value[T], len(keys))
	resp, err := d.daprClient.GetBulkState(ctx, d.store, prefixedKeys, map[string]string{"content-type": "application/json"}, int32(len(keys)))
	if err != nil {
		return nil, err
	}

	for _, bulkState := range resp {
		var (
			val      T
			stateErr error
		)

		if errString := bulkState.Error; len(errString) > 0 {
			stateErr = errors.New(errString)
		} else if len(bulkState.Value) > 0 {
			if err := json.Unmarshal(bulkState.Value, &val); err != nil {
				stateErr = err
			}
		}

		if stateErr != nil {
			return nil, stateErr
		}

		result[d.removePrefix(bulkState.Key)] = Value[T]{
			Content: val,
			Version: bulkState.Etag,
		}
	}
	return result, nil
}

// Set sets the value and optionally check for the version if it was set.
func (d *daprState[T]) BulkSet(ctx context.Context, values map[string]Value[T]) error {
	stateItems := make([]*dapr.SetStateItem, len(values))
	idx := 0
	for key, val := range values {
		value, err := json.Marshal(val.Content)
		if err != nil {
			return err
		}

		var etag *dapr.ETag
		if len(val.Version) > 0 {
			etag = &dapr.ETag{
				Value: val.Version,
			}
		}
		stateItems[idx] = &dapr.SetStateItem{
			Key:   d.addPrefix(key),
			Value: value,
			Etag:  etag,
			Options: &dapr.StateOptions{
				Concurrency: internal.Ternary(etag == nil, internal.Always(dapr.StateConcurrencyLastWrite), internal.Always(dapr.StateConcurrencyFirstWrite)),
				Consistency: dapr.StateConsistencyStrong,
			},
			Metadata: map[string]string{
				"content-type": "application/json",
			},
		}
		idx++
	}
	return d.daprClient.SaveBulkState(ctx, d.store, stateItems...)
}

// NewDaprState creates a new dapr state for the given store.
func NewDaprState[T any](client daprClient, store, keyPrefix string) Versioned[T] {
	return &daprState[T]{
		store:      store,
		daprClient: client,
		keyPrefix:  keyPrefix,
	}
}
