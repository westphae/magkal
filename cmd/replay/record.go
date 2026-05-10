package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Record is one filter step's observable state. All slices have length n
// except p_diag which has length 2n (interleaved [pk0, pl0, pk1, pl1, ...]).
type Record struct {
	Step   int
	Label  string
	Action string
	Theta  float64
	Phi    float64
	M      []float64
	NHat   []float64
	Y      float64
	S      float64
	K      []float64
	L      []float64
	PDiag  []float64
	KErr   []float64
	LErr   []float64
	TrP    float64
}

// Writer abstracts emitting a stream of Records. Both implementations are
// allowed to be no-ops (used when --no-records is set or --every drops the
// row).
type Writer interface {
	WriteHeader(n int, label string) error // label="" means initial header
	WriteRecord(r *Record) error
	Close() error
}

// --- Aligned text-table writer ----------------------------------------------

type tableWriter struct {
	w           io.Writer
	n           int
	lastLabel   string
	headerEvery int // re-print header every N rows; 0 disables
	rowsSince   int
}

func newTableWriter(w io.Writer, n int) *tableWriter {
	return &tableWriter{w: w, n: n, headerEvery: 50}
}

func (t *tableWriter) WriteHeader(n int, label string) error {
	cols := []string{"step", "segment", "theta", "phi"}
	for i := 0; i < n; i++ {
		cols = append(cols, fmt.Sprintf("k%d", i))
	}
	for i := 0; i < n; i++ {
		cols = append(cols, fmt.Sprintf("l%d", i))
	}
	cols = append(cols, "y", "tr(P)")
	// Widths: tuned to keep numerical columns readable for n=3.
	widths := tableWidths(n)
	var b strings.Builder
	for i, c := range cols {
		fmt.Fprintf(&b, "%*s", widths[i], c)
		if i < len(cols)-1 {
			b.WriteByte(' ')
		}
	}
	b.WriteByte('\n')
	_, err := io.WriteString(t.w, b.String())
	return err
}

func (t *tableWriter) WriteRecord(r *Record) error {
	if r.Label != t.lastLabel {
		// New segment: print a header so a downstream `tail` has context.
		if err := t.WriteHeader(t.n, r.Label); err != nil {
			return err
		}
		t.lastLabel = r.Label
		t.rowsSince = 0
	} else if t.headerEvery > 0 && t.rowsSince >= t.headerEvery {
		if err := t.WriteHeader(t.n, r.Label); err != nil {
			return err
		}
		t.rowsSince = 0
	}
	t.rowsSince++

	widths := tableWidths(t.n)
	cells := []string{
		strconv.Itoa(r.Step),
		r.Label,
		fmtF(r.Theta, 1),
		fmtF(r.Phi, 1),
	}
	for i := 0; i < t.n; i++ {
		cells = append(cells, fmtF(r.K[i], 4))
	}
	for i := 0; i < t.n; i++ {
		cells = append(cells, fmtF(r.L[i], 2))
	}
	cells = append(cells, fmtE(r.Y), fmtE(r.TrP))

	var b strings.Builder
	for i, c := range cells {
		fmt.Fprintf(&b, "%*s", widths[i], c)
		if i < len(cells)-1 {
			b.WriteByte(' ')
		}
	}
	b.WriteByte('\n')
	_, err := io.WriteString(t.w, b.String())
	return err
}

func (t *tableWriter) Close() error { return nil }

// tableWidths returns the column widths for a table with n axes.
// 4 fixed + n k-cols + n l-cols + 3 trailing.
func tableWidths(n int) []int {
	widths := []int{6, 14, 7, 7}
	for i := 0; i < n; i++ {
		widths = append(widths, 8)
	}
	for i := 0; i < n; i++ {
		widths = append(widths, 9)
	}
	widths = append(widths, 11, 11)
	return widths
}

// fmtF formats a float with `prec` digits past the decimal, %f style.
func fmtF(v float64, prec int) string {
	return strconv.FormatFloat(v, 'f', prec, 64)
}

// fmtE formats a float in scientific notation with one digit past the
// decimal, e.g. "-2.3e+03". Handles NaN/Inf by their stdlib representation.
func fmtE(v float64) string {
	return strconv.FormatFloat(v, 'e', 1, 64)
}

// --- CSV writer -------------------------------------------------------------

type csvWriter struct {
	w        *csv.Writer
	n        int
	wroteHdr bool
}

func newCSVWriter(w io.Writer, n int) *csvWriter {
	return &csvWriter{w: csv.NewWriter(w), n: n}
}

func (c *csvWriter) WriteHeader(n int, label string) error {
	if c.wroteHdr {
		return nil
	}
	c.wroteHdr = true
	cols := []string{"step", "label", "action", "theta", "phi"}
	for i := 0; i < n; i++ {
		cols = append(cols, fmt.Sprintf("m%d", i))
	}
	for i := 0; i < n; i++ {
		cols = append(cols, fmt.Sprintf("nhat%d", i))
	}
	cols = append(cols, "y", "s")
	for i := 0; i < n; i++ {
		cols = append(cols, fmt.Sprintf("k%d", i))
	}
	for i := 0; i < n; i++ {
		cols = append(cols, fmt.Sprintf("l%d", i))
	}
	for i := 0; i < n; i++ {
		cols = append(cols, fmt.Sprintf("pk%d", i), fmt.Sprintf("pl%d", i))
	}
	for i := 0; i < n; i++ {
		cols = append(cols, fmt.Sprintf("kerr%d", i))
	}
	for i := 0; i < n; i++ {
		cols = append(cols, fmt.Sprintf("lerr%d", i))
	}
	cols = append(cols, "trP")
	return c.w.Write(cols)
}

func (c *csvWriter) WriteRecord(r *Record) error {
	row := []string{
		strconv.Itoa(r.Step),
		r.Label,
		r.Action,
		csvF(r.Theta),
		csvF(r.Phi),
	}
	for _, v := range r.M {
		row = append(row, csvF(v))
	}
	for _, v := range r.NHat {
		row = append(row, csvF(v))
	}
	row = append(row, csvF(r.Y), csvF(r.S))
	for _, v := range r.K {
		row = append(row, csvF(v))
	}
	for _, v := range r.L {
		row = append(row, csvF(v))
	}
	for _, v := range r.PDiag {
		row = append(row, csvF(v))
	}
	for _, v := range r.KErr {
		row = append(row, csvF(v))
	}
	for _, v := range r.LErr {
		row = append(row, csvF(v))
	}
	row = append(row, csvF(r.TrP))
	return c.w.Write(row)
}

func (c *csvWriter) Close() error {
	c.w.Flush()
	return c.w.Error()
}

func csvF(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// --- no-op writer for --no-records ------------------------------------------

type nullWriter struct{}

func (nullWriter) WriteHeader(int, string) error { return nil }
func (nullWriter) WriteRecord(*Record) error     { return nil }
func (nullWriter) Close() error                  { return nil }
