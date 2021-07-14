package tracing

import (
	"errors"
	"net/http"
	"runtime"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
)

func ExtractFromHTTPRequest(req *http.Request, handlerName string) (opentracing.Span, *http.Request) {
	spanContext, err := opentracing.GlobalTracer().Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(req.Header))
	if err != nil {
		span, ctx := opentracing.StartSpanFromContext(req.Context(), "request")
		annotateSpan(span, handlerName, req)

		_ = LogError(span, err)

		return span, req.WithContext(ctx)
	}

	span := opentracing.StartSpan("request", opentracing.ChildOf(spanContext), ext.RPCServerOption(spanContext))
	annotateSpan(span, handlerName, req)

	return span, req.WithContext(opentracing.ContextWithSpan(req.Context(), span))
}

// LogError adds a span log for an error.
// Returns unchanged error, so useful to wrap as in:
//  return 0, tracing.LogError(err)
func LogError(span opentracing.Span, err error) error {
	if err == nil {
		return nil
	}

	// Get caller frame.
	var pcs [1]uintptr
	n := runtime.Callers(2, pcs[:])
	if n < 1 {
		span.LogFields(log.Error(err))
		span.LogFields(log.Error(errors.New("runtime.Callers failed")))
		return err
	}

	file, line := runtime.FuncForPC(pcs[0]).FileLine(pcs[0])
	span.LogFields(log.String("filename", file), log.Int("line", line), log.Error(err))

	return err
}

func annotateSpan(span opentracing.Span, handlerName string, req *http.Request) {
	// Default the span's "route" tag to the path. This gives the tag
	// a meaningful value for consumers where the other settings are empty.
	routeTag := req.URL.Path

	/*
		if route := httprouter.MatchedRouteFromContext(req.Context()); route != "" {
			routeTag = route
		}

		if ctx := chi.RouteContext(req.Context()); ctx != nil && ctx.RoutePath != "" {
			routeTag = ctx.RoutePath
		}
	*/

	span.SetTag("method", req.Method)
	span.SetTag("handler", handlerName)
	span.SetTag("route", routeTag)

	span.LogKV("path", req.URL.Path)
}