// MIT License
//
// # Copyright (c) 2021 hirosassa
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.
//
// Source:
//
//	https://github.com/hirosassa/zerodriver/blob/32567406b83903dc813682fd5f1999ebbf462f2d/http.go
package slogdriver

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"
)

// HTTPPayload is the struct consists of http request related components.
// Details are in following link.
// https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#HttpRequest
type HTTPPayload struct {
	RequestMethod                  string  `json:"requestMethod"`
	RequestURL                     string  `json:"requestUrl"`
	RequestSize                    string  `json:"requestSize,omitempty"`
	Status                         int     `json:"status"`
	ResponseSize                   string  `json:"responseSize,omitempty"`
	UserAgent                      string  `json:"userAgent"`
	RemoteIP                       string  `json:"remoteIp"`
	ServerIP                       string  `json:"serverIp"`
	Referer                        string  `json:"referer"`
	Latency                        Latency `json:"latency,omitempty"`
	CacheLookup                    bool    `json:"cacheLookup"`
	CacheHit                       bool    `json:"cacheHit"`
	CacheValidatedWithOriginServer bool    `json:"cacheValidatedWithOriginServer"`
	CacheFillBytes                 string  `json:"cacheFillBytes,omitempty"`
	Protocol                       string  `json:"protocol"`
}

// Latency is the interface of the request processing latency on the server.
// The format of the Latency should differ for GKE and for GAE, Cloud Run.
type Latency any

// GAELatency is the Latency for GAE and Cloud Run.
type GAELatency struct {
	Seconds int64 `json:"seconds"`
	Nanos   int32 `json:"nanos"`
}

// MakeHTTPAttr returns slog.Attr struct.
func MakeHTTPAttr(req *http.Request, res *http.Response) slog.Attr {
	return MakeHTTPAttrFromHTTPPayload(MakeHTTPPayload(req, res))
}

func MakeHTTPAttrFromHTTPPayload(p HTTPPayload) slog.Attr {
	return slog.Any(HTTPKey, p)
}

// MakeHTTPPayload returns a HTTPPayload struct.
func MakeHTTPPayload(req *http.Request, res *http.Response) HTTPPayload {
	if req == nil {
		req = &http.Request{}
	}

	if res == nil {
		res = &http.Response{}
	}

	payload := HTTPPayload{
		RequestMethod: req.Method,
		Status:        res.StatusCode,
		UserAgent:     req.UserAgent(),
		RemoteIP:      remoteIP(req),
		Referer:       req.Referer(),
		Protocol:      req.Proto,
	}

	if req.URL != nil {
		payload.RequestURL = req.URL.String()
	}

	if req.Body != nil {
		payload.RequestSize = strconv.FormatInt(req.ContentLength, 10)
	}

	if res.Body != nil {
		payload.ResponseSize = strconv.FormatInt(res.ContentLength, 10)
	}

	return payload
}

// MakeLatency returns Latency based on passed time.Duration object.
func MakeLatency(d time.Duration, isGKE bool) Latency {
	if isGKE {
		return makeGKELatency(d)
	} else {
		return makeGAELatency(d)
	}
}

// makeGKELatency returns Latency struct for GKE based on passed time.Duration object.
func makeGKELatency(d time.Duration) Latency {
	return d.Truncate(time.Millisecond).String() // need to Trucate by millis to show latency on Cloud Logging
}

// makeGAELatency returns Latency struct for Cloud Run and GAE based on passed time.Duration object.
func makeGAELatency(d time.Duration) Latency {
	nanos := d.Nanoseconds()
	secs := nanos / 1e9
	nanos -= secs * 1e9
	return GAELatency{
		Nanos:   int32(nanos),
		Seconds: secs,
	}
}

// remoteIP makes a best effort to compute the request client IP.
func remoteIP(req *http.Request) string {
	if f := req.Header.Get("X-Forwarded-For"); f != "" {
		return f
	}

	f := req.RemoteAddr
	ip, _, err := net.SplitHostPort(f)
	if err != nil {
		return f
	}

	return ip
}
