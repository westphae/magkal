var width = 400, height=400,
    margin = {top: 20, right: 2, bottom: 20, left: 40};

// Draw Magnetometer cross-sections
// 1. Draw a dot for mx,my
// 2. Draw a small circle for lx,ly, both actual and predicted
// 3. Draw an ellipse for n^2==n0^2, both actual and predicted
// 4. Draw some error ellipses for various theta for 2-sigma in m
function updateMagXS(ax, ay, el) {
    var col, mx, my, lx, ly, kx, ky, n0, lLim=-1, rLim=1, tLim=1, bLim=-1, changed,
        xbuf, ybuf;

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

    return function(data) {
        mx = data['M'+ax];
        my = data['M'+ay];
        lx = data["L"+ax];
        ly = data["L"+ay];
        kx = data["K"+ax];
        ky = data["K"+ay];
        kActx = data["KAct"+ax];
        kActy = data["KAct"+ay];
        lActx = data["LAct"+ax];
        lActy = data["LAct"+ay];
        n0 = data["N0"];

        changed = false;
        if (mx < lLim) {
            lLim = mx;
            changed = true;
        }
        if ((-n0-lx)/kx < lLim) {
            lLim = (-n0-lx)/kx;
            changed = true;
        }
        if ((-n0-lActx)/kActx < lLim) {
            lLim = (-n0-lActx)/kActx;
            changed = true;
        }
        if (mx > rLim) {
            rLim = mx;
            changed = true;
        }
        if ((n0-lx)/kx > rLim) {
            rLim = (n0-lx)/kx;
            changed = true;
        }
        if ((n0-lActx)/kActx > rLim) {
            rLim = (n0-lActx)/kActx;
            changed = true;
        }
        if (my < bLim) {
            bLim = my;
            changed = true;
        }
        if ((-n0-ly)/ky < bLim) {
            bLim = (-n0-ly)/ky;
            changed = true;
        }
        if ((-n0-lActy)/kActy < bLim) {
            bLim = (-n0-lActy)/kActy;
            changed = true;
        }
        if (my > tLim) {
            tLim = my;
            changed = true;
        }
        if ((n0-ly)/ky > tLim) {
            tLim = (n0-ly)/ky;
            changed = true;
        }
        if ((n0-lActy)/kActy > tLim) {
            tLim = (n0-lActy)/kActy;
            changed = true;
        }
        if (changed) {
            xbuf = Math.max(0, (tLim-bLim)-(rLim-lLim))/2;
            ybuf = Math.max(0, (rLim-lLim)-(tLim-bLim))/2;
            x.domain([lLim-xbuf, rLim+xbuf]);
            xAxis.scale(x);
            y.domain([bLim-ybuf, tLim+ybuf]);
            yAxis.scale(y);
            xAxisLine.attr("transform", "translate(0," + y(0) + ")")
                .call(xAxis);
            yAxisLine.attr("transform", "translate(" + x(0) + ",0)")
                .call(yAxis);
        }

        dots.append("circle")
            .attr("class", "dot")
            .attr("r", 1)
            .attr("cx", x(mx))
            .attr("cy", y(my))
            .style("fill", col);

        ctr.attr("cx", x(-lx/kx))
            .attr("cy", y(-ly/ky));

        ctrAct.attr("cx", x(-lActx/kActx))
            .attr("cy", y(-lActy/kActy));

        crc.attr("cx", x(-lx/kx))
            .attr("cy", y(-ly/ky))
            .attr("rx", (x((n0-lx)/kx) - x((-n0-lx)/kx))/2)
            .attr("ry", (y((-n0-ly)/ky) - y((n0-ly)/ky))/2);

        crcAct.attr("cx", x(-lActx/kActx))
            .attr("cy", y(-lActy/kActy))
            .attr("rx", (x((n0-lActx)/kActx) - x((-n0-lActx)/kActx))/2)
            .attr("ry", (y((-n0-lActy)/kActy) - y((n0-lActy)/kActy))/2);

        vec
            .attr("x1", x(lx))
            .attr("y1", y(ly))
            .attr("x2", x(mx))
            .attr("y2", y(my))
    }

}

