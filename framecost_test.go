package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestFrameCost measures what a frame of the running timer costs, with the
// Tester (its frames are painted on the CPU, so the figures are a ceiling
// for the CPU side of a frame on a real GPU). Run it with
//
//	FRAMECOST=1 MYGO_FRAME_STATS=all go test -run FrameCost -v .
//
// MYGO_FRAME_STATS=all makes MyGo time the phases of every frame.
func TestFrameCost(t *testing.T) {
	if os.Getenv("FRAMECOST") == "" {
		t.Skip("set FRAMECOST=1 and MYGO_FRAME_STATS=all")
	}
	re := regexp.MustCompile(`took ([\d.]+) ms: build ([\d.]+)(?: \((\d+) passes?\))?, layout ([\d.]+), paint ([\d.]+), present ([\d.]+).*about (\d+) allocations, ([\d.]+) KB`)
	measure := func(name string, setup func(a *app, tt *ui.Tester), seconds int) {
		a, tt, _ := newTest(t)
		tt.SetScale(2)
		tt.Frame()
		setup(a, tt)
		var buf bytes.Buffer
		log.SetOutput(&buf)
		defer log.SetOutput(os.Stderr)
		start := time.Now()
		tt.Advance(time.Duration(seconds) * time.Second)
		wall := time.Since(start)
		if os.Getenv("FRAMERAW") != "" {
			os.WriteFile("/tmp/frames-"+strconv.Itoa(len(name))+".log", buf.Bytes(), 0o644)
		}
		var total, build, layout, paint, present, allocs, kb []float64
		for _, m := range re.FindAllStringSubmatch(buf.String(), -1) {
			f := func(i int) float64 { v, _ := strconv.ParseFloat(m[i], 64); return v }
			total, build, layout, paint = append(total, f(1)), append(build, f(2)), append(layout, f(4)), append(paint, f(5))
			present = append(present, f(6))
			allocs, kb = append(allocs, f(7)), append(kb, f(8))
		}
		mean := func(v []float64) float64 {
			s := 0.0
			for _, x := range v {
				s += x
			}
			if len(v) == 0 {
				return 0
			}
			return s / float64(len(v))
		}
		p95 := func(v []float64) float64 {
			if len(v) == 0 {
				return 0
			}
			c := append([]float64(nil), v...)
			sort.Float64s(c)
			return c[len(c)*95/100]
		}
		sum := func(v []float64) float64 { return mean(v) * float64(len(v)) }
		cpu := (sum(build) + sum(layout) + sum(paint)) / float64(seconds)
		fmt.Printf("FRAMECOST %-24s frames/s %5.1f | per frame: total %.2f p95 %.2f ms (build %.2f layout %.2f paint %.2f raster %.2f) allocs %.0f %.1f KB | per second: build+layout+paint %.1f ms, with raster %.1f ms\n",
			name, float64(len(total))/float64(seconds), mean(total), p95(total), mean(build), mean(layout), mean(paint), mean(present),
			mean(allocs), mean(kb), cpu, sum(total)/float64(seconds))
		_ = wall
	}
	measure("idle (nothing moves)", func(a *app, tt *ui.Tester) {}, 3)
	measure("start, first 2 s", func(a *app, tt *ui.Tester) {
		tt.Key(0, ui.KeySpace)
	}, 2)
	measure("running (steady), 20 s", func(a *app, tt *ui.Tester) {
		tt.Key(0, ui.KeySpace)
		tt.Advance(3 * time.Second)
	}, 20)
	measure("running, settings open", func(a *app, tt *ui.Tester) {
		tt.Key(0, ui.KeySpace)
		a.settingsOpen = true
		tt.Frame()
		tt.Advance(3 * time.Second)
	}, 10)
}
