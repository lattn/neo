package neo

import (
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

type benchmarkResponseWriter struct {
	header http.Header
	code   int
}

func newBenchmarkResponseWriter() *benchmarkResponseWriter {
	return &benchmarkResponseWriter{
		header: make(http.Header),
	}
}

func (w *benchmarkResponseWriter) Header() http.Header {
	return w.header
}

func (w *benchmarkResponseWriter) WriteHeader(statusCode int) {
	w.code = statusCode
}

func (w *benchmarkResponseWriter) Write(p []byte) (int, error) {
	return len(p), nil
}

func (w *benchmarkResponseWriter) Flush() {}

func (w *benchmarkResponseWriter) Reset() {
	for k := range w.header {
		delete(w.header, k)
	}
	w.code = 0
}

func benchmarkHandler(*Context) error {
	return nil
}

func newBenchmarkRouter() *Router {
	r := New()
	r.Get("/healthz", benchmarkHandler)
	r.Get("/metrics", benchmarkHandler)
	r.Get("/users", benchmarkHandler)
	r.Get("/users/<id>", benchmarkHandler)
	r.Get("/users/<id>/profile", benchmarkHandler)
	r.Get("/users/<id>/posts/<postID>", benchmarkHandler)
	r.Get("/assets/<name:[^/]+\\.(css|js|png)>", benchmarkHandler)
	r.Get("/files/<path:.*>", benchmarkHandler)
	r.Post("/users", benchmarkHandler)
	r.Post("/users/<id>", benchmarkHandler)
	r.Put("/users/<id>", benchmarkHandler)
	r.Delete("/users/<id>", benchmarkHandler)
	return r
}

func benchmarkServeHTTP(b *testing.B, r *Router, method, target string) {
	b.Helper()
	b.ReportAllocs()

	req, err := http.NewRequest(method, target, nil)
	if err != nil {
		b.Fatalf("new request: %v", err)
	}
	res := newBenchmarkResponseWriter()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res.Reset()
		r.ServeHTTP(res, req)
	}
}

func BenchmarkRouterServeHTTPStatic(b *testing.B) {
	r := newBenchmarkRouter()
	benchmarkServeHTTP(b, r, http.MethodGet, "/healthz")
}

func BenchmarkRouterServeHTTPParam(b *testing.B) {
	r := newBenchmarkRouter()
	benchmarkServeHTTP(b, r, http.MethodGet, "/users/123/posts/456")
}

func BenchmarkRouterServeHTTPRegex(b *testing.B) {
	r := newBenchmarkRouter()
	benchmarkServeHTTP(b, r, http.MethodGet, "/assets/app.css")
}

func BenchmarkRouterServeHTTPNotFound(b *testing.B) {
	r := newBenchmarkRouter()
	benchmarkServeHTTP(b, r, http.MethodGet, "/does-not-exist")
}

func BenchmarkRouterServeHTTPMethodNotAllowed(b *testing.B) {
	r := newBenchmarkRouter()
	benchmarkServeHTTP(b, r, http.MethodPatch, "/users/123")
}

func BenchmarkRouterServeHTTPMethodNotAllowedParamOnly(b *testing.B) {
	r := New()
	r.Get("/users/<id>", benchmarkHandler)
	r.Post("/users/<id>", benchmarkHandler)
	benchmarkServeHTTP(b, r, http.MethodPut, "/users/123")
}

func BenchmarkRouterServeHTTPStaticVsParamSibling(b *testing.B) {
	r := New()
	r.Get("/users/profile", benchmarkHandler)
	r.Get("/users/<id>", benchmarkHandler)
	benchmarkServeHTTP(b, r, http.MethodGet, "/users/profile")
}

func BenchmarkRouterServeHTTPNotFoundNearMiss(b *testing.B) {
	r := newBenchmarkRouter()
	benchmarkServeHTTP(b, r, http.MethodGet, "/users/123/profile/extra")
}

func BenchmarkRouterServeHTTPEscapedPath(b *testing.B) {
	r := newBenchmarkRouter()
	r.UseEscapedPath = true
	benchmarkServeHTTP(b, r, http.MethodGet, "/files/a%2Fb%2Fc")
}

func BenchmarkContextQueryRepeated(b *testing.B) {
	req, err := http.NewRequest(http.MethodGet, "http://example.com/search?q=foo&q=bar&sort=desc&page=2", nil)
	if err != nil {
		b.Fatalf("new request: %v", err)
	}
	c := NewContext(newBenchmarkResponseWriter(), req)
	b.ReportAllocs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Query("q")
		_ = c.Query("sort")
		_ = c.Query("page")
	}
}

func BenchmarkContextFormURLEncoded(b *testing.B) {
	body := "z=post&both=y&prio=2&empty="
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req, err := http.NewRequest(http.MethodPost, "http://example.com/search?q=foo&both=x", strings.NewReader(body))
		if err != nil {
			b.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		c := NewContext(newBenchmarkResponseWriter(), req)
		_ = c.PostForm("z")
		_ = c.Form("both")
	}
}

func BenchmarkContextFormMultipart(b *testing.B) {
	var body strings.Builder
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("z", "post"); err != nil {
		b.Fatalf("write field z: %v", err)
	}
	if err := writer.WriteField("both", "y"); err != nil {
		b.Fatalf("write field both: %v", err)
	}
	if err := writer.Close(); err != nil {
		b.Fatalf("close writer: %v", err)
	}
	contentType := writer.FormDataContentType()
	payload := body.String()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		req, err := http.NewRequest(http.MethodPost, "http://example.com/search?q=foo&both=x", strings.NewReader(payload))
		if err != nil {
			b.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", contentType)
		c := NewContext(newBenchmarkResponseWriter(), req)
		_ = c.PostForm("z")
		_ = c.Form("both")
	}
}
