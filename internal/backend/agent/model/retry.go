// retry.go giữ lại tên gọi lịch sử của điểm vào yêu cầu HTTP provider; lỗi provider được giao cho chuỗi kết nối lại của client xử lý.
package modeladapter

import (
	"context"
	"net/http"
)

// DoProviderRequestWithRetry giữ lại tên điểm vào cũ; chế độ cục bộ không thử lại yêu cầu provider ở phía máy chủ.
func DoProviderRequestWithRetry(
	ctx context.Context,
	client *http.Client,
	provider string,
	requestID string,
	modelCallID string,
	buildRequest func(context.Context) (*http.Request, error),
) (*http.Response, error) {
	return doProviderRequestWithRetry(ctx, client, provider, requestID, modelCallID, buildRequest)
}

func doProviderRequestWithRetry(
	ctx context.Context,
	client *http.Client,
	provider string,
	requestID string,
	modelCallID string,
	buildRequest func(context.Context) (*http.Request, error),
) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	httpReq, err := buildRequest(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return nil, err
	}
	return resp, nil
}

// ProviderRetryAttemptSummary trả về giá trị rỗng; yêu cầu provider không còn tóm tắt thử lại nội bộ phía máy chủ.
func ProviderRetryAttemptSummary(resp *http.Response) string {
	return ""
}
