// Package middleware provides HTTP middleware components for fasthttp.
//
// Each middleware lives in its own sub-package and exposes a New() function
// (and Config type where applicable):
//
//	import (
//	    "github.com/orgware/fasthttp/middleware/logger"
//	    "github.com/orgware/fasthttp/middleware/recover"
//	    "github.com/orgware/fasthttp/middleware/cors"
//	    "github.com/orgware/fasthttp/middleware/requestid"
//	    "github.com/orgware/fasthttp/middleware/ratelimiter"
//	    "github.com/orgware/fasthttp/middleware/compress"
//	    "github.com/orgware/fasthttp/middleware/security"
//	    "github.com/orgware/fasthttp/middleware/basicauth"
//	    "github.com/orgware/fasthttp/middleware/ipwhitelist"
//	    "github.com/orgware/fasthttp/middleware/timeout"
//	)
package middleware
