package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"

	"github.com/westphae/magkal/pkg/kalman"
)

var (
	flagCSV       = flag.Bool("csv", false, "emit CSV instead of an aligned text table")
	flagEvery     = flag.Int("every", 1, "emit every Nth record (>=1)")
	flagSummary   = flag.Bool("summary", false, "print a one-line per-segment summary to stderr")
	flagNoRecords = flag.Bool("no-records", false, "suppress per-step output (only summaries)")
	flagVerbose   = flag.Bool("verbose", false, "let pkg/kalman's debug log.Printf reach stderr")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: replay [flags] <script.yaml>\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	if *flagEvery < 1 {
		fmt.Fprintf(os.Stderr, "replay: --every must be >= 1\n")
		os.Exit(2)
	}

	if !*flagVerbose {
		// pkg/kalman has noisy debug log.Printf calls in its update path; mute by
		// default so 100k-step replays don't drown out the records.
		log.SetOutput(io.Discard)
	}

	script, err := loadScript(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "replay: %v\n", err)
		os.Exit(1)
	}
	if err := run(script); err != nil {
		fmt.Fprintf(os.Stderr, "replay: %v\n", err)
		os.Exit(1)
	}
}

func run(s *Script) error {
	n := s.Truth.N
	rng := rand.New(rand.NewSource(s.Seed))
	kf := kalman.NewKalmanFilter(n, s.Truth.N0, s.Filter.SigmaK0, s.Filter.SigmaK, s.Filter.SigmaM)
	if s.Filter.MaxSigmaK > 0 || s.Filter.MaxSigmaL > 0 {
		kf.SetConvergenceThresholds(s.Filter.MaxSigmaK, s.Filter.MaxSigmaL)
	}
	convergenceEnabled := s.Filter.MaxSigmaK > 0 && s.Filter.MaxSigmaL > 0

	var w Writer
	switch {
	case *flagNoRecords:
		w = nullWriter{}
	case *flagCSV:
		cw := newCSVWriter(os.Stdout, n)
		cw.convergenceEnabled = convergenceEnabled
		if err := cw.WriteHeader(n, ""); err != nil {
			return err
		}
		w = cw
	default:
		tw := newTableWriter(os.Stdout, n)
		tw.convergenceEnabled = convergenceEnabled
		// No initial header; the first record's segment-change branch prints one.
		w = tw
	}
	defer w.Close()

	step := 0
	lastEmittedLabel := ""
	var (
		segLabel       string
		segCount       int
		segLastRecord  *Record
	)

	flushSegment := func() {
		if !*flagSummary || segCount == 0 || segLastRecord == nil {
			return
		}
		r := segLastRecord
		fmt.Fprintf(os.Stderr, "[%s] %d steps | k=%v (err=%v) | l=%v (err=%v) | tr(P)=%s\n",
			segLabel, segCount,
			fmtSlice(r.K), fmtSlice(r.KErr),
			fmtSlice(r.L), fmtSlice(r.LErr),
			fmtE(r.TrP),
		)
	}

	for _, st := range s.Steps {
		gens := expand(st, s.Truth, rng)
		for _, g := range gens {
			if g.Label != segLabel {
				flushSegment()
				segLabel = g.Label
				segCount = 0
				segLastRecord = nil
			}

			m := synthMeasurement(n, g.Dir.Theta, g.Dir.Phi, s.Truth.K, s.Truth.L, s.Truth.N0, s.Truth.Noise, rng)

			// Drain any stale Done signal from a prior step before sending Z so the
			// post-step <-kf.Done can't pick up a stale signal.
			select {
			case <-kf.Done:
			default:
			}
			kf.U <- kalman.Matrix{m}
			z := s.Truth.N0 * s.Truth.N0
			kf.Z <- z
			<-kf.Done

			r := buildRecord(step, g, m, z, kf, &s.Truth)
			segLastRecord = r
			segCount++

			emit := step%*flagEvery == 0 || g.Label != lastEmittedLabel
			if emit {
				if err := w.WriteRecord(r); err != nil {
					return err
				}
				lastEmittedLabel = g.Label
			}
			step++
		}
	}
	flushSegment()
	return nil
}

func buildRecord(step int, g Generated, m []float64, z float64, kf *kalman.Filter, t *Truth) *Record {
	n := t.N
	kEst := kf.K()
	lEst := kf.L()
	p := kf.P()

	nHat := make([]float64, n)
	var sumNHat2 float64
	for i := 0; i < n; i++ {
		nHat[i] = kEst[i] * (m[i] - lEst[i])
		sumNHat2 += nHat[i] * nHat[i]
	}

	pDiag := make([]float64, 2*n)
	var trP float64
	for i := 0; i < 2*n; i++ {
		pDiag[i] = p[i][i]
		trP += p[i][i]
	}

	kErr := make([]float64, n)
	lErr := make([]float64, n)
	for i := 0; i < n; i++ {
		kErr[i] = kEst[i] - t.K[i]
		lErr[i] = lEst[i] - t.L[i]
	}

	return &Record{
		Step:   step,
		Label:  g.Label,
		Action: string(g.Kind),
		Theta:  g.Dir.Theta,
		Phi:    g.Dir.Phi,
		M:      m,
		NHat:   nHat,
		Y:      z - sumNHat2,
		// S not directly exposed by Filter; leave as 0 for now. Adding a getter
		// would be the right follow-up if we want it for diagnostics.
		S:         0,
		K:         kEst,
		L:         lEst,
		PDiag:     pDiag,
		KErr:      kErr,
		LErr:      lErr,
		TrP:       trP,
		Converged: kf.Converged(),
	}
}

func fmtSlice(xs []float64) string {
	out := "["
	for i, v := range xs {
		if i > 0 {
			out += ","
		}
		out += fmtF(v, 4)
	}
	return out + "]"
}
