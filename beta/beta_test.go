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

package beta

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stockparfait/experiments"
	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/iterator"
	"github.com/stockparfait/logging"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/plot"
	"github.com/stockparfait/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBeta(t *testing.T) {
	t.Parallel()

	tmpdir, tmpdirErr := os.MkdirTemp("", "test_beta")
	defer os.RemoveAll(tmpdir)

	Convey("Test setup succeeded", t, func() {
		So(tmpdirErr, ShouldBeNil)
	})

	price := func(date string, p float64) db.PriceRow {
		d, err := db.NewDateFromString(date)
		if err != nil {
			panic(err)
		}
		return db.TestPrice(d, float32(p), float32(p), float32(p), 1000.0, true)
	}

	Convey("Beta works", t, func() {
		ctx := context.Background()
		ctx = logging.Use(ctx, logging.DefaultGoLogger(logging.Info))
		canvas := plot.NewCanvas()
		values := make(experiments.Values)
		ctx = plot.Use(ctx, canvas)
		ctx = experiments.UseValues(ctx, values)
		betaGraph, err := canvas.EnsureGraph(plot.KindXY, "beta", "group")
		So(err, ShouldBeNil)
		RGraph, err := canvas.EnsureGraph(plot.KindXY, "R", "group")
		So(err, ShouldBeNil)
		MeansGraph, err := canvas.EnsureGraph(plot.KindXY, "means", "group")
		So(err, ShouldBeNil)
		MADsGraph, err := canvas.EnsureGraph(plot.KindXY, "mads", "group")
		So(err, ShouldBeNil)
		SigmasGraph, err := canvas.EnsureGraph(plot.KindXY, "sigmas", "group")
		So(err, ShouldBeNil)
		CorrGraph, err := canvas.EnsureGraph(plot.KindXY, "corr", "group")
		So(err, ShouldBeNil)
		LengthsGraph, err := canvas.EnsureGraph(plot.KindXY, "lengths", "group")
		So(err, ShouldBeNil)
		BetaRatios, err := canvas.EnsureGraph(plot.KindXY, "beta ratios", "group")
		So(err, ShouldBeNil)

		Convey("with price data", func() {
			dbName := "db"
			tickers := map[string]db.TickerRow{
				"I": {},
				"A": {},
				"B": {},
				"C": {},
			}
			prices := map[string][]db.PriceRow{
				"I": {
					price("2020-01-01", 1000),
					price("2020-01-02", 1010),
					price("2020-01-03", 1020),
					price("2020-01-04", 990),
					price("2020-01-05", 1000),
				},
				"A": {
					price("2020-01-01", 100),
					price("2020-01-02", 102),
					price("2020-01-03", 104),
					price("2020-01-04", 95),
					price("2020-01-05", 99),
				},
				"B": {
					price("2020-01-01", 20),
					price("2020-01-02", 30),
					price("2020-01-03", 11),
					price("2020-01-04", 18),
					price("2020-01-05", 45),
				},
				"C": {
					price("2020-01-01", 5),
					price("2020-01-02", 5.5),
					price("2020-01-03", 6),
					price("2020-01-04", 4.5),
					price("2020-01-05", 8),
				},
			}
			w := db.NewWriter(tmpdir, dbName)
			So(w.WriteTickers(tickers), ShouldBeNil)
			for t, p := range prices {
				So(w.WritePrices(t, p), ShouldBeNil)
			}

			Convey("all graphs", func() {
				var cfg config.Beta
				csvFile := filepath.Join(tmpdir, "betas.csv")
				lengthsFile := filepath.Join(tmpdir, "lengths.json")
				confJSON := fmt.Sprintf(`
{
  "id": "testID",
  "reference": {"DB": {
    "DB path": "%s",
    "DB": "%s",
    "tickers": ["I"]
  }},
  "data": {"lengths file": "%s", "DB": {
    "DB path": "%s",
    "DB": "%s",
    "tickers": ["A", "B"]
  }},
  "file": "%s",
  "beta plot": {"graph": "beta"},
  "R plot": {"graph": "R"},
  "R means": {"graph": "means"},
  "R MADs": {"graph": "mads"},
  "R Sigmas": {"graph": "sigmas"},
  "lengths plot": {"graph": "lengths"},
  "beta ratios": {
    "window": 3,
    "plot": {"graph": "beta ratios"}
  }
}`, tmpdir, dbName, lengthsFile, tmpdir, dbName, csvFile)
				So(cfg.InitMessage(testutil.JSON(confJSON)), ShouldBeNil)
				var betaExp Beta
				So(betaExp.Run(ctx, &cfg), ShouldBeNil)

				So(testutil.FileExists(csvFile), ShouldBeTrue)
				So(testutil.FileExists(lengthsFile), ShouldBeTrue)
				So(len(betaGraph.Plots), ShouldEqual, 1)
				So(len(RGraph.Plots), ShouldEqual, 1)
				So(len(MeansGraph.Plots), ShouldEqual, 1)
				So(len(MADsGraph.Plots), ShouldEqual, 1)
				So(len(SigmasGraph.Plots), ShouldEqual, 1)
				So(len(LengthsGraph.Plots), ShouldEqual, 1)
				So(len(BetaRatios.Plots), ShouldEqual, 1)
			})
		})

		Convey("with synthetic data", func() {
			var cfg config.Beta
			csvFile := filepath.Join(tmpdir, "betas.csv")
			confJSON := fmt.Sprintf(`
{
  "id": "testID",
  "reference": {"close": {"name": "t"}, "samples": 10},
  "data": {
    "close": {"name": "t"},
    "tickers": 3,
    "samples": 10
  },
  "file": "%s",
  "beta plot": {"graph": "beta"},
  "R plot": {"graph": "R"},
  "R means": {"graph": "means"},
  "R MADs": {"graph": "mads"},
  "R Sigmas": {"graph": "sigmas"},
  "R correlations": {"graph": "corr"},
  "lengths plot": {"graph": "lengths"},
  "beta ratios": {
    "window": 3,
    "plot": {"graph": "beta ratios"}
  }
}`, csvFile)
			So(cfg.InitMessage(testutil.JSON(confJSON)), ShouldBeNil)
			var betaExp Beta
			So(betaExp.Run(ctx, &cfg), ShouldBeNil)

			So(testutil.FileExists(csvFile), ShouldBeTrue)
			So(len(betaGraph.Plots), ShouldEqual, 1)
			So(len(RGraph.Plots), ShouldEqual, 1)
			So(len(MeansGraph.Plots), ShouldEqual, 1)
			So(len(MADsGraph.Plots), ShouldEqual, 1)
			So(len(SigmasGraph.Plots), ShouldEqual, 1)
			So(len(CorrGraph.Plots), ShouldEqual, 1)
			So(len(LengthsGraph.Plots), ShouldEqual, 1)
			So(len(BetaRatios.Plots), ShouldEqual, 1)
		})
	})
}

func TestIterators(t *testing.T) {
	t.Parallel()

	Convey("nxnPairs works", t, func() {
		So(iterator.ToSlice[intPair](&nxnPairs{n: 4}), ShouldResemble, []intPair{
			{0, 1},
			{0, 2},
			{0, 3},
			{1, 2},
			{1, 3},
			{2, 3},
		})
	})

	Convey("randPairs works", t, func() {
		n, k := 10, 5
		pairs := iterator.ToSlice[intPair](newRandPairs(n, k, 42))
		So(len(pairs), ShouldEqual, k)
		for _, p := range pairs {
			So(p.x, ShouldBeLessThan, p.y)
			So(p.y, ShouldBeLessThan, n)
		}
	})
}
