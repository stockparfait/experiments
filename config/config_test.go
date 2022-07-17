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
	"encoding/json"
	"testing"

	"github.com/stockparfait/stockparfait/db"

	. "github.com/smartystreets/goconvey/convey"
)

func testJSON(js string) interface{} {
	var res interface{}
	if err := json.Unmarshal([]byte(js), &res); err != nil {
		panic(err)
	}
	return res
}

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey("Config works correctly", t, func() {
		Convey("top-level config, the usual case", func() {
			var c Config
			So(c.InitMessage(testJSON(`{
  "groups": [
    {
       "id": "real",
       "title": "Real Group",
       "x_log_scale": true,
       "graphs": [
         {"id": "r1", "title": "Real One", "x_label": "points"},
         {"id": "r2", "x_label": "points", "y_log_scale": true}
       ]
    },
    {
       "id": "time",
       "timeseries": true,
       "graphs": [
         {"id": "t1", "title": "Time One", "x_label": "dates"},
         {"id": "t2", "x_label": "dates", "y_log_scale": true}
       ]
    }
  ],
  "experiments": [
    {"test": {"passed": true, "graph": "r1"}},
    {"hold": {"data": {"DB": "test"}}},
    {"distribution": {"data": {"DB": "test"}, "graph": "dist"}}
  ]
  }`)), ShouldBeNil)

			var defaultData db.DataConfig
			So(defaultData.InitMessage(testJSON(`{"DB": "test"}`)), ShouldBeNil)
			var defaultBuckets Buckets
			So(defaultBuckets.InitMessage(testJSON(`{}`)), ShouldBeNil)

			So(c, ShouldResemble, Config{
				Groups: []*Group{
					{
						Timeseries: false,
						ID:         "real",
						Title:      "Real Group",
						XLogScale:  true,
						Graphs: []*Graph{
							{
								ID:        "r1",
								Title:     "Real One",
								XLabel:    "points",
								YLogScale: false,
							},
							{
								ID:        "r2",
								Title:     "",
								XLabel:    "points",
								YLogScale: true,
							},
						},
					},
					{
						Timeseries: true,
						ID:         "time",
						Title:      "time",
						XLogScale:  false,
						Graphs: []*Graph{
							{
								ID:        "t1",
								Title:     "Time One",
								XLabel:    "dates",
								YLogScale: false,
							},
							{
								ID:        "t2",
								Title:     "",
								XLabel:    "dates",
								YLogScale: true,
							},
						},
					},
				},
				Experiments: []*ExpMap{
					{Config: &TestExperimentConfig{
						Grade:  2.0,
						Passed: true,
						Graph:  "r1",
					}},
					{Config: &Hold{
						Data: defaultData,
					}},
					{Config: &Distribution{
						Data:      defaultData,
						Buckets:   defaultBuckets,
						Graph:     "dist",
						Normalize: true,
					}},
				},
			})

			So(c.Experiments[0].Config.Name(), ShouldEqual, "test")
		})

		Convey("x log-scale for timeseries is an error", func() {
			var c Config
			err := c.InitMessage(testJSON(`
{
  "groups": [{
    "timeseries": true,
    "id": "time", "x_log_scale": true,
    "graphs": [{"id": "g"}]
  }]
}`))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring,
				"timeseries group 'time' cannot have log-scale X")
		})

		Convey("duplicate group id is an error", func() {
			var c Config
			err := c.InitMessage(testJSON(`
{
  "groups": [
    {"id": "gp1", "graphs": [{"id": "r1"}, {"id": "r2"}]},
    {"id": "gp1", "graphs": [{"id": "r2"}]}
  ]
}`))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring,
				"group[1] has a duplicate id 'gp1'")
		})

		Convey("duplicate graph IDs across groups is an error", func() {
			var c Config
			err := c.InitMessage(testJSON(`
{
  "groups": [
    {"id": "gp1", "graphs": [{"id": "r1"}, {"id": "r2"}]},
    {"id": "gp2", "graphs": [{"id": "r1"}]}
  ]
}`))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring,
				"graph[0] in group 'gp2' has a duplicate id 'r1'")
		})

		Convey("group without ID is an error", func() {
			var c Config
			err := c.InitMessage(testJSON(`{"groups": [{"graphs": [{"id": "r1"}]}]}`))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "group must have a non-empty ID")
		})

		Convey("graph without ID is an error", func() {
			var c Config
			err := c.InitMessage(testJSON(`{"groups": [{"id": "g", "graphs": [{}]}]}`))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "graph must have a non-empty ID")
		})

		Convey("multi-key experiment map is an error", func() {
			var c Config
			err := c.InitMessage(testJSON(`
{
  "groups": [{"id": "g", "graphs": [{"id": "a"}]}],
  "experiments": [{"test": {}, "extra": {}}]
}`))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring,
				"experiment must be a single-element map")
		})

		Convey("unknown experiment is an error", func() {
			var c Config
			err := c.InitMessage(testJSON(`
{
  "groups": [{"id": "g", "graphs": [{"id": "a"}]}],
  "experiments": [{"foobar": {}}]
}`))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "unknown experiment foobar")
		})
	})

	Convey("Individual Experiment configs work", t, func() {

		Convey("Hold", func() {
			Convey("normal case", func() {
				var h Hold
				js := `
{
  "data": {"DB": "test"},
  "positions": [
    {"ticker": "A", "shares": 2.5},
    {"ticker": "B", "start value": 1000.0}
  ],
  "positions graph": "positions",
  "total graph": "total"
}`
				So(h.InitMessage(testJSON(js)), ShouldBeNil)
				var data db.DataConfig
				So(data.InitMessage(testJSON(`{"DB": "test"}`)), ShouldBeNil)
				So(h, ShouldResemble, Hold{
					Data: data, // must be initialized with its default values
					Positions: []HoldPosition{
						{
							Ticker: "A",
							Shares: 2.5,
						},
						{
							Ticker:     "B",
							StartValue: 1000.0,
						},
					},
					PositionsGraph: "positions",
					TotalGraph:     "total",
				})
			})

			Convey("shares and start value are checked", func() {
				var p HoldPosition
				So(p.InitMessage(testJSON(`{"ticker": "A"}`)), ShouldNotBeNil)
				So(p.InitMessage(testJSON(
					`{"ticker": "A", "shares": 1, "start value": 1}`)), ShouldNotBeNil)
			})
		})
	})
}
