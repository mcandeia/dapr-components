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
	"fmt"
	"time"

	"github.com/mcandeia/dapr-components/ledger/keyvalue"
	"github.com/mcandeia/dapr-components/ledger/transaction"
)

type Service interface {
	// GetBatch returns aggregates and a function that persist aggregate changes
	GetBatch(ctx context.Context, ids []string) (aggs *AggBatch, persist func() error, err error)
}

type Event struct {
	State         any       `json:"state"`
	Uncommitted   bool      `json:"uncommitted"`
	TransactionID *string   `json:"transactionId"`
	Deleted       bool      `json:"deleted"`
	CreatedAt     time.Time `json:"createdAt"`
}

func (evt Event) confirmed() Event {
	return Event{
		State:         evt.State,
		Uncommitted:   evt.Uncommitted,
		TransactionID: evt.TransactionID,
		Deleted:       evt.Deleted,
		CreatedAt:     evt.CreatedAt,
	}
}

type AggBatch struct {
	batch          map[string]*Agg
	hasUncommitted bool
}

func (agg *AggBatch) WithChange(id string, state any) *AggBatch {
	agg.batch[id].WithChange(state)
	agg.hasUncommitted = true
	return agg
}

func (agg *AggBatch) State(id string) (any, string) {
	return agg.batch[id].CurrentState()
}
func (agg *AggBatch) History(id string) []Event {
	return agg.batch[id].History()
}

type Agg struct {
	Events      []Event
	uncommitted []any
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

func (agg *Agg) CurrentState() (any, string) {
	for _, evnt := range agg.Events {
		if !evnt.Uncommitted {
			return evnt.State, agg.Version
		}
	}
	return nil, agg.Version
}

func (agg *Agg) WithChange(state any) {
	agg.uncommitted = append(agg.uncommitted, state)
}

func (agg *Agg) Deleted() {
	agg.uncommitted = append(agg.uncommitted, nil)
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
		pendingEvents[idx] = event
	}
	return append(agg.Events, pendingEvents...)
}

type State struct {
	Events []Event `json:"events"`
	Dirty  bool    `json:"dirty"`
}

type svc struct {
	events keyvalue.Versioned[State]
	tSvc   transaction.Service
}

var noOpCommit = func() error {
	return nil
}

type uncommitted struct {
	agg      *Agg
	eventIdx int
	event    *Event
}

func (u *uncommitted) apply(t *transaction.Transaction) {
	if t.Aborted() {
		// remove aborted event
		u.agg.Events = append(u.agg.Events[:u.eventIdx], u.agg.Events[u.eventIdx+1:]...)
	} else {
		u.event.Uncommitted = false
	}
}

func (s *svc) commitOnTransaction(ctx context.Context, aggs map[string]*Agg) error {
	if len(aggs) == 1 { // no transaction required
		var firstKey string
		for k := range aggs {
			firstKey = k
		}
		first := aggs[firstKey]
		return s.events.BulkSet(ctx, map[string]keyvalue.Value[State]{
			firstKey: {
				Content: State{
					Events: first.eventsWith(nil, true, time.Now().UTC()),
					Dirty:  false,
				},
				Version: first.Version,
			},
		})
	}

	aggsKeys := make([]string, len(aggs))
	err := s.tSvc.WithinTransaction(ctx, func(transactionID string, startedAt time.Time) error {
		uncommittedChanges := make(map[string]keyvalue.Value[State])
		idx := 0
		for k, agg := range aggs {
			uncommittedChanges[k] = keyvalue.Value[State]{
				Content: State{
					Events: agg.eventsWith(&transactionID, false, startedAt),
					Dirty:  true,
				},
				Version: agg.Version,
			}
			aggsKeys[idx] = k
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
				eventCopy := &event //shallow copy due to loop
				eventsToCheck[*event.TransactionID] = append(eventsToCheck[*event.TransactionID], &uncommitted{
					agg:      aggs.batch[key],
					eventIdx: idx,
					event:    eventCopy,
				})
			}
		}
	}

	if len(transactionsKeys) == 0 {
		return aggs, noOpCommit, nil
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
		for k, agg := range aggs.batch {
			uncommittedChanges[k] = keyvalue.Value[State]{
				Content: State{
					Events: agg.Events,
					Dirty:  false,
				},
				Version: agg.Version,
			}
		}
		return s.events.BulkSet(ctx, uncommittedChanges)
	}, nil
}

func NewService(tSvc transaction.Service, events keyvalue.Versioned[State]) Service {
	return &svc{
		events: events,
	}
}
