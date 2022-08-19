// Copyright 2022 Stock Parfait

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package experiments

import (
	"context"
	"fmt"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/stockparfait/plot"
)

type contextKey int

const (
	valuesContextKey contextKey = iota
)

type Values = map[string]string

// UseValues injects the values map into the context. It is intended to be used
// by Experiments to add key:value pairs to be printed on the terminal at the
// end of the run.
func UseValues(ctx context.Context, v Values) context.Context {
	return context.WithValue(ctx, valuesContextKey, v)
}

// GetValues previously injected by UseValues, or nil.
func GetValues(ctx context.Context) Values {
	v, ok := ctx.Value(valuesContextKey).(Values)
	if !ok {
		return nil
	}
	return v
}

// AddValue adds (or overwrites) a key:value pair to the Values in the context.
// These pairs are intended to be printed on the terminal at the end of the run
// of all the experiments.
func AddValue(ctx context.Context, key, value string) error {
	v := GetValues(ctx)
	if v == nil {
		return errors.Reason("no values map in context")
	}
	v[key] = value
	return nil
}

// Experiment is a generic experiment interface. Each implementation is expected
// to add key:value pairs using AddValue, plots using plot.AddLeft/AddRight, or
// save data in files.
type Experiment interface {
	Run(ctx context.Context, cfg config.ExperimentConfig) error
}

// TestExperiment is a fake experiment used in tests. Define actual experiments
// in their own subpackages.
type TestExperiment struct {
	cfg *config.TestExperimentConfig
}

var _ Experiment = &TestExperiment{}

// Run implements Experiment.
func (t *TestExperiment) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	t.cfg, ok = cfg.(*config.TestExperimentConfig)
	if !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	if err := AddValue(ctx, "grade", fmt.Sprintf("%g", t.cfg.Grade)); err != nil {
		return errors.Annotate(err, "cannot add grade value")
	}
	passed := "failed"
	if t.cfg.Passed {
		passed = "passed"
	}
	if err := AddValue(ctx, "test", passed); err != nil {
		return errors.Annotate(err, "cannot add pass/fail value")
	}
	p, err := plot.NewXYPlot([]float64{1.0, 2.0}, []float64{21.5, 42.0})
	if err != nil {
		return errors.Annotate(err, "failed to create XY plot")
	}
	if err := plot.Add(ctx, p, t.cfg.Graph); err != nil {
		return errors.Annotate(err, "cannot add plot")
	}
	return nil
}
