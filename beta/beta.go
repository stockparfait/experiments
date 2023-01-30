// Copyright 2023 Stock Parfait

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package beta is an experiment with cross-correlation between stocks.
//
// Specifically, it models a stock as P = beta*I+R relative to the reference
// price series I (typically, an index such as S&P500 or Nasdaq Composite) and
// studies the properties of beta and R.
package beta

import (
	"context"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
)

type Beta struct {
	config  *config.Beta
	context context.Context
}

var _ experiments.Experiment = &Beta{}

func (e *Beta) Prefix(s string) string {
	return experiments.Prefix(e.config.ID, s)
}

func (e *Beta) AddValue(ctx context.Context, k, v string) error {
	return experiments.AddValue(ctx, e.config.ID, k, v)
}

func (e *Beta) Run(ctx context.Context, cfg config.ExperimentConfig) error {
	var ok bool
	if e.config, ok = cfg.(*config.Beta); !ok {
		return errors.Reason("unexpected config type: %T", cfg)
	}
	e.context = ctx
	if e.config.RefData != nil {
		if err := e.processRefData(); err != nil {
			return errors.Annotate(err, "failed to process reference data")
		}
	}
	if e.config.RefAnalytical != nil {
		if err := e.processRefAnalytical(); err != nil {
			return errors.Annotate(err, "failed to process synthetic reference")
		}
	}
	if e.config.Data != nil {
		if err := e.processData(); err != nil {
			return errors.Annotate(err, "failed to process price data")
		}
	}
	if e.config.AnalyticalR != nil {
		if err := e.processAnalyticalR(); err != nil {
			return errors.Annotate(err, "failed to process synthetic R")
		}
	}
	return nil
}

func (e *Beta) processRefData() error {
	return nil
}

func (e *Beta) processRefAnalytical() error {
	return nil
}

func (e *Beta) processData() error {
	return nil
}

func (e *Beta) processAnalyticalR() error {
	return nil
}
