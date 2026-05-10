# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project purpose

Prototype and test a system that auto-calibrates a magnetometer using an
extended Kalman filter. The intended consumer is a small-aircraft heading
system (which feeds further derived quantities like winds aloft), though the
package is meant to be reusable by anyone working with a magnetometer. The
per-axis calibration model is `n = k * (m - l)` where `m` is the raw sensor
reading, `k` is the scale factor, `l` is the hard-iron offset, and `n` is
the true field component. The known quantity is the Earth-field magnitude
`n0 = ‖n‖`, so the filter never sees `n` directly — it only knows
`‖n‖² = n0²` and uses that scalar measurement to estimate `(k_i, l_i)` for
each axis.

**Convergence under degenerate observation distributions is the central open
problem.** In flight, an aircraft can hold a near-constant attitude for tens
or hundreds of thousands of measurements (cruising straight and level), and
this filter has historically blown up under that regime. The shelved
aggregator branch (`caf0738`) was an attempt to mitigate it by downweighting
redundant observations; that approach was abandoned as too burdensome.
Any change to the filter should be evaluated against long-hold scenarios,
not just well-distributed sweeps.

## Commands

- Build everything: `go build ./...`
- Run the CLI sim: `cd cmd/sim && go run . [n]` — `n` is the dimension count
  (1, 2, or 3; default 2). Reads angles from stdin one at a time; `a` runs
  a full sweep; `q` quits. Largely superseded by `cmd/replay` for new
  work; kept for quick interactive pokes.
- Run the web sim: `cd cmd/websim && go run .`, then open
  `http://localhost:8000`. Must be invoked from `cmd/websim/` because
  `main.go` serves `http.Dir("www")` with a relative path.
- Run a scripted scenario: `go run ./cmd/replay <script.yaml>`
  — see "Test harness: `cmd/replay`" below.
- No `*_test.go` files yet; no Makefile.
- Go 1.18 (`go.mod`).

## Architecture

### `pkg/kalman/` — the EKF

- `Filter` is created by `NewKalmanFilter`, which spawns a goroutine
  (`runFilter`) and returns the struct. Two channels drive it:
  - `U chan Matrix` — the raw magnetometer reading, treated as the "control"
    input (drives the predict step).
  - `Z chan float64` — the scalar measurement `n0²` (drives the EKF update
    step).
- On `U` the filter does no x-evolution, only `p += q`. On `Z` it forms
  innovation `y = z - Σ n̂_i²`, computes the Jacobian of `‖n̂‖²` w.r.t. the
  state, and applies a standard EKF correction.
- **State layout:** `x[2*i]` is `k_i`, `x[2*i+1]` is `l_i`. A 3-axis filter
  therefore has `n=3` and `len(x)=6`. `K()`, `L()`, the Jacobian `h`, and
  `calcMagField` all rely on this interleaving — don't change it without
  updating every reader.
- **`EPS = 0.1`** in `kalman.go` is a hand-tuned damping factor on the Kalman
  gain that makes the linearization behave; it is *not* numerical epsilon.
  Don't remove or "clean up" it.
- `matrices.go` defines `type Matrix [][]float64` and the basic linalg
  helpers. The repo deliberately uses plain slices instead of `gonum`.
- The package doc comment ("aggregates measurements") is stale — the
  aggregator concept was split out to a separate branch (commit `caf0738`).
  Don't trust it as a description of current behavior.

### `cmd/replay/` — scripted headless harness

Reads a YAML scenario (truth k/l/n0, filter init params, RNG seed, ordered
list of measurement-generating steps) and runs the same measure → estimate
loop the websim does, against a real `pkg/kalman.Filter`. Emits an aligned
text table (default) or CSV (`--csv`) one row per filter step. The point
is a fast, reproducible, scriptable iteration loop for filter work — both
for human inspection and for me (Claude) to read state without parsing
the websim's D3 plots.

Step kinds: `sweep` (walk angles), `hold` (cluster around one (theta,phi)
in measurer convention with Gaussian jitter), `random` (uniform on
circle/sphere), `body_frame` (aircraft holding a nominal (heading, pitch,
roll) with jitter on each, with the Earth field rotated into body frame
using `truth.inclination_deg`). Each step has a `label` that flows into
output so segments can be sliced.

Canonical scenarios in `cmd/replay/scripts/`:
- `box.yaml` — well-conditioned 2D baseline.
- `cruise_long_hold.yaml`, `cruise_with_turns.yaml` — older 2D-style
  scenarios using `phi=0` (no z-component); now mainly useful as
  regression tests against the historical EKF blow-up.
- `cruise_realistic.yaml` — the intended workflow: hand-rotate at
  home, then long cruise hold with realistic geomagnetic inclination
  and small attitude jitter. The current open problem (long-hold drift
  along the unobservable l-axis) is most visible here.

Useful flags: `--every N` to downsample long runs, `--csv` for
spreadsheet/plot input, `--summary` for a one-line per-segment digest to
stderr, `--no-records` to suppress per-step output, `--verbose` to let
the EKF's debug `log.Printf`s through (muted by default — they're noisy).

`pkg/kalman.Filter` exposes a buffered (size 1) `Done` channel; replay
drains it before sending Z and reads it after, to defeat the read-after-
send race that would otherwise make per-step state reads stale. Existing
callers (websim) ignore `Done` and behave unchanged.

### `cmd/sim/` — interactive CLI

Reads angles from stdin, generates synthetic measurements, feeds the filter,
prints state. Largely obsoleted by `cmd/replay` (which is scriptable and
machine-readable), but useful for quick interactive pokes. Drains/waits on
`kf.Done` like replay does, so the loop's `printState()` reflects the
post-update state.

### `cmd/websim/` — websocket-driven web UI

- `main.go` serves `www/` and `/websocket`. Each websocket connection owns
  its own `measurer` (synthetic data source) and `*kalman.Filter`.
- Wire protocol is JSON `messageIn` / `messageOut`. `messageIn` carries
  exactly one of `params` / `measure` / `estimate`; the server replies with
  the corresponding `params` / `measurement` / `state` field set.
- `measurer.go` defines the `measurer` interface and several sources:
  - `manual` — measurement direction supplied by the client.
  - `random` — auto-generated angles; the 2D version simulates a slowly
    rotating object via a small random-walk on the rotation rate.
  - `actual` — real MPU9250 sensor; **commented out** but its imports
    (`goflying`, `embd` via `go.sum`) are kept so it can be re-enabled.
  - `file` — TODO.
- `analysis.go` — `CalcEllipse` derives the semi-axes and rotation of a 2D
  confidence ellipse from a 2×2 sub-block of the state covariance, for
  plotting.
- `www/` is a Vue 2 + Materialize + D3 single-page app. `app.js` owns the
  websocket and the parameter form; `analysis.js` (~800 lines) renders the
  live D3 plots, wired together via a `d3.dispatch` event bus
  (`measure_request`, `measurement`, `estimate_request`, `estimate`).

## Repo etiquette

- Binary names in `.gitignore` are `/cmd/sim/sim`, `/cmd/websim/websim`,
  `/cmd/replay/replay`. If you add a new `cmd/<x>/` binary, follow the
  same pattern.
- The commented-out MPU9250 path in `cmd/websim/measurer.go` is the only
  consumer of the `goflying` / `embd` deps. Don't prune those deps even
  though `go mod tidy` may suggest it — re-enabling that path is the
  intended use.
