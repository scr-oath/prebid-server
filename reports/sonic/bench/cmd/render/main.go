// render parses `go test -bench` output (from stdin or a file argument)
// into a structured JSON document the HTML report can load and chart.
//
// Usage:
//
//	go run ./cmd/render < results.txt > ../performance-data.json
//	go run ./cmd/render results.txt > ../performance-data.json
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// One sample is one (benchmark, fixture, codec) row from `go test -bench`.
type sample struct {
	Benchmark   string  `json:"benchmark"`           // "Marshal" / "Unmarshal" / "RoundTrip"
	Fixture     string  `json:"fixture"`             // "br_xs_dooh.json"
	Codec       string  `json:"codec"`               // "sonic/default"
	N           int64   `json:"n"`                   // iterations
	NsPerOp     float64 `json:"ns_per_op"`           // nanoseconds per op
	MBPerSec    float64 `json:"mb_per_sec,omitempty"` // when -bench reports MB/s
	BytesPerOp  int64   `json:"bytes_per_op"`        // -benchmem
	AllocsPerOp int64   `json:"allocs_per_op"`       // -benchmem
}

// Aggregated summary that groups duplicate runs (count > 1) and reports
// median + spread. Median is more stable than mean when one run is hot
// and one is cold.
type summary struct {
	Benchmark    string  `json:"benchmark"`
	Fixture      string  `json:"fixture"`
	Codec        string  `json:"codec"`
	Runs         int     `json:"runs"`
	NsPerOpMin   float64 `json:"ns_per_op_min"`
	NsPerOpMed   float64 `json:"ns_per_op_med"`
	NsPerOpMax   float64 `json:"ns_per_op_max"`
	MBPerSecMed  float64 `json:"mb_per_sec_med,omitempty"`
	BytesPerOp   int64   `json:"bytes_per_op_med"`
	AllocsPerOp  int64   `json:"allocs_per_op_med"`
}

type document struct {
	GeneratedAt string    `json:"generated_at"`
	GoVersion   string    `json:"go_version"`
	GoOS        string    `json:"goos"`
	GoArch      string    `json:"goarch"`
	CPU         string    `json:"cpu"`
	Pkg         string    `json:"pkg"`
	Samples     []sample  `json:"samples"`
	Summaries   []summary `json:"summaries"`
}

// Sample line examples we need to parse:
//
//	BenchmarkUnmarshal/br_xs_dooh.json/encoding/json-14   111853   2022 ns/op   92.99 MB/s   1664 B/op   19 allocs/op
//	BenchmarkMarshal/br_m_all-ext.json/sonic/default-14   ...
//
// The benchmark *name* itself contains slashes for our sub-benchmarks,
// AND codec names like "encoding/json" or "sonic/default" also contain
// slashes — so we need to be careful when splitting.
var benchLineRe = regexp.MustCompile(
	`^Benchmark(\w+)/(\S+\.json)/(.+?)-\d+\s+(\d+)\s+([\d.]+)\s+ns/op` +
		`(?:\s+([\d.]+)\s+MB/s)?` +
		`(?:\s+(\d+)\s+B/op)?` +
		`(?:\s+(\d+)\s+allocs/op)?`,
)

var (
	goosRe = regexp.MustCompile(`^goos:\s*(\S+)`)
	archRe = regexp.MustCompile(`^goarch:\s*(\S+)`)
	cpuRe  = regexp.MustCompile(`^cpu:\s*(.*)`)
	pkgRe  = regexp.MustCompile(`^pkg:\s*(\S+)`)
)

func main() {
	var r io.Reader = os.Stdin
	if len(os.Args) > 1 {
		f, err := os.Open(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "open %s: %v\n", os.Args[1], err)
			os.Exit(1)
		}
		defer f.Close()
		r = f
	}

	doc := document{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		GoVersion:   readGoVersion(),
	}

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "goos:"):
			if m := goosRe.FindStringSubmatch(line); m != nil {
				doc.GoOS = m[1]
			}
		case strings.HasPrefix(line, "goarch:"):
			if m := archRe.FindStringSubmatch(line); m != nil {
				doc.GoArch = m[1]
			}
		case strings.HasPrefix(line, "cpu:"):
			if m := cpuRe.FindStringSubmatch(line); m != nil {
				doc.CPU = strings.TrimSpace(m[1])
			}
		case strings.HasPrefix(line, "pkg:"):
			if m := pkgRe.FindStringSubmatch(line); m != nil {
				doc.Pkg = m[1]
			}
		case strings.HasPrefix(line, "Benchmark"):
			if s, ok := parseBenchLine(line); ok {
				doc.Samples = append(doc.Samples, s)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "scanner: %v\n", err)
		os.Exit(1)
	}

	doc.Summaries = summarize(doc.Samples)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(1)
	}
}

func parseBenchLine(line string) (sample, bool) {
	m := benchLineRe.FindStringSubmatch(line)
	if m == nil {
		return sample{}, false
	}
	n, _ := strconv.ParseInt(m[4], 10, 64)
	ns, _ := strconv.ParseFloat(m[5], 64)
	mb, _ := strconv.ParseFloat(safe(m, 6), 64)
	bpo, _ := strconv.ParseInt(safe(m, 7), 10, 64)
	apo, _ := strconv.ParseInt(safe(m, 8), 10, 64)
	return sample{
		Benchmark:   m[1],
		Fixture:     m[2],
		Codec:       m[3],
		N:           n,
		NsPerOp:     ns,
		MBPerSec:    mb,
		BytesPerOp:  bpo,
		AllocsPerOp: apo,
	}, true
}

func safe(m []string, i int) string {
	if i < len(m) {
		return m[i]
	}
	return ""
}

func summarize(samples []sample) []summary {
	type key struct{ b, f, c string }
	groups := make(map[key][]sample)
	for _, s := range samples {
		k := key{s.Benchmark, s.Fixture, s.Codec}
		groups[k] = append(groups[k], s)
	}
	out := make([]summary, 0, len(groups))
	for k, g := range groups {
		sort.Slice(g, func(i, j int) bool { return g[i].NsPerOp < g[j].NsPerOp })
		mid := g[len(g)/2]
		out = append(out, summary{
			Benchmark:   k.b,
			Fixture:     k.f,
			Codec:       k.c,
			Runs:        len(g),
			NsPerOpMin:  g[0].NsPerOp,
			NsPerOpMed:  mid.NsPerOp,
			NsPerOpMax:  g[len(g)-1].NsPerOp,
			MBPerSecMed: mid.MBPerSec,
			BytesPerOp:  mid.BytesPerOp,
			AllocsPerOp: mid.AllocsPerOp,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Benchmark != out[j].Benchmark {
			return out[i].Benchmark < out[j].Benchmark
		}
		if out[i].Fixture != out[j].Fixture {
			return out[i].Fixture < out[j].Fixture
		}
		return out[i].Codec < out[j].Codec
	})
	return out
}

func readGoVersion() string {
	// runtime.Version() reflects the Go that built this tool, not the
	// Go that ran the bench. The bench output line "goos: ..." doesn't
	// include version; if not present, that's fine.
	return ""
}
