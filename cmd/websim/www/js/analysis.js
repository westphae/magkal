var width = 400, height=400,
    margin = {top: 20, right: 2, bottom: 20, left: 40};

// Draw Magnetometer cross-sections
// 1. Draw a dot for mx,my
// 2. Draw a small circle for lx,ly, both actual and predicted
// 3. Draw an ellipse for n^2==n0^2, both actual and predicted
function makeMagXSPlot(ax, ay, el) {
    var llLim=1e9, rrLim=-1e9, ttLim=-1e9, bbLim=1e9,
        lLim, rLim, tLim, bLim,
        data=[], col, xBuf, yBuf;

    switch (ax+ay) {
        case 3:
            col = "Blue";
            break;
        case 4:
            col = "Green";
            break;
        case 5:
            col = "Red";
    }

    var x = d3.scaleLinear()
        .domain([llLim, rrLim])
        .range([0, width-margin.left-margin.right]);

    var y = d3.scaleLinear()
        .domain([bbLim, ttLim])
        .range([height-margin.top-margin.bottom, 0]);

    var xAxis = d3.axisBottom()
        .scale(x);

    var yAxis = d3.axisLeft()
        .scale(y);

    var svg = d3.select(el).append("svg")
        .attr("width", width)
        .attr("height", height)
        .append("g")
        .attr("transform", "translate("+margin.left+","+margin.top+")");

    var xAxisLine = svg.append("g")
        .attr("class", "x axis")
        .attr("transform", "translate(0," + y(0) + ")")
        .call(xAxis);

    xAxisLine.append("text")
        .attr("x", 6)
        .attr("dx", ".71em")
        .style("text-anchor", "end")
        .text("m"+ax);

    var yAxisLine = svg.append("g")
        .attr("class", "y axis")
        .attr("transform", "translate(" + x(0) + ",0)")
        .call(yAxis);

    yAxisLine.append("text")
        .attr("y", 6)
        .attr("dy", ".71em")
        .style("text-anchor", "end")
        .text("m"+ay);

    var dots = svg.append("g");

    var ctr = svg.append("circle")
        .attr("class", "center estimated")
        .attr("r", 2)
        .attr("cx", x(0))
        .attr("cy", y(0));

    var crc = svg.append("ellipse")
        .attr("class", "ellipse estimated")
        .attr("cx", x(0))
        .attr("cy", y(0))
        .attr("rx", 0)
        .attr("ry", 0);

    var ctrAct = svg.append("circle")
        .attr("class", "center actual")
        .attr("r", 2)
        .attr("cx", x(0))
        .attr("cy", y(0));

    var crcAct = svg.append("ellipse")
        .attr("class", "ellipse actual")
        .attr("cx", x(0))
        .attr("cy", y(0))
        .attr("rx", 0)
        .attr("ry", 0);

    var vec = svg.append("line")
        .attr("class", "pointer")
        .attr("x1", x(0))
        .attr("y1", y(0))
        .attr("x2", x(0))
        .attr("y2", y(0));

    return function(datum) {
        var d = {
            'mx': datum['M'+ax], 'my': datum['M'+ay],
            'kx': datum['K'+ax], 'ky': datum['K'+ay],
            'lx': datum['L'+ax], 'ly': datum['L'+ay],
            'kActx': datum['KAct'+ax], 'kActy': datum['KAct'+ay],
            'lActx': datum['LAct'+ax], 'lActy': datum['LAct'+ay],
            'n0': datum['N0'], 'sigmaM': datum['sigmaM']
        };
        data.push(d);

        var ddots = dots.selectAll('circle').data(data);

        var changed = false;
        if (d['mx']-d['n0']*d['sigmaM'] < llLim) {
            llLim = d['mx']-d['n0']*d['sigmaM'];
            changed = true;
        }
        lLim = llLim;
        if ((-d['n0']-d['lx'])/d['kx'] < lLim) {
            lLim = (-d['n0']-d['lx'])/d['kx'];
            changed = true;
        }
        if ((-d['n0']-d['lActx'])/d['kActx'] < lLim) {
            lLim = (-d['n0']-d['lActx'])/d['kActx'];
            changed = true;
        }
        if (d['mx']+d['n0']*d['sigmaM'] > rrLim) {
            rrLim = d['mx']+d['n0']*d['sigmaM'];
            changed = true;
        }
        rLim = rrLim;
        if ((d['n0']-d['lx'])/d['kx'] > rLim) {
            rLim = (d['n0']-d['lx'])/d['kx'];
            changed = true;
        }
        if ((d['n0']-d['lActx'])/d['kActx'] > rLim) {
            rLim = (d['n0']-d['lActx'])/d['kActx'];
            changed = true;
        }
        if (d['my']-d['n0']*d['sigmaM'] < bbLim) {
            bbLim = d['my']-d['n0']*d['sigmaM'];
            changed = true;
        }
        bLim = bbLim;
        if ((-d['n0']-d['ly'])/d['ky'] < bLim) {
            bLim = (-d['n0']-d['ly'])/d['ky'];
            changed = true;
        }
        if ((-d['n0']-d['lActy'])/d['kActy'] < bLim) {
            bLim = (-d['n0']-d['lActy'])/d['kActy'];
            changed = true;
        }
        if (d['my']+d['n0']*d['sigmaM'] > ttLim) {
            ttLim = d['my']+d['n0']*d['sigmaM'];
            changed = true;
        }
        tLim = ttLim;
        if ((d['n0']-d['ly'])/d['ky'] > tLim) {
            tLim = (d['n0']-d['ly'])/d['ky'];
            changed = true;
        }
        if ((d['n0']-d['lActy'])/d['kActy'] > tLim) {
            tLim = (d['n0']-d['lActy'])/d['kActy'];
            changed = true;
        }

        if (changed) {
            xBuf = Math.max(0, (tLim-bLim)-(rLim-lLim))/2;
            yBuf = Math.max(0, (rLim-lLim)-(tLim-bLim))/2;
            x.domain([lLim-xBuf, rLim+xBuf]);
            xAxis.scale(x);
            y.domain([bLim-yBuf, tLim+yBuf]);
            yAxis.scale(y);
            xAxisLine.attr("transform", "translate(0," + y(0) + ")")
                .call(xAxis);
            yAxisLine.attr("transform", "translate(" + x(0) + ",0)")
                .call(yAxis);
            ddots.attr("cx", function(d) { return x(d['mx']); })
                .attr("cy", function(d) { return y(d['my']); });
        }

        ddots.enter().append("circle")
            .attr("class", "dot")
            .attr("r", (x(d['n0']*d['sigmaM'])-x(-d['n0']*d['sigmaM']))/2)
            .style("fill", col)
            .attr("cx", function(d) { return x(d['mx']); })
            .attr("cy", function(d) { return y(d['my']); });

        ddots.exit().remove();

        ctr.attr("cx", x(-d['lx']/d['kx']))
            .attr("cy", y(-d['ly']/d['ky']));

        ctrAct.attr("cx", x(-d['lActx']/d['kActx']))
            .attr("cy", y(-d['lActy']/d['kActy']));

        crc.attr("cx", x(-d['lx']/d['kx']))
            .attr("cy", y(-d['ly']/d['ky']))
            .attr("rx", (x((d['n0']-d['lx'])/d['kx']) - x((-d['n0']-d['lx'])/d['kx']))/2)
            .attr("ry", (y((-d['n0']-d['ly'])/d['ky']) - y((d['n0']-d['ly'])/d['ky']))/2);

        crcAct.attr("cx", x(-d['lActx']/d['kActx']))
            .attr("cy", y(-d['lActy']/d['kActy']))
            .attr("rx", (x((d['n0']-d['lActx'])/d['kActx']) - x((-d['n0']-d['lActx'])/d['kActx']))/2)
            .attr("ry", (y((-d['n0']-d['lActy'])/d['kActy']) - y((d['n0']-d['lActy'])/d['kActy']))/2);

        vec
            .attr("x1", x(-d['lx']/d['kx']))
            .attr("y1", y(-d['ly']/d['ky']))
            .attr("x2", x(d['mx']))
            .attr("y2", y(d['my']))
    }
}

// Draw K-L Plots
// Parameters are e.g. ax='K1', ay='L3'
// 1. Draw a dot at K1, L3
// 3. Draw error ellipse for K1, L3
// 4. Draw lines of constant N0 for K1,L1 etc.
function makeKLPlot(ax, ay, el) {
    var lLim=0, rLim=0, tLim=0, bLim=0,
        col, xBuf, yBuf;

    switch (ax[0] + ay[0]) {
        case 'KK':
            col = "Blue";
            lLim = 0;
            bLim = 0;
            break;
        case 'LK':
            col = "Green";
            bLim = 0;
            break;
        case 'LL':
            col = "Red";
    }

    var x = d3.scaleLinear()
        .domain([lLim, rLim])
        .range([0, width-margin.left-margin.right]);

    var y = d3.scaleLinear()
        .domain([bLim, tLim])
        .range([height-margin.top-margin.bottom, 0]);

    var xAxis = d3.axisBottom()
        .scale(x);

    var yAxis = d3.axisLeft()
        .scale(y);

    var svg = d3.select(el).append("svg")
        .attr("width", width)
        .attr("height", height)
        .append("g")
        .attr("transform", "translate("+margin.left+","+margin.top+")");

    var xAxisLine = svg.append("g")
        .attr("class", "x axis")
        .attr("transform", "translate(0," + y(0) + ")")
        .call(xAxis);

    xAxisLine.append("text")
        .attr("x", 6)
        .attr("dx", ".71em")
        .style("text-anchor", "end")
        .text("m"+ax);

    var yAxisLine = svg.append("g")
        .attr("class", "y axis")
        .attr("transform", "translate(" + x(0) + ",0)")
        .call(yAxis);

    yAxisLine.append("text")
        .attr("y", 6)
        .attr("dy", ".71em")
        .style("text-anchor", "end")
        .text("m"+ay);

    var errEllipse = svg.append("g");

    errEllipse.append("circle")
        .attr("class", "center estimated")
        .attr("r", 2);

    var crc = errEllipse.append("ellipse")
        .attr("class", "ellipse estimated")
        .attr("rx", 0)
        .attr("ry", 0);

    var ctrAct = svg.append("circle")
        .attr("class", "center actual")
        .attr("r", 2)
        .attr("cx", x(0))
        .attr("cy", y(0));

    return function(datum) {
        var d = {
            'x': datum[ax], 'y': datum[ay],
            'Actx': datum[ax[0]+'Act'+ax[1]], 'Acty': datum[ay[0]+'Act'+ay[1]],
            'Pxx': datum['P'+ax+ax], 'Pxy': datum['P'+ax+ay], 'Pyy': datum['P'+ay+ay],
            'n0': datum['N0'], 'sigmaM': datum['sigmaM']
        };

        var changed = false;
        if (d['x'] < lLim) {
            lLim = d['x'];
            changed = true;
        }
        if (d['x'] > rLim) {
            rLim = d['x'];
            changed = true;
        }
        if (d['y'] < bLim) {
            bLim = d['y'];
            changed = true;
        }
        if (d['y'] > tLim) {
            tLim = d['y'];
            changed = true;
        }

        if (changed) {
            xBuf = Math.max(0, (tLim-bLim)-(rLim-lLim))/2;
            yBuf = Math.max(0, (rLim-lLim)-(tLim-bLim))/2;
            x.domain([lLim-xBuf, rLim+xBuf]);
            xAxis.scale(x);
            y.domain([bLim-yBuf, tLim+yBuf]);
            yAxis.scale(y);
            xAxisLine.attr("transform", "translate(0," + y(0) + ")")
                .call(xAxis);
            yAxisLine.attr("transform", "translate(" + x(0) + ",0)")
                .call(yAxis);
        }

        var errEllipseData = calcEllipse(d['Pxx'], d['Pyy'], d['Pxy']);

        errEllipse.attr("transform", "translate(" + x(d['x']) + "," + y(d['y']) + ")");

        crc.attr("rx", (x(errEllipseData['a'])-x(-errEllipseData['a']))/2)
            .attr("ry", (x(errEllipseData['b'])-x(-errEllipseData['b']))/2)
            .attr("transform", "rotate("+errEllipseData['alpha']+")");

        ctrAct.attr("cx", x(d['Actx']))
            .attr("cy", y(d['Acty']));
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
function makeDThetaPlot(el) {
    var data={};

    var x = d3.scaleLinear()
        .domain([0, 360])
        .range([0, width-margin.left-margin.right]);

    var y = d3.scaleLinear()
        .domain([0, 0])
        .range([height-margin.top-margin.bottom, 0]);

    var xAxis = d3.axisBottom()
        .scale(x);

    var yAxis = d3.axisLeft()
        .scale(y);

    var svg = d3.select(el).append("svg")
        .attr("width", width)
        .attr("height", height)
        .append("g")
        .attr("transform", "translate("+margin.left+","+margin.top+")");

    var xAxisLine = svg.append("g")
        .attr("class", "x axis")
        .attr("transform", "translate(0," + y(0) + ")")
        .call(xAxis);

    xAxisLine.append("text")
        .attr("x", 6)
        .attr("dx", ".71em")
        .style("text-anchor", "end")
        .text("Actual Heading");

    var yAxisLine = svg.append("g")
        .attr("class", "y axis")
        .attr("transform", "translate(" + x(0) + ",0)")
        .call(yAxis);

    yAxisLine.append("text")
        .attr("y", 6)
        .attr("dy", ".71em")
        .style("text-anchor", "end")
        .text("Heading Error");

    var thetas = d3.range(0, 360);

    function dTheta(theta) {
        var dTheta = Math.atan2(
            data["ky"]/data["kActy"]*(data["n0"]*Math.sin(theta*Math.PI/180)-data["lActy"])+data["ly"],
            data["kx"]/data["kActx"]*(data["n0"]*Math.cos(theta*Math.PI/180)-data["lActx"])+data["lx"]
        )*180/Math.PI-theta;
        if (dTheta<-180) {
            dTheta += 360;
        }
        if (dTheta>180) {
            dTheta -= 360;
        }
        return dTheta;
    }

    var xLine = d3.line()
        .x(function (theta) { return x(theta); })
        .y(function(theta) { return y(dTheta(theta)); });

    var xPath = svg.append("g")
        .append("path")
        .datum(thetas)
        .attr("class", "line")
        .attr("d", xLine);

    return function(datum) {
        data = {
            'kx': datum['K1'], 'ky': datum['K2'],
            'lx': datum['L1'], 'ly': datum['L2'],
            'kActx': datum['KAct1'], 'kActy': datum['KAct2'],
            'lActx': datum['LAct1'], 'lActy': datum['LAct2'],
            'n0': datum['N0']
        };

        var rr = thetas.reduce(function(oMax, x) {
            var y = dTheta(x);
            if (y<-oMax) {
                oMax = -y;
            }
            if (y>oMax) {
                oMax = y;
            }
            return Math.exp(Math.ceil(Math.log(oMax)));
        }, 0);

        y.domain([-rr, rr]);
        yAxis.scale(y);
        xAxisLine.attr("transform", "translate(0," + y(0) + ")");
        yAxisLine.call(yAxis);

        xPath.attr("d", xLine);
    }
}
