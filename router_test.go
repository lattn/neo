package neo

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouterNotFound(t *testing.T) {
	r := New()
	h := func(c *Context) error {
		fmt.Fprint(c.Response, "ok")
		return nil
	}
	r.Get("/users", h)
	r.Post("/users", h)
	r.NotFound(MethodNotAllowedHandler, NotFoundHandler)

	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/users", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, "ok", res.Body.String(), "response body")
	assert.Equal(t, http.StatusOK, res.Code, "HTTP status code")

	res = httptest.NewRecorder()
	req, _ = http.NewRequest("PUT", "/users", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, "GET, POST, OPTIONS", res.Header().Get(HeaderAllow), "Allow header")
	assert.Equal(t, http.StatusMethodNotAllowed, res.Code, "HTTP status code")

	res = httptest.NewRecorder()
	req, _ = http.NewRequest("OPTIONS", "/users", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, "GET, POST, OPTIONS", res.Header().Get(HeaderAllow), "Allow header")
	assert.Equal(t, http.StatusOK, res.Code, "HTTP status code")

	res = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/users/", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, "", res.Header().Get(HeaderAllow), "Allow header")
	assert.Equal(t, http.StatusNotFound, res.Code, "HTTP status code")

	r.IgnoreTrailingSlash = true
	res = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/users/", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, "ok", res.Body.String(), "response body")
	assert.Equal(t, http.StatusOK, res.Code, "HTTP status code")

	res = httptest.NewRecorder()
	req, _ = http.NewRequest("PUT", "/users", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, "GET, POST, OPTIONS", res.Header().Get(HeaderAllow), "Allow header")
	assert.Equal(t, http.StatusMethodNotAllowed, res.Code, "HTTP status code")
}

func TestRouterUse(t *testing.T) {
	r := New()
	assert.Equal(t, 2, len(r.notFoundHandlers))
	r.Use(NotFoundHandler)
	assert.Equal(t, 3, len(r.notFoundHandlers))
}

func TestRouterRoute(t *testing.T) {
	r := New()
	r.Get("/users").Name("users")
	assert.NotNil(t, r.Route("users"))
	assert.Nil(t, r.Route("users2"))
}

func TestRouterAdd(t *testing.T) {
	r := New()
	assert.Equal(t, 0, r.maxParams)
	r.add("GET", "/users/<id>", nil)
	assert.Equal(t, 1, r.maxParams)
}

func TestRouterFindAllowedMethods(t *testing.T) {
	r := New()
	r.Post("/users", NotFoundHandler)
	r.Get("/users", NotFoundHandler)
	r.Patch("/users", NotFoundHandler)

	assert.Equal(t, []string{http.MethodGet, http.MethodPatch, http.MethodPost}, r.FindAllowedMethods("/users"))
	assert.Empty(t, r.FindAllowedMethods("/users/1"))
}

func TestRouterMethodNotAllowedHandlerPreservesRegisteredOptions(t *testing.T) {
	r := New()
	r.Get("/users", NotFoundHandler)
	r.Options("/users", NotFoundHandler)
	r.Post("/users", NotFoundHandler)

	res := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPut, "/users", nil)
	c := NewContext(res, req)
	c.router = r

	assert.Nil(t, MethodNotAllowedHandler(c))
	assert.Equal(t, "GET, OPTIONS, POST", res.Header().Get(HeaderAllow))
	assert.Equal(t, http.StatusMethodNotAllowed, res.Code)
}

func TestRouterMethodNotAllowedOnParamRoute(t *testing.T) {
	r := New()
	r.Get("/users/<id>", NotFoundHandler)
	r.Post("/users/<id>", NotFoundHandler)

	res := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/users/123", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, "GET, POST, OPTIONS", res.Header().Get(HeaderAllow), "Allow header")
	assert.Equal(t, http.StatusMethodNotAllowed, res.Code, "HTTP status code")
}

func TestRouterMethodNotAllowedOnWildcardRoute(t *testing.T) {
	r := New()
	r.Get("/files/*", NotFoundHandler)
	r.Post("/files/*", NotFoundHandler)

	res := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/files/a/b", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, "GET, POST, OPTIONS", res.Header().Get(HeaderAllow), "Allow header")
	assert.Equal(t, http.StatusMethodNotAllowed, res.Code, "HTTP status code")
}

func TestRouterNotFoundDoesNotSetAllowHeader(t *testing.T) {
	r := New()
	r.Get("/users/<id>", NotFoundHandler)

	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/users/123/profile", nil)
	r.ServeHTTP(res, req)
	assert.Equal(t, "", res.Header().Get(HeaderAllow), "Allow header")
	assert.Equal(t, http.StatusNotFound, res.Code, "HTTP status code")
}

func TestRouterUseEscapedPathPlusSign(t *testing.T) {
	r := New()
	r.UseEscapedPath = true

	var value string
	r.Get("/files/<name>", func(c *Context) error {
		value = c.Param("name")
		return nil
	})

	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/files/a+b", nil)
	r.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	assert.Equal(t, "a+b", value)
}

func TestRouterUseEscapedPathSlash(t *testing.T) {
	r := New()
	r.UseEscapedPath = true

	var value string
	r.Get("/files/<name>", func(c *Context) error {
		value = c.Param("name")
		return nil
	})

	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/files/a%2Fb", nil)
	r.ServeHTTP(res, req)

	assert.Equal(t, http.StatusOK, res.Code)
	assert.Equal(t, "a/b", value)
}

func TestRouterNormalizeRequestPath(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/", "/"},
		{"/users", "/users"},
		{"/users/", "/users"},
		{"/users//", "/users"},
		{"///", "/"},
	}
	r := New()
	r.IgnoreTrailingSlash = true
	for _, test := range tests {
		result := r.normalizeRequestPath(test.path)
		assert.Equal(t, test.expected, result)
	}
}

func TestRouterHandleError(t *testing.T) {
	r := New()
	res := httptest.NewRecorder()
	c := &Context{Response: res}
	r.handleError(c, errors.New("abc"))
	assert.Equal(t, http.StatusInternalServerError, res.Code)

	res = httptest.NewRecorder()
	c = &Context{Response: res}
	r.handleError(c, NewHTTPError(http.StatusNotFound))
	assert.Equal(t, http.StatusNotFound, res.Code)
}

func TestRouterHandleErrorDefaultNotFound(t *testing.T) {
	r := New()
	res := httptest.NewRecorder()
	res.Header().Set(HeaderContentLength, "123")
	c := &Context{Response: res}

	r.handleError(c, defaultNotFoundHTTPError)

	assert.Equal(t, http.StatusNotFound, res.Code)
	assert.Equal(t, "Not Found\n", res.Body.String())
	assert.Equal(t, MIMETextPlainCharsetUTF8, res.Header().Get(HeaderContentType))
	assert.Equal(t, "nosniff", res.Header().Get(HeaderXContentTypeOptions))
	assert.Equal(t, "", res.Header().Get(HeaderContentLength))
}

func TestRouterHandleErrorCustomNotFound(t *testing.T) {
	r := New()
	res := httptest.NewRecorder()
	c := &Context{Response: res}

	r.handleError(c, NewHTTPError(http.StatusNotFound, "custom not found"))

	assert.Equal(t, http.StatusNotFound, res.Code)
	assert.Equal(t, "custom not found\n", res.Body.String())
}

func TestHTTPHandler(t *testing.T) {
	res := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/users/", nil)
	c := NewContext(res, req)

	h1 := HTTPHandlerFunc(http.NotFound)
	assert.Nil(t, h1(c))
	assert.Equal(t, http.StatusNotFound, res.Code)

	res = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/users/", nil)
	c = NewContext(res, req)
	h2 := HTTPHandler(http.NotFoundHandler())
	assert.Nil(t, h2(c))
	assert.Equal(t, http.StatusNotFound, res.Code)
}
