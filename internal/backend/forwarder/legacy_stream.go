// legacy_stream.go provides an HTTP wrapper that is compatible with Cursor's legacy RunSSE response headers.
package forwarder

import (
	"context"
	"net/http"

	"cursor/gen/agentv1"
	"cursor/gen/aiserverv1"

	"connectrpc.com/connect"
)

const legacyRunSSEContentType = "text/event-stream"

// NewLegacyRunSSEHandler builds a Connect ServerStream handler that presents itself as text/event-stream.
func NewLegacyRunSSEHandler(
	procedure string,
	implementation func(context.Context, *connect.Request[aiserverv1.BidiRequestId], *connect.ServerStream[agentv1.AgentServerMessage]) error,
	options ...connect.HandlerOption,
) http.Handler {
	inner := connect.NewServerStreamHandler(procedure, implementation, options...)
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		inner.ServeHTTP(newLegacyRunSSEHeaderWriter(writer), request)
	})
}

// legacyRunSSEHeaderWriter overrides Content-Type when the header is actually written.
type legacyRunSSEHeaderWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

// newLegacyRunSSEHeaderWriter creates a response-header wrapper.
func newLegacyRunSSEHeaderWriter(writer http.ResponseWriter) *legacyRunSSEHeaderWriter {
	return &legacyRunSSEHeaderWriter{ResponseWriter: writer}
}

// WriteHeader fills in the legacy-required response headers before writing the status code.
func (writer *legacyRunSSEHeaderWriter) WriteHeader(statusCode int) {
	writer.applyLegacyHeaders()
	writer.wroteHeader = true
	writer.ResponseWriter.WriteHeader(statusCode)
}

// Write lazily writes the header on the first body write.
func (writer *legacyRunSSEHeaderWriter) Write(payload []byte) (int, error) {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(payload)
}

// Flush attempts to flush the underlying buffer to the client immediately.
func (writer *legacyRunSSEHeaderWriter) Flush() {
	if flusher, ok := writer.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter.
func (writer *legacyRunSSEHeaderWriter) Unwrap() http.ResponseWriter {
	return writer.ResponseWriter
}

// applyLegacyHeaders sets the response headers required by Cursor legacy RunSSE.
func (writer *legacyRunSSEHeaderWriter) applyLegacyHeaders() {
	header := writer.ResponseWriter.Header()
	header.Set("Content-Type", legacyRunSSEContentType)
	if header.Get("Cache-Control") == "" {
		header.Set("Cache-Control", "no-cache")
	}
}
