package interaction

import (
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"cursor/gen/agentv1"
)

const (
	baiduWebSearchBaseURL     = "https://www.baidu.com/s?ie=utf-8&tn=baidu&wd="
	baiduWebSearchHostURL     = "https://www.baidu.com"
	baiduSearchAbstractLimit  = 300
	baiduSearchReferenceLimit = 8
)

// extractBaiduWebSearchReferences parses the search result list from the Baidu search result page HTML.
func extractBaiduWebSearchReferences(body string) []*agentv1.WebSearchReference {
	document, err := goquery.NewDocumentFromReader(strings.NewReader(body))
	if err != nil {
		return nil
	}
	references := make([]*agentv1.WebSearchReference, 0, baiduSearchReferenceLimit)
	document.Find("#content_left > div").EachWithBreak(func(_ int, selection *goquery.Selection) bool {
		if len(references) >= baiduSearchReferenceLimit {
			return false
		}
		if !selection.HasClass("c-container") {
			return true
		}
		title, resultURL, abstract := extractBaiduSearchResult(selection)
		if title == "" || resultURL == "" {
			return true
		}
		references = append(references, &agentv1.WebSearchReference{
			Title: title,
			Url:   normalizeBaiduSearchURL(resultURL),
			Chunk: truncateBaiduSearchAbstract(abstract),
		})
		return true
	})
	return references
}

// extractBaiduSearchResult extracts the title, link, and abstract from a single Baidu search result node.
func extractBaiduSearchResult(selection *goquery.Selection) (string, string, string) {
	title := cleanBaiduSearchText(selection.Find("h3").First().Text())
	resultURL, _ := selection.Find("h3 a").First().Attr("href")
	if title == "" {
		title = firstBaiduSearchLine(selection.Text())
	}
	if resultURL == "" {
		resultURL, _ = selection.Find("a").First().Attr("href")
	}
	abstract := cleanBaiduSearchText(selection.Find(".c-abstract").First().Text())
	if abstract == "" {
		abstract = cleanBaiduSearchText(selection.ChildrenFiltered("div").First().Text())
	}
	if abstract == "" {
		abstract = baiduSearchTextAfterFirstLine(selection.Text())
	}
	return title, strings.TrimSpace(resultURL), abstract
}

// normalizeBaiduSearchURL normalizes relative or protocol-omitted links returned by Baidu into absolute URLs.
func normalizeBaiduSearchURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if strings.HasPrefix(rawURL, "//") {
		return "https:" + rawURL
	}
	if strings.HasPrefix(rawURL, "/") {
		return baiduWebSearchHostURL + rawURL
	}
	return rawURL
}

// resolveBaiduWebSearchRedirects resolves Baidu redirect links to their final destination addresses, updating the reference list in place.
func resolveBaiduWebSearchRedirects(client *http.Client, references []*agentv1.WebSearchReference) {
	for _, reference := range references {
		if reference == nil {
			continue
		}
		reference.Url = resolveBaiduRedirectURL(client, reference.GetUrl())
	}
}

// resolveBaiduRedirectURL determines whether a link is a Baidu redirect link and attempts to resolve its real destination.
func resolveBaiduRedirectURL(client *http.Client, rawURL string) string {
	resultURL := normalizeBaiduSearchURL(rawURL)
	if !isBaiduRedirectURL(resultURL) {
		return resultURL
	}
	redirectClient := baiduRedirectHTTPClient(client)
	if location := requestBaiduRedirectLocation(redirectClient, http.MethodHead, resultURL); location != "" {
		return location
	}
	if location := requestBaiduRedirectLocation(redirectClient, http.MethodGet, resultURL); location != "" {
		return location
	}
	return resultURL
}

// baiduRedirectHTTPClient builds a short-timeout client based on the base client that does not automatically follow redirects.
func baiduRedirectHTTPClient(base *http.Client) *http.Client {
	if base == nil {
		base = http.DefaultClient
	}
	client := *base
	if client.Timeout == 0 || client.Timeout > 6*time.Second {
		client.Timeout = 6 * time.Second
	}
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &client
}

// requestBaiduRedirectLocation issues a request and reads the redirect destination from the response headers.
func requestBaiduRedirectLocation(client *http.Client, method string, rawURL string) string {
	request, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return ""
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0 Safari/537.36")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	request.Header.Set("Referer", baiduWebSearchHostURL+"/")
	response, err := client.Do(request)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	location := strings.TrimSpace(response.Header.Get("Location"))
	if location == "" {
		return ""
	}
	return resolveBaiduLocationURL(rawURL, location)
}

// resolveBaiduLocationURL resolves a relative redirect address in the response headers into an absolute address.
func resolveBaiduLocationURL(baseURL string, location string) string {
	parsedLocation, err := neturl.Parse(location)
	if err != nil {
		return location
	}
	if parsedLocation.IsAbs() {
		return parsedLocation.String()
	}
	parsedBase, err := neturl.Parse(baseURL)
	if err != nil {
		return location
	}
	return parsedBase.ResolveReference(parsedLocation).String()
}

// isBaiduRedirectURL reports whether the given address is a redirect link under the Baidu domain.
func isBaiduRedirectURL(rawURL string) bool {
	parsedURL, err := neturl.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsedURL.Hostname())
	path := strings.ToLower(parsedURL.EscapedPath())
	return (host == "baidu.com" || strings.HasSuffix(host, ".baidu.com")) && strings.HasPrefix(path, "/link")
}

// truncateBaiduSearchAbstract truncates the abstract text by character count to avoid overly long results.
func truncateBaiduSearchAbstract(value string) string {
	value = cleanBaiduSearchText(value)
	if baiduSearchAbstractLimit <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= baiduSearchAbstractLimit {
		return value
	}
	return string(runes[:baiduSearchAbstractLimit])
}

// firstBaiduSearchLine returns the first non-empty line of the text.
func firstBaiduSearchLine(value string) string {
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r", "\n"), "\n") {
		line = cleanBaiduSearchText(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// baiduSearchTextAfterFirstLine returns the concatenation of the remaining text after the first non-empty line.
func baiduSearchTextAfterFirstLine(value string) string {
	nonEmpty := make([]string, 0, 8)
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r", "\n"), "\n") {
		line = cleanBaiduSearchText(line)
		if line != "" {
			nonEmpty = append(nonEmpty, line)
		}
	}
	if len(nonEmpty) <= 1 {
		return ""
	}
	return cleanBaiduSearchText(strings.Join(nonEmpty[1:], " "))
}

// cleanBaiduSearchText collapses extra whitespace and trims leading/trailing spaces.
func cleanBaiduSearchText(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
