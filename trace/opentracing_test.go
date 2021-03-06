// This package tests the OpenTracing API
// which delegates to our own tracing implementation
package trace

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stripe/veneur/ssf"

	"github.com/golang/protobuf/proto"
	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
)

// Test that the Tracer can correctly create a root-level
// span.
// This is similar to TestStartTrace, but it tests the
// OpenTracing API for the same operations.
func TestTracerRootSpan(t *testing.T) {
	// TODO test tags!
	const resource = "Robert'); DROP TABLE students;"
	const expectedParent int64 = 0

	tracer := Tracer{}

	start := time.Now()
	trace := tracer.StartSpan(resource).(*Span)
	end := time.Now()

	between := end.After(trace.Start) && trace.Start.After(start)

	assert.Equal(t, trace.TraceId, trace.SpanId)
	assert.Equal(t, trace.ParentId, expectedParent)
	assert.Equal(t, trace.Resource, resource)
	assert.True(t, between)
}

// Test that the Tracer can correctly create a child span
func TestTracerChildSpan(t *testing.T) {
	// TODO test grandchild as well
	// to ensure we can do nested children properly

	const resource = "Robert'); DROP TABLE students;"
	// This will be a *really* slow trace!
	const expectedTimestamp = 1136239445
	var expectedTags = []*ssf.SSFTag{
		{
			Name:  "foo",
			Value: "bar",
		},
		{
			Name:  "baz",
			Value: "quz",
		},
	}

	tracer := Tracer{}

	parent := StartTrace(resource)
	var expectedParent = parent.SpanId

	start := time.Now()
	opts := []opentracing.StartSpanOption{
		customSpanStart(time.Unix(expectedTimestamp, 0)),
		customSpanParent(parent),
		customSpanTags("foo", "bar"),
		customSpanTags("baz", "quz"),
	}
	trace := tracer.StartSpan(resource, opts...).(*Span)
	end := time.Now()

	// The end time should be something between these two
	between := end.After(trace.End) && trace.End.After(start)
	assert.False(t, between)

	assert.Equal(t, time.Unix(expectedTimestamp, 0), trace.Start)

	assert.Equal(t, parent.TraceId, parent.SpanId)
	assert.Equal(t, expectedParent, trace.ParentId)
	assert.Equal(t, resource, trace.Resource)

	assert.Len(t, trace.Tags, len(expectedTags))

	for _, tag := range expectedTags {
		assert.Contains(t, trace.Tags, tag)
	}
}

// DummySpan is a helper function that gives
// a simple Span to use in tests
func DummySpan() *Span {
	const resource = "Robert'); DROP TABLE students;"
	const expectedTimestamp = 1136239445
	tracer := &Tracer{}

	parent := StartTrace(resource)
	opts := []opentracing.StartSpanOption{
		customSpanStart(time.Unix(expectedTimestamp, 0)),
		customSpanParent(parent),
		customSpanTags("foo", "bar"),
		customSpanTags("baz", "quz"),
	}
	trace := tracer.StartSpan(resource, opts...).(*Span)
	return trace
}

// TestTracerInjectBinary tests that we can inject
// a protocol buffer using the Binary format.
func TestTracerInjectBinary(t *testing.T) {
	trace := DummySpan().Trace

	trace.finish()

	tracer := Tracer{}
	var b bytes.Buffer

	err := tracer.Inject(trace.context(), opentracing.Binary, &b)
	assert.NoError(t, err)

	packet, err := ioutil.ReadAll(&b)
	assert.NoError(t, err)

	sample := &ssf.SSFSample{}
	err = proto.Unmarshal(packet, sample)
	assert.NoError(t, err)

	assertContextUnmarshalEqual(t, trace, sample)
}

// TestTracerExtractBinary tests that we can extract
// a protobuf representing an SSF (using the Binary format)
func TestTracerExtractBinary(t *testing.T) {
	trace := DummySpan().Trace

	trace.finish()

	tracer := Tracer{}

	packet, err := proto.Marshal(trace.SSFSample())
	assert.NoError(t, err)

	b := bytes.NewBuffer(packet)

	_, err = tracer.Extract(opentracing.Binary, b)
	assert.NoError(t, err)
}

// TestTracerInjectExtractBinary tests that we can inject a span
// and then Extract it (end-to-end).
func TestTracerInjectExtractBinary(t *testing.T) {
	trace := DummySpan().Trace
	tracer := Tracer{}
	var b bytes.Buffer
	var _ io.Reader = &b

	err := tracer.Inject(trace.context(), opentracing.Binary, &b)
	assert.NoError(t, err)

	c, err := tracer.Extract(opentracing.Binary, &b)
	assert.NoError(t, err)

	ctx := c.(*spanContext)

	assert.Equal(t, trace.TraceId, ctx.TraceId())

	assert.Equal(t, trace.SpanId, ctx.SpanId(), "original trace and context should share the same SpanId")
	assert.Equal(t, trace.ParentId, ctx.ParentId(), "original trace and context should share the same ParentId")
	assert.Equal(t, trace.Resource, ctx.Resource())
}

// TestTracerInjectTextMap tests that we can inject
// a protocol buffer using the TextMap format.
func TestTracerInjectTextMap(t *testing.T) {
	trace := DummySpan().Trace
	trace.finish()
	tracer := Tracer{}

	tm := textMapReaderWriter(map[string]string{})

	err := tracer.Inject(trace.context(), opentracing.TextMap, tm)
	assert.NoError(t, err)

	assert.Equal(t, strconv.FormatInt(trace.TraceId, 10), tm["traceid"])
	assert.Equal(t, strconv.FormatInt(trace.ParentId, 10), tm["parentid"])
	assert.Equal(t, strconv.FormatInt(trace.SpanId, 10), tm["spanid"])
	assert.Equal(t, trace.Resource, tm["resource"])
}

// TestTracerInjectExtractBinary tests that we can inject a span
// and then Extract it (end-to-end).
func TestTracerInjectExtractExtractTextMap(t *testing.T) {
	trace := DummySpan().Trace
	trace.finish()
	tracer := Tracer{}

	tm := textMapReaderWriter(map[string]string{})

	err := tracer.Inject(trace.context(), opentracing.TextMap, tm)
	assert.NoError(t, err)

	c, err := tracer.Extract(opentracing.TextMap, tm)
	assert.NoError(t, err)

	ctx := c.(*spanContext)

	assert.Equal(t, trace.TraceId, ctx.TraceId())

	assert.Equal(t, trace.SpanId, ctx.SpanId(), "original trace and context should share the same SpanId")
	assert.Equal(t, trace.ParentId, ctx.ParentId(), "original trace and context should share the same ParentId")
	assert.Equal(t, trace.Resource, ctx.Resource())
}

// TestTracerInjectExtractHeader tests that we can inject a span
// using HTTP headers and then extract it (end-to-end)
func TestTracerInjectExtractHeader(t *testing.T) {
	trace := DummySpan().Trace
	trace.finish()
	tracer := Tracer{}

	req, err := http.NewRequest(http.MethodPost, "/test", bytes.NewBuffer(nil))
	assert.NoError(t, err)

	carrier := opentracing.HTTPHeadersCarrier(req.Header)

	err = tracer.Inject(trace.context(), opentracing.HTTPHeaders, carrier)
	assert.NoError(t, err)

	c, err := tracer.Extract(opentracing.HTTPHeaders, carrier)
	assert.NoError(t, err)

	ctx := c.(*spanContext)

	assert.Equal(t, trace.TraceId, ctx.TraceId())

	assert.Equal(t, trace.SpanId, ctx.SpanId(), "original trace and context should share the same SpanId")
	assert.Equal(t, trace.ParentId, ctx.ParentId(), "original trace and context should share the same ParentId")
	assert.Equal(t, trace.Resource, ctx.Resource())

}

// assertContextUnmarshalEqual is a helper that asserts that the given SSFSample
// matches the expected *Trace on all fields that are passed through a SpanContext.
// Since a SpanContext doesn't pass fields like tags, this function will not cause
// the assertion to fail if those differ.
func assertContextUnmarshalEqual(t *testing.T, expected *Trace, sample *ssf.SSFSample) {
	assert.Equal(t, expected.SSFSample().Metric, sample.Metric)
	assert.Equal(t, expected.SSFSample().Status, sample.Status)

	// Future-proofiing: Currently we don't actually pass this through
	// but we don't support units at all.
	assert.Equal(t, expected.SSFSample().Unit, sample.Unit)

	// The sample rate is hard-coded
	assert.Equal(t, expected.SSFSample().SampleRate, sample.SampleRate)

	// The TraceId, ParentId, and Resource should all be the same.
	assert.Equal(t, expected.SSFSample().Trace.TraceId, sample.Trace.TraceId)
	assert.Equal(t, expected.SSFSample().Trace.ParentId, sample.Trace.ParentId)
	assert.Equal(t, expected.SSFSample().Trace.Id, sample.Trace.Id)
	assert.Equal(t, expected.SSFSample().Trace.Resource, sample.Trace.Resource)

}

// TestInjectRequestExtractRequestChild tests the InjectRequest
// and ExtractRequestChild helper functions
func TestInjectRequestExtractRequestChild(t *testing.T) {
	const childResource = "my child resource"
	const traceName = "my.child.name"
	trace := DummySpan().Trace
	trace.finish()
	tracer := Tracer{}
	req, err := http.NewRequest(http.MethodPost, "/test", bytes.NewBuffer(nil))
	assert.NoError(t, err)

	err = tracer.InjectRequest(trace, req)
	assert.NoError(t, err)

	span, err := tracer.ExtractRequestChild(childResource, req, traceName)
	assert.NoError(t, err)

	assert.NotEqual(t, trace.SpanId, span.SpanId, "original trace and child should have different SpanIds")
	assert.Equal(t, trace.SpanId, span.ParentId, "child should have the original trace's SpanId as its ParentId")
	assert.Equal(t, trace.TraceId, span.TraceId)
}
