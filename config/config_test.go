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
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/stats"
	"github.com/stockparfait/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	tmpdir, tmpdirErr := ioutil.TempDir("", "test_config")
	defer os.RemoveAll(tmpdir)

	Convey("Test setup succeeded", t, func() {
		So(tmpdirErr, ShouldBeNil)
	})

	Convey("Config works correctly", t, func() {
		Convey("top-level config, the usual case", func() {
			confJSON := `
{
  "groups": [
    {
       "id": "real",
       "title": "Real Group",
       "log scale X": true,
       "graphs": [
         {"id": "r1", "title": "Real One", "X label": "points"},
         {"id": "r2", "X label": "points", "log scale Y": true}
       ]
    },
    {
       "id": "time",
       "timeseries": true,
       "graphs": [
         {"id": "t1", "title": "Time One", "X label": "dates"},
         {"id": "t2", "X label": "dates", "log scale Y": true}
       ]
    }
  ],
  "experiments": [
    {"test": {"passed": true, "graph": "r1"}},
    {"hold": {"data": {"DB": "test"}}},
    {"distribution": {
      "data": {"DB": "test"},
      "log-profits": {
        "graph": "dist",
        "normalize": true,
        "reference distribution": {"analytical source": {"name": "t"}},
        "derive alpha": {
          "min x": 2,
          "max x": 4
        }
      },
      "parallel workers": 1
    }},
    {"power distribution": {
      "distribution": {"analytical source": {"name": "normal"}},
      "cumulative mean": {"graph": "cumul mean"}
    }}
  ]
}`

			confPath := filepath.Join(tmpdir, "config.json")

			// Run in a function closure to ensure the written file is closed before
			// reading it.
			(func() {
				f, err := os.OpenFile(confPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
				So(err, ShouldBeNil)
				defer f.Close()
				_, err = f.WriteString(confJSON)
				So(err, ShouldBeNil)
			})()

			c, err := Load(confPath)
			So(err, ShouldBeNil)

			var defaultReader db.Reader
			So(defaultReader.InitMessage(testutil.JSON(`{"DB": "test"}`)), ShouldBeNil)
			var defaultBuckets stats.Buckets
			So(defaultBuckets.InitMessage(testutil.JSON(`{}`)), ShouldBeNil)
			var defaultParallelSampling stats.ParallelSamplingConfig
			So(defaultParallelSampling.InitMessage(testutil.JSON(`{}`)), ShouldBeNil)

			So(c, ShouldResemble, &Config{
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
						Reader:        &defaultReader,
						PositionsAxis: "right",
						TotalAxis:     "right",
					}},
					{Config: &Distribution{
						Reader: &defaultReader,
						LogProfits: &DistributionPlot{
							Graph:     "dist",
							Buckets:   defaultBuckets,
							ChartType: "line",
							Normalize: true,
							RefDist: &CompoundDistribution{
								AnalyticalSource: &AnalyticalDistribution{
									Name:  "t",
									Mean:  0.0,
									MAD:   1.0,
									Alpha: 3.0,
								},
								N:            1,
								CompoundType: "biased",
								Params:       defaultParallelSampling,
							},
							DeriveAlpha: &DeriveAlpha{
								MinX:          2.0,
								MaxX:          4.0,
								Epsilon:       0.01,
								MaxIterations: 1000,
								IgnoreCounts:  10,
							},
						},
						Compound:  1,
						BatchSize: 10,
						Workers:   1,
					}},
					{Config: &PowerDist{
						Dist: CompoundDistribution{
							AnalyticalSource: &AnalyticalDistribution{
								Name:  "normal",
								MAD:   1.0,
								Alpha: 3.0,
							},
							N:            1,
							CompoundType: "biased",
							Params:       defaultParallelSampling,
						},
						CumulMean: &CumulativeStatistic{
							Graph:   "cumul mean",
							Buckets: defaultBuckets,
							Samples: 10000,
							Points:  200,
						},
						AlphaParams: &DeriveAlpha{
							MinX:          1.01,
							MaxX:          100.0,
							Epsilon:       0.01,
							MaxIterations: 1000,
							IgnoreCounts:  10,
						},
						CumulSamples: 10000,
						StatSamples:  10000,
					}},
				},
			})

			So(c.Experiments[0].Config.Name(), ShouldEqual, "test")
		})

		Convey("x log-scale for timeseries is an error", func() {
			var c Config
			err := c.InitMessage(testutil.JSON(`
{
  "groups": [{
    "timeseries": true,
    "id": "time", "log scale X": true,
    "graphs": [{"id": "g"}]
  }]
}`))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring,
				"timeseries group 'time' cannot have log-scale X")
		})

		Convey("duplicate group id is an error", func() {
			var c Config
			err := c.InitMessage(testutil.JSON(`
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
			err := c.InitMessage(testutil.JSON(`
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
			err := c.InitMessage(testutil.JSON(`{"groups": [{"graphs": [{"id": "r1"}]}]}`))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "group must have a non-empty ID")
		})

		Convey("graph without ID is an error", func() {
			var c Config
			err := c.InitMessage(testutil.JSON(`{"groups": [{"id": "g", "graphs": [{}]}]}`))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "graph must have a non-empty ID")
		})

		Convey("multi-key experiment map is an error", func() {
			var c Config
			err := c.InitMessage(testutil.JSON(`
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
			err := c.InitMessage(testutil.JSON(`
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
  "total graph": "total",
  "total axis": "left"
}`
				So(h.InitMessage(testutil.JSON(js)), ShouldBeNil)
				var data db.Reader
				So(data.InitMessage(testutil.JSON(`{"DB": "test"}`)), ShouldBeNil)
				So(h, ShouldResemble, Hold{
					Reader: &data, // must be initialized with its default values
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
					PositionsAxis:  "right",
					TotalGraph:     "total",
					TotalAxis:      "left",
				})
			})

			Convey("shares and start value are checked", func() {
				var p HoldPosition
				So(p.InitMessage(testutil.JSON(`{"ticker": "A"}`)), ShouldNotBeNil)
				So(p.InitMessage(testutil.JSON(
					`{"ticker": "A", "shares": 1, "start value": 1}`)), ShouldNotBeNil)
			})
		})
	})
}
