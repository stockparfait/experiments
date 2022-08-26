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
	"testing"

	"github.com/stockparfait/experiments/config"
	"github.com/stockparfait/stockparfait/plot"
	"github.com/stockparfait/stockparfait/stats"
	"github.com/stockparfait/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExperiments(t *testing.T) {
	t.Parallel()

	Convey("FindMin", t, func() {
		var iterations int
		f := func(x float64) float64 { iterations++; x = x - 1.0; return x * x }

		Convey("stop with epsilon precision", func() {
			maxIter := 20
			res := FindMin(f, -1.0, 2.0, 0.01, maxIter)
			// f is called twice per iteration.
			So(iterations, ShouldBeLessThan, 2*maxIter)
			So(testutil.Round(res, 2), ShouldEqual, 1.0)
		})

		Convey("stop with max iterations", func() {
			maxIter := 8
			res := FindMin(f, -5.0, 2.0, 0.01, maxIter)
			So(iterations, ShouldEqual, 2*maxIter)
			So(testutil.Round(res, 1), ShouldEqual, 1.0)
		})

	})

	Convey("Experiments API works", t, func() {
		ctx := context.Background()
		canvas := plot.NewCanvas()
		values := make(Values)
		ctx = plot.Use(ctx, canvas)
		ctx = UseValues(ctx, values)

		g, err := plot.EnsureGraph(ctx, plot.KindXY, "main", "top")
		So(err, ShouldBeNil)
		cg, err := plot.EnsureGraph(ctx, plot.KindXY, "counts", "top")
		So(err, ShouldBeNil)

		Convey("PlotDistribution works", func() {
			So(err, ShouldBeNil)
			var cfg config.DistributionPlot
			js := testutil.JSON(`
{
    "graph": "main",
    "counts graph": "counts",
    "buckets": {"n": 9, "minval": -5, "maxval": 5},
    "normalize": false,
    "use means": true,
    "log Y": true,
    "chart type": "bars",
    "plot mean": true,
    "percentiles": [50],
    "reference distribution": {"name": "t"},
    "derive alpha": {
      "min x": 2,
      "max x": 4,
      "max iterations": 10
    }
}`)
			So(cfg.InitMessage(js), ShouldBeNil)
			h := stats.NewHistogram(&cfg.Buckets)
			h.Add(-2.0, -0.5, 0.5, 2.0)
			So(PlotDistribution(ctx, h, &cfg, "test"), ShouldBeNil)
			So(len(g.Plots), ShouldEqual, 4)
			So(len(cg.Plots), ShouldEqual, 1)
			So(g.Plots[0].Legend, ShouldEqual, "test p.d.f.")
			So(cg.Plots[0].Legend, ShouldEqual, "test counts")
			So(cg.Plots[0].YLabel, ShouldEqual, "counts")
			So(cg.Plots[0].X, ShouldResemble, []float64{-2, 0, 2})
			So(cg.Plots[0].Y, ShouldResemble, []float64{1, 2, 1})
		})

		Convey("for TestExperiment", func() {
			conf := config.TestExperimentConfig{
				Grade:  3.5,
				Passed: true,
				Graph:  "main",
			}
			testExp := TestExperiment{}
			So(testExp.Run(ctx, &conf), ShouldBeNil)
			So(canvas.Groups[0].Graphs[0].Plots[0].X, ShouldResemble, []float64{1.0, 2.0})
			So(values, ShouldResemble, map[string]string{"grade": "3.5", "test": "passed"})
		})
	})

}
