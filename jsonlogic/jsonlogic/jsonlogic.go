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

package jsonlogic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	contribBindings "github.com/dapr/components-contrib/bindings"

	"github.com/dapr/kit/ptr"

	"github.com/diegoholiveira/jsonlogic"
)

const (
	// evaluateOperation is the operation to evaluate a jsonlogic rule.
	evaluateOperation = "evaluate"
)

// errJsonLogicExpressionMissing is returned when no jsonlogic expression is provided.
var errJsonLogicExpressionMissing = errors.New("jsonlogic expression is missing")

// evaluationRequest is a defined JsonLogic rule using component metadata
type evaluationRequest struct {
	// Data is the data related to the evaluation.
	Data any `json:"data"`
	// Expression is the JsonLogic raw expression.
	Expression any `json:"expression"`
}

// JsonLogicOutput is the JsonLogicOutput output binding to evaluate JsonLogicOutput expressions.
type JsonLogicOutput struct{}

// Init performs metadata parsing.
func (jl *JsonLogicOutput) Init(metadata contribBindings.Metadata) error {
	return nil
}

// evaluate gets the data and the logic expression and return the bindings invoke response after the rule evaluation against the received data.
func (jl *JsonLogicOutput) evaluate(evalReq *evaluationRequest) (*contribBindings.InvokeResponse, error) {
	rawJSON, err := jsonlogic.ApplyInterface(evalReq.Expression, evalReq.Data)
	if err != nil {
		return nil, err
	}

	b, err := json.Marshal(rawJSON)
	if err != nil {
		return nil, err
	}

	return &contribBindings.InvokeResponse{
		Data:        b,
		ContentType: ptr.Of("application/json"),
	}, nil
}

// Invoke is called for output bindings.
func (jl *JsonLogicOutput) Invoke(ctx context.Context, req *contribBindings.InvokeRequest) (*contribBindings.InvokeResponse, error) {
	data, err := strconv.Unquote(string(req.Data))
	if err != nil {
		return nil, err
	}
	var evalReq *evaluationRequest

	if err := json.Unmarshal([]byte(data), &evalReq); err != nil {
		return nil, err
	}

	switch req.Operation {
	case evaluateOperation:
		return jl.evaluate(evalReq)
	default:
		return nil, fmt.Errorf("unsupported operation %s", req.Operation)
	}
}

// Operations enumerates supported binding operations.
func (jl *JsonLogicOutput) Operations() []contribBindings.OperationKind {
	return []contribBindings.OperationKind{evaluateOperation}
}
