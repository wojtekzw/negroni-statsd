package negronistatsd

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/peterbourgon/g2s"
)

const (
	sampleRate float32 = 1.0

	counterKeyFormatter = "%s.request.%d"
	timeKeyFormatter    = "%s%s"
)

func nopFilter(s string) (bool, string) {
	return true, s
}

func filterURL(path string) (bool, string) {
	return true, strings.SplitN(path, "?", 2)[0]
}

// Middleware stores the prefix and the statsd client
type Middleware struct {
	prefix         string
	client         g2s.Statter
	global         bool
	globalTiming   string
	globalReqCount string
	filter         func(string) (bool, string)
}

// NewMiddleware returns a middleware struct with a configured statsd client
func NewMiddleware(uri, prefix string) *Middleware {
	c, err := g2s.Dial("udp", uri)
	if err != nil {
		log.Printf("No statsd server on %s", uri)
		c = nopClient{}
	}
	return &Middleware{client: c, prefix: prefix, filter: filterURL, global: true,
		globalTiming:   prefix + ".request.timing",
		globalReqCount: prefix + ".request.count",
	}
}

func (m Middleware) timeRequest(startTime time.Time, r *http.Request) {
	var diffTime time.Duration

	runStat, urlPath := m.filter(r.RequestURI)
	diffTime = time.Since(startTime)
	if runStat {
		name := fmt.Sprintf(timeKeyFormatter, m.prefix, strings.Replace(urlPath, "/", ".", -1))
		m.client.Timing(sampleRate, name, diffTime)
	}
	if m.global {
		m.client.Timing(sampleRate, m.globalTiming, diffTime)
	}
}

func (m Middleware) countResponse(res negroni.ResponseWriter) {
	name := fmt.Sprintf(counterKeyFormatter, m.prefix, res.Status())
	m.client.Counter(sampleRate, name, 1)
	if m.global {
		m.client.Counter(sampleRate, m.globalReqCount, 1)
	}
}

// SetFilter - filter what should be sent to metrics
func (m Middleware) SetFilter(f func(string) (bool, string)) {
	m.filter = f
}

// SetGobalMetrics - if global number of req is recorded
func (m Middleware) SetGobalMetrics(s string, global bool) {
	m.globalTiming = m.prefix + "." + s + "." + "timing"
	m.globalReqCount = m.prefix + "." + s + "." + "count"
	m.global = global
}

func (m Middleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	res := negroni.NewResponseWriter(rw) // TODO: should we create our ResponseWriter wrapper to avoid this?

	defer func(startTime time.Time) {
		go m.timeRequest(startTime, r)
		go m.countResponse(res)
	}(time.Now())

	next(res, r)
}
