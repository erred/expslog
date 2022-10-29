package main

import (
	"context"
	"crypto/rand"
	"errors"
	"log"

	"go.opentelemetry.io/otel/trace"
	"go.seankhliao.com/expslog"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc"
)

func main() {
	conn, err := grpc.DialContext(context.Background(), "localhost:4317", grpc.WithInsecure())
	if err != nil {
		log.Println(err)
		return
	}
	h := expslog.NewOTLPHandler(conn)
	lg := slog.New(h)

	lg.Info("hello world", "foo", "bar", "fizz", 1, "buzz", true)

	lg = lg.WithGroup("my-app")

	lg.Info("hello world", "foo", "bar", "fizz", 1, "buzz", true)

	lg = lg.With("a", "b", "c", "d")

	lg.Info("hello world", "foo", "bar", "fizz", 1, "buzz", true)

	rnd := make([]byte, 24)
	rand.Reader.Read(rnd)
	var traceID [16]byte
	copy(traceID[:], rnd[:16])
	var spanID [8]byte
	copy(spanID[:], rnd[16:])
	cfg := trace.SpanContextConfig{
		TraceID: trace.TraceID(traceID),
		SpanID:  trace.SpanID(spanID),
	}
	spanCtx := trace.NewSpanContext(cfg)
	ctx := context.Background()
	ctx = trace.ContextWithSpanContext(ctx, spanCtx)

	lg.WithContext(ctx).Error("oops", errors.New("an error occurred"), "foo", "bar")
}
