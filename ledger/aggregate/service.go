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

type State struct {
	Events []Event `json:"events"`
	Dirty  bool    `json:"dirty"`
}

type svc struct {
	events keyvalue.Versioned[State]
	tSvc   transaction.Service
}

func (s *svc) commitOnTransaction(ctx context.Context, aggs map[string]*Agg) error {
	if len(aggs) == 0 {
		return nil
	}
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
