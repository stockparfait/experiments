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
	"github.com/stockparfait/logging"
	"github.com/stockparfait/stockparfait/db"
	"github.com/stockparfait/stockparfait/plot"
	"github.com/stockparfait/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBeta(t *testing.T) {
	t.Parallel()

	tmpdir, tmpdirErr := os.MkdirTemp("", "test_autocorr")
	defer os.RemoveAll(tmpdir)

	Convey("Test setup succeeded", t, func() {
		So(tmpdirErr, ShouldBeNil)
	})

	d := func(date string) db.Date {
		res, err := db.NewDateFromString(date)
		if err != nil {
			panic(err)
		}
		return res
	}
	price := func(date string, p float32) db.PriceRow {
		return db.TestPrice(d(date), p, p, p, 1000.0, true)
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

		Convey("with price data", func() {
			dbName := "db"
			tickers := map[string]db.TickerRow{
				"I": {},
				"A": {},
				"B": {},
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
			}
			w := db.NewWriter(tmpdir, dbName)
			So(w.WriteTickers(tickers), ShouldBeNil)
			for t, p := range prices {
				So(w.WritePrices(t, p), ShouldBeNil)
			}

			Convey("all graphs", func() {
				var cfg config.Beta
				csvFile := filepath.Join(tmpdir, "betas.csv")
				confJSON := fmt.Sprintf(`
{
  "id": "testID",
  "reference data": {
    "DB path": "%s",
    "DB": "%s",
    "tickers": ["I"]
  },
  "data": {
    "DB path": "%s",
    "DB": "%s",
    "tickers": ["A", "B"]
  },
  "file": "%s",
  "beta plot": {"graph": "beta"},
  "R plot": {"graph": "R"},
  "R means": {"graph": "means"},
  "R MADs": {"graph": "mads"},
  "R Sigmas": {"graph": "sigmas"}
}`, tmpdir, dbName, tmpdir, dbName, csvFile)
				So(cfg.InitMessage(testutil.JSON(confJSON)), ShouldBeNil)
				var betaExp Beta
				So(betaExp.Run(ctx, &cfg), ShouldBeNil)

				So(testutil.FileExists(csvFile), ShouldBeFalse) // TODO
				So(len(betaGraph.Plots), ShouldEqual, 1)
				So(len(RGraph.Plots), ShouldEqual, 1)
				So(len(MeansGraph.Plots), ShouldEqual, 1)
				So(len(MADsGraph.Plots), ShouldEqual, 1)
				So(len(SigmasGraph.Plots), ShouldEqual, 1)
			})
		})

		Convey("with synthetic data", func() {
			var cfg config.Beta
			csvFile := filepath.Join(tmpdir, "betas.csv")
			confJSON := fmt.Sprintf(`
{
  "id": "testID",
  "reference analytical": {"name": "t"},
  "analytical R": {"name": "t"},
  "tickers": 3,
  "samples": 10,
  "file": "%s",
  "beta plot": {"graph": "beta"},
  "R plot": {"graph": "R"},
  "R means": {"graph": "means"},
  "R MADs": {"graph": "mads"},
  "R Sigmas": {"graph": "sigmas"}
}`, csvFile)
			So(cfg.InitMessage(testutil.JSON(confJSON)), ShouldBeNil)
			var betaExp Beta
			So(betaExp.Run(ctx, &cfg), ShouldBeNil)

			So(testutil.FileExists(csvFile), ShouldBeFalse) // TODO
			So(len(betaGraph.Plots), ShouldEqual, 1)
			So(len(RGraph.Plots), ShouldEqual, 1)
			So(len(MeansGraph.Plots), ShouldEqual, 1)
			So(len(MADsGraph.Plots), ShouldEqual, 1)
			So(len(SigmasGraph.Plots), ShouldEqual, 1)
		})
	})
}
