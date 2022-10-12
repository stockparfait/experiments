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
		eg, err := plot.EnsureGraph(ctx, plot.KindXY, "errors", "top")
		So(err, ShouldBeNil)

		Convey("AnalyticalDistribution works", func() {
			var cfg config.AnalyticalDistribution

			Convey("Normal distribution", func() {
				js := testutil.JSON(`
{
  "name": "normal",
  "mean": 1.0
}`)
				So(cfg.InitMessage(js), ShouldBeNil)
				d, name, err := AnalyticalDistribution(ctx, &cfg)
				So(err, ShouldBeNil)
				So(name, ShouldEqual, "Gauss")
				So(d.Mean(), ShouldEqual, 1.0)
			})

			Convey("t-distribution", func() {
				js := testutil.JSON(`
{
  "name": "t",
  "mean": 1.0,
  "alpha": 2.0
}`)
				So(cfg.InitMessage(js), ShouldBeNil)
				d, name, err := AnalyticalDistribution(ctx, &cfg)
				So(err, ShouldBeNil)
				So(name, ShouldEqual, "T(a=2.00)")
				So(d.Mean(), ShouldEqual, 1.0)
			})
		})

		Convey("CompoundDistribution works", func() {
			var seed uint64 = 42
			var cfg config.CompoundDistribution

			Convey("Fast Compounded normal distribution", func() {
				js := testutil.JSON(`
{
  "analytical source": {
    "name": "normal",
    "mean": 1.0
  },
  "n": 10,
  "compound type": "fast",
  "parameters": {
    "samples": 1000,
    "workers": 1
  }
}`)
				So(cfg.InitMessage(js), ShouldBeNil)
				d, name, err := CompoundDistribution(ctx, &cfg)
				So(err, ShouldBeNil)
				So(name, ShouldEqual, "Gauss x 10")
				d.Seed(seed)
				So(testutil.Round(d.Mean(), 2), ShouldEqual, 10.0)
			})

			Convey("Directly compounded normal sample distribution", func() {
				js := testutil.JSON(`
{
  "analytical source": {
    "name": "normal",
    "mean": 1.0
  },
  "n": 10,
  "compound type": "direct",
  "source samples": 2000,
  "seed samples": 42,
  "parameters": {
    "samples": 1000,
    "workers": 1
  }
}`)
				So(cfg.InitMessage(js), ShouldBeNil)
				d, name, err := CompoundDistribution(ctx, &cfg)
				So(err, ShouldBeNil)
				d.Seed(seed)
				So(testutil.Round(d.Mean(), 2), ShouldEqual, 10.0)
				So(name, ShouldEqual, "Gauss[samples=2000] x 10")
			})

			Convey("Biased compounded normal distribution", func() {
				js := testutil.JSON(`
{
  "analytical source": {
    "name": "normal",
    "mean": 1.0
  },
  "n": 10,
  "compound type": "biased",
  "parameters": {
    "buckets": {
      "min": -4,
      "max": 6
    },
    "bias scale": 3,
    "bias power": 3,
    "bias shift": 1,
    "samples": 1000,
    "workers": 1,
    "seed": 42
  }
}`)
				So(cfg.InitMessage(js), ShouldBeNil)
				d, name, err := CompoundDistribution(ctx, &cfg)
				So(err, ShouldBeNil)
				d.Seed(seed)
				So(testutil.Round(d.Mean(), 2), ShouldEqual, 10.0)
				So(name, ShouldEqual, "Gauss x 10")
			})

			Convey("Double compounded distribution", func() {
				js := testutil.JSON(`
{
  "compound source": {
    "analytical source": {
      "name": "normal",
      "mean": 1
    },
    "n": 2,
    "compound type": "biased",
    "parameters": {
      "bias power": 2,
      "bias scale": 3,
      "bias shift": 1,
      "buckets": {
        "min": -8,
        "max": 12
      },
      "bias shift": 1,
      "samples": 10000,
      "workers": 1,
      "seed": 42
    }
  },
  "n": 5,
  "compound type": "biased",
  "parameters": {
    "bias power": 2,
    "bias scale": 6,
    "bias shift": 2,
    "buckets": {
      "min": -40,
      "max": 60
    },
    "samples": 10000,
    "workers": 1,
    "seed": 42
  }
}`)
				So(cfg.InitMessage(js), ShouldBeNil)
				d, name, err := CompoundDistribution(ctx, &cfg)
				So(err, ShouldBeNil)
				d.Seed(seed)
				So(testutil.Round(d.Mean(), 1), ShouldEqual, 10.0)
				So(name, ShouldEqual, "Gauss x 2 x 5")
			})
		})

		Convey("PlotDistribution works", func() {
			var cfg config.DistributionPlot
			js := testutil.JSON(`
{
    "graph": "main",
    "counts graph": "counts",
    "errors graph": "errors",
    "buckets": {"n": 9, "min": -5, "max": 5, "auto bounds": false},
    "normalize": false,
    "use means": true,
    "log Y": true,
    "chart type": "bars",
    "plot mean": true,
    "percentiles": [50],
    "reference distribution": {"analytical source": {"name": "t"}},
    "derive alpha": {
      "min x": 2,
      "max x": 4,
      "max iterations": 10
    }
}`)
			So(cfg.InitMessage(js), ShouldBeNil)
			d := stats.NewSampleDistribution(
				[]float64{-2.0, -0.5, 0.5, 2.0}, &cfg.Buckets)
			So(PlotDistribution(ctx, d, &cfg, "test"), ShouldBeNil)

			So(len(g.Plots), ShouldEqual, 4)
			So(g.Plots[0].Legend, ShouldEqual, "test p.d.f.")

			So(len(cg.Plots), ShouldEqual, 1)
			So(cg.Plots[0].Legend, ShouldEqual, "test counts")
			So(cg.Plots[0].YLabel, ShouldEqual, "counts")
			So(cg.Plots[0].X, ShouldResemble, []float64{-2, 0, 2})
			So(cg.Plots[0].Y, ShouldResemble, []float64{1, 2, 1})

			So(len(eg.Plots), ShouldEqual, 1)
			So(eg.Plots[0].Legend, ShouldEqual, "test errors")
			So(cg.Plots[0].YLabel, ShouldEqual, "counts")
		})

		Convey("CumulativeStatistic works", func() {
			js := testutil.JSON(`
{
  "graph": "main",
  "skip": 2,
  "percentiles": [5, 95],
  "plot expected": true
}`)
			var cfg config.CumulativeStatistic
			So(cfg.InitMessage(js), ShouldBeNil)
			cs := NewCumulativeStatistic(&cfg)
			cs.SetExpected(5.0)
			for i := 0; i < 10; i++ {
				cs.AddToAverage(float64(i))
			}
			cs.Map(func(x float64) float64 { return x + 1.0 })
			So(cs.Plot(ctx, "numbers", "average of one to ten"), ShouldBeNil)
			So(len(g.Plots), ShouldEqual, 4) // avg + 2 percentiles + expected
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
