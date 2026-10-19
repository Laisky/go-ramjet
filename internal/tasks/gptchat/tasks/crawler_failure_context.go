package tasks

import (
	"context"
	"encoding/json"
	"mime"
	"net/url"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/laisky-blog-graphql/library/crawleregress"
	"github.com/Laisky/zap"
	"golang.org/x/net/html"
)

const crawlerFailureContextMaxBytes = 2048

// crawlerFailureContext contains observed scalar diagnostics, never page data.
// Pointer counts distinguish observed zero from a stage that was never reached.
type crawlerFailureContext struct {
	TargetScheme                 string `json:"target_scheme,omitempty"`
	TargetHostname               string `json:"target_hostname,omitempty"`
	FinalScheme                  string `json:"final_scheme,omitempty"`
	FinalHostname                string `json:"final_hostname,omitempty"`
	UpstreamStatus               *int64 `json:"upstream_status,omitempty"`
	ContentType                  string `json:"content_type,omitempty"`
	RedirectCount                *int64 `json:"redirect_count,omitempty"`
	RenderedHTMLBytes            *int64 `json:"rendered_html_bytes,omitempty"`
	ConversionInputBytes         *int64 `json:"conversion_input_bytes,omitempty"`
	ConversionOutputBytes        *int64 `json:"conversion_output_bytes,omitempty"`
	ConversionOutputTrimmedBytes *int64 `json:"conversion_output_trimmed_bytes,omitempty"`
	BodyTextRunes                *int64 `json:"body_text_runes,omitempty"`
	BodyTextNodes                *int64 `json:"body_text_nodes,omitempty"`
	Converter                    string `json:"converter,omitempty"`
}

// crawlerContextKey identifies the task-local observation holder.
type crawlerContextKey struct{}

// crawlerDocumentObservation tracks only actual primary-document response chains.
type crawlerDocumentObservation struct{ depth, status int64 }

// crawlerContextObservation snapshots diagnostics independently of task data.
type crawlerContextObservation struct {
	mu              sync.Mutex
	value           crawlerFailureContext
	documents       map[string]crawlerDocumentObservation
	documentsSealed bool
}

// newCrawlerContextObservation returns a holder for an already-admitted target.
func newCrawlerContextObservation(ctx context.Context, target string) (context.Context, *crawlerContextObservation) {
	scheme, hostname := crawlerContextOrigin(target)
	value := &crawlerContextObservation{value: crawlerFailureContext{TargetScheme: scheme, TargetHostname: hostname}, documents: map[string]crawlerDocumentObservation{}}
	return context.WithValue(ctx, crawlerContextKey{}, value), value
}

// crawlerContextOrigin projects only canonical scheme and hostname; it performs no I/O.
func crawlerContextOrigin(raw string) (string, string) {
	parsed, err := url.Parse(crawleregress.RequestURL(raw))
	if err != nil || parsed.Hostname() == "" {
		return "", ""
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if len(host) > 253 {
		return "", ""
	}
	return parsed.Scheme, host
}

// cloneCrawlerContext copies scalar pointers so later observations cannot mutate a receipt.
func cloneCrawlerContext(value *crawlerFailureContext) *crawlerFailureContext {
	if value == nil {
		return nil
	}
	copyValue := *value
	pointers := []**int64{&copyValue.UpstreamStatus, &copyValue.RedirectCount, &copyValue.RenderedHTMLBytes, &copyValue.ConversionInputBytes,
		&copyValue.ConversionOutputBytes, &copyValue.ConversionOutputTrimmedBytes, &copyValue.BodyTextRunes, &copyValue.BodyTextNodes}
	for _, pointer := range pointers {
		if *pointer != nil {
			number := **pointer
			*pointer = &number
		}
	}
	return &copyValue
}

// snapshot returns an independent observation without exposing the holder or document map.
func (o *crawlerContextObservation) snapshot() *crawlerFailureContext {
	o.mu.Lock()
	defer o.mu.Unlock()
	return cloneCrawlerContext(&o.value)
}

// sealDocuments stops late renderer callbacks while retaining conversion observations.
func (o *crawlerContextObservation) sealDocuments() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.documentsSealed = true
}

// crawlerObservation returns the optional task-local holder without changing context.
func crawlerObservation(ctx context.Context) *crawlerContextObservation {
	value, _ := ctx.Value(crawlerContextKey{}).(*crawlerContextObservation)
	return value
}

// observeCrawlerDocumentResponse records only actual pinned primary-document response headers.
// The caller supplies kind after the existing root-frame classification and pin checks.
func observeCrawlerDocumentResponse(ctx context.Context, kind, id, parent, raw string, status int, contentType string) {
	if kind != crawleregress.OriginDocument {
		return
	}
	observation := crawlerObservation(ctx)
	if observation == nil {
		return
	}
	scheme, host := crawlerContextOrigin(raw)
	if scheme == "" || host == "" || status < 100 || status > 599 {
		return
	}
	media, _, mediaErr := mime.ParseMediaType(contentType)
	if mediaErr != nil || len(media) > 127 || !strings.Contains(media, "/") {
		media = ""
	}
	observation.mu.Lock()
	defer observation.mu.Unlock()
	if observation.documentsSealed {
		return
	}
	depth := int64(0)
	known := parent == ""
	if previous, ok := observation.documents[parent]; parent != "" && ok {
		switch previous.status {
		case 301, 302, 303, 307, 308:
			depth = previous.depth + 1
			known = true
		}
	}
	observation.value.FinalScheme, observation.value.FinalHostname = scheme, host
	number := int64(status)
	observation.value.UpstreamStatus = &number
	observation.value.ContentType = media
	observation.value.RedirectCount = nil
	if known && depth <= 20 {
		observation.value.RedirectCount = &depth
		observation.documents[id] = crawlerDocumentObservation{depth: depth, status: number}
	}
}

// observeCrawlerRenderedBytes records the successfully returned rendered HTML length.
func observeCrawlerRenderedBytes(ctx context.Context, size int) {
	if observation := crawlerObservation(ctx); observation != nil {
		observation.mu.Lock()
		defer observation.mu.Unlock()
		number := int64(size)
		observation.value.RenderedHTMLBytes = &number
	}
}

// observeCrawlerConversionInput records the exact serialized input and parsed text.
// Text counts exclude script/style nodes and whitespace-only nodes; they are not CSS visibility.
func observeCrawlerConversionInput(ctx context.Context, body *html.Node, size int) {
	observation := crawlerObservation(ctx)
	if observation == nil {
		return
	}
	var runes, nodes int64
	for node := body; node != nil; {
		skip := node.Type == html.ElementNode && (node.Data == "script" || node.Data == "style")
		if node.Type == html.TextNode {
			text := strings.TrimSpace(node.Data)
			if text != "" {
				runes += int64(utf8.RuneCountInString(text))
				nodes++
			}
		}
		if !skip && node.FirstChild != nil {
			node = node.FirstChild
			continue
		}
		for node != body && node.NextSibling == nil {
			node = node.Parent
		}
		if node == body {
			break
		}
		node = node.NextSibling
	}
	observation.mu.Lock()
	defer observation.mu.Unlock()
	bytes := int64(size)
	observation.value.ConversionInputBytes = &bytes
	observation.value.BodyTextRunes, observation.value.BodyTextNodes = &runes, &nodes
	observation.value.Converter = "html-to-markdown/v2"
}

// observeCrawlerConversionOutput records actual pre/post-trim byte counts, including zero.
func observeCrawlerConversionOutput(ctx context.Context, raw, trimmed string) {
	if observation := crawlerObservation(ctx); observation != nil {
		observation.mu.Lock()
		defer observation.mu.Unlock()
		before, after := int64(len(raw)), int64(len(trimmed))
		observation.value.ConversionOutputBytes, observation.value.ConversionOutputTrimmedBytes = &before, &after
	}
}

// crawlerContextError adds immutable diagnostics without changing the original error chain.
type crawlerContextError struct {
	cause error
	value *crawlerFailureContext
}

// Error delegates to the existing typed crawler failure text.
func (e *crawlerContextError) Error() string { return e.cause.Error() }

// Unwrap preserves the original code, status and lifecycle identities.
func (e *crawlerContextError) Unwrap() error { return e.cause }

// withCrawlerObservedFailure attaches one already-cloned bounded observation.
func withCrawlerObservedFailure(err error, value *crawlerFailureContext) error {
	if value == nil {
		return err
	}
	return errors.WithStack(&crawlerContextError{cause: err, value: cloneCrawlerContext(value)})
}

// crawlerFailureContextFrom returns a safe snapshot only for internal typed failures.
func crawlerFailureContextFrom(err error) *crawlerFailureContext {
	var observed *crawlerContextError
	if errors.As(err, &observed) {
		return cloneCrawlerContext(observed.value)
	}
	return nil
}

// crawlerFailureContextFields emits actual optional scalars without a log infrastructure filter.
func crawlerFailureContextFields(err error) []zap.Field {
	value := crawlerFailureContextFrom(err)
	if value == nil {
		return nil
	}
	raw, marshalErr := json.Marshal(value)
	if marshalErr != nil || len(raw) > crawlerFailureContextMaxBytes {
		return nil
	}
	var fields []zap.Field
	for _, item := range []struct{ key, value string }{{"target_scheme", value.TargetScheme}, {"target_hostname", value.TargetHostname},
		{"final_scheme", value.FinalScheme}, {"final_hostname", value.FinalHostname}, {"content_type", value.ContentType}, {"converter", value.Converter}} {
		if item.value != "" {
			fields = append(fields, zap.String(item.key, item.value))
		}
	}
	for _, item := range []struct {
		key   string
		value *int64
	}{{"upstream_status", value.UpstreamStatus}, {"redirect_count", value.RedirectCount},
		{"rendered_html_bytes", value.RenderedHTMLBytes}, {"conversion_input_bytes", value.ConversionInputBytes},
		{"conversion_output_bytes", value.ConversionOutputBytes}, {"conversion_output_trimmed_bytes", value.ConversionOutputTrimmedBytes},
		{"body_text_runes", value.BodyTextRunes}, {"body_text_nodes", value.BodyTextNodes}} {
		if item.value != nil {
			fields = append(fields, zap.Int64(item.key, *item.value))
		}
	}
	return fields
}
