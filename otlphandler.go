package expslog

import (
	"context"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
	rpclogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	"golang.org/x/exp/slog"
	"google.golang.org/grpc"
)

var _ slog.Handler = (*otlpHandler)(nil)

type otlpHandler struct {
	level  slog.Level
	attrs  []*commonv1.KeyValue
	groups []string
	client rpclogsv1.LogsServiceClient
}

func NewOTLPHandler(conn *grpc.ClientConn) slog.Handler {
	return &otlpHandler{
		client: rpclogsv1.NewLogsServiceClient(conn),
	}
}

func (h *otlpHandler) Enabled(l slog.Level) bool {
	return l >= h.level
}

func (h *otlpHandler) Handle(r slog.Record) error {
	out := recordSlogToOTLP(r, h.attrs, h.groups)
	_, err := h.client.Export(context.Background(), &rpclogsv1.ExportLogsServiceRequest{
		ResourceLogs: []*logsv1.ResourceLogs{
			{
				ScopeLogs: []*logsv1.ScopeLogs{
					{
						LogRecords: []*logsv1.LogRecord{
							out,
						},
					},
				},
			},
		},
	})
	return err
}

func (h *otlpHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := h
	h2.attrs = make([]*commonv1.KeyValue, len(h.attrs), len(h.attrs)+len(attrs))
	copy(h2.attrs, h.attrs)
	for _, a := range attrs {
		h2.attrs = append(h2.attrs, attrSlogToOTLP(a, h.groups))
	}
	return h2
}

func (h *otlpHandler) WithGroup(name string) slog.Handler {
	h2 := h
	h2.groups = make([]string, len(h.groups)+1)
	copy(h2.groups, h.groups)
	h2.groups[len(h2.groups)-1] = name
	return h2
}

func recordSlogToOTLP(in slog.Record, baseAttrs []*commonv1.KeyValue, groups []string) *logsv1.LogRecord {
	out := &logsv1.LogRecord{
		TimeUnixNano:         uint64(in.Time.UnixNano()),
		ObservedTimeUnixNano: uint64(in.Time.UnixNano()),
		SeverityNumber:       logsv1.SeverityNumber(in.Level + 9),
		Body:                 &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: in.Message}},
	}

	out.Attributes = make([]*commonv1.KeyValue, 0, len(baseAttrs)+in.NumAttrs())
	out.Attributes = append(out.Attributes, baseAttrs...)
	in.Attrs(func(a slog.Attr) {
		out.Attributes = append(out.Attributes, attrSlogToOTLP(a, groups))
	})

	spanCtx := trace.SpanContextFromContext(in.Context)
	if spanCtx.IsValid() {
		out.Flags = uint32(spanCtx.TraceFlags())
		traceID := spanCtx.TraceID()
		out.TraceId = traceID[:]
		spanID := spanCtx.SpanID()
		out.SpanId = spanID[:]
	}

	return out
}

func attrSlogToOTLP(a slog.Attr, groups []string) *commonv1.KeyValue {
	return &commonv1.KeyValue{
		Key:   strings.Join(append(groups, a.Key), "."),
		Value: valueSlogToOTLP(a.Value),
	}
}

func valueSlogToOTLP(v slog.Value) *commonv1.AnyValue {
	switch v.Kind() {
	// shared primitive kinds
	case slog.BoolKind:
		return &commonv1.AnyValue{Value: &commonv1.AnyValue_BoolValue{BoolValue: v.Bool()}}
	case slog.Float64Kind:
		return &commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: v.Float64()}}
	case slog.Int64Kind:
		return &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: v.Int64()}}
	case slog.StringKind:
		return &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: v.String()}}

	case slog.DurationKind:
		return &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: v.Duration().String()}} // TODO: correct representation?
	case slog.Uint64Kind:
		return &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: int64(v.Uint64())}} // TODO: overflow?
	case slog.TimeKind:
		return &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: v.Time().Format(time.RFC3339Nano)}}
	case slog.GroupKind:
		group := v.Group()
		vals := make([]*commonv1.KeyValue, 0, len(group))
		for _, a := range group {
			vals = append(vals, attrSlogToOTLP(a, nil))
		}
		return &commonv1.AnyValue{Value: &commonv1.AnyValue_KvlistValue{KvlistValue: &commonv1.KeyValueList{Values: vals}}}

	case slog.LogValuerKind:
		return valueSlogToOTLP(v.Resolve())

	case slog.AnyKind:
		fallthrough
	default:
		return &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: v.String()}} // TODO: recurse slice/map?
	}
}
