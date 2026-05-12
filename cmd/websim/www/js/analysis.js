let width = 400, height=400,
    margin = {top: 20, right: 2, bottom: 20, left: 40};

// Draw Magnetometer cross-sections
// 1. Draw a dot for mx,my
// 2. Draw a small circle for lx,ly, both actual and predicted
// 3. Draw an ellipse for n^2==n0^2, both actual and predicted
function MagXSPlot(ax, ay, el) {
    let self = this;
    this.el = el;
    this.ax = ax;
    this.ay = ay;
    this.llLim=1e9;
    this.rrLim=-1e9;
    this.ttLim=-1e9;
    this.bbLim=1e9;
    this.xBuf = 0;
    this.yBuf = 0;
    this.data=[];

    this.x = d3.scaleLinear()
        .domain([this.llLim, this.rrLim])
        .range([0, width-margin.left-margin.right]);

    this.y = d3.scaleLinear()
        .domain([this.bbLim, this.ttLim])
        .range([height-margin.top-margin.bottom, 0]);

    this.xAxis = d3.axisBottom()
        .scale(this.x);

    this.yAxis = d3.axisLeft()
        .scale(this.y);

    this.svg = d3.select(el).append("svg")
        .attr("width", width)
        .attr("height", height)
        .append("g")
        .attr("transform", "translate("+margin.left+","+margin.top+")");

    this.xAxisLine = this.svg.append("g")
        .attr("class", "x axis")
        .attr("transform", "translate(0," + this.y(0) + ")")
        .call(this.xAxis);

    this.xAxisLine.append("text")
        .attr("x", 6)
        .attr("dx", ".71em")
        .style("text-anchor", "end")
        .text("m"+this.ax);

    this.yAxisLine = this.svg.append("g")
        .attr("class", "y axis")
        .attr("transform", "translate(" + this.x(0) + ",0)")
        .call(this.yAxis);

    this.yAxisLine.append("text")
        .attr("y", 6)
        .attr("dy", ".71em")
        .style("text-anchor", "end")
        .text("m"+ay);

    this.cur = this.svg.append("g");

    this.dots = this.svg.append("g");

    this.ctr = this.svg.append("circle")
        .attr("class", "center estimated")
        .attr("r", 2)
        .attr("cx", this.x(0))
        .attr("cy", this.y(0));

    this.crc = this.svg.append("ellipse")
        .attr("class", "ellipse estimated")
        .attr("cx", this.x(0))
        .attr("cy", this.y(0))
        .attr("rx", 0)
        .attr("ry", 0);

    this.ctrAct = this.svg.append("circle")
        .attr("class", "center actual")
        .attr("r", 2)
        .attr("cx", this.x(0))
        .attr("cy", this.y(0));

    this.crcAct = this.svg.append("ellipse")
        .attr("class", "ellipse actual")
        .attr("cx", this.x(0))
        .attr("cy", this.y(0))
        .attr("rx", 0)
        .attr("ry", 0);

    this.vec = this.svg.append("line")
        .attr("class", "pointer")
        .attr("x1", this.x(0))
        .attr("y1", this.y(0))
        .attr("x2", this.x(0))
        .attr("y2", this.y(0));

    this.update_state = function(datum) {
        let d = {
            'mx': datum['M'+self.ax], 'my': datum['M'+self.ay],
            'kx': datum['K'+self.ax], 'ky': datum['K'+self.ay],
            'lx': datum['L'+self.ax], 'ly': datum['L'+self.ay],
            'kActx': datum['KAct'+self.ax], 'kActy': datum['KAct'+self.ay],
            'lActx': datum['LAct'+self.ax], 'lActy': datum['LAct'+self.ay],
            'n0': datum['N0'], 'sigmaM': datum['sigmaM']
        };
        self.data.push(d);

        let ddots = self.dots.selectAll('circle').data(self.data);

        let changed = false;
        if (d['mx']-d['n0']*d['sigmaM'] < self.llLim) {
            self.llLim = d['mx']-d['n0']*d['sigmaM'];
            changed = true;
        }
        self.lLim = self.llLim;
        if (-d['n0']/d['kx']+d['lx'] < self.lLim) {
            self.lLim = -d['n0']/d['kx']+d['lx'];
            changed = true;
        }
        if (-d['n0']/d['kActx']+d['lActx'] < self.lLim) {
            self.lLim = -d['n0']/d['kActx']+d['lActx'];
            changed = true;
        }
        if (d['mx']+d['n0']*d['sigmaM'] > self.rrLim) {
            self.rrLim = d['mx']+d['n0']*d['sigmaM'];
            changed = true;
        }
        self.rLim = self.rrLim;
        if (d['n0']/d['kx']+d['lx'] > self.rLim) {
            self.rLim = d['n0']/d['kx']+d['lx'];
            changed = true;
        }
        if (d['n0']/d['kActx']+d['lActx'] > self.rLim) {
            self.rLim = d['n0']/d['kActx']+d['lActx'];
            changed = true;
        }
        if (d['my']-d['n0']*d['sigmaM'] < self.bbLim) {
            self.bbLim = d['my']-d['n0']*d['sigmaM'];
            changed = true;
        }
        self.bLim = self.bbLim;
        if (-d['n0']/d['ky']+d['ly'] < self.bLim) {
            self.bLim = -d['n0']/d['ky']+d['ly'];
            changed = true;
        }
        if (-d['n0']/d['kActy']+d['lActy'] < self.bLim) {
            self.bLim = -d['n0']/d['kActy']+d['lActy'];
            changed = true;
        }
        if (d['my']+d['n0']*d['sigmaM'] > self.ttLim) {
            self.ttLim = d['my']+d['n0']*d['sigmaM'];
            changed = true;
        }
        self.tLim = self.ttLim;
        if (d['n0']/d['ky']+d['ly'] > self.tLim) {
            self.tLim = d['n0']/d['ky']+d['ly'];
            changed = true;
        }
        if (d['n0']/d['kActy']+d['lActy'] > self.tLim) {
            self.tLim = d['n0']/d['kActy']+d['lActy'];
            changed = true;
        }

        if (changed) {
            self.xBuf = Math.max(0, (self.tLim-self.bLim)-(self.rLim-self.lLim))/2;
            self.yBuf = Math.max(0, (self.rLim-self.lLim)-(self.tLim-self.bLim))/2;
            self.x.domain([self.lLim-self.xBuf, self.rLim+self.xBuf]);
            self.xAxis.scale(self.x);
            self.y.domain([self.bLim-self.yBuf, self.tLim+self.yBuf]);
            self.yAxis.scale(self.y);
            self.xAxisLine.attr("transform", "translate(0," + self.y(0) + ")")
                .call(self.xAxis);
            self.yAxisLine.attr("transform", "translate(" + self.x(0) + ",0)")
                .call(self.yAxis);
            ddots.attr("cx", function(d) { return self.x(d['mx']); })
                .attr("cy", function(d) { return self.y(d['my']); });
        }

        let dnn = Math.max(-1, Math.min(1, (Math.sqrt(
       	        (datum["K1"]*(datum["M1"]-datum["L1"]))**2 +
                (datum["K2"]*(datum["M2"]-datum["L2"]))**2 +
                (datum["K3"]*(datum["M3"]-datum["L3"]))**2)/datum["N0"]
                - 1)/(5*datum["sigmaM"])))
        ddots.enter().append("circle")
            .attr("class", "dot")
            .attr("r", (self.x(d['n0']*d['sigmaM'])-self.x(-d['n0']*d['sigmaM']))/2)
            .style("fill", "rgb("+255*(0.5+dnn)+", 0, "+255*(0.5-dnn)+")")
            .attr("cx", function(d) { return self.x(d['mx']); })
            .attr("cy", function(d) { return self.y(d['my']); });

        ddots.exit().remove();

        self.ctr.attr("cx", self.x(d['lx']))
            .attr("cy", self.y(d['ly']));

        self.ctrAct.attr("cx", self.x(d['lActx']))
            .attr("cy", self.y(d['lActy']));

        self.crc.attr("cx", self.x(d['lx']))
            .attr("cy", self.y(d['ly']))
            .attr("rx", (self.x(d['n0']/d['kx']+d['lx']) - self.x(-d['n0']/d['kx']+d['lx']))/2)
            .attr("ry", (self.y(-d['n0']/d['ky']+d['ly']) - self.y(d['n0']/d['ky']+d['ly']))/2);

        self.crcAct.attr("cx", self.x(d['lActx']))
            .attr("cy", self.y(d['lActy']))
            .attr("rx", (self.x(d['n0']/d['kActx']+d['lActx']) - self.x(-d['n0']/d['kActx']+d['lActx']))/2)
            .attr("ry", (self.y(-d['n0']/d['kActy']+d['lActy']) - self.y(d['n0']/d['kActy']+d['lActy']))/2);

        self.vec
            .attr("x1", self.x(d['lx']))
            .attr("y1", self.y(d['ly']))
            .attr("x2", self.x(d['mx']))
            .attr("y2", self.y(d['my']))
    };

    this.update_measurement = function(datum) {
        let d = {
            'mx': datum['M'+self.ax], 'my': datum['M'+self.ay],
            'kx': datum['K'+self.ax], 'ky': datum['K'+self.ay],
            'lx': datum['L'+self.ax], 'ly': datum['L'+self.ay],
            'n0': datum['N0'], 'sigmaM': datum['sigmaM']
        };

        let changed = false;
        if (d['mx']-d['n0']*d['sigmaM'] < self.lLim) {
            self.lLim = d['mx']-d['n0']*d['sigmaM'];
            changed = true;
        }
        if (d['mx']+d['n0']*d['sigmaM'] > self.rLim) {
            self.rLim = d['mx']+d['n0']*d['sigmaM'];
            changed = true;
        }
        if (d['my']-d['n0']*d['sigmaM'] < self.bLim) {
            self.bLim = d['my']-d['n0']*d['sigmaM'];
            changed = true;
        }
        if (d['my']+d['n0']*d['sigmaM'] > self.tLim) {
            self.tLim = d['my']+d['n0']*d['sigmaM'];
            changed = true;
        }

        if (changed) {
            self.xBuf = Math.max(0, (self.tLim-self.bLim)-(self.rLim-self.lLim))/2;
            self.yBuf = Math.max(0, (self.rLim-self.lLim)-(self.tLim-self.bLim))/2;
            self.x.domain([self.lLim-self.xBuf, self.rLim+self.xBuf]);
            self.xAxis.scale(self.x);
            self.y.domain([self.bLim-self.yBuf, self.tLim+self.yBuf]);
            self.yAxis.scale(self.y);
            self.xAxisLine.attr("transform", "translate(0," + self.y(0) + ")")
                .call(self.xAxis);
            self.yAxisLine.attr("transform", "translate(" + self.x(0) + ",0)")
                .call(self.yAxis);
        }

        let dcur = self.cur.selectAll('circle').data([d]);

        dcur.enter()
            .append("circle")
            .attr("class", "cur")
            .merge(dcur)
            .attr("r", (self.x(d['n0']*d['sigmaM'])-self.x(-d['n0']*d['sigmaM']))/2)
            .attr("cx", function(d) { return self.x(d['mx']); })
            .attr("cy", function(d) { return self.y(d['my']); });

        self.vec
            .attr("x1", self.x(d['lx']))
            .attr("y1", self.y(d['ly']))
            .attr("x2", self.x(d['mx']))
            .attr("y2", self.y(d['my']))
    };

    // clear_history wipes the accumulated dot history and resets the
    // auto-fit axis limits so the next update starts from scratch. Used
    // by app.js when params arrive on Restart but the plot doesn't need
    // a full DOM rebuild (i.e., n is unchanged).
    this.clear_history = function() {
        self.data = [];
        self.dots.selectAll('circle').remove();
        self.llLim = 1e9; self.rrLim = -1e9;
        self.ttLim = -1e9; self.bbLim = 1e9;
    };
}

// Draw K-L Plots
// Parameters are e.g. ax='K1', ay='L3'
// 1. Draw a dot at K1, L3
// 3. Draw error ellipse for K1, L3
// 4. Draw lines of constant N0 for K1,L1 etc.
function KLPlot(ax, ay, el) {
    let self = this;
    this.el = el;
    this.ax = ax;
    this.ay = ay;
    this.lLim=0;
    this.rLim=0;
    this.tLim=0;
    this.bLim=0;
    this.rescaleX = false;
    this.rescaleY = false;
    this.xBuf = 0;
    this.yBuf = 0;

    this.col = "Black";
    switch (this.ax[0] + this.ay[0]) {
        case 'KK':
            this.col = "Blue";
            this.lLim = 0;
            this.bLim = 0;
            this.rescaleX = false;
            this.rescaleY = false;
            break;
        case 'LK':
            this.col = "Green";
            this.bLim = 0;
            this.rescaleX = true;
            this.rescaleY = false;
            break;
        case 'LL':
            this.col = "Red";
            this.rescaleX = true;
            this.rescaleY = true;
    }

    this.x = d3.scaleLinear()
        .domain([this.lLim, this.rLim])
        .range([0, width-margin.left-margin.right]);

    this.y = d3.scaleLinear()
        .domain([this.bLim, this.tLim])
        .range([height-margin.top-margin.bottom, 0]);

    this.xAxis = d3.axisBottom()
        .scale(this.x);

    this.yAxis = d3.axisLeft()
        .scale(this.y);

    this.svg = d3.select(this.el).append("svg")
        .attr("width", width)
        .attr("height", height)
        .append("g")
        .attr("transform", "translate("+margin.left+","+margin.top+")");

    this.xAxisLine = this.svg.append("g")
        .attr("class", "x axis")
        .attr("transform", "translate(0," + this.y(0) + ")")
        .call(this.xAxis);

    this.xAxisLine.append("text")
        .attr("x", 6)
        .attr("dx", ".71em")
        .style("text-anchor", "end")
        .text("m"+this.ax);

    this.yAxisLine = this.svg.append("g")
        .attr("class", "y axis")
        .attr("transform", "translate(" + this.x(0) + ",0)")
        .call(this.yAxis);

    this.yAxisLine.append("text")
        .attr("y", 6)
        .attr("dy", ".71em")
        .style("text-anchor", "end")
        .text("m"+this.ay);

    this.errEllipse = this.svg.append("g");

    this.errEllipse.append("circle")
        .attr("class", "center estimated")
        .attr("r", 2);

    this.crc = this.errEllipse.append("ellipse")
        .attr("class", "ellipse estimated")
        .attr("rx", 0)
        .attr("ry", 0);

    this.ctrAct = this.svg.append("circle")
        .attr("class", "center actual")
        .attr("r", 2)
        .attr("cx", this.x(0))
        .attr("cy", this.y(0));

    this.update_state = function(datum) {
        let d = {
            'x': datum[self.ax] / (self.rescaleX ? datum['N0'] : 1),
            'y': datum[self.ay] / (self.rescaleY ? datum['N0'] : 1),
            'Actx': datum[self.ax[0]+'Act'+self.ax[1]] / (self.rescaleX ? datum['N0'] : 1),
            'Acty': datum[self.ay[0]+'Act'+self.ay[1]] / (self.rescaleY ? datum['N0'] : 1),
            'Pxx': datum['P'+self.ax+self.ax] / (self.rescaleX ? datum['N0'] : 1)**2,
            'Pxy': datum['P'+self.ax+self.ay] / (self.rescaleX ? datum['N0'] : 1) / (self.rescaleY ? datum['N0'] : 1),
            'Pyy': datum['P'+self.ay+self.ay] / (self.rescaleY ? datum['N0'] : 1)**2,
            'n0': datum['N0'], 'sigmaM': datum['sigmaM']
        };

        let changed = false;
        if (d['x'] < self.lLim) {
            self.lLim = d['x'];
            changed = true;
        }
        if (d['x'] > self.rLim) {
            self.rLim = d['x'];
            changed = true;
        }
        if (d['y'] < self.bLim) {
            self.bLim = d['y'];
            changed = true;
        }
        if (d['y'] > self.tLim) {
            self.tLim = d['y'];
            changed = true;
        }

        if (changed) {
            self.xBuf = Math.max(0, (self.tLim-self.bLim)-(self.rLim-self.lLim))/2;
            self.yBuf = Math.max(0, (self.rLim-self.lLim)-(self.tLim-self.bLim))/2;
            self.x.domain([self.lLim-self.xBuf, self.rLim+self.xBuf]);
            self.xAxis.scale(self.x);
            self.y.domain([self.bLim-self.yBuf, self.tLim+self.yBuf]);
            self.yAxis.scale(self.y);
            self.xAxisLine.attr("transform", "translate(0," + self.y(0) + ")")
                .call(self.xAxis);
            self.yAxisLine.attr("transform", "translate(" + self.x(0) + ",0)")
                .call(self.yAxis);
        }

        let errEllipseData = calcEllipse(d['Pxx'], d['Pyy'], d['Pxy']);

        self.errEllipse.attr("transform", "translate(" + self.x(d['x']) + "," + self.y(d['y']) + ")");

        self.crc.attr("rx", (self.x(errEllipseData['a'])-self.x(-errEllipseData['a']))/2)
            .attr("ry", (self.x(errEllipseData['b'])-self.x(-errEllipseData['b']))/2)
            .attr("transform", "rotate("+errEllipseData['alpha']+")");

        self.ctrAct.attr("cx", self.x(d['Actx']))
            .attr("cy", self.y(d['Acty']));
    }
}

function calcEllipse(pKK, pLL, pKL) {
    if (pKK<=0) {
        console.log("invalid vcv: k,k element not positive");
        return {'a': 0, 'b': 0, 'alpha': 0}
    }
    if (pLL<=0) {
        console.log("invalid vcv: l,l element not positive");
        return {'a': 0, 'b': 0, 'alpha': 0}
    }
    let sK = Math.sqrt(pKK);
    let sL = Math.sqrt(pLL);
    if (pKL===0) {
        return {'a': sL, 'b': sK, 'alpha': 0}
    }
    let rho = pKL/(sK*sL);
    if (Math.abs(rho)>1) {
        console.log("invalid vcv: k,l element not a covariance");
        return {'a': 0, 'b': 0, 'alpha': 0}
    }

    let alpha = Math.atan2(2*rho*sK*sL, pKK-pLL)/2;
    let num = 2*pKK*pLL*(1-rho*rho);
    let den = 2*pKL/Math.sin(2*alpha);
    let a = Math.sqrt(num/(pKK+den+pLL));
    let b = Math.sqrt(num/(pKK-den+pLL));
    return {'a': a, 'b': b, 'alpha': alpha*180/Math.PI}
}

// Draw dTheta Plot
function DThetaPlot(el) {
    let self = this;
    this.el = el;
    this.tbLim = 1e9;
    this.data={};

    this.x = d3.scaleLinear()
        .domain([0, 360])
        .range([0, width-margin.left-margin.right]);

    this.y = d3.scaleLinear()
        .domain([0, 0])
        .range([height-margin.top-margin.bottom, 0]);

    this.xAxis = d3.axisBottom()
        .scale(this.x);

    this.yAxis = d3.axisLeft()
        .scale(this.y);

    this.svg = d3.select(this.el).append("svg")
        .attr("width", width)
        .attr("height", height)
        .append("g")
        .attr("transform", "translate("+margin.left+","+margin.top+")");

    this.xAxisLine = this.svg.append("g")
        .attr("class", "x axis")
        .attr("transform", "translate(0," + this.y(0) + ")")
        .call(this.xAxis);

    this.xAxisLine.append("text")
        .attr("x", 6)
        .attr("dx", ".71em")
        .style("text-anchor", "end")
        .text("Actual Heading");

    this.yAxisLine = this.svg.append("g")
        .attr("class", "y axis")
        .attr("transform", "translate(" + this.x(0) + ",0)")
        .call(this.yAxis);

    this.yAxisLine.append("text")
        .attr("y", 6)
        .attr("dy", ".71em")
        .style("text-anchor", "end")
        .text("Heading Error");

    this.thetas = d3.range(0, 360);

    this.dTheta = function(theta) {
        let dTheta = Math.atan2(
            self.data["ky"]*(self.data["n0"]*Math.sin(theta*Math.PI/180)/self.data["kActy"]+self.data["lActy"]-self.data["ly"]),
            self.data["kx"]*(self.data["n0"]*Math.cos(theta*Math.PI/180)/self.data["kActx"]+self.data["lActx"]-self.data["lx"])
        )*180/Math.PI-theta;
        if (dTheta<-180) {
            dTheta += 360;
        }
        if (dTheta>180) {
            dTheta -= 360;
        }
        return dTheta;
    };

    this.xLine = d3.line()
        .x(function (theta) { return self.x(theta); })
        .y(function(theta) { return self.y(self.dTheta(theta)); });

    this.xPath = this.svg.append("g")
        .append("path")
        .datum(this.thetas)
        .attr("class", "line")
        .attr("d", this.xLine);

    this.update_state = function(datum) {
        self.data = {
            'kx': datum['K1'], 'ky': datum['K2'],
            'lx': datum['L1'], 'ly': datum['L2'],
            'kActx': datum['KAct1'], 'kActy': datum['KAct2'],
            'lActx': datum['LAct1'], 'lActy': datum['LAct2'],
            'n0': datum['N0']
        };

        if (self.tbLim>1) {
            let rr = self.thetas.reduce(function (oMax, x) {
                let y = self.dTheta(x);
                if (y < -oMax) {
                    oMax = -y;
                }
                if (y > oMax) {
                    oMax = y;
                }
                return oMax;
            }, 0);

            let changed = false;
            if (rr<self.tbLim/2) {
                if (rr > 0.5) {
                    self.tbLim = 2 * rr;
                } else {
                    self.tbLim = 1;
                }
                changed = true;
            }
            if (rr>self.tbLim*2) {
                self.tbLim = rr;
                changed = true;
            }
            if (changed) {
                self.y.domain([-self.tbLim, self.tbLim]);
                self.yAxis.scale(self.y);
                self.xAxisLine.attr("transform", "translate(0," + self.y(0) + ")");
                self.yAxisLine.call(self.yAxis);
            }
        }

        self.xPath.attr("d", self.xLine);
    }
}

function MagInputArea(el, n, callback) {
    let self = this, i;
    this.el = el;
    this.n = n;
    this.callback = callback;
    this.width = 800;
    this.height = 400;
    this.activated = false;

    this.x = d3.scaleLinear()
        .domain([0, 360])
        .range([0, this.width]);

    this.y = d3.scaleLinear()
        .domain([-90, 90])
        .range([this.height, 0]);

    this.svg = d3.select(el).append("svg")
        .attr("width", this.width)
        .attr("height", this.height)
        .append("g");

    this.svg.append("rect")
        .attr("class", "sel-bg")
        .attr("width", this.width)
        .attr("height", this.height);

    if (this.n===1) {
        this.svg.append("rect")
            .attr("class", "sel-bg-pos")
            .attr("width", this.width/2)
            .attr("height", this.height)
            .attr("x", this.x(90));
    } else {
        for (i = 30; i < 360; i += 30) {
            this.svg.append("line")
                .attr("class", "sel-bg-grid minor")
                .attr("x1", this.x(i))
                .attr("x2", this.x(i))
                .attr("y1", 0)
                .attr("y2", this.height);
        }

        for (i = 0; i < 360; i += 90) {
            this.svg.append("line")
                .attr("class", "sel-bg-grid major")
                .attr("x1", this.x(i))
                .attr("x2", this.x(i))
                .attr("y1", 0)
                .attr("y2", this.height);
        }

        if (this.n>2) {
            for (i = -60; i < 90; i += 30) {
                this.svg.append("line")
                    .attr("class", "sel-bg-grid minor")
                    .attr("x1", 0)
                    .attr("x2", self.width)
                    .attr("y1", self.y(i))
                    .attr("y2", self.y(i));
            }
        }
    }

    this.svg.append("line")
        .attr("class", "sel-bg-grid major")
        .attr("x1", 0)
        .attr("x2", this.width)
        .attr("y1", this.y(0))
        .attr("y2", this.y(0));

    this.svg.append("line")
        .attr("class", "sel-bg-grid major")
        .attr("x1", this.x(90))
        .attr("x2", this.x(90))
        .attr("y1", 0)
        .attr("y2", this.height);

    this.svg.append("line")
        .attr("class", "sel-bg-grid major")
        .attr("x1", this.x(270))
        .attr("x2", this.x(270))
        .attr("y1", 0)
        .attr("y2", this.height);

    this.xline = this.svg.append("line")
        .attr("class", "sel")
        .attr("visibility", "hidden")
        .attr("y1", 0)
        .attr("y2", this.height);

    this.yline = this.svg.append("line")
        .attr("class", "sel")
        .attr("visibility", "hidden")
        .attr("x1", 0)
        .attr("x2", this.width);

    // Order matters for z-order: history dots underneath the current
    // marker so the crosshair stays legible.
    this.dots = this.svg.append("g");
    this.cur = this.svg.append("g");
    this.history = [];

    let ptrMove = function(ev) {
        if (self.activated) {
            self.xline.attr("visibility", "visible")
                .attr("x1", ev[0])
                .attr("x2", ev[0]);
            self.yline.attr("visibility", "visible")
                .attr("y1", ev[1])
                .attr("y2", ev[1]);
            self.callback([self.x.invert(ev[0]), self.y.invert(ev[1])]);
        }
    };

    this.svg.append("rect")
        .attr("width", this.width)
        .attr("height", this.height)
        .attr("opacity", 0)
        .on("touchstart", function() {
            let ev = d3.touches(this)[0];
            d3.event.preventDefault();
            console.log("touchstart");
            self.activated = true;
            ptrMove(ev);
        }, {"passive": false, "capture": true})
        .on("touchmove", function() {
            let ev = d3.touches(this)[0];
            d3.event.preventDefault();
            console.log("touchmove");
            ptrMove(ev);
        }, {"passive": false, "capture": true})
        .on("touchend", function() {
            let ev = d3.touches(this)[0];
            d3.event.preventDefault();
            console.log("touchend");
            self.activated = false;
            self.xline.attr("visibility", "hidden");
            self.yline.attr("visibility", "hidden");
            self.callback([self.x.invert(ev[0]), self.y.invert(ev[1])]);
        }, {"passive": false, "capture": true})
        .on("touchcancel", function() {
            let ev = d3.touches(this)[0];
            d3.event.preventDefault();
            console.log("touchcancel");
            self.activated = false;
            self.xline.attr("visibility", "hidden");
            self.yline.attr("visibility", "hidden");
        })
        .on("touchleave", function() {
            let ev = d3.touches(this)[0];
            d3.event.preventDefault();
            console.log("touchleave");
            self.activated = false;
            self.xline.attr("visibility", "hidden");
            self.yline.attr("visibility", "hidden");
        })
        .on("mousedown", function() {
            let ev = d3.mouse(this);
            console.log("mousedown");
            self.activated = true;
            ptrMove(ev);
        }, true)
        .on("mousemove", function() {
            let ev = d3.mouse(this);
            console.log("mousemove");
            ptrMove(ev);
        }, true)
        .on("mouseup", function() {
            let ev = d3.mouse(this);
            console.log("mouseup");
            self.activated = false;
            self.xline.attr("visibility", "hidden");
            self.yline.attr("visibility", "hidden");
            self.callback([self.x.invert(ev[0]), self.y.invert(ev[1])])
        }, {"passive": false, "capture": true})
        .on("mousecancel", function() {
            let ev = d3.mouse(this);
            console.log("mousecancel");
            self.activated = false;
            self.xline.attr("visibility", "hidden");
            self.yline.attr("visibility", "hidden");
        }, {"passive": false, "capture": true})
        .on("mouseleave", function() {
            let ev = d3.mouse(this);
            console.log("mouseleave");
            self.activated = false;
            self.xline.attr("visibility", "hidden");
            self.yline.attr("visibility", "hidden");
        }, {"passive": false, "capture": true});

    this.update_measurement = function(datum) {
        // Plot each measurement at the corrected (theta, phi) implied by
        // the filter's CURRENT k/l estimate — not by KAct/LAct (which are
        // truth or placeholder values, useless for the Actual source).
        // Each dot is frozen at the position computed at the moment it
        // came in; we deliberately don't re-project old dots when the
        // estimate later improves. That makes the trail itself a
        // diagnostic: as the EKF converges, fresh dots cluster on the
        // unit sphere while early ones (taken with a worse estimate)
        // stay where they were placed.
        let n1 = datum['K1']*(datum['M1']-datum['L1']),
            n2 = datum['K2']*(datum['M2']-datum['L2']),
            n3 = datum['K3']*(datum['M3']-datum['L3']);
        let nn = Math.sqrt(n1*n1+n2*n2+n3*n3);
        let phi = this.n===3 ? Math.asin(n3/nn) : 0;
        let theta = Math.atan2(n2/(nn*Math.cos(phi)), n1/(nn*Math.cos(phi)));
        let d = {
            'theta': (theta<0 ? theta+2*Math.PI : theta)*180/Math.PI,
            'phi': phi*180/Math.PI,
            'n0': datum['N0'], 'sigmaM': datum['sigmaM']
        };

        let dcur = self.cur.selectAll('circle').data([d]);

        dcur.enter()
            .append("circle")
            .attr("class", "cur")
            .attr("r", 4)
            .merge(dcur)
            .attr("cx", function(d) { return self.x(d['theta']); })
            .attr("cy", function(d) { return self.y(d['phi']); });

        // Accumulate history. Skip duplicates that arrive from the
        // continuous mouse-move stream by only adding once per distinct
        // (theta, phi) pair. .dot is the same class as MagXSPlot uses,
        // so it gets the same low-opacity styling.
        let last = self.history[self.history.length - 1];
        if (!last || last.theta !== d.theta || last.phi !== d.phi) {
            self.history.push(d);
            let ddots = self.dots.selectAll('circle.dot').data(self.history);
            ddots.enter().append('circle')
                .attr('class', 'dot')
                .attr('r', 3)
                .style('fill', 'steelblue')
                .merge(ddots)
                .attr('cx', function(p) { return self.x(p.theta); })
                .attr('cy', function(p) { return self.y(p.phi); });
        }
    };

    this.clear_history = function() {
        self.history = [];
        self.dots.selectAll('circle.dot').remove();
        self.cur.selectAll('circle').remove();
    };
}
