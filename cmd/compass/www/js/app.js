(function () {
  function wsURL() {
    var loc = window.location;
    var proto = loc.protocol === 'https:' ? 'wss:' : 'ws:';
    return proto + '//' + loc.host + '/websocket';
  }

  function vecMag(v) {
    if (!v) return null;
    return Math.sqrt(v.x*v.x + v.y*v.y + v.z*v.z);
  }

  function shortestDelta(a, b) {
    if (a == null || b == null) return null;
    var d = a - b;
    while (d > 180) d -= 360;
    while (d <= -180) d += 360;
    return d;
  }

  new Vue({
    el: '#app',
    data: {
      connected: false,
      ws: null,
      error: '',
      magEMA: false,
      manualHeadingInput: '',

      // calibration (from server on connect)
      k: null,
      l: null,

      // align
      alignActive: false,
      alignHeadingDeg: null,
      alignSavedAt: null,

      // geomag (WMM model at current GPS location, µT / deg)
      n0Ut: null,
      declDeg: null,
      inclDeg: null,
      fUt: null,
      hUt: null,
      xUt: null,
      yUt: null,
      zUt: null,
      geomagFallback: false,

      // headings (deg, compass convention)
      headingSensorDeg: null,
      headingVehDeg: null,
      trackTrueDeg: null,
      trackMagDeg: null,

      // imu
      accel: null,
      gyro: null,
      magRaw: null,
      magCal: null,
      magPred: null,
      tempC: null,

      // measured geomag (from cal mag + accel + GPS, where available)
      measF: null,
      measH: null,
      measZ: null,
      measInclDeg: null,
      measDeclDeg: null,
      measX: null,
      measY: null,

      // gps
      gpsLat: null,
      gpsLon: null,
      gpsAlt: null,
      gpsSpeed: null,
      gpsMode: 0,
      gpsTime: '',
    },
    computed: {
      alignReady: function () {
        if (!this.connected) return false;
        var manual = this.parsedManualHeading;
        if (manual != null) return true;
        return this.gpsMode >= 2 && this.trackTrueDeg != null;
      },
      parsedManualHeading: function () {
        var raw = this.manualHeadingInput;
        if (raw == null) return null;
        var s = raw.toString().trim();
        if (s === '') return null;
        var v = Number(s);
        if (isNaN(v)) return null;
        return v;
      },
      magCalMag: function () { return vecMag(this.magCal); },
      accelMag: function () { return vecMag(this.accel); },
      gyroMag: function () { return vecMag(this.gyro); },
      headingErrorDeg: function () {
        return shortestDelta(this.headingVehDeg, this.trackMagDeg);
      },
      gpsModeLabel: function () {
        return ['unknown','no fix','2D','3D'][this.gpsMode] || ('mode ' + this.gpsMode);
      },
      tickMarks: function () {
        var marks = [];
        for (var d = 0; d < 360; d += 30) {
          var rad = (d - 90) * Math.PI / 180; // 0° at top
          var r1 = 100, r2 = (d % 90 === 0) ? 86 : 92;
          marks.push({
            deg: d,
            x1: r1 * Math.cos(rad), y1: r1 * Math.sin(rad),
            x2: r2 * Math.cos(rad), y2: r2 * Math.sin(rad),
            lx: (d % 90 === 0) ? 76 * Math.cos(rad) : null,
            ly: (d % 90 === 0) ? 76 * Math.sin(rad) : null,
            label: (d === 0) ? 'N' : (d === 90) ? 'E' : (d === 180) ? 'S' : (d === 270) ? 'W' : null,
          });
        }
        return marks;
      },
    },
    methods: {
      needle: function (deg) {
        // Compass deg: 0=N=up, 90=E=right.
        var rad = (deg - 90) * Math.PI / 180;
        var r = 85;
        return { x: r * Math.cos(rad), y: r * Math.sin(rad) };
      },
      fmtDeg: function (v) {
        if (v == null || isNaN(v)) return '—';
        var d = v;
        while (d < 0) d += 360;
        while (d >= 360) d -= 360;
        return d.toFixed(2) + '°';
      },
      fmt2: function (v) { return (v == null || isNaN(v)) ? '—' : v.toFixed(2); },
      fmt6: function (v) { return (v == null || isNaN(v)) ? '—' : v.toFixed(6); },
      fmtVec3: function (v) {
        if (!v) return '—';
        return this.fmt2(v.x) + ', ' + this.fmt2(v.y) + ', ' + this.fmt2(v.z);
      },
      connect: function () {
        var self = this;
        var ws = new WebSocket(wsURL());
        ws.onopen = function () { self.connected = true; self.error = ''; };
        ws.onclose = function () {
          self.connected = false;
          setTimeout(self.connect, 1500);
        };
        ws.onerror = function () { /* close will follow */ };
        ws.onmessage = function (ev) { self.handle(JSON.parse(ev.data)); };
        this.ws = ws;
      },
      handle: function (m) {
        if (m.error) this.error = m.error;
        if (m.cal) { this.k = m.cal.k; this.l = m.cal.l; }
        if (m.align) {
          this.alignActive = !!m.align.active;
          this.alignHeadingDeg = this.alignActive ? m.align.alignHeadingDeg : null;
          this.alignSavedAt = (this.alignActive && m.align.savedAt)
            ? new Date(m.align.savedAt).toLocaleString()
            : null;
        }
        if (m.geomag) {
          this.n0Ut = m.geomag.n0Ut;
          this.declDeg = m.geomag.declDeg;
          this.inclDeg = m.geomag.inclDeg;
          this.fUt = m.geomag.fUt;
          this.hUt = m.geomag.hUt;
          this.xUt = m.geomag.xUt;
          this.yUt = m.geomag.yUt;
          this.zUt = m.geomag.zUt;
          this.geomagFallback = !!m.geomag.fallback;
        }
        if (m.magEma != null) this.magEMA = m.magEma;
        if (m.imu) {
          this.accel = m.imu.accel;
          this.gyro = m.imu.gyro;
          this.magRaw = m.imu.magRaw;
          this.magCal = m.imu.magCal;
          this.tempC = m.imu.tempC;
        }
        if (m.gps) {
          this.gpsLat = m.gps.lat;
          this.gpsLon = m.gps.lon;
          this.gpsAlt = m.gps.altM;
          this.gpsSpeed = m.gps.speedMps;
          this.gpsMode = m.gps.mode;
          this.trackTrueDeg = isNaN(m.gps.trackTrueDeg) ? null : m.gps.trackTrueDeg;
          this.trackMagDeg = isNaN(m.gps.trackMagDeg) ? null : m.gps.trackMagDeg;
          this.gpsTime = m.gps.t ? new Date(m.gps.t).toISOString() : '';
        }
        if (m.headingSensorDeg != null) this.headingSensorDeg = m.headingSensorDeg;
        if (m.headingVehDeg != null) this.headingVehDeg = m.headingVehDeg;
        if (m.predicted) this.magPred = m.predicted.magPred;
        if (m.geomagMeasured) {
          var gm = m.geomagMeasured;
          this.measF       = gm.f != null       ? gm.f       : null;
          this.measH       = gm.h != null       ? gm.h       : null;
          this.measZ       = gm.zDown != null   ? gm.zDown   : null;
          this.measInclDeg = gm.inclDeg != null ? gm.inclDeg : null;
          this.measDeclDeg = gm.declDeg != null ? gm.declDeg : null;
          this.measX       = gm.x != null       ? gm.x       : null;
          this.measY       = gm.y != null       ? gm.y       : null;
        }
      },
      send: function (msg) {
        if (this.ws && this.ws.readyState === 1) this.ws.send(JSON.stringify(msg));
      },
      setEMA: function () { this.send({ magEma: this.magEMA }); },
      doAlign: function () {
        this.error = '';
        var msg = { action: 'align' };
        var manual = this.parsedManualHeading;
        if (manual != null) msg.manualHeadingDeg = manual;
        this.send(msg);
      },
    },
    mounted: function () { this.connect(); },
  });
})();
