// Package middleware provides HTTP middleware components for fasthttp.
//
// Each middleware lives in its own sub-package and exposes a New() function
// (and Config type where applicable):
//
//	import (
//	    "github.com/oarkflow/fasthttp/middleware/logger"
//	    "github.com/oarkflow/fasthttp/middleware/recover"
//	    "github.com/oarkflow/fasthttp/middleware/cors"
//	    "github.com/oarkflow/fasthttp/middleware/requestid"
//	    "github.com/oarkflow/fasthttp/middleware/ratelimiter"
//	    "github.com/oarkflow/fasthttp/middleware/compress"
//	    "github.com/oarkflow/fasthttp/middleware/security"
//	    "github.com/oarkflow/fasthttp/middleware/basicauth"
//	    "github.com/oarkflow/fasthttp/middleware/ipwhitelist"
//	    "github.com/oarkflow/fasthttp/middleware/timeout"
//	)
package middleware
