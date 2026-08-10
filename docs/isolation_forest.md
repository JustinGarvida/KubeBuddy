# Isolation Forest

Reference doc on the Isolation Forest algorithm and scikit-learn's
`IsolationForest` implementation, for when the Python anomaly detector
moves beyond the statistical baseline (z-score/EWMA) to an ML model —
see [`architecture.md`](architecture.md#open-questions--future-work).
Not implemented yet; this is a config/behavior reference to use when it is.

## How it works

Isolation Forest detects anomalies by how *easy they are to isolate*,
rather than by modeling what "normal" looks like (as density- or
distance-based methods do).

1. Build an ensemble of random trees (`ExtraTreeRegressor` internally).
   Each tree is grown from a random subsample of the data.
2. At each node, pick a random feature and a random split value between
   that feature's min and max in the current subsample.
3. Recurse until every point is isolated in its own leaf, or a max
   depth is reached.
4. A point's **path length** — the number of splits from the root to
   the leaf that isolates it — is the raw signal. Outliers tend to sit
   far from the bulk of the data, so far fewer random splits are needed
   to separate them; normal points, packed together, need many more
   splits to isolate.
5. Average the path length for a point across all trees, then
   normalize against the expected path length for a dataset of that
   size, giving each point an anomaly score:

   ```
   s(x, n) = 2^( -E[h(x)] / c(n) )
   ```

   where `E[h(x)]` is the mean path length for point `x` over the
   forest and `c(n)` is the average path length of an unsuccessful
   search in a binary search tree of `n` points — the expected path
   length if points were isolated with no structure to exploit. Scores
   close to `1` indicate anomalies, scores well below `0.5` indicate
   normal points, and scores near `0.5` mean the whole forest found
   nothing to distinguish. (This is the scoring formula from the
   original paper: Liu, Ting & Zhou, ["Isolation
   Forest"](https://ieeexplore.ieee.org/document/4781136), ICDM 2008.
   scikit-learn exposes an equivalent, differently-scaled version of
   this via `score_samples`/`decision_function` — see below.)

Each tree's max depth is capped at `ceil(log2(n))`, where `n` is the
number of samples used to build that tree — trees don't need to run to
full depth to be useful, since only path length up to that cap matters.

**Why this fits pod metrics well**: it needs no assumption that
CPU/memory usage is Gaussian (it usually isn't — bursty, skewed), no
labeled training data, and it stays cheap as the number of
tracked pods/metrics grows. That's also exactly the tradeoff the
current statistical baseline makes on purpose (see
[`architecture.md`](architecture.md#python-anomaly-detector)) — Isolation
Forest is a documented upgrade path once there's enough historical
data to train on, not a prerequisite.

## Configuration

```python
from sklearn.ensemble import IsolationForest

clf = IsolationForest(
    n_estimators=100,
    max_samples="auto",
    contamination="auto",
    max_features=1.0,
    bootstrap=False,
    n_jobs=None,
    random_state=None,
)
```

| Parameter | Type | Default | What it controls |
|---|---|---|---|
| `n_estimators` | int | `100` | Number of trees in the forest. More trees → more stable scores, more compute. Diminishing returns past a few hundred for most datasets. |
| `max_samples` | `"auto"`, int, or float | `"auto"` | Samples drawn per tree. `"auto"` = `min(256, n_samples)`. Isolation Forest was specifically designed to work well with small per-tree subsamples — larger isn't better here, since it lets anomalies "hide" among more normal points, requiring longer paths to isolate. |
| `contamination` | `"auto"` or float in `(0, 0.5]` | `"auto"` | Expected proportion of outliers. `"auto"` uses the threshold from the original paper (offset `-0.5`). An explicit float (e.g. `0.02`) sets the `predict()` decision boundary directly — use this when you have a rough estimate of the true anomaly rate; leave it at `"auto"` otherwise. |
| `max_features` | int or float | `1.0` | Number of features drawn per tree. `1.0` = all features. Lowering this decorrelates trees when there are many features, at the cost of each tree seeing less of the picture. |
| `bootstrap` | bool | `False` | Sample with (`True`) or without (`False`) replacement when drawing `max_samples`. |
| `n_jobs` | int or `None` | `None` | Parallel workers for `fit`. `-1` uses all cores. |
| `random_state` | int, `RandomState`, or `None` | `None` | Seed for reproducible tree structure — set this for reproducible anomaly scores across runs/deploys. |
| `warm_start` | bool | `False` | When `True`, a later `fit()` call adds `n_estimators - len(estimators_)` new trees to the existing forest instead of refitting from scratch — cheap way to grow the ensemble incrementally as more history accumulates. |

### Tuning notes

- **`contamination` is the main knob for alert volume.** Too high →
  noisy anomaly feed (defeats the point of the dashboard); too low →
  real incidents get missed. Given no labeled anomaly rate at first,
  start with `"auto"` and tune against what actually lands in
  `anomalies.detected` during a soak period.
- **`max_samples` shouldn't be scaled up with dataset size** the way
  it would for most other ensemble methods — the paper's finding, and
  the reason for the `256` default, is that small subsamples isolate
  anomalies *better*, not worse, because there's less normal-point
  "traffic" between an anomaly and the rest of the tree.
- **One model per metric (or per pod+metric), not one global model.**
  A pod's CPU and memory usage have different distributions, and a
  global model conflates "unusual for this pod" with "unusual across
  all pods" — the latter is closer to what the current per-pod
  z-score/EWMA baseline already does, so preserve that framing.
- Isolation Forest doesn't need to hit 100% training purity — mild
  contamination in the training set (rare, already-occurred anomalies)
  is fine, since the whole method is built around outliers being a
  minority.

## Usage

```python
from sklearn.ensemble import IsolationForest
import numpy as np

X_train = np.array([...])  # historical (cpu, memory) samples for one pod+metric

clf = IsolationForest(contamination=0.02, random_state=0)
clf.fit(X_train)

# Classify new samples
labels = clf.predict(X_new)          # -1 = anomaly, 1 = normal
scores = clf.decision_function(X_new)  # signed score; negative = anomaly
raw = clf.score_samples(X_new)        # unsigned score; lower = more anomalous
```

- `predict(X)` → `-1` (outlier) / `1` (inlier); this is what maps
  directly onto whether to publish an `anomalies.detected` event.
- `decision_function(X)` → `score_samples(X) - offset_`; signed so
  that `0` is the decision boundary — useful for severity ("how far
  past the threshold"), which the anomaly event schema already
  captures as a field (see [`architecture.md`](architecture.md#message-schemas-conceptual)).
- `score_samples(X)` → the unsigned anomaly score; lower is more
  anomalous, independent of the `contamination` threshold.
- `fit_predict(X)` → `fit` + `predict` in one call, for the common
  case of scoring the same data used to fit.

`predict`/`decision_function` can be parallelized independently of
`fit`'s `n_jobs`, via a joblib context:

```python
from joblib import parallel_backend

with parallel_backend("threading", n_jobs=4):
    clf.predict(X_new)
```

## When to reach for something else

Per scikit-learn's [outlier detection
guide](https://scikit-learn.org/stable/modules/outlier_detection.html),
Isolation Forest tends to outperform One-Class SVM for general outlier
detection and doesn't assume a Gaussian distribution the way
`EllipticEnvelope` does, which is why it's the documented ML upgrade
path here rather than those alternatives. `LocalOutlierFactor` is the
other reasonable candidate — it's density-based and can be more
sensitive to local structure, but scales worse and is less of a
natural fit for the mostly-1D-or-2D (single metric, or
CPU+memory-jointly) per-pod series this project would feed it.

## References

- [`sklearn.ensemble.IsolationForest` API
  reference](https://scikit-learn.org/stable/modules/generated/sklearn.ensemble.IsolationForest.html)
- [scikit-learn outlier detection
  guide](https://scikit-learn.org/stable/modules/outlier_detection.html)
- Liu, F. T., Ting, K. M., & Zhou, Z.-H. (2008). ["Isolation
  Forest."](https://ieeexplore.ieee.org/document/4781136) 2008 Eighth
  IEEE International Conference on Data Mining (ICDM), 413–422.
