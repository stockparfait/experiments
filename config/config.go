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

// Package config implements configuration schema for all the experiments.
package config

import (
	"runtime"

	"github.com/stockparfait/errors"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/message"
	"github.com/stockparfait/stockparfait/stats"
)

// ExperimentConfig is a custom configuration for an experiment.
type ExperimentConfig interface {
	message.Message
	Name() string
}

// TestExperimentConfig is only used in tests.
type TestExperimentConfig struct {
	Grade  float64 `json:"grade" default:"2.0"`
	Passed bool    `json:"passed"`
	Graph  string  `json:"graph" required:"true"`
}

var _ ExperimentConfig = &TestExperimentConfig{}

// Name implements ExperimentConfig.
func (t *TestExperimentConfig) Name() string { return "test" }

// InitMessage implements message.Message.
func (t *TestExperimentConfig) InitMessage(js interface{}) error {
	return errors.Annotate(message.Init(t, js), "failed to parse test config")
}

// HoldPosition configures a single position within the Hold portfolio. Exactly
// one of "shares" (possibly fractional) or "start value" (the initial market
// value at Hold.Data.Start date) must be non-zero.
type HoldPosition struct {
	Ticker     string  `json:"ticker" required:"true"`
	Shares     float64 `json:"shares"`
	StartValue float64 `json:"start value"`
}

func (p *HoldPosition) InitMessage(js interface{}) error {
	if err := message.Init(p, js); err != nil {
		return errors.Annotate(err, "failed to parse HoldPosition")
	}
	if (p.Shares == 0.0) == (p.StartValue == 0.0) {
		return errors.Reason(
			`exactly one of "shares" or "start value" must be non-zero for ticker %s`,
			p.Ticker)
	}
	return nil
}

// Hold "experiment" configuration.
type Hold struct {
	Reader         *db.Reader     `json:"data" required:"true"`
	Positions      []HoldPosition `json:"positions"`
	PositionsGraph string         `json:"positions graph"` // plots per position
	PositionsAxis  string         `json:"positions axis" choices:"left,right" default:"right"`
	TotalGraph     string         `json:"total graph"` // plot portfolio value
	TotalAxis      string         `json:"total axis" choices:"left,right" default:"right"`
}

var _ ExperimentConfig = &Hold{}

func (h *Hold) InitMessage(js interface{}) error {
	return errors.Annotate(message.Init(h, js), "failed to parse Hold config")
}

func (h *Hold) Name() string { return "hold" }

// AnalyticalDistribution configures the type and parameters of a distibution.
type AnalyticalDistribution struct {
	Name  string  `json:"name" required:"true" choices:"t,normal"`
	Mean  float64 `json:"mean" default:"0.0"`
	MAD   float64 `json:"MAD" default:"1.0"`
	Alpha float64 `json:"alpha" default:"3.0"` // T dist. parameter
}

var _ message.Message = &AnalyticalDistribution{}

func (d *AnalyticalDistribution) InitMessage(js interface{}) error {
	if err := message.Init(d, js); err != nil {
		return errors.Annotate(err, "failed to init AnalyticalDistribution")
	}
	if d.Name == "t" && d.Alpha <= 1.0 {
		return errors.Reason("T-distribution requires alpha=%f > 1.0", d.Alpha)
	}
	if d.MAD <= 0.0 {
		return errors.Reason("MAD=%f must be positive", d.MAD)
	}
	return nil
}

// Distribution is the experiment config for deriving the distribution of
// log-profits. By default, it normalizes the log-profits to have 0.0 mean and
// 1.0 MAD; set "normalize" to false for the original distribution.  When
// plotting the reference (analytical) distribution for non-normalized samples,
// setting "adjust reference distribution" flag sets the mean and MAD of the
// reference to that of the sample.
type Distribution struct {
	Reader           *db.Reader              `json:"data" required:"true"`
	Buckets          stats.Buckets           `json:"buckets"`
	UseMeans         bool                    `json:"use means"`  // use bucket means rather than middles
	KeepZeros        bool                    `json:"keep zeros"` // by default, skip y==0 points
	Graph            string                  `json:"graph" required:"true"`
	ChartType        string                  `json:"chart type" choices:"line,bars" default:"line"`
	SamplesGraph     string                  `json:"samples graph"`
	SamplesRightAxis bool                    `json:"samples right axis"`
	Normalize        bool                    `json:"normalize" default:"true"`
	RefDist          *AnalyticalDistribution `json:"reference distribution"`
	AdjustRef        bool                    `json:"adjust reference distribution"`
	BatchSize        int                     `json:"batch size" default:"10"` // must be >0
	Workers          int                     `json:"parallel workers"`        // >0; default = 2*runtime.NumCPU()
}

var _ ExperimentConfig = &Distribution{}

func (e *Distribution) InitMessage(js interface{}) error {
	if err := message.Init(e, js); err != nil {
		return errors.Annotate(err, "failed to init Distribution")
	}
	if e.BatchSize <= 0 {
		return errors.Reason("batch size = %d must be positive", e.BatchSize)
	}
	if e.Workers <= 0 {
		e.Workers = 2 * runtime.NumCPU()
	}
	return nil
}

func (e *Distribution) Name() string { return "distribution" }

// ExpMap represents a Message which reads a single-element map {name:
// Experiment} and knows how to populate specific implementations of the
// Experiment interface.
type ExpMap struct {
	Config ExperimentConfig `json:"-"` // populated directly in Init
}

var _ message.Message = &ExpMap{}

func (e *ExpMap) InitMessage(js interface{}) error {
	m, ok := js.(map[string]interface{})
	if !ok || len(m) != 1 {
		return errors.Reason("experiment must be a single-element map: %v", js)
	}
	for name, jsConfig := range m {
		switch name { // add specific experiment implementations here
		case "test":
			e.Config = &TestExperimentConfig{}
		case "hold":
			e.Config = &Hold{}
		case "distribution":
			e.Config = &Distribution{}
		default:
			return errors.Reason("unknown experiment %s", name)
		}
		return errors.Annotate(e.Config.InitMessage(jsConfig),
			"failed to parse experiment config")
	}
	return nil
}

// Graph is a config for a plot graph, a single canvas holding potentially
// multiple plots.
type Graph struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	XLabel    string `json:"x_label"`
	YLogScale bool   `json:"y_log_scale"`
}

var _ message.Message = &Graph{}

// InitMessage implements message.Message.
func (g *Graph) InitMessage(js interface{}) error {
	if err := message.Init(g, js); err != nil {
		return errors.Annotate(err, "cannot parse graph")
	}
	if g.ID == "" {
		return errors.Reason("graph must have a non-empty ID")
	}
	return nil
}

// Group is a config for a group of plots with a common X axis.
type Group struct {
	Timeseries bool     `json:"timeseries"`
	ID         string   `json:"id"`
	Title      string   `json:"title"` // default: same as ID
	XLogScale  bool     `json:"x_log_scale"`
	Graphs     []*Graph `json:"graphs"`
}

var _ message.Message = &Group{}

// InitMessage implements message.Message.
func (g *Group) InitMessage(js interface{}) error {
	if err := message.Init(g, js); err != nil {
		return errors.Annotate(err, "cannot parse group")
	}
	if g.ID == "" {
		return errors.Reason("group must have a non-empty ID")
	}
	if g.Title == "" {
		g.Title = g.ID
	}
	if len(g.Graphs) < 1 {
		return errors.Reason("at least one graph is required in group '%s'",
			g.ID)
	}
	if g.Timeseries && g.XLogScale {
		return errors.Reason("timeseries group '%s' cannot have log-scale X",
			g.ID)
	}
	return nil
}

// Config is the top-level configuration of the app.
type Config struct {
	Groups      []*Group  `json:"groups"`
	Experiments []*ExpMap `json:"experiments"`
}

var _ message.Message = &Config{}

func (c *Config) InitMessage(js interface{}) error {
	if err := message.Init(c, js); err != nil {
		return errors.Annotate(err, "failed to parse top-level config")
	}
	groups := make(map[string]struct{})
	graphs := make(map[string]struct{})
	for i, g := range c.Groups {
		if _, ok := groups[g.ID]; ok {
			return errors.Reason("group[%d] has a duplicate id '%s'", i, g.ID)
		}
		groups[g.ID] = struct{}{}
		for j, gr := range g.Graphs {
			if _, ok := graphs[gr.ID]; ok {
				return errors.Reason(
					"graph[%d] in group '%s' has a duplicate id '%s'",
					j, g.ID, gr.ID)
			}
			graphs[gr.ID] = struct{}{}
		}
	}
	return nil
}
