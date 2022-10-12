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
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dapr/components-contrib/state/utils"
	"github.com/mcandeia/dapr-components/ledger/keyvalue"
	"github.com/mcandeia/dapr-components/ledger/transaction"
)

type Service interface {
	// GetBatch returns aggregates and a function that persist aggregate changes
	GetBatch(ctx context.Context, ids []string) (aggs *AggBatch, persist func() error, err error)
}

type Event struct {
	State         []byte    `json:"state"`
	Uncommitted   bool      `json:"uncommitted"`
	TransactionID *string   `json:"transactionId"`
	Deleted       bool      `json:"deleted"`
	CreatedAt     time.Time `json:"createdAt"`
}

func (evt Event) confirmed() Event {
	return Event{
		State:         evt.State,
		Uncommitted:   false,
		TransactionID: evt.TransactionID,
		Deleted:       evt.Deleted,
		CreatedAt:     evt.CreatedAt,
	}
}

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

type State struct {
	Events []Event `json:"events"`
	Dirty  bool    `json:"dirty"`
}

type svc struct {
	events keyvalue.Versioned[State]
	tSvc   transaction.Service
}

type uncommitted struct {
	agg      *Agg
	eventIdx int
	event    Event
}

func (u *uncommitted) apply(t *transaction.Transaction) {
	if t.Aborted() {
		// remove aborted event
		u.agg.Events = append(u.agg.Events[:u.eventIdx], u.agg.Events[u.eventIdx+1:]...)
	} else {
		u.agg.Events[u.eventIdx] = u.event.confirmed()
	}
}

func (s *svc) commitOnTransaction(ctx context.Context, aggs map[string]*Agg) error {
	if len(aggs) == 1 { // no transaction required
		var firstKey string
		for k := range aggs {
			firstKey = k
		}
		return s.events.BulkSet(ctx, map[string]keyvalue.Value[State]{
			firstKey: aggs[firstKey].preparePersistNoDirty(),
		})
	}

	aggsKeys := make([]string, len(aggs))
	err := s.tSvc.WithinTransaction(ctx, func(transactionID string, startedAt time.Time) error {
		uncommittedChanges := make(map[string]keyvalue.Value[State])
		idx := 0
		for key, agg := range aggs {
			uncommittedChanges[key] = agg.preparePersist(&transactionID, false, startedAt)
			aggsKeys[idx] = key
			idx++
		}

		return s.events.BulkSet(ctx, uncommittedChanges)
	})
	if err != nil {
		return err
	}

	go func() {
		_, commit, err := s.GetBatch(context.Background(), aggsKeys)
		if err != nil {
			return
		}
		// commit no dirty
		commit() // ignore err
	}()
	return nil
}

func (s *svc) GetBatch(ctx context.Context, ids []string) (aggs *AggBatch, persist func() error, err error) {
	states, err := s.events.BulkGet(context.TODO(), ids)
	if err != nil {
		return nil, nil, err
	}

	aggs = &AggBatch{
		batch: make(map[string]*Agg),
	}
	eventsToCheck := make(map[string][]*uncommitted)

	transactionsKeys := make([]string, 0)
	for key, state := range states {
		aggs.batch[key] = &Agg{
			Events:  state.Content.Events,
			Version: state.Version,
		}

		// should check for uncommitted events
		if !state.Content.Dirty {
			continue
		}
		for idx, event := range state.Content.Events {
			if event.Uncommitted && event.TransactionID != nil {
				transactionsKeys = append(transactionsKeys, *event.TransactionID)
				eventsToCheck[*event.TransactionID] = append(eventsToCheck[*event.TransactionID], &uncommitted{
					agg:      aggs.batch[key],
					eventIdx: idx,
					event:    event,
				})
			}
		}
	}

	transactions, err := s.tSvc.BulkGet(ctx, transactionsKeys)
	if err != nil {
		return nil, nil, err
	}

	for transactionID, events := range eventsToCheck {
		transaction, ok := transactions[transactionID]
		if !ok || transaction == nil {
			return nil, nil, fmt.Errorf("transaction %s not found", transactionID)
		}

		if transaction.IsOngoing() {
			continue
		}

		for _, event := range events {
			event.apply(transaction)
		}
	}

	return aggs, func() error {
		if aggs.hasUncommitted {
			return s.commitOnTransaction(ctx, aggs.batch)
		}
		uncommittedChanges := make(map[string]keyvalue.Value[State])
		for key, agg := range aggs.batch {
			uncommittedChanges[key] = agg.preparePersistNoDirty()
		}
		return s.events.BulkSet(ctx, uncommittedChanges)
	}, nil
}

func NewService(tSvc transaction.Service, events keyvalue.Versioned[State]) Service {
	return &svc{
		tSvc:   tSvc,
		events: events,
	}
}
