// http_error.go chịu trách nhiệm biến phản hồi HTTP không phải 2xx thành lỗi kèm tóm tắt thân phản hồi.
package modeladapter

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	// maxErrorBodyBytes is the maximum number of bytes read from the error response body.
	maxErrorBodyBytes = 8192
)

// buildHTTPStatusError reads the response body summary and generates an error carrying the status code.
func buildHTTPStatusError(prefix string, resp *http.Response) error {
	if resp == nil {
		return fmt.Errorf("%s response is nil", strings.TrimSpace(prefix))
	}

	limitedBody, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	if err != nil {
		if retrySummary := ProviderRetryAttemptSummary(resp); retrySummary != "" {
			return fmt.Errorf("%s status=%d %s body_read_error=%v", strings.TrimSpace(prefix), resp.StatusCode, retrySummary, err)
		}
		return fmt.Errorf("%s status=%d body_read_error=%v", strings.TrimSpace(prefix), resp.StatusCode, err)
	}
	retrySummary := ProviderRetryAttemptSummary(resp)
	bodyText := strings.TrimSpace(string(limitedBody))
	if bodyText == "" {
		if retrySummary != "" {
			return fmt.Errorf("%s status=%d %s", strings.TrimSpace(prefix), resp.StatusCode, retrySummary)
		}
		return fmt.Errorf("%s status=%d", strings.TrimSpace(prefix), resp.StatusCode)
	}
	if retrySummary != "" {
		return fmt.Errorf("%s status=%d %s body=%s", strings.TrimSpace(prefix), resp.StatusCode, retrySummary, bodyText)
	}
	return fmt.Errorf("%s status=%d body=%s", strings.TrimSpace(prefix), resp.StatusCode, bodyText)
}
