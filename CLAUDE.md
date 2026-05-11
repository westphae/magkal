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

**Intended operating model is calibrate-then-freeze, not always-on.** The
target use case is: hand-rotate the device at home to seed an initial
calibration, then install on the glareshield where it sits at near-constant
attitude in cruise. Continuous EKF updates during cruise walk the
calibration along the unobservable `k_i(m_i − l_i)` ridge (most visibly on
the z-axis at non-trivial magnetic inclination — `cmd/replay/scripts/cruise_realistic.yaml`
shows ~460 nT of `l_z` drift over 1M cruise samples even with the EKF
properly tuned).

The intended remedy is a state machine around the filter, not more EKF
math: CALIBRATING → LOCKED when convergence is detected, LOCKED →
CALIBRATING when divergence is detected (e.g. an iPad enters the cabin
and perturbs the magnetic environment). See "Open questions / TODO" at
the bottom of this file.

The historical EKF blow-up under long-hold scenarios was fixed by sizing
`r` correctly (it had been the variance of one component of `m` rather
than of `z = ‖n̂‖²`); the residual ridge walk is much slower but still
present, and is what the lock/unlock architecture is meant to neutralize.

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
- `r` is sized as `var(z = ‖n̂‖²) ≈ (2·n0²·sigmaM)²`, not as the variance
  of one component of `m`. This is a non-obvious factor of `(2·n0)²` and
  was a multi-year latent bug; the comment in `NewKalmanFilter` explains
  the derivation. Don't "simplify" it back.
- No damping factor on the Kalman gain — the standard EKF gain (`1/s`)
  works once `r` is right. Earlier versions had `EPS = 0.1` to compensate
  for the oversized gain caused by undersized `r`; that's been removed.
- **Calibrate-then-lock state machine** (opt-in via `EnableStateMachine`):
  `Mode()` reports `ModeCalibrating` (default — every `Z` applies a full
  EKF update) or `ModeLocked` (Z is observed for NIS monitoring but
  state and P are NOT updated). Transitions:
    - CAL→LCK after `lockHysteresis` consecutive `Converged()=true`
      samples;
    - LCK→CAL when the rolling mean of `y²/s` over `nisWindow` LOCKED
      samples exceeds `nisThreshold`. On unlock the state `x` is
      preserved but `P` is re-inflated to its initial diagonal values
      (using `n0`, `sigmaK0` saved at construction) so the filter can
      relearn quickly.
  Consumers that don't call `EnableStateMachine` see no behavior change
  (cmd/websim, cmd/sim).
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
using `truth.inclination_deg`), `perturb` (additive shift to `truth.l`
with no measurements; used to simulate an external magnetic disturbance
mid-script). Each step has a `label` that flows into output so segments
can be sliced.

Canonical scenarios in `cmd/replay/scripts/`:
- `box.yaml` — well-conditioned 2D baseline.
- `cruise_long_hold.yaml`, `cruise_with_turns.yaml` — older 2D-style
  scenarios using `phi=0` (no z-component); now mainly useful as
  regression tests against the historical EKF blow-up.
- `cruise_realistic.yaml` — hand-rotate at home then long cruise hold
  with realistic geomagnetic inclination, *without* the state machine
  enabled. Demonstrates the unobservable-ridge drift on cruise.
- `cruise_locked.yaml` — same flow but with the state machine enabled.
  After home_init the filter locks and stays locked through 1M cruise
  samples with zero drift.
- `cruise_ipad.yaml` — full lock/unlock/relock cycle: home_init →
  cruise (locks) → `perturb` adds 5000 nT to the y-axis hard iron →
  cruise (NIS spike unlocks the filter; y-axis quickly relearned) →
  `random` recal (full recovery to within ~20 nT of new truth).

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

## Open questions / TODO

These are flagged for the project owner to address with real hardware and
follow-up design work; Claude should not try to resolve them autonomously.

- **Validate drift behavior against real hardware.** All current evidence
  for long-hold ridge walk comes from simulation. Run `cmd/replay`-style
  scenarios against a real magnetometer (the MPU9250 path in
  `cmd/websim/measurer.go` is the obvious re-entry point) and compare
  observed drift rates. The simulated drift may overstate or understate
  what a real sensor's noise distribution produces.
- **Tune `Converged()` thresholds against real-hardware noise.** A first
  cut of the criterion is implemented as `(*Filter).Converged()` with
  per-axis `P`-diagonal bounds (`maxSigmaK`, `maxSigmaL`) set via
  `SetConvergenceThresholds`. The `1e-3` / `5.0` values in
  `cmd/replay/scripts/cruise_realistic.yaml` are placeholders chosen
  for the simulation. Real defaults depend on (a) MEMS-magnetometer
  noise figures and (b) the operational heading-accuracy target. Owner
  should decide the latter and verify the former. If the diagonal
  criterion turns out to lie about convergence under degenerate
  observation, the follow-up is max-eigenvalue(P) instead.
- **Tune state-machine thresholds against real-hardware NIS
  distributions.** `EnableStateMachine(lockHysteresis, nisWindow,
  nisThreshold)` is implemented; the `(10, 100, 4.0)` triple in
  `cruise_locked.yaml`/`cruise_ipad.yaml` is a sim-tuned starting
  point. χ²(1) baseline says expected per-sample NIS is 1.0; in sim
  with the corrected `r`, the rolling mean sits around 0.7–1.2 during
  LCK, so a threshold around 2–4 is sensible. Real-hardware noise
  distributions may not be exactly Gaussian — verify the LCK-mode NIS
  histogram against χ²(1) and re-tune. Bigger iPads (≥5000 nT) cleanly
  trigger the unlock; smaller disturbances get absorbed as noise. This
  is the right place to put the original "sphericity / don't
  over-favor one direction" intuition, but applied at the regime-
  selection layer rather than as an EKF regularizer.

## Repo etiquette

- Binary names in `.gitignore` are `/cmd/sim/sim`, `/cmd/websim/websim`,
  `/cmd/replay/replay`. If you add a new `cmd/<x>/` binary, follow the
  same pattern.
- The commented-out MPU9250 path in `cmd/websim/measurer.go` is the only
  consumer of the `goflying` / `embd` deps. Don't prune those deps even
  though `go mod tidy` may suggest it — re-enabling that path is the
  intended use.
