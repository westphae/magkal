var params = {},
    self;

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
        connected: false
    },

    created: function() {
        self = this;
        this.ws = new WebSocket('ws://' + window.location.host + '/websocket');
        this.ws.addEventListener('open', function() { self.connected = true; });
        this.ws.addEventListener('close', function() { self.connected = false; });
        this.ws.addEventListener('message', handleMessages);

        params = {
            source: this.source,
            n: this.n,
            n0: this.n0,
            kAct: this.kAct,
            lAct: this.lAct,
            nSigma: this.nSigma,
            epsilon: this.epsilon
        };
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
            console.log("this", this.kAct);
            console.log("pars", params.kAct);
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
                kAct: this.kAct,
                lAct: this.lAct,
                nSigma: this.nSigma,
                epsilon: this.epsilon
            };

            this.ws.send(
                JSON.stringify({"params": params})
            );
            console.log("sent: " + JSON.stringify(params));
        },
        measureOnce: function () {
            console.log("measuring once");
        },
        measureMany: function () {
            this.measuring = true;
            console.log("measuring many");
        },
        pause: function () {
            this.measuring = false;
            console.log("pausing");
        }
    }
});

function handleMessages(e) {
    console.log("received: " + e.data);
    var msg = JSON.parse(e.data);
    self.msgContent += '<div class="chip">' +
        JSON.stringify(msg) +
        '</div>' +
        '<br/>';
    console.log(self.msgContent);
    console.log(self.connected);

    var element = document.getElementById('messages');
    element.scrollTop = element.scrollHeight; // Auto scroll to the bottom
}

document.addEventListener('DOMContentLoaded', function() {
    var elems = document.querySelectorAll('select');
    var options = {};
    var instances = M.FormSelect.init(elems, options);
});
