[![Build Status](https://travis-ci.org/blizzy78/conditional-http.svg?branch=master)](https://travis-ci.org/blizzy78/conditional-http) [![Coverage Status](https://coveralls.io/repos/github/blizzy78/conditional-http/badge.svg?branch=master)](https://coveralls.io/github/blizzy78/conditional-http?branch=master) [![GoDoc](https://pkg.go.dev/badge/github.com/blizzy78/conditional-http)](https://pkg.go.dev/github.com/blizzy78/conditional-http)


Conditional HTTP Middleware for Go
==================================

This package for Go provides middleware for conditional HTTP requests supporting the ETag, Last-Modified,
If-Modified-Since, and If-None-Match headers, according to [RFC 7232]. When matches are successful,
it will automatically send the 304 Not Modified status code.


Usage
-----

```go
import "github.com/blizzy78/conditional-http/handler"
```

```go
// your regular downstream handler
var h http.Handler = ...

// add Last-Modified header to responses
h, _ = handler.LastModifiedHandler(
	func(w http.ResponseWriter, r *http.Request) (time.Time, bool) {
		// produce last modification date for r and w
		lastMod := ...
		return lastMod, true
	},
	handler.BeforeHeaders, h)

// add ETag header to responses
h = handler.ETagHandler(
	func(w http.ResponseWriter, r *http.Request) (handler.ETag, bool) {
		// produce entity-tag for r and w
		eTag := handler.ETag{
			Tag: "...",
			Weak: true,
		}
		return eTag, true
	},
	handler.AfterHeaders, h)

// check requests for If-Modified-Since and If-None-Match headers,
// and send 304 Not Modified in responses if successful
h = handler.IfNoneMatchIfModifiedSinceHandler(true, h)
```


License
-------

This package is licensed under the MIT license.



[RFC 7232]: https://www.rfc-editor.org/rfc/rfc7232.txt
