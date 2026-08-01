package mitm

import (
	"fmt"
	"io"
	"net/http"
	"sync"
)

// localRelayResponseWriter defines the localRelayResponseWriter type in this module.
type localRelayResponseWriter struct {
	// header represents the header field in this declaration.
	header http.Header
	// pipeWriter represents the pipeWriter field in this declaration.
	pipeWriter *io.PipeWriter
	// ready represents the ready field in this declaration.
	ready chan struct{}
	// wroteHeader represents the wroteHeader field in this declaration.
	wroteHeader bool
	// statusCode represents the statusCode field in this declaration.
	statusCode int
	// once represents the once field in this declaration.
	once sync.Once
	// mu represents the mu field in this declaration.
	mu sync.Mutex
}

// newLocalRelayResponseWriter handles logic related to newLocalRelayResponseWriter.
func newLocalRelayResponseWriter(pipeWriter *io.PipeWriter) *localRelayResponseWriter {
	return &localRelayResponseWriter{
		header:     make(http.Header),
		pipeWriter: pipeWriter,
		ready:      make(chan struct{}),
		statusCode: http.StatusOK,
	}
}

// Header handles logic related to Header.
func (w *localRelayResponseWriter) Header() http.Header {
	return w.header
}

// WriteHeader handles logic related to WriteHeader.
func (w *localRelayResponseWriter) WriteHeader(statusCode int) {
	w.mu.Lock()
	if !w.wroteHeader {
		w.wroteHeader = true
		w.statusCode = statusCode
		w.once.Do(func() {
			close(w.ready)
		})
	}
	w.mu.Unlock()
}

// Write handles logic related to Write.
func (w *localRelayResponseWriter) Write(body []byte) (int, error) {
	if !w.headerWritten() {
		w.WriteHeader(http.StatusOK)
	}
	return w.pipeWriter.Write(body)
}

// Flush handles logic related to Flush.
func (w *localRelayResponseWriter) Flush() {
	if !w.headerWritten() {
		w.WriteHeader(http.StatusOK)
	}
}

// Finish handles logic related to Finish.
func (w *localRelayResponseWriter) Finish(err error) {
	if err != nil && !w.headerWritten() {
		w.WriteHeader(http.StatusInternalServerError)
	}
	if !w.headerWritten() {
		w.WriteHeader(http.StatusOK)
	}
	if err != nil {
		_ = w.pipeWriter.CloseWithError(err)
		return
	}
	_ = w.pipeWriter.Close()
}

// Ready handles logic related to Ready.
func (w *localRelayResponseWriter) Ready() <-chan struct{} {
	return w.ready
}

// Response handles logic related to Response.
func (w *localRelayResponseWriter) Response(request *http.Request, body io.ReadCloser) *http.Response {
	w.mu.Lock()
	statusCode := w.statusCode
	headers := w.header.Clone()
	w.mu.Unlock()
	return &http.Response{
		StatusCode:    statusCode,
		Status:        fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		Header:        headers,
		Body:          body,
		ContentLength: -1,
		Request:       request,
	}
}

// headerWritten handles logic related to headerWritten.
func (w *localRelayResponseWriter) headerWritten() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.wroteHeader
}
