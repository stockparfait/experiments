# Hypothesis Testing, Confidence Intervals and Monte Carlo

In this section, we establish a methodology for working with sampled data and
estimating the precision of the results.

## Hypothesis Testing and Confidence Intervals

When we compute any statistic or estimate a parameter from sample data, there is always a sampling error. A generally accepted way to estimate such an error is to compute a [confidence interval] for a given _confidence level_ `P`, usually expressed as `p=1-P`, or a _p-value_.

More precisely, given the data samples `x = (x_1, ..., x_n)` of a known
distribution with a parameter `theta` and an interval `[u(x)..v(x)]`, if the
probability that the true value of `theta` is in this interval is `P`, then this
interval is called a _confidence interval_ with the confidence level `P`, or
equivalently, a p-value of `1-P`.

It may be infeasible to obtain a generic confidence interval computationally
using this definition, but we can specialize it to [hypothesis testing].

Specifically, we'll be interested in the case when our null hypothesis states
that the samples are generated using a particular distribution `f(X|theta)` for
a specific `theta`.  An example of such a null hypothesis suggested in the
previous section is that log-profits (normalized to `mean=0` and `MAD=1`) are
distributed as a t-distribution with `a=3`, the alternative hypothesis being
`a!=3`.

In this case, we can construct the following experiment. Under the null
hypothesis, we know the true value of `theta` for the distribution `f(X|theta)`.
Let's sample this distribution to obtain a sample `x=(x_1, ..., x_n)`, and use
an _estimator_ `s(x)` to estimate the value of `theta` from `x`.  This
estimator, in turn, can be viewed as a random variable `s(X)` with its own
distribution `g(X|theta)`, which we assume to be a non-degenerate p.d.f., finite
for any `x`.

Next, for a given probability `P`, we construct an interval `I=[theta-u..theta+v]`
such that:

```
u = theta - Q[ (1-P)/2 ]
v = Q[ (1+P)/2 ] - theta
```

where `Q[p]` is a `p`-th quantile of `g(X|theta)`.  By construction, the
probability that `s(X)` falls within this interval is `P`.

Now consider an interval `CI(X)=[s(X)-v, s(X)+u]`. My claim now is that this is
precisely the confidence interval for `theta` with the confidence level `P`
under the null hypothesis.

Indeed, by construction, a specific sample `s(x)` can be outside of the interval
`I` on either side with equal probability of `(1-P)/2`. When `s(x)` is on the
right side of the interval, `s(x) > theta+v`, and hence, `s(x)-v > theta`, so
`theta` is out of `CI(x)`. Similarly, when `s(x)` is on the left of `CI(x)` we
have that `s(x) < theta-u`, hence `s(x)+u < theta`, and again `theta` is out of
the interval. Conversely, when `s(x)` is within `I`, `theta` is within
`CI(x)`. Therefore, `theta` belongs to `CI(X)` with the probability `P`, hence
`CI(X)` is its confidence interval with the confidence level `P`.

Notice, that in this argument `s(x)` does not need to estimate `theta` with any
particular accuracy. However, for practical purposes, it is desirable to have
the mean of `s(X)` approach the actual `theta`, and that the distribution of
`s(X)` is (approximately) symmetric around the mean. If this is the case, then
the confidence interval becomes symmetric around `s(x)`, and we can reasonably
expect that `s(x)` indeed represents an approximation of `theta`.

### Monte Carlo of Confidence Intervals

The above result gives us a way to approximate confidence intervals of various
statistics and parameters of an analytical distribution computationally as
follows:

- Sample `s(x)` by sampling `x=(x_1, ..., x_n)` from `f(X|theta)`;
- Construct a histogram of `s(x)` samples to approximate the p.d.f. of `s(X)`;
- Compute the (approximation of the) interval `I`from the histogram;
- Define `CI(x)` as above.

Along the way, we can estimate the quality of the estimator `s(X)` by comparing
its mean to `theta` and evaluating the width of `I` as an indicator of its
precision.

## The Tale of Fat Tails

Earlier, we [have established](../distribution/) that log-profits can be
reasonably accurately modeled by a random variable `X` with a Student's
[t-distribution] with (approximately) `a=3` degrees of freedom, it is time to
look closer at this distribution and understand some of its fundamental
properties.

 As a reminder, the p.d.f. of the t-distribution with `a` degrees of freedom is:

```
f_a(x) = C * (1 + x^2/a)^(-(a+1)/2)
```

where `C=Gamma((a+1)/2) / [ sqrt[a*Pi] * Gamma(a/2) ]` is a normalizing
coefficient. The important part is that when the absolute value of `x`
approaches infinity, the p.d.f. becomes proportional to a [power law] with the
exponent `a+1`:

```
f_a(x) [ abs(x) --> Inf] ~  1/ [ abs(x)^(a+1) ]
```

and hence, the c.d.f. (cumulative distribution function) approaches 0 or 1 as a
power law with the exponent `a`. Therefore, a Student's t-distribution is a
[heavy-tailed distribution], since for any `a` it has infinite moments, and for
`a<=3` it is also a [fat-tailed distribution], since its skewness and kurtosis
are infinite.

[t-distribution]: https://en.wikipedia.org/wiki/Student%27s_t-distribution
[power law]: https://en.wikipedia.org/wiki/Power_law
[heavy-tailed distribution]: https://en.wikipedia.org/wiki/Heavy-tailed_distribution
[fat-tailed distribution]: https://en.wikipedia.org/wiki/Fat-tailed_distribution
[confidence interval]: https://en.wikipedia.org/wiki/Confidence_interval
[hypothesis testing]: https://en.wikipedia.org/wiki/Statistical_hypothesis_testing
