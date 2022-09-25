# Methodology

*Up: [Table of Contents]*

I purposefully do not follow any speficific textbook, any established trading or
investment approaches, indicators, quantitative trading strategies, or
econometric theories. Instead, I decided to start from the ground up with the
actual historical data, and see for myself what the data can tell me.

Having said that, I may provide an occasional reference to a book, an article or
a person if they provided an insight or an idea for the experiment.

The experiments follow these basic principles derived from the
[scientific method]:

- _Clear, intuitive definitions_ - every methodology, formula or model I use in
  the experiments has to be precisely defined and have a good intuitive reason
  behind it;
- _Leave no assumptions unchallenged_ - take nothing on faith, any hypothesis or
  claim must be eventually tested;

- _Verify with real data_ - every hypothesis or claim must be properly supported
  by actual data;
  - _There is no proof, only not-yet-rejected hypotheses_ - any generalizing
    model is only a hypothesis; but if it doesn't fit the data, it is definitely
    rejected;
  - _Any hypothesis must be falsifiable_ - there should always be an experiment
    that may potentially reject the hypothesis; otherwise the model has no
    value, since it fits any data, and therefore predicts nothing;
  - _Mind the p-values_ - most of the models (== hypotheses) will be statistical
    in nature, and therefore, their rejection / non-rejection should have an
    associated probability. It is of no use to say that we've rejected a
    hypothesis with 50% probability (p = 0.5), but 95% (p = 0.05) may give some
    reasonable confidence.
- _KISS: keep it simple, stupid_ - avoid unnecessary complexity; in particular,
  if something can be quickly simulated or estimated computationally, no need to
  bother with complex math.

Having said all that, the last (KISS) principle dictates that sometimes we may
take justifiable shortcuts.  For instance, when estimating a p-value formally is
too complex, practically infeasible, or simply not worth it (e.g. when it's
obviously very very small), we may resort to simpler and more intuitive methods,
such as visually noting the noise level in the graphs and concluding that this
noise likely reflects the typical error margin.

In any case, the justification for such shortcuts needs to be spelled out
explicitly, so that a thorough reader may challenge the justification and decide
if it's indeed acceptable for them.

*Next: [Market, Random Walks and Log-Profits]*

[Table of Contents]: ../README.md
[Market, Random Walks and Log-Profits]: ../logprofits/
[scientific method]: https://en.wikipedia.org/wiki/Scientific_method
