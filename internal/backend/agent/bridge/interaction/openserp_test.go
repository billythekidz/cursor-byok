package interaction

import (
	"fmt"
	"reflect"
	"testing"

	"cursor/gen/agentv1"
	"cursor/internal/search/openserp"
)

func TestOpenSERPReferencesLabelsPrimaryAndFallbackSources(t *testing.T) {
	groups := []openserp.EngineResults{
		{
			RequestedEngine: "google",
			SourceEngine:    "google",
			Results: []openserp.Result{{
				Title:   "  Go documentation  ",
				URL:     "https://go.dev/doc",
				Snippet: "The Go programming language.",
			}},
		},
		{
			RequestedEngine: "baidu",
			SourceEngine:    "ecosia",
			Fallback:        true,
			Results: []openserp.Result{{
				URL:     "https://example.test/ecosia",
				Snippet: "Fallback result.",
			}},
		},
	}

	got := openSERPReferences(groups)
	want := []*agentv1.WebSearchReference{
		{
			Title: "[Google] Go documentation",
			Url:   "https://go.dev/doc",
			Chunk: "The Go programming language.",
		},
		{
			Title: "[Ecosia fallback for Baidu] https://example.test/ecosia",
			Url:   "https://example.test/ecosia",
			Chunk: "Fallback result.",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("openSERPReferences() = %#v, want %#v", got, want)
	}
}

func TestOpenSERPReferencesPreservesResultOrder(t *testing.T) {
	results := make([]openserp.Result, 0, 5)
	for index := 1; index <= 5; index++ {
		results = append(results, openserp.Result{
			Title: fmt.Sprintf("Result %d", index),
			URL:   fmt.Sprintf("https://example.test/%d", index),
		})
	}
	got := openSERPReferences([]openserp.EngineResults{{
		RequestedEngine: "duckduckgo",
		SourceEngine:    "duckduckgo",
		Results:         results,
	}})
	if len(got) != len(results) {
		t.Fatalf("got %d references, want %d", len(got), len(results))
	}
	for index, reference := range got {
		wantTitle := fmt.Sprintf("[DuckDuckGo] Result %d", index+1)
		if reference.Title != wantTitle || reference.Url != fmt.Sprintf("https://example.test/%d", index+1) {
			t.Errorf("reference %d = %#v, want title %q and matching URL", index, reference, wantTitle)
		}
	}
}
