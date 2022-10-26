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

package component

import (
	dapr "github.com/dapr-sandbox/components-go-sdk"
	"github.com/dapr-sandbox/components-go-sdk/bindings/v1"
	"github.com/dapr-sandbox/components-go-sdk/state/v1"

	ledger "github.com/mcandeia/dapr-components/ledger/ledger"
)

func init() {
	dapr.Register("ledger",
		dapr.WithStateStore(func() state.Store {
			return ledger.New()
		}),
		dapr.WithOutputBinding(func() bindings.OutputBinding {
			return ledger.NewJournal()
		}),
	)
}
