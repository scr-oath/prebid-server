# Evaluating `bytedance/sonic` as a JSON library for prebid-server

**Author:** Sheridan Rawlins (Yahoo)  
**Date:** 2026-04-28  
**Parent issue:** [prebid/prebid-server#3437 — Performance Optimization Discussion](https://github.com/prebid/prebid-server/issues/3437)  
**Sub-issue:** [prebid/prebid-server#4759 — Evaluate bytedance/sonic as JSON library replacement](https://github.com/prebid/prebid-server/issues/4759)  
**Performance data:** [`./performance.html`](./performance.html)

> **TL;DR.** Swapping `json-iterator/go` for `bytedance/sonic` is a real, measurable win on response-shaped payloads (~9× faster on Unmarshal, ~5× fewer allocations on a 48 KB `BidResponse`). It is a much more modest, sometimes-mixed result (0.86×–1.3×) on the request-sized payloads (200 B – 1 KB) that dominate today's adapter test corpus. The migration cost is small in surface area (one wrapper package, `util/jsonutil`) but **not zero in semantics** — sonic does not expose `jsoniter`'s `RegisterExtension` API, and the `MergeClone` codepath relies on it. This document lays out the trade-offs so the PBS-Go committee can make a clean go/no-go call.

---

## Why this matters

Issue #3437 names sonic as a *candidate* for "Switch to bytedance/sonic (faster but no ARM support)." Two things have changed since that issue was written:

- **Sonic now supports ARM64** on Linux/macOS/Windows (>=1.16, fully supported by v1.15.x). The "no ARM support" objection no longer applies.
- **Sonic now ships `ConfigStd`**, a drop-in replacement for `encoding/json`. The migration friction this objection implied is largely gone.

What hasn't changed:

- Sonic's wins are **payload-size-dependent**. The advantage is dramatic on large payloads, marginal on small ones.
- Sonic does **not** support custom encoder/decoder extensions the way jsoniter does. PBS-Go's `MergeClone` and `RawMessageExtension` patterns assume that API.

So the question is no longer *"can we use sonic?"* — it's *"is the speedup worth the surgery to `util/jsonutil`?"*

---

## What we measured

We benchmarked seven fixtures × seven codec configurations × three operations (`Marshal`, `Unmarshal`, `RoundTrip`), `count=2` for stability, on Apple M4 Pro (arm64). Every benchmark target is a real `openrtb2.BidRequest` or `openrtb2.BidResponse` from this repo's test corpus — no synthetic structs.

Codecs under test:

| Codec | Why it's there |
|---|---|
| `encoding/json` | Stdlib baseline. |
| `jsoniter/std-compat` | Matches `jsonutil.UnmarshalValid` and `jsonutil.Marshal` (today's response path). |
| `jsoniter/validation-off` | Matches `jsonutil.Unmarshal` (today's adapter / exchange path). |
| `jsoniter/merge-clone` | Matches `jsonutil.MergeClone` (stored-request merging) — **registers the merge extension**. |
| `sonic/std` | Drop-in for stdlib (HTML escape, sort keys). |
| `sonic/default` | Sonic's recommended default (no HTML escape, no key sort). |
| `sonic/fastest` | Skips UTF-8 validation; trades safety for speed. |

Fixtures:

| File | Size | Source |
|---|---|---|
| `br_xs_dooh.json` | 188 B | `endpoints/openrtb2/sample-requests/valid-whole/exemplary/dooh.json` |
| `br_s_simple.json` | 211 B | `…/exemplary/simple.json` |
| `br_l_2.6-downconvert.json` | 702 B | `…/exemplary/ortb-2.6-to-2.5-downconvert.json` |
| `br_m_all-ext.json` | 916 B | `…/exemplary/all-ext.json` |
| `br_xl_30imps.json` | 7 KB | synthesized — `all-ext.json` with 30 imps |
| `bresp_s_typical.json` | 293 B | `adapters/aceex/aceextest/exemplary/banner-web.json` |
| `bresp_xl_silverpush.json` | 48 KB | `adapters/silverpush/silverpushtest/exemplary/video.json` |

Full numbers, charts, and per-cell allocations are in [`./performance.html`](./performance.html).

### Headline: speedup vs `jsoniter/validation-off`

| Fixture | Op | sonic/default | sonic/fastest | sonic/std | encoding/json |
|---|---|---|---|---|---|
| 48 KB BidResponse | RoundTrip | **8.9×** | 8.2× | 3.2× | 0.89× |
| 48 KB BidResponse | Unmarshal | **9.8×** | **10.1×** | 4.4× | 0.83× |
| 48 KB BidResponse | Marshal | **3.7×** | 3.7× | 1.5× | 1.2× |
| 7 KB BidRequest (30 imps) | RoundTrip | **3.6×** | 2.1× | 1.5× | 0.53× |
| 916 B BidRequest | RoundTrip | 0.86× | 1.2× | 0.63× | 0.39× |
| 188 B BidRequest | RoundTrip | 1.3× | 1.2× | 0.94× | 0.62× |

Read this as: at production-realistic payload sizes (KB-scale and up), sonic's wins are real and big. At tiny request sizes (200 bytes – 1 KB), the libraries trade leadership case-by-case and the migration risk starts to dominate the speedup. **Note the 916 B row in particular** — sonic/default is *slower* than jsoniter/validation-off here. At `count=2` this could be noise, but it is also exactly the size band that matters for the dominant /openrtb2/auction request shape, so it would need to be re-measured at `count=10` before signing off.

> **The bottom line for the committee:** the question isn't "is sonic faster" (yes, demonstrably) — it's "what fraction of prebid-server's compute is spent on JSON, and at what payload sizes." That's a question only the committee, with production telemetry, can answer.

---

## What the committee needs to decide

These are the questions someone should answer before this lands as a PR.

### 1. What fraction of CPU does prebid-server spend in JSON today?

The benchmarks show *relative* speedups. The *absolute* impact depends on what fraction of a request's wall-time is JSON. If JSON is 30% of a request, sonic on the response path could trim ~25% off total cost; if it's 5%, the realistic upper bound is 4%.

> **Action for the committee:** profile a production node with `pprof`. If JSON shows up in the top 5 of `cpu` and `alloc_objects` profiles, sonic is high-leverage. If not, this is technical debt with marginal payoff.

### 2. What payload-size distribution does production traffic actually hit?

Sonic's win curve is non-linear — small payloads get marginal speedup, large ones get massive speedup. The repo's test corpus skews tiny (most fixtures are <1 KB). Production may differ — header bidding requests with 30+ impressions and full identity stitching can be 5–20 KB; bid-cache lookups can be larger; some adapter responses (silverpush video being one example we found in this repo) hit 50 KB+.

> **Action for the committee:** sample request body sizes and response body sizes from a production load balancer log for ~1 hour. The shape of that histogram is the deciding factor.

### 3. Are we OK losing the `RegisterExtension` API?

Today's `util/jsonutil/merge.go` registers four custom decoders against jsoniter:

- `extMergeDecoder` — merges two `json.RawMessage` payloads via `jsonpatch.MergePatch`. This is how stored requests/imps "patch" incoming requests.
- `ptrCloneDecoder` — clones pointers before unmarshalling onto them, so caller's struct isn't mutated.
- `sliceCloneDecoder` — clears the destination slice so unmarshalling produces a fresh slice rather than appending.
- `mapCloneDecoder` — clones map values during unmarshal.

Sonic does not expose an equivalent extension API. Replacing this means **either**:

- **(a)** Move the merge/clone logic out of the decode path and run it as a post-unmarshal step. Loses some efficiency (extra `json.RawMessage` walk) but is decoupled from the JSON library.
- **(b)** Keep `MergeClone` on jsoniter — only the hot adapter/response path moves to sonic. Easy migration, but now the codebase has two JSON libraries.
- **(c)** Replace `MergeClone` semantics with explicit struct-level merge methods. Most invasive.

> **Action for the committee:** decide on (a), (b), or (c) up front. (b) is the lowest-risk landing strategy.

### 4. Are we OK losing the `RawMessageExtension` compaction?

The `RawMessageExtension` registered globally in `main.go` calls `json.Compact()` on every `json.RawMessage` field at encode time. This guarantees response bodies are well-formed and don't carry stray whitespace. Sonic doesn't have an equivalent hook; equivalent behavior would require either:

- pre-compacting `RawMessage` values once at ingest (cheap, single-pass), or
- wrapping `json.RawMessage` in a custom type with `MarshalJSON` (which sonic *does* honor).

> **Action for the committee:** pick one. Both are easy; we just need a decision.

### 5. Are we OK with sonic's quieter error messages?

`util/jsonutil/jsonutil.go:tryExtractErrorMessage` exists specifically to massage jsoniter's error output. Sonic's errors have a different shape. Some downstream systems may parse error strings (we did not audit this exhaustively).

> **Action for the committee:** grep for callers that match on jsoniter error substrings; commit to a translation layer if any exist.

### 6. What's the rollout plan?

Sonic recommends `Pretouch()` to JIT-compile your types at process start. Without it, the first request that hits each new type pays the JIT cost (10s–100s of µs). This is a tractable startup hook but it must exist.

A safe rollout would be:

1. **Phase 1** — add sonic as an optional codec inside `util/jsonutil`, gated by an env var, default off.
2. **Phase 2** — flip the default in canary and watch p99 latency, error rates, GC pause time.
3. **Phase 3** — remove jsoniter once burnt-in for two release cycles.

> **Action for the committee:** ratify the phased rollout, or counter-propose.

### 7. What about the other items in #3437?

The parent issue lists five performance recommendations. Sonic is one. The others (klauspost/compress, zerolog, fasthttp) interact:

- compression and JSON together compound — switching only one is half the win.
- a faster logger reduces the noise floor; without it, the JSON p99 win may not show up clearly in metrics.

> **Action for the committee:** decide whether sonic ships solo or as part of a "PBS-Go performance pass" RFC. Both are reasonable; the committee should pick one.

---

## What we are *not* arguing for

- **Replacing `buger/jsonparser` or `tidwall/gjson`.** Those are byte-level path-lookup tools, not codecs. Sonic's `ast.Node` could replace them eventually but that's a separate evaluation.
- **Removing `encoding/json` entirely.** The stdlib remains the right baseline for low-volume admin endpoints, config parsing, etc.
- **Sonic on adapters.** Adapters are independently maintained; many have already imported jsoniter directly. The simplest migration moves only `util/jsonutil`; adapter-internal JSON is left alone.

---

## Risks

| Risk | Severity | Mitigation |
|---|---|---|
| `RegisterExtension` not available — `MergeClone` semantics break | **High** | Strategy (a)/(b)/(c) above. Pick before coding. |
| Sonic's JIT first-call latency triggers timeout alarms | Medium | Call `sonic.Pretouch(reflect.TypeOf(openrtb2.BidRequest{}))` (and friends) at startup. |
| Behavioral diff in error strings breaks downstream log parsing | Low | Audit log/error pattern matchers; translate if any. |
| Sonic versions ahead of arm64 fallbacks | Low | Pin a known-good sonic version; sonic falls back to encoding/json on unsupported toolchains. |
| Performance gains don't materialize in production due to small payload distribution | Medium | Profile production *before* committing. |
| Allocation savings hide a regression elsewhere | Low | Keep both codecs available behind a feature flag for one cycle. |

---

## Tracking

This document is the living version. The official issue lives at:

- **Sub-issue:** [prebid/prebid-server#4759](https://github.com/prebid/prebid-server/issues/4759)
- **Bench source:** [`bench/`](./bench/)
- **Raw results:** [`bench/results.txt`](./bench/results.txt) → [`performance-data.json`](./performance-data.json)
- **Public site:** https://scr-oath.github.io/prebid-server/reports/sonic/

To re-run the benchmarks on your own hardware:

```sh
cd docs/reports/sonic/bench
go test -bench=. -benchmem -benchtime=1s -run='^$' -count=2 ./... | tee results.txt
go run ./cmd/render < results.txt > ../performance-data.json
```

For the full performance breakdown — every fixture, every codec, allocation counts, throughput in MB/s — see [`./performance.html`](./performance.html).
