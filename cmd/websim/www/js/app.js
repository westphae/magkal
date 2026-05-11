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
        kAct0: 1.0, kAct1: 1.0, kAct2: 1.0,
        lAct0: 0.0, lAct1: 0.0, lAct2: 0.0,
        sigmaK0: 0.25,
        sigmaK: 0.01,
        sigmaM: 0.05,
        // Convergence + state-machine tuning
        maxSigmaK: 0,
        maxSigmaL: 0,
        smOn: false,
        lockHysteresis: 10,
        nisWindow: 100,
        nisThreshold: 4.0,
        // Server-reported state-machine status
        mode: '',
        nis: null,
        converged: false,
        // Scenario state
        scenarios: [],
        scenarioPick: '',
        playback: { step: 0, total: 0, segment: '', playing: false, seeking: false, rateHz: 10, loaded: '' },
        rateHz: 10,
        msgContent: '',
        params: params,
        measuring: false,
        connected: false,
        msmts: null,
        mxs_update: null,
        k1l1_update: null,
        dTheta_update: null,
        k2l2_update: null,
        kk_update: null,
        ll_update: null,
        data: {}
    },

    created: function() {
        self = this;
        this.ws = new WebSocket('ws://' + window.location.host + '/websocket');
        this.ws.addEventListener('open', function() { self.connected = true; });
        this.ws.addEventListener('close', function() { self.connected = false; });
        this.ws.addEventListener('message', this.handleMessages);
        dispatch.on("measure_request", function(msg) {
            self.ws.send(JSON.stringify({"measure": msg}));
        });
        dispatch.on("estimate_request", function(msg) {
            self.ws.send(JSON.stringify({"estimate": msg}));
        });
        dispatch.on("measurement", function() {
            dispatch.call("estimate_request", this, {"nn": self.n0 * self.n0});
        });
    },

    watch: {
        // Materialize wraps <select> elements with a styled overlay that's
        // built at init time; when v-if reveals a new <select> or the
        // <option> list changes, we need to reinitialize so the visible
        // dropdown matches the underlying element.
        source: function () { this.reinitSelects(); },
        scenarios: function () { this.reinitSelects(); }
    },

    methods: {
        reinitSelects: function () {
            this.$nextTick(function () {
                var elems = document.querySelectorAll('select');
                M.FormSelect.init(elems, {});
            });
        },
        check_n: function() {
            if (this.n !== Math.floor(this.n) || this.n < 1 || this.n > 3) { this.n = params.n; }
        },
        check_n0: function() { if (this.n0 <= 0) { this.n0 = params.n0; } },
        check_kAct: function() {
            var n = parseInt(this.n);
            if (this.kAct0<=0) { this.kAct0 = params.kAct[0]; }
            if (n>=2 && this.kAct1<=0) { this.kAct1 = params.kAct[1]; }
            if (n===3 && this.kAct2<=0) { this.kAct2 = params.kAct[2]; }
        },
        check_lAct: function() { },
        check_sigmaK0: function() { if (this.sigmaK0 <= 0) { this.sigmaK0 = params.sigmaK0; } },
        check_sigmaK: function() { if (this.sigmaK <= 0) { this.sigmaK = params.sigmaK; } },
        check_sigmaM: function() { if (this.sigmaM <= 0) { this.sigmaM = params.sigmaM; } },
        check_params_changed: function() {
            if (!params.kAct || !params.lAct) { return true; }
            return !(
                params.source === parseInt(this.source) &&
                params.n === this.n &&
                params.n0 === this.n0 &&
                params.kAct[0] === this.kAct0 &&
                params.kAct[1] === this.kAct1 &&
                params.kAct[2] === this.kAct2 &&
                params.lAct[0] === this.lAct0 &&
                params.lAct[1] === this.lAct1 &&
                params.lAct[2] === this.lAct2 &&
                params.sigmaK0 === this.sigmaK0 &&
                params.sigmaK === this.sigmaK &&
                params.sigmaM === this.sigmaM
            );
        },
        restart: function () {
            params = {
                source: parseInt(this.source),
                n: this.n,
                n0: this.n0,
                kAct: [this.kAct0, this.kAct1, this.kAct2],
                lAct: [this.lAct0, this.lAct1, this.lAct2],
                sigmaK0: this.sigmaK0,
                sigmaK: this.sigmaK,
                sigmaM: this.sigmaM,
                maxSigmaK: this.maxSigmaK || 0,
                maxSigmaL: this.maxSigmaL || 0,
                stateMachineOn: !!this.smOn,
                lockHysteresis: this.lockHysteresis,
                nisWindow: this.nisWindow,
                nisThreshold: this.nisThreshold
            };
            this.ws.send(JSON.stringify({"params": params}));
        },
        measureOnce: function () {
            dispatch.call("measure_request", this, {"a": null});
        },
        measureMany: function () {
            measureInterval = setInterval(function () {
                dispatch.call("measure_request", this, {"a": null});
            }, 50);
            this.measuring = true;
        },
        pause: function () {
            clearInterval(measureInterval);
            this.measuring = false;
        },

        // Scenario controls
        loadScenario: function () {
            if (!this.scenarioPick) { return; }
            this.ws.send(JSON.stringify({"loadScenario": this.scenarioPick}));
        },
        playPause: function () {
            if (this.playback.playing) {
                this.ws.send(JSON.stringify({"playbackCmd": {"action": "pause"}}));
            } else {
                this.ws.send(JSON.stringify({"playbackCmd": {"action": "play", "rateHz": parseInt(this.rateHz)}}));
            }
        },
        stepOne: function () {
            this.ws.send(JSON.stringify({"playbackCmd": {"action": "step"}}));
        },
        resetScenario: function () {
            this.ws.send(JSON.stringify({"playbackCmd": {"action": "reset"}}));
        },
        setRate: function () {
            this.ws.send(JSON.stringify({"playbackCmd": {"action": "setRate", "rateHz": parseInt(this.rateHz)}}));
        },
        onScrub: function (e) {
            // Throttle slider input to ~10 Hz; the server cancels in-flight
            // seeks when a new one arrives, so rapid drag is responsive.
            var target = parseInt(e.target.value);
            if (scrubThrottleTimer) { return; }
            scrubThrottleTimer = setTimeout(function () {
                scrubThrottleTimer = null;
                self.ws.send(JSON.stringify({"playbackCmd": {"action": "seek", "step": target}}));
            }, 100);
        },

        // State machine manual controls
        forceLock: function () {
            this.ws.send(JSON.stringify({"setMode": "LCK"}));
        },
        forceUnlock: function () {
            this.ws.send(JSON.stringify({"setMode": "CAL"}));
        },

        handleMessages: function(e) {
            var msg = JSON.parse(e.data);

            // Initial list of available scenarios
            if (msg.scenarios) {
                this.scenarios = msg.scenarios;
            }

            // State machine fields (may arrive on any message)
            if (msg.hasOwnProperty('mode') && msg.mode !== null) { this.mode = msg.mode; }
            if (msg.hasOwnProperty('nis') && msg.nis !== null)   { this.nis = msg.nis; }
            if (msg.hasOwnProperty('converged') && msg.converged !== null) { this.converged = msg.converged; }
            if (msg.hasOwnProperty('playback') && msg.playback !== null)   { this.playback = msg.playback; }

            // Received params (initial or on source/scenario change)
            if (msg.params) {
                params = msg.params;
                params.source = parseInt(params.source);
                this.params = params;
                this.source = params.source;
                this.n = params.n;
                this.n0 = params.n0;
                if (params.kAct) {
                    this.kAct0 = params.kAct[0] || 1.0;
                    this.kAct1 = params.kAct[1] || 1.0;
                    this.kAct2 = params.kAct[2] || 1.0;
                }
                if (params.lAct) {
                    this.lAct0 = params.lAct[0] || 0.0;
                    this.lAct1 = params.lAct[1] || 0.0;
                    this.lAct2 = params.lAct[2] || 0.0;
                }
                this.sigmaK0 = params.sigmaK0;
                this.sigmaK = params.sigmaK;
                this.sigmaM = params.sigmaM;
                this.maxSigmaK = params.maxSigmaK || 0;
                this.maxSigmaL = params.maxSigmaL || 0;
                this.smOn = !!params.stateMachineOn;
                if (params.lockHysteresis) { this.lockHysteresis = params.lockHysteresis; }
                if (params.nisWindow)      { this.nisWindow = params.nisWindow; }
                if (params.nisThreshold)   { this.nisThreshold = params.nisThreshold; }

                this.data['M1'] = 0;
                this.data['M2'] = 0;
                this.data['M3'] = 0;
                this.data['KAct1'] = this.kAct0;
                this.data['KAct2'] = this.kAct1;
                this.data['KAct3'] = this.kAct2;
                this.data['LAct1'] = this.lAct0;
                this.data['LAct2'] = this.lAct1;
                this.data['LAct3'] = this.lAct2;
                this.data['N0'] = this.n0;
                this.data['sigmaK0'] = this.sigmaK0;
                this.data['sigmaK'] = this.sigmaK;
                this.data['sigmaM'] = this.sigmaM;

                this.msgContent = '<div class="chip">' + JSON.stringify(msg.params) + '</div><br/>';

                // Rebuild plots if anything that affects them changed (n, truth).
                // Cheap; the existing implementation already does this on every
                // params message.
                d3.select('#m-plot').selectAll('svg').remove();
                this.mxs_update = new MagXSPlot(1, 2, "#m-plot");
                dispatch.on("estimate.mxs", this.mxs_update.update_state);
                dispatch.on("measurement.mxs", this.mxs_update.update_measurement);
                this.msmts = new MagInputArea('#m-plot', this.n, function(d) {
                    dispatch.call("measure_request", self, {"a": d});
                });
                dispatch.on("measurement.msmts", this.msmts.update_measurement);
                this.k1l1_update = new KLPlot("L1", "K1", "#m-plot");
                dispatch.on("estimate.k1l1", this.k1l1_update.update_state);
                if (this.n>1) {
                    this.k2l2_update = new KLPlot("L2", "K2", "#m-plot");
                    dispatch.on("estimate.k2l2", this.k2l2_update.update_state);
                    this.kk_update = new KLPlot("K1", "K2", "#m-plot");
                    dispatch.on("estimate.kk", this.kk_update.update_state);
                    this.ll_update = new KLPlot("L1", "L2", "#m-plot");
                    dispatch.on("estimate.ll", this.ll_update.update_state);
                    this.dTheta_update = new DThetaPlot("#m-plot");
                    dispatch.on("estimate.dTheta", this.dTheta_update.update_state);
                }

                // Re-init Materialize selects since options changed.
                this.reinitSelects();
            }

            if (msg.measurement) {
                this.msgContent = '<div class="chip">' + JSON.stringify(msg.measurement) + '</div><br/>';
                this.data['M1'] = msg.measurement[0];
                if (this.n>=2) { this.data['M2'] = msg.measurement[1]; }
                if (this.n===3) { this.data['M3'] = msg.measurement[2]; }
                dispatch.call("measurement", this, this.data);
            }

            if (msg.state) {
                this.msgContent = '<div class="chip">' + JSON.stringify(msg.state) + '</div><br/>';
                this.data['K1'] = msg.state.k[0];
                this.data['L1'] = msg.state.l[0];
                this.data['PK1K1'] = msg.state.p[0][0];
                this.data['PK1L1'] = msg.state.p[0][1];
                this.data['PL1K1'] = msg.state.p[1][0];
                this.data['PL1L1'] = msg.state.p[1][1];
                if (this.n>=2) {
                    this.data['K2'] = msg.state.k[1];
                    this.data['L2'] = msg.state.l[1];
                    this.data['PK1K2'] = msg.state.p[0][2];
                    this.data['PK1L2'] = msg.state.p[0][3];
                    this.data['PL1K2'] = msg.state.p[1][2];
                    this.data['PL1L2'] = msg.state.p[1][3];
                    this.data['PK2K1'] = msg.state.p[2][0];
                    this.data['PK2L1'] = msg.state.p[2][1];
                    this.data['PK2K2'] = msg.state.p[2][2];
                    this.data['PK2L2'] = msg.state.p[2][3];
                    this.data['PL2K1'] = msg.state.p[3][0];
                    this.data['PL2L1'] = msg.state.p[3][1];
                    this.data['PL2K2'] = msg.state.p[3][2];
                    this.data['PL2L2'] = msg.state.p[3][3];
                }
                if (this.n===3) {
                    this.data['K3'] = msg.state.k[2];
                    this.data['L3'] = msg.state.l[2];
                    this.data['PK1K3'] = msg.state.p[0][4];
                    this.data['PK1L3'] = msg.state.p[0][5];
                    this.data['PL1K3'] = msg.state.p[1][4];
                    this.data['PL1L3'] = msg.state.p[1][5];
                    this.data['PK2K3'] = msg.state.p[2][4];
                    this.data['PK2L3'] = msg.state.p[2][5];
                    this.data['PL2K3'] = msg.state.p[3][4];
                    this.data['PL2L3'] = msg.state.p[3][5];
                    this.data['PK3K1'] = msg.state.p[4][0];
                    this.data['PK3L1'] = msg.state.p[4][1];
                    this.data['PK3K2'] = msg.state.p[4][2];
                    this.data['PK3L2'] = msg.state.p[4][3];
                    this.data['PK3K3'] = msg.state.p[4][4];
                    this.data['PK3L3'] = msg.state.p[4][5];
                    this.data['PL3K1'] = msg.state.p[5][0];
                    this.data['PL3L1'] = msg.state.p[5][1];
                    this.data['PL3K2'] = msg.state.p[5][2];
                    this.data['PL3L2'] = msg.state.p[5][3];
                    this.data['PL3K3'] = msg.state.p[5][4];
                    this.data['PL3L3'] = msg.state.p[5][5];
                }
                dispatch.call("estimate", this, this.data);
            }

            var element = document.getElementById('messages');
            if (element) { element.scrollTop = element.scrollHeight; }
        }
    }
});

document.addEventListener('DOMContentLoaded', function() {
    var elems = document.querySelectorAll('select');
    M.FormSelect.init(elems, {});
});
