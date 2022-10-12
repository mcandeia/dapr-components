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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	contribBindings "github.com/dapr/components-contrib/bindings"
	"github.com/dapr/components-contrib/state"
	"github.com/dapr/kit/ptr"
	"github.com/mcandeia/dapr-components/ledger/aggregate"
)

const (
	historyOperation = "history"
	keyMetadataKey   = "key"
)

type journal struct {
	*ledger
}

func (j *journal) Init(metadata contribBindings.Metadata) error {
	j.ledger = New()
	return j.ledger.Init(state.Metadata{
		Base: metadata.Base,
	})
}

type RawBytes []byte

func (r RawBytes) MarshalJSON() ([]byte, error) {
	return r, nil
}

type EventData struct {
	State RawBytes  `json:"state"`
	Date  time.Time `json:"date"`
}

func (j *journal) Invoke(ctx context.Context, req *contribBindings.InvokeRequest) (*contribBindings.InvokeResponse, error) {
	key, ok := req.Metadata[keyMetadataKey]
	if !ok {
		return nil, errors.New("`key` metadata must be provided.")
	}

	switch req.Operation {
	case historyOperation:
		events, err := j.history(key)
		if err != nil {
			return nil, err
		}
		result := make([]EventData, len(events))

		for idx, evnt := range events {
			result[idx] = EventData{
				State: evnt.State,
				Date:  evnt.CreatedAt,
			}
		}
		bts, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}

		return &contribBindings.InvokeResponse{
			Data:        bts,
			ContentType: ptr.Of("application/json"),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported operation %s", req.Operation)
	}
}
func (j *journal) Operations() []contribBindings.OperationKind {
	return []contribBindings.OperationKind{historyOperation}
}

func (j *journal) history(key string) ([]aggregate.Event, error) {
	batch, _, err := j.ledger.agg.GetBatch(context.Background(), []string{key})
	if err != nil {
		return nil, err
	}
	return batch.History(key), nil
}

func NewJournal() *journal {
	return &journal{}
}
