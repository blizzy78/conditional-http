package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/matryer/is"
)

func TestETag_String(t *testing.T) {
	tests := []struct {
		eTag     ETag
		wantETag string
	}{
		{
			eTag: ETag{
				Tag: "foo",
			},
			wantETag: `"foo"`,
		},
		{
			eTag: ETag{
				Tag:  "bar",
				Weak: true,
			},
			wantETag: `W/"bar"`,
		},
	}

	for _, test := range tests {
		t.Run(test.wantETag, func(t *testing.T) {
			is := is.New(t)
			is.Equal(test.eTag.String(), test.wantETag)
		})
	}
}

func TestETag_Compare(t *testing.T) {
	tests := []struct {
		name           string
		e1             ETag
		e2             ETag
		weakComparison bool
		wantResult     bool
	}{
		{
			name:       "weak vs weak (strong comparison)",
			e1:         ETag{Tag: "1", Weak: true},
			e2:         ETag{Tag: "1", Weak: true},
			wantResult: false,
		},
		{
			name:       "weak1 vs weak2 (strong comparison)",
			e1:         ETag{Tag: "1", Weak: true},
			e2:         ETag{Tag: "2", Weak: true},
			wantResult: false,
		},
		{
			name:       "weak vs strong (strong comparison)",
			e1:         ETag{Tag: "1", Weak: true},
			e2:         ETag{Tag: "1", Weak: false},
			wantResult: false,
		},
		{
			name:       "strong vs strong (strong comparison)",
			e1:         ETag{Tag: "1", Weak: false},
			e2:         ETag{Tag: "1", Weak: false},
			wantResult: true,
		},
		{
			name:           "weak vs weak (weak comparison)",
			e1:             ETag{Tag: "1", Weak: true},
			e2:             ETag{Tag: "1", Weak: true},
			weakComparison: true,
			wantResult:     true,
		},
		{
			name:           "weak1 vs weak2 (weak comparison)",
			e1:             ETag{Tag: "1", Weak: true},
			e2:             ETag{Tag: "2", Weak: true},
			weakComparison: true,
			wantResult:     false,
		},
		{
			name:           "weak vs strong (weak comparison)",
			e1:             ETag{Tag: "1", Weak: true},
			e2:             ETag{Tag: "1", Weak: false},
			weakComparison: true,
			wantResult:     true,
		},
		{
			name:           "strong vs strong (strong comparison)",
			e1:             ETag{Tag: "1", Weak: false},
			e2:             ETag{Tag: "1", Weak: false},
			weakComparison: true,
			wantResult:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			is := is.New(t)
			is.Equal(test.e1.equal(test.e2, test.weakComparison), test.wantResult)
		})
	}
}

func TestETagFromString(t *testing.T) {
	tests := []struct {
		s        string
		wantOK   bool
		wantTag  string
		wantWeak bool
	}{
		{
			s:        `"foo"`,
			wantOK:   true,
			wantTag:  "foo",
			wantWeak: false,
		},
		{
			s:        `W/"foo"`,
			wantOK:   true,
			wantTag:  "foo",
			wantWeak: true,
		},
		{
			s:      "bad",
			wantOK: false,
		},
	}

	for _, test := range tests {
		t.Run(test.s, func(t *testing.T) {
			is := is.New(t)
			e, ok := eTagFromString(test.s)
			is.Equal(ok, test.wantOK)
			if ok {
				is.Equal(e.Tag, test.wantTag)
				is.Equal(e.Weak, test.wantWeak)
			}
		})
	}
}

func TestETagHandler(t *testing.T) {
	is := is.New(t)

	etag := ETag{
		Tag: "foo",
	}
	f := func(w http.ResponseWriter, r *http.Request) (ETag, bool) {
		return etag, true
	}
	h := ETagHandler(f, BeforeHeaders, contentHandler([]byte{}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	h.ServeHTTP(w, r)

	is.Equal(w.Result().StatusCode, http.StatusOK)
	is.Equal(w.Result().Header.Get("ETag"), etag.String())
}

func TestETagHandler_NotOK(t *testing.T) {
	is := is.New(t)

	f := func(w http.ResponseWriter, r *http.Request) (ETag, bool) {
		return ETag{}, false
	}
	h := ETagHandler(f, BeforeHeaders, contentHandler([]byte{}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	h.ServeHTTP(w, r)

	is.Equal(w.Result().StatusCode, http.StatusOK)
	is.Equal(w.Result().Header.Get("ETag"), "")
}

func TestLastModifiedHandler(t *testing.T) {
	is := is.New(t)

	now := time.Now()
	f := func(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
		return now, true
	}
	h, _ := LastModifiedHandler(f, BeforeHeaders, contentHandler([]byte{}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	h.ServeHTTP(w, r)

	is.Equal(w.Result().StatusCode, http.StatusOK)
	loc, _ := time.LoadLocation("GMT")
	is.Equal(w.Result().Header.Get("Last-Modified"), now.In(loc).Format(time.RFC1123))
}

func TestLastModifiedHandler_NotOK(t *testing.T) {
	is := is.New(t)

	f := func(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
		return time.Time{}, false
	}
	h, _ := LastModifiedHandler(f, BeforeHeaders, contentHandler([]byte{}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	h.ServeHTTP(w, r)

	is.Equal(w.Result().StatusCode, http.StatusOK)
	is.Equal(w.Result().Header.Get("Last-Modified"), "")
}

func TestLastModifiedHandlerConstant(t *testing.T) {
	is := is.New(t)

	now := time.Now()
	h, _ := LastModifiedHandlerConstant(now, contentHandler([]byte{}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	h.ServeHTTP(w, r)

	is.Equal(w.Result().StatusCode, http.StatusOK)
	loc, _ := time.LoadLocation("GMT")
	is.Equal(w.Result().Header.Get("Last-Modified"), now.In(loc).Format(time.RFC1123))
}

func TestIfNoneMatchIfModifiedSinceHandler_NoHeaders(t *testing.T) {
	is := is.New(t)

	h := IfNoneMatchIfModifiedSinceHandler(true, contentHandler([]byte{}, "ETag", `"foo"`))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	h.ServeHTTP(w, r)

	is.Equal(w.Result().StatusCode, http.StatusOK)
}

func TestIfNoneMatchIfModifiedSinceHandler_IfNoneMatch(t *testing.T) {
	tests := []struct {
		ifNoneMatchTag string
		wantStatus     int
	}{
		{
			ifNoneMatchTag: "foo",
			wantStatus:     http.StatusNotModified,
		},
		{
			ifNoneMatchTag: "bar",
			wantStatus:     http.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.ifNoneMatchTag, func(t *testing.T) {
			is := is.New(t)

			h := IfNoneMatchIfModifiedSinceHandler(true, contentHandler([]byte{}, "ETag", ETag{Tag: "foo"}.String()))
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			eTag := ETag{
				Tag: test.ifNoneMatchTag,
			}
			r.Header.Set("If-None-Match", eTag.String())

			h.ServeHTTP(w, r)

			is.Equal(w.Result().StatusCode, test.wantStatus)
		})
	}
}

func TestIfNoneMatchIfModifiedSinceHandler_IfNoneMatch_NoETag(t *testing.T) {
	is := is.New(t)

	h := IfNoneMatchIfModifiedSinceHandler(true, contentHandler([]byte{}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("If-None-Match", `"foo"`)

	h.ServeHTTP(w, r)

	is.Equal(w.Result().StatusCode, http.StatusOK)
}

func TestIfNoneMatchIfModifiedSinceHandler_IfNoneMatch_RequestParseError(t *testing.T) {
	is := is.New(t)

	h := IfNoneMatchIfModifiedSinceHandler(true, contentHandler([]byte{}, "ETag", ETag{Tag: "foo"}.String()))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("If-None-Match", "bad")

	h.ServeHTTP(w, r)

	is.Equal(w.Result().StatusCode, http.StatusOK)
}

func TestIfNoneMatchIfModifiedSinceHandler_IfNoneMatch_ResponseParseError(t *testing.T) {
	is := is.New(t)

	h := IfNoneMatchIfModifiedSinceHandler(true, contentHandler([]byte{}, "ETag", "bad"))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("If-None-Match", ETag{Tag: "foo"}.String())

	h.ServeHTTP(w, r)

	is.Equal(w.Result().StatusCode, http.StatusOK)
}

func TestIfNoneMatchIfModifiedSinceHandler_IfModifiedSince(t *testing.T) {
	lastModifiedTime := time.Now()

	tests := []struct {
		name                string
		ifModifiedSinceTime time.Time
		wantStatus          int
	}{
		{
			name:                "modified",
			ifModifiedSinceTime: lastModifiedTime.Add(-10 * time.Minute),
			wantStatus:          http.StatusOK,
		},
		{
			name:                "same date",
			ifModifiedSinceTime: lastModifiedTime,
			wantStatus:          http.StatusNotModified,
		},
		{
			name:                "not modified",
			ifModifiedSinceTime: lastModifiedTime.Add(10 * time.Minute),
			wantStatus:          http.StatusNotModified,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			is := is.New(t)

			loc, _ := time.LoadLocation("GMT")
			h := IfNoneMatchIfModifiedSinceHandler(true, contentHandler([]byte{}, "Last-Modified", lastModifiedTime.In(loc).Format(time.RFC1123)))
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Header.Set("If-Modified-Since", test.ifModifiedSinceTime.In(loc).Format(time.RFC1123))

			h.ServeHTTP(w, r)

			is.Equal(w.Result().StatusCode, test.wantStatus)
		})
	}
}

func TestIfNoneMatchIfModifiedSinceHandler_IfModifiedSince_NoLastModified(t *testing.T) {
	is := is.New(t)

	h := IfNoneMatchIfModifiedSinceHandler(true, contentHandler([]byte{}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	loc, _ := time.LoadLocation("GMT")
	r.Header.Set("If-Modified-Since", time.Now().In(loc).Format(time.RFC1123))

	h.ServeHTTP(w, r)

	is.Equal(w.Result().StatusCode, http.StatusOK)
}

func TestIfNoneMatchIfModifiedSinceHandler_IfModifiedSince_RequestParseError(t *testing.T) {
	is := is.New(t)

	loc, _ := time.LoadLocation("GMT")
	h := IfNoneMatchIfModifiedSinceHandler(true, contentHandler([]byte{}, "Last-Modified", time.Now().In(loc).Format(time.RFC1123)))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("If-Modified-Since", "bad")

	h.ServeHTTP(w, r)

	is.Equal(w.Result().StatusCode, http.StatusOK)
}

func TestIfNoneMatchIfModifiedSinceHandler_IfModifiedSince_ResponseParseError(t *testing.T) {
	is := is.New(t)

	h := IfNoneMatchIfModifiedSinceHandler(true, contentHandler([]byte{}, "Last-Modified", "bad"))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	loc, _ := time.LoadLocation("GMT")
	r.Header.Set("If-Modified-Since", time.Now().In(loc).Format(time.RFC1123))

	h.ServeHTTP(w, r)

	is.Equal(w.Result().StatusCode, http.StatusOK)
}

func TestHeaderHandler_BeforeHeaders(t *testing.T) {
	is := is.New(t)

	fCalled := false
	f := func(_ http.ResponseWriter, r *http.Request, statusCode int) int {
		fCalled = true
		return statusCode
	}
	body := []byte("body")
	h := headerHandler(f, BeforeHeaders, contentHandler(body, "X-Test", "testValue"))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	h.ServeHTTP(w, r)

	is.True(fCalled)
	is.Equal(w.Result().StatusCode, http.StatusOK)
	b, _ := io.ReadAll(w.Result().Body)
	is.Equal(b, body)
}

func TestHeaderHandler_AfterHeaders(t *testing.T) {
	is := is.New(t)

	fCalled := false
	var headerValue string
	var bodyContent []byte
	f := func(w http.ResponseWriter, r *http.Request, statusCode int) int {
		fCalled = true
		headerValue = w.Header().Get("X-Test")
		bodyContent = Body(w)
		return statusCode
	}
	body := []byte("body")
	h := headerHandler(f, AfterHeaders, contentHandler(body, "X-Test", "testValue"))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	h.ServeHTTP(w, r)

	is.True(fCalled)
	is.Equal(w.Result().StatusCode, http.StatusOK)
	is.Equal(headerValue, "testValue")
	is.True(bodyContent == nil)
	b, _ := io.ReadAll(w.Result().Body)
	is.Equal(b, body)
}

func TestHeaderHandler_AfterHeaders_NoContent(t *testing.T) {
	is := is.New(t)

	fCalled := false
	var bodyContent []byte
	f := func(w http.ResponseWriter, r *http.Request, statusCode int) int {
		fCalled = true
		bodyContent = Body(w)
		return statusCode
	}
	h := headerHandler(f, AfterHeaders, noContentHandler())
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	h.ServeHTTP(w, r)

	is.True(fCalled)
	is.Equal(w.Result().StatusCode, http.StatusNoContent)
	is.True(bodyContent == nil)
}

func TestHeaderHandler_AfterHeaders_ChangeStatus(t *testing.T) {
	is := is.New(t)

	fCalled := false
	var headerValue string
	var bodyContent []byte
	f := func(w http.ResponseWriter, r *http.Request, statusCode int) int {
		fCalled = true
		headerValue = w.Header().Get("X-Test")
		bodyContent = Body(w)
		return http.StatusCreated
	}
	body := []byte("body")
	h := headerHandler(f, AfterHeaders, contentHandler(body, "X-Test", "testValue"))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	h.ServeHTTP(w, r)

	is.True(fCalled)
	is.Equal(w.Result().StatusCode, http.StatusCreated)
	is.Equal(headerValue, "testValue")
	is.True(bodyContent == nil)
	b, _ := io.ReadAll(w.Result().Body)
	is.Equal(b, body)
}

func TestHeaderHandler_AfterResponse(t *testing.T) {
	is := is.New(t)

	fCalled := false
	var headerValue string
	var bodyContent []byte
	f := func(w http.ResponseWriter, r *http.Request, statusCode int) int {
		fCalled = true
		headerValue = w.Header().Get("X-Test")
		bodyContent = Body(w)
		return statusCode
	}
	body := []byte("body")
	h := headerHandler(f, AfterResponse, contentHandler(body, "X-Test", "testValue"))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	h.ServeHTTP(w, r)

	is.True(fCalled)
	is.Equal(w.Result().StatusCode, http.StatusOK)
	is.Equal(headerValue, "testValue")
	is.Equal(bodyContent, body)
	b, _ := io.ReadAll(w.Result().Body)
	is.Equal(b, body)
}

func TestHeaderHandler_AfterResponse_ChangeStatus(t *testing.T) {
	is := is.New(t)

	fCalled := false
	var headerValue string
	var bodyContent []byte
	f := func(w http.ResponseWriter, r *http.Request, statusCode int) int {
		fCalled = true
		headerValue = w.Header().Get("X-Test")
		bodyContent = Body(w)
		return http.StatusCreated
	}
	body := []byte("body")
	h := headerHandler(f, AfterResponse, contentHandler(body, "X-Test", "testValue"))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	h.ServeHTTP(w, r)

	is.True(fCalled)
	is.Equal(w.Result().StatusCode, http.StatusCreated)
	is.Equal(headerValue, "testValue")
	is.Equal(bodyContent, body)
	b, _ := io.ReadAll(w.Result().Body)
	is.Equal(b, body)
}

func contentHandler(b []byte, headerKV ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < len(headerKV); i += 2 {
			w.Header().Set(headerKV[i], headerKV[i+1])
		}
		_, _ = w.Write(b)
	})
}

func noContentHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "No Content", http.StatusNoContent)
	})
}
