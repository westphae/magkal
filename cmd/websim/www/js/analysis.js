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

    var ctrAct = svg.append("circle")
        .attr("class", "center actual")
        .attr("r", 2)
        .attr("cx", x(0))
        .attr("cy", y(0));

    var ctr = svg.append("circle")
        .attr("class", "center estimated")
        .attr("r", 2)
        .attr("cx", x(0))
        .attr("cy", y(0));

    var crcAct = svg.append("ellipse")
        .attr("class", "ellipse actual")
        .attr("cx", x(0))
        .attr("cy", y(0))
        .attr("rx", 0)
        .attr("ry", 0);

    var crc = svg.append("ellipse")
        .attr("class", "ellipse estimated")
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
            'n0': datum['N0'], 'nSigma': datum['NSigma'], 'epsilon': datum['Epsilon']
        };
        data.push(d);

        var ddots = dots.selectAll('circle').data(data);

        var changed = false;
        if (d['mx']-d['n0']*d['nSigma'] < llLim) {
            llLim = d['mx']-d['n0']*d['nSigma'];
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
        if (d['mx']+d['n0']*d['nSigma'] > rrLim) {
            rrLim = d['mx']+d['n0']*d['nSigma'];
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
        if (d['my']-d['n0']*d['nSigma'] < bbLim) {
            bbLim = d['my']-d['n0']*d['nSigma'];
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
        if (d['my']+d['n0']*d['nSigma'] > ttLim) {
            ttLim = d['my']+d['n0']*d['nSigma'];
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
            .attr("r", x(d['n0']*d['nSigma'])-x(-d['n0']*d['nSigma']))
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
