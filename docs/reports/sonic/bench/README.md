# Sonic vs jsoniter vs encoding/json — benchmark suite

A self-contained Go module that benchmarks JSON marshal/unmarshal across:

- `encoding/json` (Go standard library — baseline)
- `github.com/json-iterator/go` with three configs:
  - `ConfigCompatibleWithStandardLibrary` (matches prebid-server's `UnmarshalValid` / `Marshal`)
  - prebid-server's `validation-off` config (matches `Unmarshal`)
  - prebid-server's `merge-clone` config (matches `MergeClone`, with the merge extension registered)
- `github.com/bytedance/sonic` with three configs:
  - `ConfigStd` (drop-in replacement for stdlib)
  - `ConfigDefault`
  - `ConfigFastest`

Each combination is exercised over fixture payloads of varying size, taken
from this repo's actual sample-requests and adapter test data:

- `br_xs_dooh.json` — DOOH single imp (~190 B)
- `br_s_simple.json` — simple banner (~210 B)
- `br_m_all-ext.json` — every Prebid extension (~920 B)
- `br_l_2.6-downconvert.json` — 2.6 → 2.5 downconvert sample (~700 B)
- `br_xl_30imps.json` — synthesized 30-imp request (~7 KB)
- `bresp_s_typical.json` — typical small adapter `BidResponse` (~290 B)
- `bresp_xl_silverpush.json` — large video `BidResponse` (~48 KB)

The fixtures are kept in `../fixtures/` and committed (compact, normalized).

## Running

```sh
cd docs/reports/sonic/bench
go test -bench=. -benchmem -benchtime=2s -run='^$' ./... | tee results.txt
go run ./cmd/render < results.txt > ../performance-data.json
```

The `cmd/render` binary parses `go test -bench` output into a JSON file
that the HTML report (`../performance.html`) loads at view time.

## Why a separate go.mod?

Sonic isn't (yet) a runtime dependency of prebid-server; we don't want the
benchmark suite to drag it into the main module's go.sum. The bench
module pins its own deps and pulls openrtb/v20 directly.
