// Vue app for the websim UI.  The websocket message handler is the largest
// piece; everything else is wiring or display helpers.
var params = {},
    self,
    measureInterval,
    scrubThrottleTimer = null;

var dispatch = d3.dispatch("measure_request", "measurement", "estimate_request", "estimate");

vm = new Vue({
    el: '#app',

    data: {
        ws: null,
        source: 0,
        n: 3,
        n0: 1.0,
        // Truth: array form so v-for can drive the inputs.
        kAct: [1.0, 1.0, 1.0],
        lAct: [0.0, 0.0, 0.0],
        // Server-persisted "known best" calibration for the Actual source.
        // Loaded from ~/.config/magkal/best_fit.json on connect; updated only on
        // explicit Save Best clicks. Drives the dark-ellipse reference,
        // the Actx/Acty markers on the plots, and the Δk/Δl column in
        // the state table; Δ then meaningfully shows how the live filter
        // diverges from the saved baseline.
        bestK: [1.0, 1.0, 1.0],
        bestL: [0.0, 0.0, 0.0],
        bestSavedAt: '',
        // Filter tuning.
        sigmaK0: 0.25,
        sigmaK: 0.01,
        sigmaM: 0.05,
        maxSigmaK: 0,
        maxSigmaL: 0,
        smOn: false,
        lockHysteresis: 10,
        nisWindow: 100,
        nisThreshold: 4.0,
        // Live filter state (for the right-hand readout, independent of D3 plots).
        estK: [1, 1, 1],
        estL: [0, 0, 0],
        trP: 0,
        // State-machine status from server.
        mode: '',
        nis: null,
        converged: false,
        // Running count of measurements dropped by the server-side outlier
        // filter (NaN/Inf, |n_est| > 10·n0, or >2× step change vs. the
        // previous accepted value). Surfaces so a glitch burst is visible.
        rejected: 0,
        // Actual-source recording: when non-blank, server appends each raw
        // measurement to cmd/replay/scripts/<recordFile>.yaml as a samples step.
        recordFile: '',
        // Live recording status mirrored from the server. Null when no
        // recording is active.
        recording: null,
        // Guided manual-calibration state. While mode==='INIT' the server
        // sends initStats on every measurement; client uses min/max to
        // preview the seed (k, l) before the user clicks Finish.
        initStats: null,
        // Latest raw measurement, mirrored from msg.measurement into a
        // reactive field so the INIT-bar's current-value dot re-renders.
        // (this.data is non-reactive — used only as a passive store for
        // analysis.js plots.)
        currentM: [null, null, null],
        // SVG dimensions for the per-axis INIT coverage bar.
        barW: 320,
        barH: 28,
        // Scenario state.
        scenarios: [],
        scenarioPick: '',
        playback: { step: 0, total: 0, segment: '', playing: false, seeking: false, rateHz: 10, loaded: '' },
        rateHz: 10,
        // Misc UI state.
        params: params,
        measuring: false,
        connected: false,
        // D3 / plot handles.  Holding them outside of `data` (Vue reactivity)
        // would be slightly cleaner, but the codebase already keeps them here.
        msmts: null,
        mxsPlots: [],   // one MagXSPlot per shown axis-pair (xy, xz, yz)
        data: {},
        _lastN: 0   // last n that drove a plot rebuild
    },

    computed: {
        manualOrRandom() { return this.source == 0 || this.source == 1; },
        kErr() { return this.estK.map((v, i) => v - (this.kAct[i] || 0)); },
        lErr() { return this.estL.map((v, i) => v - (this.lAct[i] || 0)); }
    },

    created: function() {
        self = this;
        this.ws = new WebSocket('ws://' + window.location.host + '/websocket');
        this.ws.addEventListener('open',  () => self.connected = true);
        this.ws.addEventListener('close', () => self.connected = false);
        this.ws.addEventListener('message', this.handleMessages);
        dispatch.on("measure_request",  msg => self.ws.send(JSON.stringify({"measure": msg})));
        dispatch.on("estimate_request", msg => self.ws.send(JSON.stringify({"estimate": msg})));
        dispatch.on("measurement", function() {
            // In scenario mode the server already drove the filter for this
            // measurement; we just received the raw m for the magXS plot.
            if (parseInt(self.source) === 4) return;
            dispatch.call("estimate_request", this, {"nn": self.n0 * self.n0});
        });
    },

    mounted: function() { this.refreshMaterialize(); },

    watch: {
        // Materialize wraps <select>s and <input>s with overlays built at
        // init time. When v-if reveals new inputs or values change from
        // server, we need to (a) re-init selects so dropdowns show options,
        // and (b) call updateTextFields so floating labels detect that
        // their inputs now have values and move above instead of overlapping.
        source: function () {
            // Stop any in-flight measureMany loop and drop INIT preview
            // state so old activity doesn't bleed across the switch.
            if (this.measuring) this.pause();
            this.initStats = null;
            // Auto-commit the source change to the server. Without this
            // the user must click Restart before the new source takes
            // effect — Start Cal in particular would silently drive the
            // previous (manual) measurer, which with a=null returns a
            // random angle and so behaves like the Random source. Skip
            // when the change is just echoing a server-driven params push.
            if (params.source !== parseInt(this.source)) this.restart();
            this.refreshMaterialize();
        },
        scenarios: function () { this.refreshMaterialize(); },
        kAct: { deep: true, handler() { this.refreshMaterialize(); } },
        lAct: { deep: true, handler() { this.refreshMaterialize(); } }
    },

    methods: {
        refreshMaterialize: function () {
            this.$nextTick(() => {
                // Destroy any prior FormSelect instances first. Re-init
                // without destroy stacks event handlers and -- per a
                // reported bug -- can cause the source dropdown change
                // event to fail to propagate to Vue when transitioning
                // out of Scenario mode.
                document.querySelectorAll('select').forEach(function (el) {
                    var inst = M.FormSelect.getInstance(el);
                    if (inst) inst.destroy();
                });
                M.FormSelect.init(document.querySelectorAll('select'), {});
                M.updateTextFields();
            });
        },

        // --- validation helpers (truth + filter tuning) ----------------
        check_n:       function() { if (this.n !== Math.floor(this.n) || this.n < 1 || this.n > 3) this.n = params.n; },
        check_n0:      function() { if (this.n0 <= 0) this.n0 = params.n0; },
        // Array-element setters that trigger Vue reactivity (vue 2's
        // direct-index assignment caveat). Both also validate.
        setKAct: function(i, v) {
            if (!isFinite(v) || v === 0) { v = (params.kAct && params.kAct[i]) || 1.0; }
            this.$set(this.kAct, i, v);
        },
        setLAct: function(i, v) {
            if (!isFinite(v)) { v = (params.lAct && params.lAct[i]) || 0.0; }
            this.$set(this.lAct, i, v);
        },
        check_sigmaK0: function() { if (this.sigmaK0 <= 0) this.sigmaK0 = params.sigmaK0; },
        check_sigmaK:  function() { if (this.sigmaK  <= 0) this.sigmaK  = params.sigmaK; },
        check_sigmaM:  function() { if (this.sigmaM  <= 0) this.sigmaM  = params.sigmaM; },

        check_params_changed: function() {
            if (!params.kAct || !params.lAct) return true;
            if (params.source !== parseInt(this.source) ||
                params.n !== this.n ||
                params.n0 !== this.n0 ||
                params.sigmaK0 !== this.sigmaK0 ||
                params.sigmaK !== this.sigmaK ||
                params.sigmaM !== this.sigmaM ||
                (params.recordFile || '') !== (this.recordFile || '')) return true;
            for (var i = 0; i < 3; i++) {
                if ((params.kAct[i] || 0) !== (this.kAct[i] || 0)) return true;
                if ((params.lAct[i] || 0) !== (this.lAct[i] || 0)) return true;
            }
            return false;
        },

        // --- formatting helpers used by the state readout --------------
        fmt: function(v, prec, signed) {
            if (v == null || !isFinite(v)) return '—';
            var s = Number(v).toFixed(prec);
            if (signed && v >= 0) s = '+' + s;
            return s;
        },
        fmtSci: function(v) {
            if (v == null || !isFinite(v)) return '—';
            return Number(v).toExponential(2);
        },
        formatVec: function(arr, prec) {
            if (!arr) return '';
            return arr.slice(0, this.n).map(v => Number(v).toFixed(prec)).join(', ');
        },
        errClass: function(err, smallThreshold) {
            if (err == null || !isFinite(err)) return '';
            var a = Math.abs(err);
            if (a < smallThreshold) return 'err-good';
            if (a < smallThreshold * 10) return 'err-warn';
            return 'err-bad';
        },

        // --- actions ---------------------------------------------------
        restart: function () {
            params = {
                source:  parseInt(this.source),
                n:       this.n,
                n0:      this.n0,
                kAct:    this.kAct.slice(0, 3),
                lAct:    this.lAct.slice(0, 3),
                sigmaK0: this.sigmaK0,
                sigmaK:  this.sigmaK,
                sigmaM:  this.sigmaM,
                maxSigmaK: this.maxSigmaK || 0,
                maxSigmaL: this.maxSigmaL || 0,
                stateMachineOn: !!this.smOn,
                lockHysteresis: this.lockHysteresis,
                nisWindow:      this.nisWindow,
                nisThreshold:   this.nisThreshold,
                recordFile:     this.recordFile || ''
                // Server-side Restart-time seeding now consults
                // ~/.config/magkal/best_fit.json directly (via SeedKLWithP, so the
                // saved covariance is restored too). The client no
                // longer ships seedK/seedL.
            };
            this.ws.send(JSON.stringify({"params": params}));
        },
        measureOnce: function () { dispatch.call("measure_request", this, {"a": null}); },
        measureMany: function () {
            measureInterval = setInterval(() => dispatch.call("measure_request", this, {"a": null}), 50);
            this.measuring = true;
        },
        pause: function () { clearInterval(measureInterval); this.measuring = false; },

        // Scenario controls
        loadScenario:  function () { if (this.scenarioPick) this.ws.send(JSON.stringify({"loadScenario": this.scenarioPick})); },
        playPause:     function () {
            var action = this.playback.playing ? "pause" : "play";
            var msg = {"action": action};
            if (action === "play") msg.rateHz = parseInt(this.rateHz);
            this.ws.send(JSON.stringify({"playbackCmd": msg}));
        },
        stepOne:       function () { this.ws.send(JSON.stringify({"playbackCmd": {"action": "step"}})); },
        resetScenario: function () { this.ws.send(JSON.stringify({"playbackCmd": {"action": "reset"}})); },
        setRate:       function () { this.ws.send(JSON.stringify({"playbackCmd": {"action": "setRate", "rateHz": parseInt(this.rateHz)}})); },
        onScrub:       function (e) {
            var target = parseInt(e.target.value);
            if (scrubThrottleTimer) return;
            scrubThrottleTimer = setTimeout(() => {
                scrubThrottleTimer = null;
                self.ws.send(JSON.stringify({"playbackCmd": {"action": "seek", "step": target}}));
            }, 100);
        },

        forceLock:   function () { this.ws.send(JSON.stringify({"setMode": "LCK"})); },
        forceUnlock: function () { this.ws.send(JSON.stringify({"setMode": "CAL"})); },

        // --- Server-persisted "known best" calibration -----------------
        // The save is deliberate: clicking Save Best sends the current
        // filter (k, l, P) to the server, which writes ~/.config/magkal/best_fit.json.
        // The server then echoes the new snapshot back so bestK/bestL
        // refresh without needing a separate load.
        saveBestServer: function () {
            this.ws.send(JSON.stringify({"saveBest": true}));
        },
        saveRecording: function () {
            // Force the server-side recorder to flush its buffered
            // samples to disk as a new labelled segment. Useful before
            // exiting so the user doesn't depend on graceful-shutdown
            // cleanup for the most recent samples.
            this.ws.send(JSON.stringify({"saveRecording": true}));
        },
        // Push bestK/bestL into the kAct/lAct arrays (drives the kErr/lErr
        // computeds and the editable-input v-model) and into this.data
        // (drives the dark-ellipse and Actx/Acty references inside
        // analysis.js). Called whenever best changes or Actual mode is
        // (re-)entered.
        applyBestToData: function () {
            this.kAct = this.bestK.slice();
            this.lAct = this.bestL.slice();
            for (var i = 0; i < 3; i++) {
                this.data['KAct' + (i+1)] = this.bestK[i];
                this.data['LAct' + (i+1)] = this.bestL[i];
            }
        },
        resetBest: function () {
            // Server-side reset: clears ~/.config/magkal/best_fit.json. The reply
            // (empty Best snapshot) drives the local state back to
            // (1, 0); plots refresh via the existing estimate dispatch.
            this.ws.send(JSON.stringify({"resetBest": true}));
        },

        // Guided-calibration controls. The whole calibration boils down
        // to "measure while hand-rotating, then commit the per-axis
        // (max+min)/2 as l-seed". So Start auto-begins sampling, Reset
        // wipes the server's INIT buffer without stopping sampling, and
        // Finish captures the seed into the persisted best estimate
        // before applying it to the filter.
        startInit:   function () {
            // Apply any pending param edits (notably recordFile) before
            // we ask the server to enter INIT. Without this, a freshly-
            // typed filename only takes effect on the next explicit
            // Restart, so the calibration-phase samples wouldn't be
            // captured in the recording.
            if (this.check_params_changed()) this.restart();
            this.initStats = null;
            this.ws.send(JSON.stringify({"startInit": true}));
            if (!this.measuring) this.measureMany();
        },
        resetInit:   function () {
            // Server-side startInit handler creates a fresh initBuffer
            // either way, so re-sending it during INIT just clears the
            // current samples. Useful if the user spots a stray magnet
            // mid-rotation and wants to restart the bracketing.
            this.initStats = null;
            this.ws.send(JSON.stringify({"startInit": true}));
        },
        finishInit:  function () {
            // Server seeds the filter from the INIT samples with a
            // principled P (σ²·(HᵀH)⁻¹). The Best Estimate is NOT
            // auto-updated here — user must click Save Best to promote
            // the new calibration once they've watched it converge.
            this.ws.send(JSON.stringify({"finishInit": true}));
        },

        // Per-axis seed preview shown in the INIT table. Matches the
        // server's seed math: l_i = (max+min)/2, k_i = n0/((max-min)/2).
        initSeedL:   function (i) {
            if (!this.initStats || this.initStats.count === 0) return null;
            return (this.initStats.max[i] + this.initStats.min[i]) / 2;
        },
        initSeedK:   function (i) {
            if (!this.initStats || this.initStats.count === 0) return null;
            var half = (this.initStats.max[i] - this.initStats.min[i]) / 2;
            if (half <= 0) return 1;
            return this.n0 / half;
        },

        // INIT-bar math. Each bar is centered on its own L_seed, but the
        // half-width (in µT) is shared across all axes so relative widths
        // are visually comparable. The default floor is ±1.15·N0; if any
        // axis's observed range or current measurement falls outside that,
        // the scale grows so the dot/rect stays on screen.
        barScale:    function () {
            if (!this.n0 || this.n0 <= 0) return 1;
            var half = this.n0 * 1.15;
            if (this.initStats && this.initStats.count > 0) {
                for (var i = 0; i < this.n; i++) {
                    var L = this.initSeedL(i);
                    if (L == null) continue;
                    half = Math.max(half,
                        Math.abs(this.initStats.min[i] - L),
                        Math.abs(this.initStats.max[i] - L));
                    var cm = this.currentM[i];
                    if (cm != null) half = Math.max(half, Math.abs(cm - L));
                }
                // Slight padding so the dot/rect doesn't kiss the edge.
                half *= 1.05;
            }
            return (this.barW / 2) / half;
        },
        barXFromVal: function (i, v) {
            var L = this.initSeedL(i);
            if (L == null || v == null) return this.barW / 2;
            return this.barW / 2 + (v - L) * this.barScale();
        },
        barWidthOf:  function (width) {
            if (!isFinite(width) || width < 0) return 0;
            return width * this.barScale();
        },

        // Fires when the user edits any convergence/state-machine field.
        // In scenario mode we apply immediately to the loaded scenario's
        // filter (the user can toggle SM mid-cruise). In other modes the
        // values are picked up on the next Restart along with the rest of
        // the params, so this is a no-op there.
        onFilterCfgChange: function () {
            if (parseInt(this.source) !== 4) return;
            if (!this.playback.loaded) return;
            this.ws.send(JSON.stringify({"playbackCmd": {
                "action":         "applyFilter",
                "maxSigmaK":      this.maxSigmaK || 0,
                "maxSigmaL":      this.maxSigmaL || 0,
                "stateMachineOn": !!this.smOn,
                "lockHysteresis": this.lockHysteresis || 0,
                "nisWindow":      this.nisWindow || 0,
                "nisThreshold":   this.nisThreshold || 0
            }}));
        },

        // --- plot rebuild ---------------------------------------------
        // analysis.js plots are created against a specific n. We only need
        // to tear them down and rebuild when n changes; previously the
        // whole graph was discarded on every params message.
        //
        // Layout: triptych of magnetic-field cross-sections (xy/xz/yz for
        // n=3, just xy for n=2, none for n=1), then the manual input area
        // below. Both for the calibration story this is meant to tell;
        // the old k/l vs k/l plots and dTheta plot have been removed.
        rebuildPlots: function () {
            var root = d3.select('#m-plot');
            root.selectAll('*').remove();

            // Drop any prior mxs* listeners; a transition to a smaller n
            // would otherwise leave dispatch entries pointing at the
            // torn-down plots from the previous build.
            for (var i = 0; i < 3; i++) {
                dispatch.on('estimate.mxs' + i,    null);
                dispatch.on('measurement.mxs' + i, null);
            }

            this.mxsPlots = [];
            var pairs = [];
            if (this.n === 2) pairs = [[1, 2]];
            else if (this.n >= 3) pairs = [[1, 2], [1, 3], [2, 3]];

            if (pairs.length > 0) {
                root.append('div').attr('id', 'triptych');
                pairs.forEach((p, idx) => {
                    var plot = new MagXSPlot(p[0], p[1], '#triptych');
                    this.mxsPlots.push(plot);
                    // Each plot needs its own dispatch namespace so all
                    // three instances receive every estimate/measurement.
                    dispatch.on('estimate.mxs' + idx,    plot.update_state);
                    dispatch.on('measurement.mxs' + idx, plot.update_measurement);
                });
            }

            root.append('div').attr('id', 'input-area');
            this.msmts = new MagInputArea('#input-area', this.n, (d) => {
                dispatch.call("measure_request", self, {"a": d});
            });
            dispatch.on("measurement.msmts", this.msmts.update_measurement);

            this._lastN = this.n;
        },

        // --- message dispatch ----------------------------------------
        handleMessages: function(e) {
            var msg = JSON.parse(e.data);

            if (msg.scenarios) this.scenarios = msg.scenarios;
            if (msg.hasOwnProperty('mode')      && msg.mode !== null)      this.mode = msg.mode;
            if (msg.hasOwnProperty('nis')       && msg.nis !== null)       this.nis = msg.nis;
            if (msg.hasOwnProperty('converged') && msg.converged !== null) this.converged = msg.converged;
            if (msg.hasOwnProperty('playback')  && msg.playback !== null)  this.playback = msg.playback;
            if (msg.hasOwnProperty('rejected')  && msg.rejected  !== null) this.rejected = msg.rejected;
            if (msg.hasOwnProperty('recording')) this.recording = msg.recording || null;
            if (msg.best) {
                // Server-side persisted best (k, l). Empty K/L (or N==0)
                // means "no saved best yet" — fall back to identity.
                var sn = msg.best;
                var hasBest = sn.n > 0 && Array.isArray(sn.k) && sn.k.length === sn.n
                              && Array.isArray(sn.l) && sn.l.length === sn.n;
                if (hasBest) {
                    this.bestK = sn.k.slice(0, 3).concat([1,1,1]).slice(0, 3);
                    this.bestL = sn.l.slice(0, 3).concat([0,0,0]).slice(0, 3);
                    this.bestSavedAt = sn.savedAt || '';
                } else {
                    this.bestK = [1, 1, 1];
                    this.bestL = [0, 0, 0];
                    this.bestSavedAt = '';
                }
                if (parseInt(this.source) === 3) {
                    this.applyBestToData();
                    // Refresh plot references so the dark ellipse / centre
                    // dot snap to the new (or reset) values immediately.
                    dispatch.call("estimate", this, this.data);
                }
            }
            if (msg.initStats) this.initStats = msg.initStats;
            // Once the server transitions out of INIT (Finish or Restart),
            // drop the stale stats so the panel collapses cleanly.
            if (msg.mode && msg.mode !== 'INIT') this.initStats = null;

            if (msg.params) {
                params = msg.params;
                params.source = parseInt(params.source);
                this.params = params;
                this.source = params.source;
                this.n = params.n;
                this.n0 = params.n0;
                // Reactive replacement of arrays so Vue tracks the change.
                if (params.kAct) this.kAct = params.kAct.slice(0, 3).concat([1,1,1]).slice(0, 3);
                if (params.lAct) this.lAct = params.lAct.slice(0, 3).concat([0,0,0]).slice(0, 3);
                // Actual mode: ignore the server-side kAct/lAct placeholder
                // and substitute the persisted best estimate so the dark
                // ellipse / delta column have meaningful values.
                if (params.source === 3) this.applyBestToData();
                this.sigmaK0 = params.sigmaK0;
                this.sigmaK  = params.sigmaK;
                this.sigmaM  = params.sigmaM;
                this.maxSigmaK = params.maxSigmaK || 0;
                this.maxSigmaL = params.maxSigmaL || 0;
                this.smOn = !!params.stateMachineOn;
                if (params.lockHysteresis) this.lockHysteresis = params.lockHysteresis;
                if (params.nisWindow)      this.nisWindow      = params.nisWindow;
                if (params.nisThreshold)   this.nisThreshold   = params.nisThreshold;
                if (params.recordFile != null) this.recordFile = params.recordFile;

                // Seed the data object used by analysis.js plots.
                this.data['N0'] = this.n0;
                this.data['sigmaK0'] = this.sigmaK0;
                this.data['sigmaK']  = this.sigmaK;
                this.data['sigmaM']  = this.sigmaM;
                for (var i = 0; i < 3; i++) {
                    this.data['M' + (i+1)]    = 0;
                    this.data['KAct' + (i+1)] = this.kAct[i] || 1;
                    this.data['LAct' + (i+1)] = this.lAct[i] || 0;
                }

                if (this._lastN !== this.n) {
                    this.rebuildPlots();
                } else {
                    // Same n -> keep DOM but reset per-plot history so
                    // Restart visibly wipes the prior measurement trail.
                    this.mxsPlots.forEach(p => {
                        if (p && p.clear_history) p.clear_history();
                    });
                    if (this.msmts && this.msmts.clear_history) this.msmts.clear_history();
                }
                this.refreshMaterialize();
            }

            if (msg.measurement) {
                for (var i = 0; i < this.n; i++) this.data['M' + (i+1)] = msg.measurement[i];
                // Reactive mirror for the INIT-bar's current-value dot.
                // Replace the whole array so Vue 2's reactivity picks it up.
                var cm = [null, null, null];
                for (var i = 0; i < Math.min(3, msg.measurement.length); i++) cm[i] = msg.measurement[i];
                this.currentM = cm;
                dispatch.call("measurement", this, this.data);
            }

            if (msg.state) {
                var p = msg.state.p;
                this.estK = msg.state.k.slice();
                this.estL = msg.state.l.slice();
                var trace = 0;
                for (var i = 0; i < p.length; i++) trace += p[i][i];
                this.trP = trace;

                // Per-axis values + cross-covariances expected by analysis.js.
                for (var i = 0; i < this.n; i++) {
                    this.data['K' + (i+1)] = msg.state.k[i];
                    this.data['L' + (i+1)] = msg.state.l[i];
                }
                for (var i = 0; i < this.n; i++) {
                    for (var j = 0; j < this.n; j++) {
                        this.data['PK' + (i+1) + 'K' + (j+1)] = p[2*i  ][2*j  ];
                        this.data['PK' + (i+1) + 'L' + (j+1)] = p[2*i  ][2*j+1];
                        this.data['PL' + (i+1) + 'K' + (j+1)] = p[2*i+1][2*j  ];
                        this.data['PL' + (i+1) + 'L' + (j+1)] = p[2*i+1][2*j+1];
                    }
                }
                dispatch.call("estimate", this, this.data);
            }
        }
    }
});

document.addEventListener('DOMContentLoaded', function() {
    M.FormSelect.init(document.querySelectorAll('select'), {});
});
