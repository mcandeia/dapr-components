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

package transaction

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mcandeia/dapr-components/ledger/keyvalue"
)

type TransactionStatus int

const (
	transactionTimeout = time.Minute
)
const (
	STARTED TransactionStatus = iota
	COMMITTED
)

type Transaction struct {
	Status      TransactionStatus `json:"status"`
	StartedAt   time.Time         `json:"startedAt"`
	CommittedAt time.Time         `json:"comittedAt"`
}

func (t Transaction) HasTimedOut() bool {
	return t.StartedAt.After(t.StartedAt.Add(transactionTimeout))
}

func (t Transaction) IsOngoing() bool {
	return t.Status == STARTED && !t.HasTimedOut()
}

func (t Transaction) Aborted() bool {
	return t.Status != COMMITTED && t.IsOngoing()
}

func (t Transaction) Commit() Transaction {
	return Transaction{
		Status:      COMMITTED,
		StartedAt:   t.StartedAt,
		CommittedAt: time.Now().UTC(),
	}
}

type Service interface {
	BulkGet(ctx context.Context, ids []string) (map[string]*Transaction, error)
	WithinTransaction(context.Context, func(transactionID string, startedAt time.Time) error) error
}

type svc struct {
	transactions keyvalue.Versioned[Transaction]
}

func (s *svc) BulkGet(ctx context.Context, ids []string) (map[string]*Transaction, error) {
	transactions, err := s.transactions.BulkGet(ctx, ids)
	if err != nil {
		return nil, err
	}

	result := make(map[string]*Transaction)
	for transactionKey, transaction := range transactions {
		result[transactionKey] = &Transaction{
			Status:    transaction.Content.Status,
			StartedAt: transaction.Content.StartedAt,
		}
	}
	return result, nil
}

func (s *svc) WithinTransaction(ctx context.Context, exec func(transactionID string, startedAt time.Time) error) error {
	startedAt := time.Now().UTC()
	transactionID := uuid.New().String()
	aggTransaction := Transaction{
		Status:    STARTED,
		StartedAt: startedAt,
	}
	errChan := make(chan error, 2)
	reqFinished := sync.WaitGroup{}
	reqFinished.Add(2)
	go func() {
		defer reqFinished.Done()
		if err := exec(transactionID, startedAt); err != nil {
			errChan <- err
		}
	}()

	go func() {
		defer reqFinished.Done()
		err := s.transactions.BulkSet(ctx, map[string]keyvalue.Value[Transaction]{
			transactionID: {
				Content: aggTransaction,
			},
		})
		if err != nil {
			errChan <- err
		}
	}()

	go func() {
		reqFinished.Wait()
		close(errChan)
	}()

	if err := <-errChan; err != nil {
		return err
	}

	if aggTransaction.Aborted() {
		return errors.New("transaction has timed out")
	}

	return s.transactions.BulkSet(ctx, map[string]keyvalue.Value[Transaction]{
		transactionID: {
			Content: aggTransaction.Commit(),
		},
	})
}

func NewService(transactions keyvalue.Versioned[Transaction]) Service {
	return &svc{
		transactions: transactions,
	}
}
