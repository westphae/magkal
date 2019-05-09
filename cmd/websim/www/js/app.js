var params = {},
    self,
    measureInterval;

vm = new Vue({
    el: '#app',

    data: {
        ws: null,              // Our websocket
        source: 0,             // Data source selected by the user: manual, random, file, actual
        n: 3,                  // Number of dimensions
        n0: 1.0,               // Strength of magnetic field
        kAct: [1.0, 1.0, 1.0], // Actual k for manual or random measurement sources
        lAct: [0.0, 0.0, 0.0], // Actual l for manual or random measurement sources
        nSigma: 0.1,           // Standard deviation of uncertainty of k
        epsilon: 0.01,         // Small noise scale
        msgContent: '',        // A running list of data messages displayed on the screen
        params: params,        // Actual parameters currently being used, will be replaced by above when sent to server
        measuring: false,      // Are we measuring continuously?
        connected: false,      // Whether or not the websocket is connected
        mxs_update: null,             // M cross-section plot
        data: {}               // Data to pass into plots
    },

    created: function() {
        self = this;

        this.ws = new WebSocket('ws://' + window.location.host + '/websocket');
        this.ws.addEventListener('open', function() { self.connected = true; });
        this.ws.addEventListener('close', function() { self.connected = false; });
        this.ws.addEventListener('message', this.handleMessages);
    },

    methods: {
        check_n: function() {
            if (
                this.n !== Math.floor(this.n) ||
                this.n < 1 ||
                this.n > 3
            ) { this.n = params.n; }
        },
        check_n0: function() {
            if (this.n0 <= 0) { this.n0 = params.n0; }
        },
        check_kAct: function() {
            var n = parseInt(this.n);
            if (this.kAct[0]<=0) { this.kAct[0] = params.kAct[0]; }
            if (n>=2 && this.kAct[1]<=0) { this.kAct[1] = params.kAct[1]; }
            if (n===3 && this.kAct[2]<=0) { this.kAct[2] = params.kAct[2]; }
        },
        check_lAct: function() { },
        check_nSigma: function() {
            if (this.nSigma <= 0) { this.nSigma = params.nSigma; }
        },
        check_epsilon: function() {
            if (this.epsilon <= 0) { this.epsilon = params.epsilon; }
        },
        check_params_changed: function() {
            return !(
                params.source === parseInt(this.source) &&
                params.n === this.n &&
                params.n0 === this.n0 &&
                params.kAct[0] === this.kAct[0] &&
                params.kAct[1] === this.kAct[1] &&
                params.kAct[2] === this.kAct[2] &&
                params.lAct[0] === this.lAct[0] &&
                params.lAct[1] === this.lAct[1] &&
                params.lAct[2] === this.lAct[2] &&
                params.nSigma === this.nSigma &&
                params.epsilon === this.epsilon
            )
        },
        restart: function () {
            params = {
                source: parseInt(this.source),
                n: this.n,
                n0: this.n0,
                kAct: [this.kAct[0], this.kAct[1], this.kAct[2]],
                lAct: [this.lAct[0], this.lAct[1], this.lAct[2]],
                nSigma: this.nSigma,
                epsilon: this.epsilon
            };

            var msg = {"params": params};
            this.ws.send(
                JSON.stringify(msg)
            );
            console.log("sent: ");
            console.log(msg);
        },
        measureOnce: function () {
            var msg = {"measure": {"m0": null}};
            this.ws.send(
                JSON.stringify(msg)
            );
            console.log("measuring once, sent: ");
            console.log(msg);
        },
        measureMany: function () {
            measureInterval = setInterval(function () {
                var msg = {"measure": {"m0": null}};
                self.ws.send(
                    JSON.stringify(msg)
                );
            }, 50);
            this.measuring = true;
            console.log("measuring many");
        },
        pause: function () {
            clearInterval(measureInterval);
            this.measuring = false;
            console.log("pausing");
        },
        handleMessages: function(e) {
            var msg = JSON.parse(e.data);
            console.log("received:");
            console.log(msg);

            // Handle received params
            if (msg.hasOwnProperty('params') && msg.params!==null) {
                params = msg['params'];
                params.source = parseInt(params.source);
                this.params = params;
                this.source = params.source;
                this.n = params.n;
                this.n0 = params.n0;
                this.kAct = params.kAct;
                this.lAct = params.lAct;
                this.nSigma = params.nSigma;
                this.epsilon = params.epsilon;

                this.data['M1'] = 0;
                this.data['M2'] = 0;
                this.data['M3'] = 0;
                this.data['KAct1'] = this.kAct[0];
                this.data['KAct2'] = this.kAct[1];
                this.data['KAct3'] = this.kAct[2];
                this.data['LAct1'] = this.lAct[0];
                this.data['LAct2'] = this.lAct[1];
                this.data['LAct3'] = this.lAct[2];
                this.data['N0'] = this.n0;
                this.data['NSigma'] = this.nSigma;
                this.data['Epsilon'] = this.epsilon;

                this.msgContent += '<div class="chip">' +
                    JSON.stringify(msg.params) +
                    '</div>' +
                    '<br/>';

                d3.select('#m-plot').selectAll('svg').remove();
                this.mxs_update = updateMagXS(1, 2, "#m-plot");
            }

            // Handle received measurement
            if (msg.hasOwnProperty('measurement') && msg.measurement!==null) {
                this.msgContent += '<div class="chip">' +
                    JSON.stringify(msg.measurement) +
                    '</div>' +
                    '<br/>';

                this.data['M1'] = msg.measurement[0];
                if (this.n>=2) {
                    this.data['M2'] = msg.measurement[1];
                }
                if (this.n===3) {
                    this.data['M3'] = msg.measurement[2];
                }
            }

            var element = document.getElementById('messages');
            element.scrollTop = element.scrollHeight; // Auto scroll to the bottom

            this.mxs_update(this.data);
            console.log(this.data);
        }
    }
});

document.addEventListener('DOMContentLoaded', function() {
    var elems = document.querySelectorAll('select');
    var options = {};
    var instances = M.FormSelect.init(elems, options);
});
