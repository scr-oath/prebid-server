package bench

import (
	"os"
	"path/filepath"
	"runtime"
)

// Fixture is one sample payload exercised by the benchmark suite.
type Fixture struct {
	Name string
	Kind string // "BidRequest" or "BidResponse"
	Data []byte
}

// LoadFixtures reads the JSON fixtures from ../fixtures/ relative to this file.
// We resolve the path from the source file's location so `go test` can run from
// any cwd.
func LoadFixtures() ([]Fixture, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(thisFile), "..", "fixtures")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var fixtures []Fixture
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		kind := "BidRequest"
		if len(e.Name()) >= 6 && e.Name()[:6] == "bresp_" {
			kind = "BidResponse"
		}
		fixtures = append(fixtures, Fixture{
			Name: e.Name(),
			Kind: kind,
			Data: data,
		})
	}
	return fixtures, nil
}
