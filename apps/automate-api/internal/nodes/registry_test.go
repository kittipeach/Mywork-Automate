package nodes

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// TestAll_MatchesTSRegistry proves the Go node catalog serialises to the exact
// same structure as the web nodeRegistry.ts NODE_TYPES (testdata golden was
// extracted from that TS literal). We compare as decoded generic values so map
// key ordering is irrelevant but presence/absence of keys (omitempty) and every
// value must match.
func TestAll_MatchesTSRegistry(t *testing.T) {
	goldenBytes, err := os.ReadFile("testdata/nodes_golden.json")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var golden any
	if err := json.Unmarshal(goldenBytes, &golden); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}

	goBytes, err := json.Marshal(All())
	if err != nil {
		t.Fatalf("marshal registry: %v", err)
	}
	var got any
	if err := json.Unmarshal(goBytes, &got); err != nil {
		t.Fatalf("unmarshal registry: %v", err)
	}

	if !reflect.DeepEqual(got, golden) {
		t.Fatalf("registry JSON mismatch.\n got=%s\nwant=%s", goBytes, goldenBytes)
	}
}

// TestAll_Count is a fast guard that the catalog size matches the TS registry.
func TestAll_Count(t *testing.T) {
	if len(All()) != 9 {
		t.Fatalf("node count = %d, want 9", len(All()))
	}
}

// TestAll_KeysPresentPerNode asserts every node carries the required top-level
// keys the UI palette reads.
func TestAll_KeysPresentPerNode(t *testing.T) {
	b, err := json.Marshal(All())
	if err != nil {
		t.Fatal(err)
	}
	var arr []map[string]any
	if err := json.Unmarshal(b, &arr); err != nil {
		t.Fatal(err)
	}
	want := []string{"type", "category", "label", "description", "icon", "accent", "inputs", "outputs", "schema"}
	for _, n := range arr {
		for _, k := range want {
			if _, ok := n[k]; !ok {
				t.Errorf("node %v missing key %q", n["type"], k)
			}
		}
	}
}
