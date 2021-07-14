package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/mkmik/go-jaeger-demo/pkg/tracing"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"github.com/uber/jaeger-client-go"
)

const serviceName = "mkm-test"

func work(ctx context.Context) error {
	span, _ := opentracing.StartSpanFromContext(ctx, "work")
	defer span.Finish()

	time.Sleep(1 * time.Millisecond)

	span.LogFields(log.String("baz", "qux"))

	return nil
}

func BackgroundContextWithValues(ctx context.Context) context.Context {
	return valueOnlyContext{ctx}
}

type valueOnlyContext struct{ context.Context }

func (valueOnlyContext) Deadline() (deadline time.Time, ok bool) { return }
func (valueOnlyContext) Done() <-chan struct{}                   { return nil }
func (valueOnlyContext) Err() error                              { return nil }

func background(ctx context.Context) {
	span, _ := opentracing.StartSpanFromContext(ctx, "background")
	defer span.Finish()

	span.LogFields(log.Bool("canceled", ctx.Err() != nil))
	time.Sleep(2 * time.Millisecond)
	span.LogFields(log.Bool("canceled", ctx.Err() != nil))
}

func tee(request *http.Request) {
	ctx := BackgroundContextWithValues(request.Context())
	request = request.WithContext(ctx)
	span, request := tracing.ExtractFromHTTPRequest(request, "tee")
	defer span.Finish()

	ctx = request.Context()
	background(ctx)
}

func serve(_ context.Context) error {
	http.HandleFunc("/", func(w http.ResponseWriter, request *http.Request) {
		span, request := tracing.ExtractFromHTTPRequest(request, "myserve")
		defer span.Finish()
		ctx := request.Context()

		span.LogFields(log.String("foo", "bar"))

		go tee(request)

		work(ctx)

		fmt.Fprintf(w, "ok\n")
	})

	return http.ListenAndServe(":8822", nil)
}

func reporter() (jaeger.Reporter, error) {
	var reporters []jaeger.Reporter

	reporters = append(reporters, jaeger.NewLoggingReporter(jaeger.StdLogger))

	agentHost := "localhost"
	if agentHost != "" {
		var agentHostPort string
		if agentPortStr := os.Getenv("JAEGER_AGENT_PORT"); agentPortStr == "" {
			agentHostPort = fmt.Sprintf("%s:%d", agentHost, jaeger.DefaultUDPSpanServerPort)
		} else {
			agentHostPort = fmt.Sprintf("%s:%s", agentHost, agentPortStr)
		}

		sender, err := jaeger.NewUDPTransport(agentHostPort, 0)
		if err != nil {
			return nil, err
		}
		reporter := jaeger.NewRemoteReporter(
			sender,
			jaeger.ReporterOptions.Logger(jaeger.StdLogger))
		reporters = append(reporters, reporter)
	}

	return jaeger.NewCompositeReporter(reporters...), nil
}

func InitTracing() io.Closer {
	reporter, err := reporter()
	if err != nil {
		panic(err)
	}

	tracer, closer := jaeger.NewTracer(
		serviceName,
		jaeger.NewConstSampler(true),
		reporter)

	opentracing.SetGlobalTracer(tracer)

	span := opentracing.GlobalTracer().StartSpan("init-test")
	span.Finish()

	return closer
}

func mainE() error {
	defer InitTracing().Close()

	ctx := context.Background()
	serve(ctx)

	return nil
}

func main() {
	if err := mainE(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
