package openserp

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newTestClient(serverURL string, httpClient *http.Client) *Client {
	return &Client{manager: &manager{
		httpClient: httpClient,
		ensureFunc: func(context.Context) (string, error) {
			return serverURL, nil
		},
	}}
}

func TestClientSearchCollectsTopFivePerPrimaryEngine(t *testing.T) {
	var requestsMu sync.Mutex
	requests := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		engine := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/"), "/search")
		requestsMu.Lock()
		requests[engine]++
		requestsMu.Unlock()
		if request.URL.Query().Get("text") != "golang" || request.URL.Query().Get("limit") != "5" || request.URL.Query().Get("format") != "json" {
			http.Error(response, "unexpected query", http.StatusBadRequest)
			return
		}
		items := make([]Result, 0, 7)
		for index := 1; index <= 7; index++ {
			items = append(items, Result{
				Title:   fmt.Sprintf("%s result %d", engine, index),
				URL:     fmt.Sprintf("https://example.test/%s/%d", engine, index),
				Snippet: fmt.Sprintf("snippet %d", index),
				Engine:  engine,
			})
		}
		_ = json.NewEncoder(response).Encode(struct {
			Results []Result `json:"results"`
		}{Results: items})
	}))
	defer server.Close()

	groups, err := newTestClient(server.URL, server.Client()).Search(context.Background(), "golang")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	wantEngines := []string{"google", "baidu", "duckduckgo", "yandex"}
	if len(groups) != len(wantEngines) {
		t.Fatalf("got %d engine groups, want %d", len(groups), len(wantEngines))
	}
	for index, group := range groups {
		if group.RequestedEngine != wantEngines[index] || group.SourceEngine != wantEngines[index] || group.Fallback {
			t.Errorf("group %d = %#v, want primary %s", index, group, wantEngines[index])
		}
		if len(group.Results) != 5 {
			t.Errorf("group %d returned %d results, want 5", index, len(group.Results))
			continue
		}
		if group.Results[0].Title != wantEngines[index]+" result 1" || group.Results[4].Title != wantEngines[index]+" result 5" {
			t.Errorf("group %d did not preserve the first five results: %#v", index, group.Results)
		}
	}
	requestsMu.Lock()
	defer requestsMu.Unlock()
	for _, engine := range wantEngines {
		if requests[engine] != 1 {
			t.Errorf("engine %s was requested %d times, want once", engine, requests[engine])
		}
	}
}

func TestClientSearchUsesSuccessfulYandexAsFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		engine := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/"), "/search")
		if engine == "google" {
			http.Error(response, "google unavailable", http.StatusBadGateway)
			return
		}
		writeTestResults(response, engine)
	}))
	defer server.Close()

	groups, err := newTestClient(server.URL, server.Client()).Search(context.Background(), "golang")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(groups) != 4 {
		t.Fatalf("got %d groups, want 4", len(groups))
	}
	if groups[0].RequestedEngine != "google" || groups[0].SourceEngine != "yandex" || !groups[0].Fallback {
		t.Fatalf("google replacement = %#v, want Yandex fallback", groups[0])
	}
	if len(groups[0].Results) != 5 {
		t.Fatalf("Yandex fallback returned %d results, want 5", len(groups[0].Results))
	}
}

func TestClientSearchUsesEcosiaWhenYandexFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		engine := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/"), "/search")
		if engine == "yandex" {
			http.Error(response, "yandex unavailable", http.StatusBadGateway)
			return
		}
		writeTestResults(response, engine)
	}))
	defer server.Close()

	groups, err := newTestClient(server.URL, server.Client()).Search(context.Background(), "golang")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(groups) != 4 {
		t.Fatalf("got %d groups, want 4", len(groups))
	}
	if groups[3].RequestedEngine != "yandex" || groups[3].SourceEngine != "ecosia" || !groups[3].Fallback {
		t.Fatalf("Yandex replacement = %#v, want Ecosia fallback", groups[3])
	}
	if len(groups[3].Results) != 5 {
		t.Fatalf("Ecosia fallback returned %d results, want 5", len(groups[3].Results))
	}
}

func TestClientSearchStopsAfterThreeEngineErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		engine := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/"), "/search")
		if engine == "google" || engine == "baidu" || engine == "yandex" {
			http.Error(response, engine+" unavailable", http.StatusBadGateway)
			return
		}
		writeTestResults(response, engine)
	}))
	defer server.Close()

	groups, err := newTestClient(server.URL, server.Client()).Search(context.Background(), "golang")
	if !errors.Is(err, ErrTooManyErrors) {
		t.Fatalf("Search error = %v, want ErrTooManyErrors", err)
	}
	if groups != nil {
		t.Fatalf("Search returned groups after three errors: %#v", groups)
	}
}

func TestMaterializeEmbeddedBinary(t *testing.T) {
	previous := embeddedBinary
	embeddedBinary = []byte("test-openserp-binary")
	defer func() { embeddedBinary = previous }()

	cacheDir := t.TempDir()
	path, err := materializeEmbeddedBinary(cacheDir)
	if err != nil {
		t.Fatalf("materializeEmbeddedBinary returned error: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) returned error: %v", path, err)
	}
	if !bytes.Equal(content, embeddedBinary) {
		t.Fatalf("materialized content = %q, want %q", content, embeddedBinary)
	}
	secondPath, err := materializeEmbeddedBinary(cacheDir)
	if err != nil {
		t.Fatalf("second materializeEmbeddedBinary returned error: %v", err)
	}
	if secondPath != path {
		t.Fatalf("second path = %q, want %q", secondPath, path)
	}
}

func TestExtractBinary(t *testing.T) {
	archivePath := writeTestArchive(t, "nested/openserp", []byte("test-openserp-binary"))
	binaryPath := filepath.Join(t.TempDir(), "openserp")
	if err := extractBinary(archivePath, binaryPath); err != nil {
		t.Fatalf("extractBinary returned error: %v", err)
	}
	content, err := os.ReadFile(binaryPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) returned error: %v", binaryPath, err)
	}
	if string(content) != "test-openserp-binary" {
		t.Fatalf("extracted content = %q, want test-openserp-binary", content)
	}
}

func writeTestResults(response http.ResponseWriter, engine string) {
	items := make([]Result, 0, 5)
	for index := 1; index <= 5; index++ {
		items = append(items, Result{
			Title:   fmt.Sprintf("%s result %d", engine, index),
			URL:     fmt.Sprintf("https://example.test/%s/%d", engine, index),
			Snippet: fmt.Sprintf("snippet %d", index),
			Engine:  engine,
		})
	}
	_ = json.NewEncoder(response).Encode(struct {
		Results []Result `json:"results"`
	}{Results: items})
}

func writeTestArchive(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "openserp.tgz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("os.Create returned error: %v", err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o700, Size: int64(len(content))}); err != nil {
		t.Fatalf("WriteHeader returned error: %v", err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatalf("tar Write returned error: %v", err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("tar Close returned error: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("gzip Close returned error: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file Close returned error: %v", err)
	}
	return path
}
