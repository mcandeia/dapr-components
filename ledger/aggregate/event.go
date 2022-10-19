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
	"time"

	"github.com/mcandeia/dapr-components/ledger/transaction"
)

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
