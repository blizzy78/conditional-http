package handler

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"time"
)

// ETag represents an entity-tag as specified by RFC 7232, section 2.
type ETag struct {
	// Tag is the entity-tag's opaque-tag. The double-quotes required by RFC 7232 should be omitted.
	Tag string

	// Weak specifies if this is a weak entity-tag.
	Weak bool
}

// ETagFunc returns an entity-tag for w, which is r's response.
// If the response mode in use is BeforeHeaders, w will be nil.
// If the response mode in use is AfterResponse, w's body can be obtained using Body.
// If the function cannot produce an entity-tag, it returns ok==false.
type ETagFunc func(w http.ResponseWriter, r *http.Request) (ETag, bool)

// LastModifiedFunc returns the last modification date for w, which is r's response.
// If the response mode in use is BeforeHeaders, w will be nil.
// If the response mode in use is AfterResponse, w's body can be obtained using Body.
// If the function cannot produce a last modification date, it returns ok==false.
type LastModifiedFunc func(w http.ResponseWriter, r *http.Request) (time.Time, bool)

// ResponseMode determines the amount of response data available when calling ETagFunc or LastModifiedFunc.
type ResponseMode int

const (
	// BeforeHeaders is the response mode used to call functions before any response data has been produced.
	BeforeHeaders = ResponseMode(iota)

	// AfterHeaders is the response mode used to call functions after response headers have been produced,
	// but before the body is sent.
	AfterHeaders

	// AfterResponse is the response mode used to call functions after both response headers and body have
	// been produced.
	//
	// Note that using AfterResponse will cause handlers returned by this package to buffer the response produced
	// by a downstream handler entirely in memory, which may not be desirable.
	AfterResponse
)

type responseWriter struct {
	w                 http.ResponseWriter
	r                 *http.Request
	statusCode        int
	bodyBuf           *bytes.Buffer
	beforeWriteHeader beforeWriteHeaderFunc
	bufferBody        bool
	headerWritten     bool
}

type beforeWriteHeaderFunc func(int) int

type headerFunc func(http.ResponseWriter, *http.Request, int) int

// ETagHandler returns a handler that uses f to set the ETag header in responses.
// If rm is BeforeHeaders, the response passed to f will be nil.
// If rm is AfterHeaders, the response passed to f will contain the headers set by next.
// If rm is AfterResponse, the response passed to f will contain both headers and body produced by next.
// If f cannot produce an entity-tag (ok result is false), then the ETag header will not be set.
func ETagHandler(f ETagFunc, rm ResponseMode, next http.Handler) http.Handler {
	return headerHandler(
		func(w http.ResponseWriter, r *http.Request, statusCode int) int {
			e, ok := f(w, r)
			if !ok {
				return statusCode
			}
			w.Header().Set("ETag", e.String())
			return statusCode
		},
		rm, next)
}

// LastModifiedHandler returns a handler that uses f to set the Last-Modified header in responses.
// If rm is BeforeHeaders, the response passed to f will be nil.
// If rm is AfterHeaders, the response passed to f will contain the headers set by next.
// If rm is AfterResponse, the response passed to f will contain both headers and body produced by next.
// If f cannot produce a last modification date (ok result is false), then the Last-Modification header
// will not be set.
func LastModifiedHandler(f LastModifiedFunc, rm ResponseMode, next http.Handler) (http.Handler, error) {
	loc, err := time.LoadLocation("GMT")
	if err != nil {
		return nil, err
	}

	return headerHandler(
		func(w http.ResponseWriter, r *http.Request, statusCode int) int {
			lm, ok := f(w, r)
			if !ok {
				return statusCode
			}
			w.Header().Set("Last-Modified", lm.In(loc).Format(time.RFC1123))
			return statusCode
		},
		rm, next), nil
}

// LastModifiedHandlerConstant returns a handler that sets the Last-Modification header in responses to t.
func LastModifiedHandlerConstant(t time.Time, next http.Handler) (http.Handler, error) {
	loc, err := time.LoadLocation("GMT")
	if err != nil {
		return nil, err
	}

	ts := t.In(loc).Format(time.RFC1123)

	return headerHandler(
		func(w http.ResponseWriter, r *http.Request, statusCode int) int {
			w.Header().Set("Last-Modified", ts)
			return statusCode
		},
		BeforeHeaders, next), nil
}

// IfNoneMatchIfModifiedSinceHandler returns a handler that returns the 304 Not Modified status code
// in responses if either the entity-tag in the request's If-None-Match header matches the entity-tag
// of the response's ETag header, or if the response's Last-Modified header is later than the request's
// If-Modified-Since header.
//
// If the request contains an If-None-Match header, the request's If-Modified-Since header is ignored,
// in accordance with RFC 7232, section 3.3.
// If weakETagComparison==true, entity-tags are compared weakly.
// If neither entity-tags nor last modification date checks are successful, the response will not be modified.
func IfNoneMatchIfModifiedSinceHandler(weakETagComparison bool, next http.Handler) http.Handler {
	return headerHandler(
		func(w http.ResponseWriter, r *http.Request, statusCode int) int {
			if statusCode, ok := tryMatchETag(w, r, weakETagComparison, statusCode); ok {
				return statusCode
			}
			return tryMatchLastModified(w, r, statusCode)
		},
		AfterHeaders, next)
}

func tryMatchETag(w http.ResponseWriter, r *http.Request, weakETagComparison bool, statusCode int) (int, bool) {
	inm := r.Header.Get("If-None-Match")
	if inm == "" {
		return 0, false
	}

	eTag := w.Header().Get("ETag")
	if eTag == "" {
		return statusCode, true
	}

	inmE, ok := eTagFromString(inm)
	if !ok {
		return statusCode, true
	}

	e, ok := eTagFromString(eTag)
	if !ok {
		return statusCode, true
	}

	if inmE.equal(e, weakETagComparison) {
		return http.StatusNotModified, true
	}

	return statusCode, true
}

func tryMatchLastModified(w http.ResponseWriter, r *http.Request, statusCode int) int {
	ims := r.Header.Get("If-Modified-Since")
	lm := w.Header().Get("Last-Modified")
	switch {
	case ims == "", lm == "":
		return statusCode
	case ims == lm:
		return http.StatusNotModified
	}

	imsT, err := time.Parse(time.RFC1123, ims)
	if err != nil {
		return statusCode
	}

	lmT, err := time.Parse(time.RFC1123, lm)
	if err != nil {
		return statusCode
	}

	if lmT.Before(imsT) || lmT.Equal(imsT) {
		return http.StatusNotModified
	}

	return statusCode
}

func headerHandler(f headerFunc, rm ResponseMode, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch rm {
		case BeforeHeaders:
			f(w, r, 0)
			next.ServeHTTP(w, r)

		case AfterHeaders, AfterResponse:
			var rw *responseWriter
			rw = &responseWriter{
				w:          w,
				r:          r,
				bufferBody: rm == AfterResponse,
				beforeWriteHeader: func(statusCode int) int {
					return f(rw, r, statusCode)
				},
			}
			next.ServeHTTP(rw, r)
			rw.flush()
		}
	})
}

// Header implements http.Handler.
func (w *responseWriter) Header() http.Header {
	return w.w.Header()
}

// Header implements http.Handler.
func (w *responseWriter) Write(b []byte) (int, error) {
	if w.bufferBody {
		if w.bodyBuf == nil {
			w.bodyBuf = &bytes.Buffer{}
		}
		return w.bodyBuf.Write(b)
	}

	w.writeHeader()
	return w.w.Write(b)
}

// Header implements http.Handler.
func (w *responseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

func (w *responseWriter) flush() {
	if w.bodyBuf == nil {
		return
	}
	w.writeHeader()
	_, _ = io.Copy(w.w, w.bodyBuf)
}

func (w *responseWriter) writeHeader() {
	if w.headerWritten {
		return
	}

	statusCode := w.statusCode
	if statusCode < 100 {
		statusCode = http.StatusOK
	}

	if w.beforeWriteHeader != nil {
		defer func() {
			w.beforeWriteHeader = nil
		}()
		statusCode = w.beforeWriteHeader(statusCode)
	}

	defer func() {
		w.headerWritten = true
	}()
	w.w.WriteHeader(statusCode)
}

// Body returns w's body content. If w is a buffering response writer produced by this package,
// Body will return the buffered body contents if any. In all other cases, it will return nil.
func Body(w http.ResponseWriter) []byte {
	rw, ok := w.(*responseWriter)
	if !ok || rw.bodyBuf == nil {
		return nil
	}
	return rw.bodyBuf.Bytes()
}

func eTagFromString(s string) (ETag, bool) {
	weak := false
	if strings.HasPrefix(s, "W/") {
		weak = true
		s = s[2:]
	}

	if !strings.HasPrefix(s, `"`) || !strings.HasSuffix(s, `"`) {
		return ETag{}, false
	}

	return ETag{
		Tag:  s[1 : len(s)-1],
		Weak: weak,
	}, true
}

// String implements fmt.Stringer.
func (e ETag) String() string {
	s := e.Tag
	if !strings.HasPrefix(s, `"`) && !strings.HasSuffix(s, `"`) {
		s = `"` + s + `"`
	}
	if e.Weak {
		s = "W/" + s
	}
	return s
}

func (e ETag) equal(e2 ETag, weakComparison bool) bool {
	if !weakComparison && (e.Weak || e2.Weak) {
		return false
	}
	return e.Tag == e2.Tag
}
