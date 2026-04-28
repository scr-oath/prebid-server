package bench

import (
	"strings"
	"testing"

	"github.com/prebid/openrtb/v20/openrtb2"
)

// loadAll grabs the fixtures once for the whole package; failure here is fatal
// and stops the entire benchmark run.
var allFixtures = func() []Fixture {
	f, err := LoadFixtures()
	if err != nil {
		panic(err)
	}
	return f
}()

// targetFor returns a fresh zero-value of the right shape for unmarshalling
// the given fixture. We test against the actual openrtb2 types used in
// prebid-server's hot path, not generic map[string]interface{}.
func targetFor(kind string) any {
	switch kind {
	case "BidRequest":
		return new(openrtb2.BidRequest)
	case "BidResponse":
		return new(openrtb2.BidResponse)
	default:
		panic("unknown fixture kind: " + kind)
	}
}

// BenchmarkUnmarshal exercises every (codec × fixture) pair on the
// "bytes -> Go struct" direction. ReportAllocs is on because allocation
// pressure is one of the core reasons this whole change is being
// considered (see issue #3437).
func BenchmarkUnmarshal(b *testing.B) {
	for _, fx := range allFixtures {
		fx := fx
		b.Run(fx.Name, func(b *testing.B) {
			for _, c := range Codecs {
				c := c
				b.Run(c.Name, func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(int64(len(fx.Data)))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						v := targetFor(fx.Kind)
						if err := c.Unmarshal(fx.Data, v); err != nil {
							// surface real correctness failures so we don't
							// celebrate a "fast" codec that is silently
							// dropping data
							b.Fatalf("%s on %s: %v", c.Name, fx.Name, err)
						}
					}
				})
			}
		})
	}
}

// BenchmarkMarshal exercises every (codec × fixture) pair on the
// "Go struct -> bytes" direction. We marshal a value the codec just
// unmarshalled (using stdlib so we don't bias toward whichever codec
// is under test).
func BenchmarkMarshal(b *testing.B) {
	for _, fx := range allFixtures {
		fx := fx
		v := targetFor(fx.Kind)
		// canonicalize via stdlib so every codec marshals from the
		// same in-memory shape
		stdlib := Codecs[0]
		if !strings.HasPrefix(stdlib.Name, "encoding/json") {
			b.Fatalf("Codecs[0] is expected to be encoding/json, got %s", stdlib.Name)
		}
		if err := stdlib.Unmarshal(fx.Data, v); err != nil {
			b.Fatalf("seed unmarshal of %s failed: %v", fx.Name, err)
		}

		b.Run(fx.Name, func(b *testing.B) {
			for _, c := range Codecs {
				c := c
				b.Run(c.Name, func(b *testing.B) {
					b.ReportAllocs()
					var lastSize int64
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						out, err := c.Marshal(v)
						if err != nil {
							b.Fatalf("%s on %s: %v", c.Name, fx.Name, err)
						}
						lastSize = int64(len(out))
					}
					b.SetBytes(lastSize)
				})
			}
		})
	}
}

// BenchmarkRoundTrip is the realistic per-request path: parse the
// inbound JSON, then re-encode it (e.g. forwarding to an adapter or
// emitting the response).
func BenchmarkRoundTrip(b *testing.B) {
	for _, fx := range allFixtures {
		fx := fx
		b.Run(fx.Name, func(b *testing.B) {
			for _, c := range Codecs {
				c := c
				b.Run(c.Name, func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(int64(len(fx.Data)))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						v := targetFor(fx.Kind)
						if err := c.Unmarshal(fx.Data, v); err != nil {
							b.Fatalf("unmarshal %s on %s: %v", c.Name, fx.Name, err)
						}
						if _, err := c.Marshal(v); err != nil {
							b.Fatalf("marshal %s on %s: %v", c.Name, fx.Name, err)
						}
					}
				})
			}
		})
	}
}
