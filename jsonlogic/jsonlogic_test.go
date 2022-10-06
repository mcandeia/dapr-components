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

package main

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	contribBindings "github.com/dapr/components-contrib/bindings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJsonLogicBinding(t *testing.T) {
	jsonLogic := jsonLogicOutput{}
	t.Run("invoking an unsupported operation should return an error", func(t *testing.T) {
		const fakeOperation = "fake-operation"
		_, err := jsonLogic.Invoke(context.Background(), &contribBindings.InvokeRequest{
			Operation: fakeOperation,
			Data:      []byte(`"{}"`),
		})

		assert.NotNil(t, err)
		assert.Contains(t, err.Error(), "unsupported")
	})

	t.Run("invoking without a valid data should return syntax err", func(t *testing.T) {
		const fakeOperation = "fake-operation"
		_, err := jsonLogic.Invoke(context.Background(), &contribBindings.InvokeRequest{
			Operation: fakeOperation,
			Data:      []byte(``),
		})

		assert.Equal(t, err, strconv.ErrSyntax)
	})

	t.Run("invoking should evaluate operation", func(t *testing.T) {
		resp, err := jsonLogic.Invoke(context.Background(), &contribBindings.InvokeRequest{
			Operation: evaluateOperation,
			Data:      []byte(`"{ \"data\": { \"x\": 10 }, \"expression\": { \"var\": \"x\"}}"`),
		})

		require.NoError(t, err)

		var x int
		err = json.Unmarshal(resp.Data, &x)
		require.NoError(t, err)

		assert.Equal(t, x, 10)
	})
}
