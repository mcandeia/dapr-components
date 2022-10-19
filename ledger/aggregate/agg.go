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
	"encoding/json"
	"time"

	"github.com/dapr/components-contrib/state/utils"
	"github.com/mcandeia/dapr-components/ledger/keyvalue"
)

type AggBatch struct {
	batch          map[string]*Agg
	hasUncommitted bool
}

func (agg *AggBatch) WithChange(id string, state any) error {
	serialized, err := utils.Marshal(state, json.Marshal)
	if err != nil {
		return err
	}
	agg.batch[id].WithChange(serialized)
	agg.hasUncommitted = true
	return nil
}

func (agg *AggBatch) State(id string) ([]byte, string) {
	return agg.batch[id].CurrentState()
}
func (agg *AggBatch) History(id string) []Event {
	return agg.batch[id].History()
}

type Agg struct {
	Events      []Event
	uncommitted [][]byte
	Version     string
}

func (agg *Agg) History() []Event {
	events := make([]Event, 0)
	for _, evnt := range agg.Events {
		if evnt.Uncommitted {
			continue
		}
		events = append(events, evnt)
	}
	return events
}

func (agg *Agg) CurrentState() ([]byte, string) {
	for _, evnt := range agg.Events {
		if !evnt.Uncommitted {
			return evnt.State, agg.Version
		}
	}
	return nil, agg.Version
}

func (agg *Agg) WithChange(state []byte) {
	agg.uncommitted = append(agg.uncommitted, state)
}

func (agg *Agg) eventsWith(transactionID *string, committed bool, startedAt time.Time) []Event {
	pendingEvents := make([]Event, len(agg.uncommitted))

	for idx, uncommitted := range agg.uncommitted {
		event := Event{
			State:         uncommitted,
			Uncommitted:   !committed,
			TransactionID: transactionID,
			Deleted:       uncommitted == nil,
			CreatedAt:     startedAt,
		}
		pendingEvents[len(pendingEvents)-idx-1] = event // back to front
	}
	return append(pendingEvents, agg.Events...)
}

func (agg *Agg) preparePersist(transactionID *string, committed bool, startedAt time.Time) keyvalue.Value[State] {
	return keyvalue.Value[State]{
		Content: State{
			Events: agg.eventsWith(transactionID, committed, startedAt),
			Dirty:  !committed,
		},
		Version: agg.Version,
	}
}

func (agg *Agg) preparePersistNoDirty() keyvalue.Value[State] {
	return agg.preparePersist(nil, true, time.Now().UTC())
}
