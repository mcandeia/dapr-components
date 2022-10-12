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

package ledger

import (
	"context"
	"errors"
	"time"

	contribState "github.com/dapr/components-contrib/state"
	dapr "github.com/dapr/go-sdk/client"
	"github.com/dapr/kit/ptr"
	"github.com/mcandeia/dapr-components/internal"
	"github.com/mcandeia/dapr-components/ledger/aggregate"
	"github.com/mcandeia/dapr-components/ledger/keyvalue"
	"github.com/mcandeia/dapr-components/ledger/transaction"
)

const (
	daprPortMetadataKey   = "dapr-port"
	stateStoreMetadataKey = "dapr-store"
	defaultDaprPort       = "3500"
)

var errMissingStateStore = errors.New("`dapr-store` metadata must be provided")

type ledger struct {
	agg aggregate.Service
	ctx context.Context
}

func (s *ledger) Init(metadata contribState.Metadata) error {
	daprPort, ok := metadata.Properties[daprPortMetadataKey]
	if !ok {
		daprPort = defaultDaprPort
	}

	daprStateStore := metadata.Properties[stateStoreMetadataKey]

	if len(daprStateStore) == 0 {
		return errMissingStateStore
	}

	go func() {
		// FIXME racing condition between dapr initialization and component initialization
		<-time.After(4 * time.Second)
		daprClient, err := dapr.NewClientWithPort(daprPort)
		if err != nil {
			return
		}
		transactions := transaction.NewService(keyvalue.NewDaprState[transaction.Transaction](daprClient, daprStateStore, "transactions"))
		s.agg = aggregate.NewService(transactions, keyvalue.NewDaprState[aggregate.State](daprClient, daprStateStore, "events"))
		s.ctx = context.TODO()
	}()

	return nil
}

func (s *ledger) Multi(request *contribState.TransactionalStateRequest) error {
	keys := make([]string, len(request.Operations))
	uncommitted := make(map[string][]any)
	for idx, op := range request.Operations {
		if deleteReq, ok := op.Request.(contribState.DeleteRequest); ok {
			keys[idx] = deleteReq.Key
			// missing etag support
			uncommitted[deleteReq.Key] = append(uncommitted[deleteReq.Key], nil)
		} else {
			upsertReq := op.Request.(contribState.SetRequest)
			keys[idx] = upsertReq.Key
			// missing etag support
			uncommitted[upsertReq.Key] = append(uncommitted[upsertReq.Key], upsertReq.Value)
		}
		idx++
	}
	batch, persist, err := s.agg.GetBatch(s.ctx, keys)
	if err != nil {
		return err
	}

	for k, u := range uncommitted {
		for _, change := range u {
			if err := batch.WithChange(k, change); err != nil {
				return err
			}
		}
	}
	return persist()
}

func (s *ledger) Features() []contribState.Feature {
	return []contribState.Feature{contribState.FeatureTransactional, contribState.FeatureETag}
}

func (s *ledger) Delete(req *contribState.DeleteRequest) error {
	return s.Multi(&contribState.TransactionalStateRequest{
		Operations: []contribState.TransactionalStateOperation{{
			Operation: contribState.Delete,
			Request:   req,
		}},
	})
}

func (s *ledger) Set(req *contribState.SetRequest) error {
	return s.Multi(&contribState.TransactionalStateRequest{
		Operations: []contribState.TransactionalStateOperation{{
			Operation: contribState.Upsert,
			Request:   req,
		}},
	})
}

func (s *ledger) BulkDelete(req []contribState.DeleteRequest) error {
	ops := make([]contribState.TransactionalStateOperation, len(req))
	idx := 0
	for _, delete := range req {
		ops[idx] = contribState.TransactionalStateOperation{
			Operation: contribState.Delete,
			Request:   delete,
		}
		idx++
	}
	return s.Multi(&contribState.TransactionalStateRequest{
		Operations: ops,
	})
}

func (s *ledger) BulkSet(req []contribState.SetRequest) error {
	ops := make([]contribState.TransactionalStateOperation, len(req))
	idx := 0
	for _, set := range req {
		ops[idx] = contribState.TransactionalStateOperation{
			Operation: contribState.Upsert,
			Request:   set,
		}
		idx++
	}
	return s.Multi(&contribState.TransactionalStateRequest{
		Operations: ops,
	})
}

func (s *ledger) Get(req *contribState.GetRequest) (*contribState.GetResponse, error) {
	_, resp, err := s.BulkGet([]contribState.GetRequest{*req})
	if err != nil {
		return nil, err
	}

	state := resp[0]
	if len(state.Error) > 0 {
		return nil, errors.New(state.Error)
	}

	return &contribState.GetResponse{
		Data:        state.Data,
		ETag:        state.ETag,
		Metadata:    state.Metadata,
		ContentType: state.ContentType,
	}, nil
}

func (s *ledger) BulkGet(req []contribState.GetRequest) (bool, []contribState.BulkGetResponse, error) {
	keys := make([]string, len(req))
	for idx, r := range req {
		keys[idx] = r.Key
	}

	batch, persist, err := s.agg.GetBatch(s.ctx, keys)
	if err != nil {
		return true, nil, err
	}

	go persist()

	result := make([]contribState.BulkGetResponse, len(req))
	for idx, key := range keys {
		state, version := batch.State(key)
		result[idx] = contribState.BulkGetResponse{
			Key:  key,
			Data: state,
			ETag: &version,
			Error: internal.Ternary(err == nil, internal.Always(""), func() string {
				return err.Error()
			}),
			ContentType: ptr.Of("application/json"),
		}
	}

	return true, result, nil
}

func New() *ledger {
	return &ledger{}
}
