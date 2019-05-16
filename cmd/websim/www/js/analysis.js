var width = 400, height=400,
    margin = {top: 20, right: 2, bottom: 20, left: 40};

// Draw Magnetometer cross-sections
// 1. Draw a dot for mx,my
// 2. Draw a small circle for lx,ly, both actual and predicted
// 3. Draw an ellipse for n^2==n0^2, both actual and predicted
function MagXSPlot(ax, ay, el) {
    var self = this;
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

    this.col = "Black";
    switch (this.ax+this.ay) {
        case 3:
            this.col = "Blue";
            break;
        case 4:
            this.col = "Green";
            break;
        case 5:
            this.col = "Red";
    }

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
        var d = {
            'mx': datum['M'+self.ax], 'my': datum['M'+self.ay],
            'kx': datum['K'+self.ax], 'ky': datum['K'+self.ay],
            'lx': datum['L'+self.ax], 'ly': datum['L'+self.ay],
            'kActx': datum['KAct'+self.ax], 'kActy': datum['KAct'+self.ay],
            'lActx': datum['LAct'+self.ax], 'lActy': datum['LAct'+self.ay],
            'n0': datum['N0'], 'sigmaM': datum['sigmaM']
        };
        self.data.push(d);

        var ddots = self.dots.selectAll('circle').data(self.data);

        var changed = false;
        if (d['mx']-d['n0']*d['sigmaM'] < self.llLim) {
            self.llLim = d['mx']-d['n0']*d['sigmaM'];
            changed = true;
        }
        self.lLim = self.llLim;
        if ((-d['n0']-d['lx'])/d['kx'] < self.lLim) {
            self.lLim = (-d['n0']-d['lx'])/d['kx'];
            changed = true;
        }
        if ((-d['n0']-d['lActx'])/d['kActx'] < self.lLim) {
            self.lLim = (-d['n0']-d['lActx'])/d['kActx'];
            changed = true;
        }
        if (d['mx']+d['n0']*d['sigmaM'] > self.rrLim) {
            self.rrLim = d['mx']+d['n0']*d['sigmaM'];
            changed = true;
        }
        self.rLim = self.rrLim;
        if ((d['n0']-d['lx'])/d['kx'] > self.rLim) {
            self.rLim = (d['n0']-d['lx'])/d['kx'];
            changed = true;
        }
        if ((d['n0']-d['lActx'])/d['kActx'] > self.rLim) {
            self.rLim = (d['n0']-d['lActx'])/d['kActx'];
            changed = true;
        }
        if (d['my']-d['n0']*d['sigmaM'] < self.bbLim) {
            self.bbLim = d['my']-d['n0']*d['sigmaM'];
            changed = true;
        }
        self.bLim = self.bbLim;
        if ((-d['n0']-d['ly'])/d['ky'] < self.bLim) {
            self.bLim = (-d['n0']-d['ly'])/d['ky'];
            changed = true;
        }
        if ((-d['n0']-d['lActy'])/d['kActy'] < self.bLim) {
            self.bLim = (-d['n0']-d['lActy'])/d['kActy'];
            changed = true;
        }
        if (d['my']+d['n0']*d['sigmaM'] > self.ttLim) {
            self.ttLim = d['my']+d['n0']*d['sigmaM'];
            changed = true;
        }
        self.tLim = self.ttLim;
        if ((d['n0']-d['ly'])/d['ky'] > self.tLim) {
            self.tLim = (d['n0']-d['ly'])/d['ky'];
            changed = true;
        }
        if ((d['n0']-d['lActy'])/d['kActy'] > self.tLim) {
            self.tLim = (d['n0']-d['lActy'])/d['kActy'];
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

        ddots.enter().append("circle")
            .attr("class", "dot")
            .attr("r", (self.x(d['n0']*d['sigmaM'])-self.x(-d['n0']*d['sigmaM']))/2)
            .style("fill", self.col)
            .attr("cx", function(d) { return self.x(d['mx']); })
            .attr("cy", function(d) { return self.y(d['my']); });

        ddots.exit().remove();

        self.ctr.attr("cx", self.x(-d['lx']/d['kx']))
            .attr("cy", self.y(-d['ly']/d['ky']));

        self.ctrAct.attr("cx", self.x(-d['lActx']/d['kActx']))
            .attr("cy", self.y(-d['lActy']/d['kActy']));

        self.crc.attr("cx", self.x(-d['lx']/d['kx']))
            .attr("cy", self.y(-d['ly']/d['ky']))
            .attr("rx", (self.x((d['n0']-d['lx'])/d['kx']) - self.x((-d['n0']-d['lx'])/d['kx']))/2)
            .attr("ry", (self.y((-d['n0']-d['ly'])/d['ky']) - self.y((d['n0']-d['ly'])/d['ky']))/2);

        self.crcAct.attr("cx", self.x(-d['lActx']/d['kActx']))
            .attr("cy", self.y(-d['lActy']/d['kActy']))
            .attr("rx", (self.x((d['n0']-d['lActx'])/d['kActx']) - self.x((-d['n0']-d['lActx'])/d['kActx']))/2)
            .attr("ry", (self.y((-d['n0']-d['lActy'])/d['kActy']) - self.y((d['n0']-d['lActy'])/d['kActy']))/2);

        self.vec
            .attr("x1", self.x(-d['lx']/d['kx']))
            .attr("y1", self.y(-d['ly']/d['ky']))
            .attr("x2", self.x(d['mx']))
            .attr("y2", self.y(d['my']))
    }
}

// Draw K-L Plots
// Parameters are e.g. ax='K1', ay='L3'
// 1. Draw a dot at K1, L3
// 3. Draw error ellipse for K1, L3
// 4. Draw lines of constant N0 for K1,L1 etc.
function KLPlot(ax, ay, el) {
    var self = this;
    this.el = el;
    this.ax = ax;
    this.ay = ay;
    this.lLim=0;
    this.rLim=0;
    this.tLim=0;
    this.bLim=0;
    this.xBuf = 0;
    this.yBuf = 0;

    this.col = "Black";
    switch (this.ax[0] + this.ay[0]) {
        case 'KK':
            this.col = "Blue";
            this.lLim = 0;
            this.bLim = 0;
            break;
        case 'LK':
            this.col = "Green";
            this.bLim = 0;
            break;
        case 'LL':
            this.col = "Red";
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
        var d = {
            'x': datum[self.ax], 'y': datum[self.ay],
            'Actx': datum[self.ax[0]+'Act'+self.ax[1]], 'Acty': datum[self.ay[0]+'Act'+self.ay[1]],
            'Pxx': datum['P'+self.ax+self.ax], 'Pxy': datum['P'+self.ax+self.ay], 'Pyy': datum['P'+self.ay+self.ay],
            'n0': datum['N0'], 'sigmaM': datum['sigmaM']
        };

        var changed = false;
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

        var errEllipseData = calcEllipse(d['Pxx'], d['Pyy'], d['Pxy']);

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
    var sK = Math.sqrt(pKK);
    var sL = Math.sqrt(pLL);
    if (pKL===0) {
        return {'a': sL, 'b': sK, 'alpha': 0}
    }
    var rho = pKL/(sK*sL);
    if (Math.abs(rho)>1) {
        console.log("invalid vcv: k,l element not a covariance");
        return {'a': 0, 'b': 0, 'alpha': 0}
    }

    var alpha = Math.atan2(2*rho*sK*sL, pKK-pLL)/2;
    var num = 2*pKK*pLL*(1-rho*rho);
    var den = 2*pKL/Math.sin(2*alpha);
    var a = Math.sqrt(num/(pKK+den+pLL));
    var b = Math.sqrt(num/(pKK-den+pLL));
    return {'a': a, 'b': b, 'alpha': alpha*180/Math.PI}
}

// Draw dTheta Plot
function DThetaPlot(el) {
    var self = this;
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
        var dTheta = Math.atan2(
            self.data["ky"]/self.data["kActy"]*(self.data["n0"]*Math.sin(theta*Math.PI/180)-self.data["lActy"])+self.data["ly"],
            self.data["kx"]/self.data["kActx"]*(self.data["n0"]*Math.cos(theta*Math.PI/180)-self.data["lActx"])+self.data["lx"]
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
            var rr = self.thetas.reduce(function (oMax, x) {
                var y = self.dTheta(x);
                if (y < -oMax) {
                    oMax = -y;
                }
                if (y > oMax) {
                    oMax = y;
                }
                return oMax;
            }, 0);

            var changed = false;
            if (rr<self.tbLim/10) {
                if (rr > 0.1) {
                    self.tbLim = 10 * rr;
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

function MagInputArea(el) {

}
