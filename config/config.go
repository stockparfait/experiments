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
	"github.com/stockparfait/errors"
	"github.com/stockparfait/stockparfait/message"
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

// Init implements message.Message.
func (t *TestExperimentConfig) Init(js interface{}) error {
	return errors.Annotate(message.Init(t, js), "failed to parse test config")
}

// ExpMap represents a Message which reads a single-element map {name:
// Experiment} and knows how to populate specific implementations of the
// Experiment interface.
type ExpMap struct {
	Config ExperimentConfig `json:"-"` // populated directly in Init
}

var _ message.Message = &ExpMap{}

func (e *ExpMap) Init(js interface{}) error {
	m, ok := js.(map[string]interface{})
	if !ok || len(m) != 1 {
		return errors.Reason("experiment must be a single-element map: %v", js)
	}
	for name, jsConfig := range m {
		switch name { // add specific experiment implementations here
		case "test":
			e.Config = &TestExperimentConfig{}
		default:
			return errors.Reason("unknown experiment %s", name)
		}
		return errors.Annotate(e.Config.Init(jsConfig),
			"failed to parse experiment config")
	}
	return nil
}

// Graph is a config for a plot graph, a single canvas holding potentially
// multiple plots.
type Graph struct {
	Name      string `json:"name"`
	Title     string `json:"title"`
	XLabel    string `json:"x_label"`
	YLogScale bool   `json:"y_log_scale"`
}

var _ message.Message = &Graph{}

// Init implements message.Message.
func (g *Graph) Init(js interface{}) error {
	if err := message.Init(g, js); err != nil {
		return errors.Annotate(err, "cannot parse graph")
	}
	if g.Name == "" {
		return errors.Reason("graph must have a name")
	}
	return nil
}

// Group is a config for a group of plots with a common X axis.
type Group struct {
	Timeseries bool     `json:"timeseries"`
	Name       string   `json:"name"`
	XLogScale  bool     `json:"x_log_scale"`
	Graphs     []*Graph `json:"graphs"`
}

var _ message.Message = &Group{}

// Init implements message.Message.
func (g *Group) Init(js interface{}) error {
	if err := message.Init(g, js); err != nil {
		return errors.Annotate(err, "cannot parse group")
	}
	if g.Name == "" {
		return errors.Reason("group must have a name")
	}
	if len(g.Graphs) < 1 {
		return errors.Reason("at least one graph is required in group '%s'",
			g.Name)
	}
	if g.Timeseries && g.XLogScale {
		return errors.Reason("timeseries group '%s' cannot have log-scale X",
			g.Name)
	}
	return nil
}

// Config is the top-level configuration of the app.
type Config struct {
	Groups      []*Group  `json:"groups"`
	Experiments []*ExpMap `json:"experiments"`
}

var _ message.Message = &Config{}

func (c *Config) Init(js interface{}) error {
	if err := message.Init(c, js); err != nil {
		return errors.Annotate(err, "failed to parse top-level config")
	}
	if len(c.Groups) < 1 {
		return errors.Reason("at least one group is required")
	}
	groups := make(map[string]struct{})
	graphs := make(map[string]struct{})
	for i, g := range c.Groups {
		if _, ok := groups[g.Name]; ok {
			return errors.Reason("group[%d] has a duplicate name '%s'", i, g.Name)
		}
		groups[g.Name] = struct{}{}
		for j, gr := range g.Graphs {
			if _, ok := graphs[gr.Name]; ok {
				return errors.Reason(
					"graph[%d] in group '%s' has a duplicate name '%s'",
					j, g.Name, gr.Name)
			}
			graphs[gr.Name] = struct{}{}
		}
	}
	return nil
}
