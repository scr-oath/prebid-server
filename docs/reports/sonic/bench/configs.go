package bench

import (
	"encoding/json"

	"github.com/bytedance/sonic"
	jsoniter "github.com/json-iterator/go"
)

// Codec is a JSON marshal/unmarshal pair, named so we can label results.
type Codec struct {
	Name      string
	Marshal   func(v any) ([]byte, error)
	Unmarshal func(data []byte, v any) error
}

// Codecs are the libraries / configurations under test.
//
// jsoniter configs mirror what prebid-server actually uses today
// (see util/jsonutil/jsonutil.go and util/jsonutil/merge.go):
//
//   - "jsoniter/std-compat" matches `jsonConfigValidationOn` (drives
//     UnmarshalValid + Marshal).
//   - "jsoniter/validation-off" matches `jsonConfigValidationOff`
//     (drives Unmarshal — the lenient path used in adapters/exchange).
//   - "jsoniter/merge-clone" matches `jsonConfigMergeClone`. The
//     merge-clone extension is registered against this profile because
//     that's how stored-request merging actually runs.
//
// sonic configs are the three pre-baked profiles the project ships:
//
//   - "sonic/std" — drop-in for encoding/json (EscapeHTML, SortMapKeys).
//   - "sonic/default" — Sonic's recommended default; trades a small set
//     of stdlib quirks for speed (no HTML escape, no key sort).
//   - "sonic/fastest" — even fewer guarantees; skips UTF8 validation
//     and a couple of textmarshaler checks.
var Codecs []Codec

var (
	jsonIterValidationOn  = jsoniter.ConfigCompatibleWithStandardLibrary
	jsonIterValidationOff = jsoniter.Config{
		EscapeHTML:             true,
		SortMapKeys:            true,
		ValidateJsonRawMessage: false,
	}.Froze()
	jsonIterMergeClone = jsoniter.Config{
		EscapeHTML:             true,
		SortMapKeys:            true,
		ValidateJsonRawMessage: true,
	}.Froze()
)

func init() {
	jsonIterMergeClone.RegisterExtension(&mergeCloneExtensionStub{})

	Codecs = []Codec{
		{
			Name:      "encoding/json",
			Marshal:   json.Marshal,
			Unmarshal: json.Unmarshal,
		},
		{
			Name:      "jsoniter/std-compat",
			Marshal:   jsonIterValidationOn.Marshal,
			Unmarshal: jsonIterValidationOn.Unmarshal,
		},
		{
			Name:      "jsoniter/validation-off",
			Marshal:   jsonIterValidationOff.Marshal,
			Unmarshal: jsonIterValidationOff.Unmarshal,
		},
		{
			Name:      "jsoniter/merge-clone",
			Marshal:   jsonIterMergeClone.Marshal,
			Unmarshal: jsonIterMergeClone.Unmarshal,
		},
		{
			Name:      "sonic/std",
			Marshal:   sonic.ConfigStd.Marshal,
			Unmarshal: sonic.ConfigStd.Unmarshal,
		},
		{
			Name:      "sonic/default",
			Marshal:   sonic.ConfigDefault.Marshal,
			Unmarshal: sonic.ConfigDefault.Unmarshal,
		},
		{
			Name:      "sonic/fastest",
			Marshal:   sonic.ConfigFastest.Marshal,
			Unmarshal: sonic.ConfigFastest.Unmarshal,
		},
	}
}
