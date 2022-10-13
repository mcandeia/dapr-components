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
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mcandeia/dapr-components/ledger/keyvalue"
	"github.com/mcandeia/dapr-components/ledger/transaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeTransactionSvc struct {
	bulkGetCalled             atomic.Int64
	bulkGetResp               map[string]*transaction.Transaction
	bulkGetErr                error
	onBulkGetCalled           func([]string)
	withinTransactionCalled   atomic.Int64
	withinTransactionErr      error
	onWithinTransactionCalled func(func(transactionID string, startedAt time.Time) error) error
}

func (f *fakeTransactionSvc) BulkGet(ctx context.Context, ids []string) (map[string]*transaction.Transaction, error) {
	f.bulkGetCalled.Add(1)
	if f.onBulkGetCalled != nil {
		f.onBulkGetCalled(ids)
	}
	return f.bulkGetResp, f.bulkGetErr
}
func (f *fakeTransactionSvc) WithinTransaction(_ context.Context, funcs func(transactionID string, startedAt time.Time) error) error {
	f.withinTransactionCalled.Add(1)
	if f.onWithinTransactionCalled != nil {
		return f.onWithinTransactionCalled(funcs)
	}
	return f.withinTransactionErr
}

type fakeEvents struct {
	bulkGetCalled   atomic.Int64
	bulkGetResp     map[string]keyvalue.Value[State]
	bulkGetErr      error
	onBulkGetCalled func([]string)
	bulkSetCalled   atomic.Int64
	bulkSetErr      error
	onBulkSetCalled func(map[string]keyvalue.Value[State])
}

func (f *fakeEvents) BulkGet(ctx context.Context, keys []string) (map[string]keyvalue.Value[State], error) {
	f.bulkGetCalled.Add(1)
	if f.onBulkGetCalled != nil {
		f.onBulkGetCalled(keys)
	}
	return f.bulkGetResp, f.bulkGetErr
}

func (f *fakeEvents) BulkSet(ctx context.Context, values map[string]keyvalue.Value[State]) error {
	f.bulkSetCalled.Add(1)
	if f.onBulkSetCalled != nil {
		f.onBulkSetCalled(values)
	}
	return f.bulkSetErr
}

func TestService(t *testing.T) {
	t.Run("commitOnTransaction should not commit when empty changes are privided", func(t *testing.T) {
		service := &svc{}
		assert.Nil(t, service.commitOnTransaction(context.Background(), nil))
	})
	t.Run("commitOnTransaction should return error when bulkset returns an error", func(t *testing.T) {
		const fakeID = "fake-id"
		fakeErr := errors.New("fake-set-kv-err")
		events := &fakeEvents{
			bulkSetErr: fakeErr,
		}
		service := &svc{
			events: events,
		}
		assert.Equal(t, service.commitOnTransaction(context.Background(), map[string]*Agg{
			fakeID: {},
		}), fakeErr)
		assert.Equal(t, int64(1), events.bulkSetCalled.Load())
	})

	t.Run("commitOnTransaction should error when transaction returns an error", func(t *testing.T) {
		const fakeID1, fakeID2 = "fake-id-1", "fake-id-2"
		fakeErr := errors.New("fake-transaction-err")
		events := &fakeEvents{}
		transactionSvc := &fakeTransactionSvc{
			withinTransactionErr: fakeErr,
		}
		service := &svc{
			events: events,
			tSvc:   transactionSvc,
		}
		assert.Equal(t, service.commitOnTransaction(context.Background(), map[string]*Agg{
			fakeID1: {},
			fakeID2: {},
		}), fakeErr)
		assert.Equal(t, int64(0), events.bulkSetCalled.Load())
		assert.Equal(t, int64(1), transactionSvc.withinTransactionCalled.Load())
	})
	t.Run("commitOnTransaction should commit using transaction when multiple aggregates have changed", func(t *testing.T) {
		const fakeID1, fakeID2 = "fake-id-1", "fake-id-2"
		called := 0

		var waitForBackgroundCommit sync.WaitGroup
		waitForBackgroundCommit.Add(1)
		events := &fakeEvents{
			onBulkSetCalled: func(_ map[string]keyvalue.Value[State]) {
				called++
				if called == 2 {
					waitForBackgroundCommit.Done()
				}
			},
		}
		transactionSvc := &fakeTransactionSvc{
			onWithinTransactionCalled: func(f func(transactionID string, startedAt time.Time) error) error {
				return f("", time.Now().UTC())
			},
		}
		service := &svc{
			tSvc:   transactionSvc,
			events: events,
		}
		require.NoError(t, service.commitOnTransaction(context.Background(), map[string]*Agg{
			fakeID1: {},
			fakeID2: {},
		}))
		waitForBackgroundCommit.Wait()
		assert.Equal(t, int64(2), events.bulkSetCalled.Load())
		assert.Equal(t, int64(1), transactionSvc.withinTransactionCalled.Load())
	})
	t.Run("GetBatch should return error when event transaction is not found", func(t *testing.T) {
		fakeID := "fake-id"
		events := &fakeEvents{
			bulkGetResp: map[string]keyvalue.Value[State]{
				fakeID: {
					Content: State{
						Events: []Event{{
							State:         []byte{},
							Uncommitted:   true,
							TransactionID: &fakeID,
							Deleted:       false,
							CreatedAt:     time.Time{},
						}},
						Dirty: true,
					},
					Version: "",
				},
			},
		}
		transactionSvc := &fakeTransactionSvc{}
		service := &svc{
			tSvc:   transactionSvc,
			events: events,
		}
		_, _, err := service.GetBatch(context.TODO(), []string{})
		assert.EqualError(t, err, fmt.Sprintf("transaction %s not found", fakeID))
		assert.Equal(t, int64(1), events.bulkGetCalled.Load())
		assert.Equal(t, int64(1), transactionSvc.bulkGetCalled.Load())
	})
	t.Run("GetBatch should return only valid events", func(t *testing.T) {
		fakeID := "fake-id"
		events := &fakeEvents{
			bulkGetResp: map[string]keyvalue.Value[State]{
				fakeID: {
					Content: State{
						Events: []Event{{
							State:         []byte{},
							Uncommitted:   true,
							TransactionID: &fakeID,
							Deleted:       false,
							CreatedAt:     time.Time{},
						}},
						Dirty: true,
					},
					Version: "",
				},
			},
		}
		transactionSvc := &fakeTransactionSvc{
			bulkGetResp: map[string]*transaction.Transaction{
				fakeID: {
					Status:    transaction.STARTED,
					StartedAt: time.Now().UTC().Add(-time.Hour),
				},
			},
		}
		service := &svc{
			tSvc:   transactionSvc,
			events: events,
		}
		aggs, _, err := service.GetBatch(context.TODO(), []string{})
		require.NoError(t, err)
		assert.Empty(t, aggs.History(fakeID))
		assert.Equal(t, int64(1), events.bulkGetCalled.Load())
		assert.Equal(t, int64(1), transactionSvc.bulkGetCalled.Load())
	})
	t.Run("GetBatch should check for committed transactions", func(t *testing.T) {
		fakeID := "fake-id"
		events := &fakeEvents{
			bulkGetResp: map[string]keyvalue.Value[State]{
				fakeID: {
					Content: State{
						Events: []Event{{
							State:         []byte{},
							Uncommitted:   true,
							TransactionID: &fakeID,
							Deleted:       false,
							CreatedAt:     time.Time{},
						}},
						Dirty: true,
					},
					Version: "",
				},
			},
		}
		transactionSvc := &fakeTransactionSvc{
			bulkGetResp: map[string]*transaction.Transaction{
				fakeID: {
					Status: transaction.COMMITTED,
				},
			},
		}
		service := &svc{
			tSvc:   transactionSvc,
			events: events,
		}
		aggs, _, err := service.GetBatch(context.TODO(), []string{})
		require.NoError(t, err)
		assert.NotEmpty(t, aggs.History(fakeID))
		assert.Equal(t, int64(1), events.bulkGetCalled.Load())
		assert.Equal(t, int64(1), transactionSvc.bulkGetCalled.Load())
	})
	t.Run("Persist after GetBatch should commit uncommitted events", func(t *testing.T) {
		fakeID := "fake-id"
		events := &fakeEvents{
			bulkGetResp: map[string]keyvalue.Value[State]{
				fakeID: {
					Content: State{
						Events: []Event{{
							State:         []byte{},
							Uncommitted:   true,
							TransactionID: &fakeID,
							Deleted:       false,
							CreatedAt:     time.Time{},
						}},
						Dirty: true,
					},
					Version: "",
				},
			},
		}
		transactionSvc := &fakeTransactionSvc{
			bulkGetResp: map[string]*transaction.Transaction{
				fakeID: {
					Status: transaction.COMMITTED,
				},
			},
		}
		service := &svc{
			tSvc:   transactionSvc,
			events: events,
		}
		aggs, persist, err := service.GetBatch(context.TODO(), []string{})
		require.NoError(t, err)
		assert.NotEmpty(t, aggs.History(fakeID))
		assert.Equal(t, int64(1), events.bulkGetCalled.Load())
		assert.Equal(t, int64(1), transactionSvc.bulkGetCalled.Load())
		assert.Equal(t, int64(0), events.bulkSetCalled.Load())

		persist()

		assert.Equal(t, int64(1), events.bulkSetCalled.Load())
	})
	t.Run("Persist after GetBatch should commit on transaction when multiple aggregates have changed", func(t *testing.T) {
		fakeID := "fake-id"
		const fakeID2 = "fake-id-2"
		events := &fakeEvents{
			bulkGetResp: map[string]keyvalue.Value[State]{
				fakeID2: {
					Content: State{
						Events: []Event{},
						Dirty:  false,
					},
					Version: "",
				},
				fakeID: {
					Content: State{
						Events: []Event{{
							State:         []byte{},
							Uncommitted:   true,
							TransactionID: &fakeID,
							Deleted:       false,
							CreatedAt:     time.Time{},
						}},
						Dirty: true,
					},
					Version: "",
				},
			},
		}
		transactionSvc := &fakeTransactionSvc{
			onWithinTransactionCalled: func(f func(transactionID string, startedAt time.Time) error) error {
				return f("", time.Now())
			},
			bulkGetResp: map[string]*transaction.Transaction{
				fakeID: {
					Status: transaction.COMMITTED,
				},
			},
		}
		service := &svc{
			tSvc:   transactionSvc,
			events: events,
		}
		aggs, persist, err := service.GetBatch(context.TODO(), []string{})
		require.NoError(t, err)
		assert.NotEmpty(t, aggs.History(fakeID))
		assert.Equal(t, int64(1), events.bulkGetCalled.Load())
		assert.Equal(t, int64(1), transactionSvc.bulkGetCalled.Load())
		assert.Equal(t, int64(0), events.bulkSetCalled.Load())
		aggs.WithChange(fakeID2, []byte(`change`))

		persist()

		assert.Equal(t, int64(1), transactionSvc.withinTransactionCalled.Load())
		assert.Equal(t, int64(1), events.bulkSetCalled.Load())
	})
}
