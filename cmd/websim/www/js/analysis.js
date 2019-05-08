var width = 400, height=400,
    margin = {top: 20, right: 2, bottom: 20, left: 40};

// Draw Magnetometer cross-sections
// 1. Draw a dot for mx,my
// 2. Draw a small circle for lx,ly, both actual and predicted
// 3. Draw an ellipse for n^2==n0^2, both actual and predicted
// 4. Draw some error ellipses for various theta for 2-sigma in m
function updateMagXS(ax, ay, el) {
    var lLim=-1, rLim=1, tLim=1, bLim=-1,
        data=[], changed, col, xBuf, yBuf;

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

    var dots = svg.append("g");

    var ctr = svg.append("circle")
        .attr("class", "center estimated")
        .attr("r", 2)
        .attr("cx", x(0))
        .attr("cy", y(0));

    var ctrAct = svg.append("circle")
        .attr("class", "center actual")
        .attr("r", 2)
        .attr("cx", x(0))
        .attr("cy", y(0));

    var crc = svg.append("ellipse")
        .attr("class", "ellipse estimated")
        .attr("cx", x(0))
        .attr("cy", y(0))
        .attr("rx", 0)
        .attr("ry", 0);

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

    function mx(d) {
        return d['M'+ax];
    }

    function my(d) {
        return d['M'+ay];
    }

    function kx(d) {
        return d['K'+ax];
    }

    function ky(d) {
        return d['K'+ay];
    }

    function lx(d) {
        return d['L'+ax];
    }

    function ly(d) {
        return d['L'+ay];
    }

    function kActx(d) {
        return d['KAct'+ax];
    }

    function kActy(d) {
        return d['KAct'+ay];
    }

    function lActx(d) {
        return d['LAct'+ax];
    }

    function lActy(d) {
        return d['LAct'+ay];
    }

    function n0(d) {
        return d['N0'];
    }

    return function(d) {
        data.push(Object.assign({}, d));
        changed = false;
        if (mx(d) < lLim) {
            lLim = mx(d);
            changed = true;
        }
        if ((-n0(d)-lx(d))/kx(d) < lLim) {
            lLim = (-n0(d)-lx(d))/kx(d);
            changed = true;
        }
        if ((-n0(d)-lActx(d))/kActx(d) < lLim) {
            lLim = (-n0(d)-lActx(d))/kActx(d);
            changed = true;
        }
        if (mx(d) > rLim) {
            rLim = mx(d);
            changed = true;
        }
        if ((n0(d)-lx(d))/kx(d) > rLim) {
            rLim = (n0(d)-lx(d))/kx(d);
            changed = true;
        }
        if ((n0(d)-lActx(d))/kActx(d) > rLim) {
            rLim = (n0(d)-lActx(d))/kActx(d);
            changed = true;
        }
        if (my(d) < bLim) {
            bLim = my(d);
            changed = true;
        }
        if ((-n0(d)-ly(d))/ky(d) < bLim) {
            bLim = (-n0(d)-ly(d))/ky(d);
            changed = true;
        }
        if ((-n0(d)-lActy(d))/kActy(d) < bLim) {
            bLim = (-n0(d)-lActy(d))/kActy(d);
            changed = true;
        }
        if (my(d) > tLim) {
            tLim = my(d);
            changed = true;
        }
        if ((n0(d)-ly(d))/ky(d) > tLim) {
            tLim = (n0(d)-ly(d))/ky(d);
            changed = true;
        }
        if ((n0(d)-lActy(d))/kActy(d) > tLim) {
            tLim = (n0(d)-lActy(d))/kActy(d);
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

        var dd = dots.selectAll('circle').data(data);

        dd
            .enter().append("circle")
            .attr("class", "dot")
            .attr("r", 1)
            .style("fill", col)
            .merge(dd)
            .attr("cx", function(d) { return x(mx(d)); })
            .attr("cy", function(d) { return y(my(d)); });

        dd.exit().remove();

        ctr.attr("cx", x(-lx(d)/kx(d)))
            .attr("cy", y(-ly(d)/ky(d)));

        ctrAct.attr("cx", x(-lActx(d)/kActx(d)))
            .attr("cy", y(-lActy(d)/kActy(d)));

        crc.attr("cx", x(-lx(d)/kx(d)))
            .attr("cy", y(-ly(d)/ky(d)))
            .attr("rx", (x((n0(d)-lx(d))/kx(d)) - x((-n0(d)-lx(d))/kx(d)))/2)
            .attr("ry", (y((-n0(d)-ly(d))/ky(d)) - y((n0(d)-ly(d))/ky(d)))/2);

        crcAct.attr("cx", x(-lActx(d)/kActx(d)))
            .attr("cy", y(-lActy(d)/kActy(d)))
            .attr("rx", (x((n0(d)-lActx(d))/kActx(d)) - x((-n0(d)-lActx(d))/kActx(d)))/2)
            .attr("ry", (y((-n0(d)-lActy(d))/kActy(d)) - y((n0(d)-lActy(d))/kActy(d)))/2);

        vec
            .attr("x1", x(lx(d)))
            .attr("y1", y(ly(d)))
            .attr("x2", x(mx(d)))
            .attr("y2", y(my(d)))
    }
}
