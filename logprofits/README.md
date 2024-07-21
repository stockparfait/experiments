# Market, Random Walks and Log-Profits

*Previous: [Methodology]*

Before we begin our studies, let's decide on what exactly is worth studying.

## Traders and Investors

First, let's agree that the sole purpose of studying the stock market is to make
or preserve money. Hence, any study must aim at predicting certain market
behaviors and optimizing for certain parameters for maximum expected profit and minimum possible loss.

For the majority of retail traders and investors there are several ways to use
the stock market, which can roughly be broken down into these categories:
- Day trader: buy / sell during the day, close all positions overnight;
- Swing trader: hold each position for a few days;
- Position trader: hold each position for a few weeks / months;
- Investor: hold positions potentially indefinitely (years).

My primary focus in this study will be in the investor category, for the reasons
eventually revealed later in the study (cliffhanger!). For now, let's just say
that all of the trading approaches are speculative, that is, they attempt to
make money primarily on the volatility. On the contrary, investors attempt to
make money on the added value of the busness that issued a stock, and largely
ignore the short-term volatility.

My personal and somewhat philosophical take on a trader vs. an investor
distinction is that a trader plays mostly a zero-sum game against other traders
(and often, a negative-sum game, because the broker gets its fees regardless),
while an investor puts its money into a business which allows the business to
grow and in turn enrich the investor, which is a positive-sum game (win-win).
Phisolophically, I believe that there is no free lunch (and certainly no free
money), and you're more likely to make a buck if you've produced some tangible
value for someone else. I'm not trying to put down traders - institutional
traders, for example, provide a lot of value by supporting very useful financial
products such as ETFs (which I personally use, among other things) - and are
getting paid handsomely for it. But this only supports my philosophical thesis.

## Price Series

A casual look at any stock price graph over any time range (minutes, days,
months, even years) usually looks like a random series of ups and downs with a
tendency of a gradual growth over the span of multiple years. But in any given
year (not to mention any given week or day) the price may go up or down, and a
short-term (sub-year) steady income is never guaranteed.

Another observation is that the price change in any given day is usually not
very large (about 1% on average, as we'll see later), and in absolute value is
proportional to the price itself. That is, a typical stock share worth $1000
will normally be up and down by about $10 each day, whereas a typical stock
share worth $100 will fluctuate by about $1 a day or so. This property becomes
even more intuitive if we consider a stock split: a 10-for-1 split of a $1000
share will yield ten $100 shares, and if it fluctuated by around $10 a day
before, the resulting 10 shares will continue to fluctuate by about $10 a day,
or $1 per share after the split.

This gives rise to an idea that a stock price series $P(t)$ could be modeled as
a _geometric random walk_:

$$
P(t+1) = X \cdot P(t)
$$

where $X$ is a random variable from $[0..+\infty]$ with a mean close to $1.0$ and a
mean absolute deviation (_MAD_) typically about $0.01$ for daily series.
Though, for a multiplicative process a _geometric mean_ and similarly _geometric
MAD_ are more appropriate measures (see later).

Note: I tend to use MAD instead of the standard deviation a.k.a. $\sigma$ ("sigma") since
it's more intuitive, and also for more practical reasons to be revealed later -
another cliffhanger! Formally:

$$
MAD(X) = E[ \ | E[X] - X |\ ]
$$

where $E[X]$ denotes the mean of $X$:

$$
\begin{array}{rcll}
E[X] & = & (X_1 + X_2 + ... + X_n) / n & \mbox{for $n$ samples  of }X, \\
E[X] & = & \int_{-\infty}^{+\infty} x f(x) dx & \mbox{for a continuous $X$ with the p.d.f. }f(x).\\
\end{array}
$$

In practice, most statistical mathematics and machinery is developed for
_additive_ processes and for distributions that range from $[-\infty..+\infty]$.
Therefore, it is more practical to consider an equivalent process over
_log-prices_:

$$
\log P(t+1) = X + \log P(t)
$$

where $X$ now ranges from $[-\infty..+\infty]$ with a mean around $0$ and MAD still
around $0.01$ for daily series. The reason MAD doesn't change much in this
transformation is because $\log(1+x) \approx x$ for small $x$, and:

$$
\log(x+\delta) - \log(x) = \log( (x+\delta)/x ) = \log(1 + \frac{\delta}{x}) \approx \frac{\delta}{x}
$$

for small $delta$, which in this case represents a typical daily price change.

In particular, the _geometric mean and MAD_ can be defined as exponentiated
regular (arithmetic) mean and MAD of $\log x$:

$$
\begin{array}{rcl}
GE[X] & = & e^{E[ \log x ]} \\
GMAD[X] & = & e^{ MAD[ \log x ] }
\end{array}
$$

In the experiments we will often use the geometric versions for printing the
values in the numerical experiments, since they are usually more intuitive than
the logarithms. But internally, all the computations will be done in log-space.

To be more specific, we are going to study $X$ by defining it as:

$$
X(t+1) = \log P(t+1) - \log P(t)
$$

and call it a _log-profit_ (one of the standard terms used in the literature).

From here on, whenever I refer to a "price series" and its properties, I will in
fact be referring to its log-profit series $X$, unless explicitly stated
otherwise.

 The intuitive advantage of log-profits (vs. the direct geometric version) is
that a sum of log-profits represents a log-profit over the corresponding longer
time period.  In particular:
- Summing up 5 daily log-profit samples results in a single weekly log-profit
  sample - a very convenient resampling procedure;
- A mean of yearly log-profits is the average annualized log-growth; and
  exponentiating such a mean results in the classical average annualized
  growth - again, a very intuitive and familiar measure;
- If a price ever goes to zero (or $-\infty$ in log-space), it will stay zero
  thereafter, resulting in a zero geometric mean (since $-\infty$ will dominate the
  sum). This correctly models the possibility of a total loss, after which there
  is obviously no recovery. On the contrary, an arithmetic mean in the original
  (non-log) space will gloss over such a catastrophic possibility and will tell
  you that a game of Russian roulette with one of the 6 chambers of the revolver
  loaded, and doubling the price on survival has a sure-fire (pun intended) 67%
  average profit: $\frac{5}{6} \cdot 2 \approx 1.67$. Play it 27 times,
  and you'll turn your $1K into $1B. I leave the probability of survival as an
  exercise to the reader.

There are, of course, still many questions about the validity of such modeling
which we are going to ask and research further in this study, but as our first
working hypothesis, we are going to assume that a price series can be modeled
this way, and that $X$ is in fact a random variable with a statistical
distribution.

## Daily Closing Prices

Note, that the formula for $X$ is independent of the frequency of price
sampling. That is, $t+1$ can mean the next second, the next day, or the next
year, and the definition will still work (the typical mean and MAD will, of
course, change). In this study, I'm going to use the daily frequency, and
specifically, the daily closing prices as $P(t)$ data points.

One of the reasons is that daily prices is the highest possible frequency that
may be relevant to an investor.  If we take any higher frequency, say, hourly
prices, they usually have an unusually large jump between trading sessions, and
therefore, modeling stock prices at such frequency requires two different random
variables with different distributions - intra-session and inter-session, which
in my opinion unnecessarily complicates things - see the KISS principle in the
[Methodology] section.

Daily prices are also the most conveniently available form of data to the
general public. For instance, [Nasdaq Data Link] has equity and ETF daily prices
for US stock exchanges going back from 1998 to the current day.

Note: I don't have any affiliation with Nasdaq Data Link or Sharadar, and I only
mention it because these experiments are done using their data, and there is an
app in [stockparfait/stockparfait] repository for downloading their price series
tables.

*Next: [Distribution of Log-Profits]*

[Methodology]: ../methodology/
[Distribution of Log-Profits]: ../distribution/
[Nasdaq Data Link]: https://data.nasdaq.com/databases/SFB/data
[stockparfait/stockparfait]: https://github.com/stockparfait/stockparfait
