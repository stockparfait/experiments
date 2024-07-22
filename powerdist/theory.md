# Hypothesis Testing and Confidence Intervals

*Previous: [section's TOC](README.md)*

When we compute any statistic or estimate a parameter from sample data, there is
always a sampling error. A generally accepted way to estimate such an error is
to compute a [confidence interval] for a given _confidence level_ $P$, sometimes
expressed as $p=1-P$, or a _p-value_.

More precisely, given a vector of random variables $X = (X_1, \ldots, X_n)$ with a
distribution $f(X\mid \theta)$, an estimator $s(X)$ to estimate the parameter $\theta$,
and an interval $CI(X)=[u(X)..v(X)]$ for some statistics $u$ and $v$ such that
$u(X)\le s(X) \le v(X)$, if the probability that $CI(X)$ includes the true value of
$\theta$ is $P$, then this interval is called a _confidence interval_ with the
confidence level $P$.

Intuitively, a confidence interval is useful for giving an idea about the
accuracy of an estimator.

Specifically, we'll be interested in the case of [hypothesis testing] when our
null hypothesis states that the samples are generated using a particular
distribution $f(X\mid \theta)$ for a specific $\theta$.  An example of such a null
hypothesis suggested in the previous section is that log-profits (normalized to
mean $=0$ and MAD $=1$) are distributed as a t-distribution with $\alpha=3$, the
alternative hypothesis being $\alpha \ne 3$.

In this case, we can construct the following experiment. Under the null
hypothesis, we know the true value of $\theta$ for the distribution $f(X\mid \theta)$.
Let's sample this distribution to obtain a sample $x=(x_1, ..., x_n)$, and use
an _estimator_ $s(x)$ to estimate the value of $\theta$ from $x$.  This
estimator, in turn, can be viewed as a random variable $S=s(X)$ with its own
distribution $g(S\mid\theta)$, which we assume to have a non-degenerate p.d.f.,
finite for any value of $S$.

Next, for a given probability $P$, we construct an interval $I=[\theta-u..\theta+v]$
such that:

$$
\begin{array}{rcl}
u & = & \theta - Q[ \frac{1-P}{2} ] \\
v & = & Q[ \frac{1+P}{2} ] - \theta\\
\end{array}
$$

where $Q[p]$ is the $p$-th quantile of $g(S\mid\theta)$.  By construction, the
probability that $S$ falls within this interval is $P$.

Now consider an interval $CI(S)=[S-v..S+u]$. My claim now is that this is
precisely the confidence interval for $\theta$ with the confidence level $P$
under the null hypothesis.

Indeed, by construction, $S$ can be outside of the interval $I$ on either side
with equal probability of $\frac{1-P}{2}$. When a specific sample $s(x)$ is on the
right side of the interval $I$, $s(x) > \theta+v$, and hence, $s(x)-v > \theta$,
so $theta` is out of $CI(s(x))$. Similarly, when $s(x)$ is on the left of
$CI(s(x))$ we have that $s(x) < \theta-u$, hence $s(x)+u < \theta$, and again
$\theta$ is out of the interval. Conversely, when $s(x)$ is within $I$, $\theta$
is within $CI(s(x))$. Therefore, $\theta$ belongs to $CI(S)$ with the probability
$P$, hence $CI(S)$ is its confidence interval with the confidence level $P$.

Notice, that in this argument $s(x)$ does not need to estimate $\theta$ with any
particular accuracy. However, for practical purposes, it is desirable to have
the mean of $s(X)$ approach the actual $\theta$, and that the distribution of
$s(X)$ is (approximately) symmetric around the mean. If this is the case, then
the confidence interval becomes symmetric around $s(x)$, and we can reasonably
expect that $s(x)$ indeed represents an approximation of $\theta$.

## Monte Carlo of Confidence Intervals

The above result gives us a way to approximate confidence intervals of various
statistics and parameters of an analytical distribution computationally using a
[Monte Carlo method]:

- Sample $s(x)$ by sampling $x=(x_1, ..., x_n)$ from $f(X\mid\theta)$;
- Construct a histogram of $s(x)$ samples to approximate the p.d.f. of $S$;
- Compute the (approximation of the) interval $I$ from the histogram;
- Define $CI(s(x))$ as above.

Along the way, we can estimate the quality of the estimator $s(X)$ by comparing
its mean to $\theta$ and evaluating the width of $I$ as an indicator of its
precision.

## Mean, MAD and Sigma

As an illustration, our first set of experiments will estimate (computationally)
the basic statistics of our two distributions of interests, t-distribution and
Gaussian.

We start with the mean, MAD and stardand deviation $\sigma$ for several sample
sizes denoted in the plot by $N$. That is, we draw $N$ samples from the source
distribution $x=(x_1, ..., x_N)$ and compute the statistics using their
definitions (ignoring the "sample" vs. "population" distinction for $\sigma$):

$$
\begin{array}{rcl}
E[x] & = & \frac{1}{N}\cdot\Sigma_{i=1}^N x_i \\
MAD[x] & = & \frac{1}{N}\cdot\Sigma_{i=1}^N | E[x] - x_i| \\
\sigma(x) & = & \sqrt{ \frac{1}{N} \cdot \Sigma_{i=1}^N (E[x] - x_i)^2} \\
\end{array}
$$

A value of each statistic becomes a single sample of its own histogram. We then
repeat this process 10K times, so that 1% still contains 100 samples to estimate
a confidence interval of 99% confidence level with a reasonable accuracy.  All
in all, we'll be sampling the distribution $10,000 \cdot N$ times for each
statistic.

For uniformity, our source distribution will always have $E[x]=0$ and $\mathrm{MAD}[x]=1$,
and t-distribution will usually have $\alpha=3$ unless stated otherwise.  We use the
following values of $N$:

- $250$ - approximately the number of trading days in a year,
- $5000$ - 20 years, the rounded maximum duration of a single stock in our dataset,
- $20,000,000$ - the rounded number of daily samples in our dataset for the
stocks with the average daily volume of $>$ $1M.

*Next: [Normal distributinon](normal.md)*

[confidence interval]: https://en.wikipedia.org/wiki/Confidence_interval
[hypothesis testing]: https://en.wikipedia.org/wiki/Statistical_hypothesis_testing
[Monte Carlo method]: https://en.wikipedia.org/wiki/Monte_Carlo_method
