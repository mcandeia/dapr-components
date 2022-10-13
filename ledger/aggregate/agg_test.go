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

package aggregate

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAggBatch(t *testing.T) {
	t.Run("WithChange should set to uncommitted and add change", func(t *testing.T) {
		const fakeID = "fake-id"
		agg := &AggBatch{
			batch: map[string]*Agg{
				fakeID: {},
			},
			hasUncommitted: false,
		}
		require.NoError(t, agg.WithChange(fakeID, []byte(``)))

		assert.True(t, agg.hasUncommitted)

		assert.NotEmpty(t, agg.batch[fakeID].uncommitted)
	})

	t.Run("State should return nil when has only uncommitted events", func(t *testing.T) {
		const fakeID = "fake-id"
		agg := &AggBatch{
			batch: map[string]*Agg{
				fakeID: {},
			},
			hasUncommitted: false,
		}
		require.NoError(t, agg.WithChange(fakeID, []byte(``)))

		assert.True(t, agg.hasUncommitted)

		state, version := agg.State(fakeID)

		assert.Nil(t, state)
		assert.Empty(t, version)
	})

	t.Run("State should return state when has committed events", func(t *testing.T) {
		const fakeID = "fake-id"
		agg := &AggBatch{
			batch: map[string]*Agg{
				fakeID: {},
			},
			hasUncommitted: false,
		}
		require.NoError(t, agg.WithChange(fakeID, []byte(``)))

		assert.True(t, agg.hasUncommitted)

		agg.batch[fakeID].Events = agg.batch[fakeID].eventsWith(nil, true, time.Now().UTC())

		state, _ := agg.State(fakeID)

		assert.NotNil(t, state)
	})

	t.Run("History should be empty when has only uncommitted events", func(t *testing.T) {
		const fakeID = "fake-id"
		agg := &AggBatch{
			batch: map[string]*Agg{
				fakeID: {},
			},
			hasUncommitted: false,
		}
		require.NoError(t, agg.WithChange(fakeID, []byte(``)))

		assert.True(t, agg.hasUncommitted)

		events := agg.History(fakeID)

		assert.Empty(t, events)
	})

	t.Run("preparePersistNoDirty should not save as dirty state", func(t *testing.T) {
		const fakeID = "fake-id"
		agg := &AggBatch{
			batch: map[string]*Agg{
				fakeID: {},
			},
			hasUncommitted: false,
		}
		require.NoError(t, agg.WithChange(fakeID, []byte(``)))
		assert.True(t, agg.hasUncommitted)
		state := agg.batch[fakeID].preparePersistNoDirty()

		assert.False(t, state.Content.Dirty)
		assert.NotEmpty(t, state.Content.Events)
	})

	t.Run("preparePersist should not save as dirty state", func(t *testing.T) {
		const fakeID = "fake-id"
		agg := &AggBatch{
			batch: map[string]*Agg{
				fakeID: {},
			},
			hasUncommitted: false,
		}
		require.NoError(t, agg.WithChange(fakeID, []byte(``)))
		assert.True(t, agg.hasUncommitted)
		state := agg.batch[fakeID].preparePersist(nil, false, time.Now().UTC())

		assert.False(t, !state.Content.Dirty)
		assert.NotEmpty(t, state.Content.Events)
	})
}
