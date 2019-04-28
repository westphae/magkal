var params = {},
    self;

vm = new Vue({
    el: '#app',

    data: {
        ws: null, // Our websocket
        source: 0, // Data source selected by the user: manual, random, file, actual
        n: 1, // Number of dimensions
        n0: 1.0, // Strength of magnetic field
        nSigma: 0.1, // Standard deviation of uncertainty of k
        epsilon: 0.01, // Small noise scale
        msgContent: '', // A running list of data messages displayed on the screen
        params: params
    },

    created: function() {
        self = this;
        this.ws = new WebSocket('ws://' + window.location.host + '/websocket');
        this.ws.addEventListener('message', handleMessages);

        params = {
            source: this.source,
            n: this.n,
            n0: this.n0,
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
        check_nSigma: function() {
            if (this.nSigma <= 0) { this.nSigma = params.nSigma; }
        },
        check_epsilon: function() {
            if (this.epsilon <= 0) { this.epsilon = params.epsilon; }
        },
        start: function () {
            params = {
                source: parseInt(this.source),
                n: this.n,
                n0: this.n0,
                nSigma: this.nSigma,
                epsilon: this.epsilon
            };

            console.log("sent: " + JSON.stringify(params));
            this.ws.send(
                JSON.stringify(params)
            );
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

    var element = document.getElementById('messages');
    element.scrollTop = element.scrollHeight; // Auto scroll to the bottom
}

document.addEventListener('DOMContentLoaded', function() {
    var elems = document.querySelectorAll('select');
    var options = {};
    var instances = M.FormSelect.init(elems, options);
});
