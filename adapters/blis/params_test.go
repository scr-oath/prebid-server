package blis

import (
	"encoding/json"
	"testing"

	"github.com/prebid/prebid-server/v3/openrtb_ext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testSpec struct {
	name    string
	json    string
	wantErr bool
}

func testParams(t *testing.T, specs []testSpec) {
	validator, err := openrtb_ext.NewBidderParamsValidator("../../static/bidder-params")
	require.NoError(t, err, "Failed to fetch the json schema")
	for _, spec := range specs {
		t.Run(spec.name, func(t *testing.T) {
			if spec.wantErr {
				assert.Error(t, validator.Validate(openrtb_ext.BidderBlis, json.RawMessage(spec.json)))
			} else {
				assert.NoError(t, validator.Validate(openrtb_ext.BidderBlis, json.RawMessage(spec.json)))
			}
		})
	}
}

func TestValidParams(t *testing.T) {
	testParams(t, []testSpec{
		{
			name: "Valid params with spid",
			json: `{"spid": "9999"}`,
		},
	})
}

func TestInvalidParams(t *testing.T) {
	testParams(t, []testSpec{
		{
			name:    "Empty params",
			json:    `{}`,
			wantErr: true,
		},
		{
			name:    "Empty spid",
			json:    `{"spid": ""}`,
			wantErr: true,
		},
		{
			name:    "Invalid spid type",
			json:    `{"spid": 9999}`,
			wantErr: true,
		},
	})
}

var validParams = []string{
	`{"spid": "9999"}`,
}

var invalidParams = []string{
	`{}`,
	`{"spid": ""}`,
	`{"spid": 9999}`,
}
