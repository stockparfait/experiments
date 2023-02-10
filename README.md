# Statistical experiments with stock data

[![Build Status](https://github.com/stockparfait/experiments/workflows/Tests/badge.svg)](https://github.com/stockparfait/experiments/actions?query=workflow%3ATests)
[![GoDoc](https://godoc.org/github.com/stockparfait/experiments?status.svg)](http://godoc.org/github.com/stockparfait/experiments)

The purpose of this repository is to hold and document a series of statistical
experiments with real financial data, primarily over the US stock market daily
price data from 1998 to mid-2022. The code and the necessary configurations are
provided, so the experiments can be repeated by anyone with access to the data
and some basic understanding of how to compile and run a command-line app.

A few random conventions throughout the study:

- Even though I'm a single person writing the study, I use the pronoun "we" to
  refer to myself and my readers, implying that "we're doing this together" (and
  not "We, Louis XIV").  Whenever I make a personal choice (without the reader),
  I use the pronoun "I" indicating that it's me who's doing it, it's my personal
  choice. The distinction is somewhat arbitrary, but it explains why sometimes I
  write "we" and sometimes "I".

- Important definitions or key moments will be _in italics_, while math and code
  fragments will be either an `inlined code` or in a

```
multi-line
code block.
```

Most plots and other types of experimental results are accompanied by a
corresponding _config_ to reproduce the plot or experiment. See the usage
section for details on how to use such configs with the tool.

## Table of Contents

- [Methodology](methodology/)
- [Market, Random Walks and Log-Profits](logprofits/)
- [Distribution of Log-Profits](distribution/)
- [Hypothesis Testing, Confidence Intervals and Monte Carlo](powerdist/)
  - [Theory](powerdist/theory.md)
  - [Normal distributinon](powerdist/normal.md)
  - [Student's t-distribution](powerdist/students.md)
  - [The Tale of Fat Tails](powerdist/fat_tails.md) - finding `a` (alpha)
- [History and Market Timing: Auto-Correlation of Log-Profits](autocorr/)

## Installation

Requirements:
- [Google Go](https://go.dev/dl/) 1.18 or higher;
- `parfait-sharadar` app to download data; see [stockparfait/stockparfait] for
  installation instructions;
- Subscription to Sharadar Equities Prices on [Nasdaq Data Link]

```sh
git clone https://github.com/stockparfait/experiments.git
cd experiments
make init
make install
```

This installs an executable `experiments` in your `${GOPATH}/bin`, where
```GOPATH=`go env GOPATH` ```.

## Quick start

- Subscribe to the data source on [Nasdaq Data Link]; most of these experiments
  use only the equities prices.
- Download the data by running `parfait-sharadar` - see
  [stockparfait/stockparfait] for details.
- Copy `stockparfait/stockparfait/js` folder contents to a separate working
  directory, for example `~/plots/`; here I'll refer to it as `${PLOTS}`.
- Run an experiment.

An example of these steps together:

```sh
${GOPATH}/bin/parfait-sharadar
mkdir -p ${PLOTS}
cp stockparfait/stockparfait/js/* ${PLOTS}
experiments -conf ${CONFIG}.json -js ${PLOTS}/data.js
```

where `${CONFIG}.json` is the config of your choice from one of the experiments,
or your own. The schema of such configs is in
[config/config.go](config/config.go).

- Open `${PLOTS}/plot.html` in your browser to see the resulting plots.

## Contributing to Stock Parfait Experiments

Pull requests are welcome. We suggest to contact us beforehand to coordinate
your code contributions.

Having said that, this repository serves primarily as the documentation of my
own research into the behavior of the US stock market, and also as an example
and the purpose and motivation behind the core libraries in
[stockparfait/stockparfait].

[stockparfait/stockparfait]: https://github.com/stockparfait/stockparfait
[Nasdaq Data Link]: https://data.nasdaq.com/databases/SFB/data
