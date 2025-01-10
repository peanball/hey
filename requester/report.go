// Copyright 2014 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package requester

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"sort"
	"time"
)

const (
	barChar = "■"
)

// We report for max 1M results.
const maxRes = 1000000

type report struct {
	avgTotal float64
	fastest  float64
	slowest  float64
	average  float64
	rps      float64

	avgConn     float64
	avgDNS      float64
	avgTLS      float64
	avgReq      float64
	avgRes      float64
	avgDelay    float64
	connLats    []float64
	dnsLats     []float64
	tlsLats     []float64
	reqLats     []float64
	resLats     []float64
	delayLats   []float64
	offsets     []float64
	statusCodes []int

	results chan *result
	done    chan bool
	total   time.Duration

	errorDist map[string]int
	lats      []float64
	sizeTotal int64
	numRes    int64
	output    string

	w io.Writer
}

func newReport(w io.Writer, results chan *result, output string, n int) *report {
	capacity := min(n, maxRes)
	return &report{
		output:      output,
		results:     results,
		done:        make(chan bool, 1),
		errorDist:   make(map[string]int),
		w:           w,
		connLats:    make([]float64, 0, capacity),
		dnsLats:     make([]float64, 0, capacity),
		tlsLats:     make([]float64, 0, capacity),
		reqLats:     make([]float64, 0, capacity),
		resLats:     make([]float64, 0, capacity),
		delayLats:   make([]float64, 0, capacity),
		lats:        make([]float64, 0, capacity),
		statusCodes: make([]int, 0, capacity),
	}
}

func runReporter(r *report) {
	// Loop will continue until channel is closed
	for res := range r.results {
		r.numRes++
		if res.err != nil {
			r.errorDist[res.err.Error()]++
		} else {
			r.avgTotal += res.duration.Seconds()
			r.avgConn += res.connDuration.Seconds()
			r.avgDelay += res.delayDuration.Seconds()
			r.avgDNS += res.dnsDuration.Seconds()
			r.avgTLS += res.tlsDuration.Seconds()
			r.avgReq += res.reqDuration.Seconds()
			r.avgRes += res.resDuration.Seconds()
			if len(r.resLats) < maxRes {
				r.lats = append(r.lats, res.duration.Seconds())
				r.connLats = append(r.connLats, res.connDuration.Seconds())
				r.dnsLats = append(r.dnsLats, res.dnsDuration.Seconds())
				r.tlsLats = append(r.tlsLats, res.tlsDuration.Seconds())
				r.reqLats = append(r.reqLats, res.reqDuration.Seconds())
				r.delayLats = append(r.delayLats, res.delayDuration.Seconds())
				r.resLats = append(r.resLats, res.resDuration.Seconds())
				r.statusCodes = append(r.statusCodes, res.statusCode)
				r.offsets = append(r.offsets, res.offset.Seconds())
			}
			if res.contentLength > 0 {
				r.sizeTotal += res.contentLength
			}
		}
	}
	// Signal reporter is done.
	r.done <- true
}

func (r *report) finalize(total time.Duration) {
	r.total = total
	r.rps = float64(r.numRes) / r.total.Seconds()
	r.average = r.avgTotal / float64(len(r.lats))
	r.avgConn = r.avgConn / float64(len(r.connLats))
	r.avgDelay = r.avgDelay / float64(len(r.delayLats))
	r.avgDNS = r.avgDNS / float64(len(r.dnsLats))
	r.avgTLS = r.avgTLS / float64(len(r.tlsLats))
	r.avgReq = r.avgReq / float64(len(r.reqLats))
	r.avgRes = r.avgRes / float64(len(r.resLats))
	r.print()
}

func (r *report) print() {
	buf := &bytes.Buffer{}
	if err := newTemplate(r.output).Execute(buf, r.snapshot()); err != nil {
		log.Println("error:", err.Error())
		return
	}
	r.printf(buf.String())

	r.printf("\n")
}

func (r *report) printf(s string, v ...interface{}) {
	fmt.Fprintf(r.w, s, v...)
}

func (r *report) snapshot() Report {
	snapshot := Report{
		AvgTotal:    r.avgTotal,
		Average:     r.average,
		Rps:         r.rps,
		SizeTotal:   r.sizeTotal,
		AvgConn:     r.avgConn,
		AvgDNS:      r.avgDNS,
		AvgTLS:      r.avgTLS,
		AvgReq:      r.avgReq,
		AvgRes:      r.avgRes,
		AvgDelay:    r.avgDelay,
		Total:       r.total,
		ErrorDist:   r.errorDist,
		NumRes:      r.numRes,
		Lats:        make([]float64, len(r.lats)),
		ConnLats:    make([]float64, len(r.lats)),
		DnsLats:     make([]float64, len(r.lats)),
		TlsLats:     make([]float64, len(r.lats)),
		ReqLats:     make([]float64, len(r.lats)),
		ResLats:     make([]float64, len(r.lats)),
		DelayLats:   make([]float64, len(r.lats)),
		Offsets:     make([]float64, len(r.lats)),
		StatusCodes: make([]int, len(r.lats)),
	}

	if len(r.lats) == 0 {
		return snapshot
	}

	snapshot.SizeReq = r.sizeTotal / int64(len(r.lats))

	copy(snapshot.Lats, r.lats)
	copy(snapshot.ConnLats, r.connLats)
	copy(snapshot.DnsLats, r.dnsLats)
	copy(snapshot.TlsLats, r.tlsLats)
	copy(snapshot.ReqLats, r.reqLats)
	copy(snapshot.ResLats, r.resLats)
	copy(snapshot.DelayLats, r.delayLats)
	copy(snapshot.StatusCodes, r.statusCodes)
	copy(snapshot.Offsets, r.offsets)

	sort.Float64s(r.lats)
	r.fastest = r.lats[0]
	r.slowest = r.lats[len(r.lats)-1]

	sort.Float64s(r.connLats)
	sort.Float64s(r.dnsLats)
	sort.Float64s(r.tlsLats)
	sort.Float64s(r.reqLats)
	sort.Float64s(r.resLats)
	sort.Float64s(r.delayLats)

	// TODO: consider other histograms?
	snapshot.Histogram = r.histogram(r.lats)
	snapshot.LatencyDistribution = r.latencies()

	snapshot.Fastest = r.fastest
	snapshot.Slowest = r.slowest
	snapshot.ConnMax = r.connLats[0]
	snapshot.ConnMin = r.connLats[len(r.connLats)-1]
	snapshot.DnsMax = r.dnsLats[0]
	snapshot.DnsMin = r.dnsLats[len(r.dnsLats)-1]
	if len(r.tlsLats) > 0 {
		snapshot.TlsMax = r.tlsLats[0]
		snapshot.TlsMin = r.tlsLats[len(r.tlsLats)-1]
	}
	snapshot.ReqMax = r.reqLats[0]
	snapshot.ReqMin = r.reqLats[len(r.reqLats)-1]
	snapshot.DelayMax = r.delayLats[0]
	snapshot.DelayMin = r.delayLats[len(r.delayLats)-1]
	snapshot.ResMax = r.resLats[0]
	snapshot.ResMin = r.resLats[len(r.resLats)-1]

	statusCodeDist := make(map[int]int, len(snapshot.StatusCodes))
	for _, statusCode := range snapshot.StatusCodes {
		statusCodeDist[statusCode]++
	}
	snapshot.StatusCodeDist = statusCodeDist

	return snapshot
}

func (r *report) latencies() []LatencyDistribution {
	pctls := []int{10, 25, 50, 75, 90, 95, 99}
	data := make([]float64, len(pctls))
	j := 0

	// TODO: loop over percentiles
	// For each percentile, create the latency distribution directly
	for i := 0; i < len(r.lats) && j < len(pctls); i++ {
		current := i * 100 / len(r.lats)
		if current >= pctls[j] {
			data[j] = r.lats[i]
			j++
		}
	}
	res := make([]LatencyDistribution, len(pctls))
	for i := 0; i < len(pctls); i++ {
		if data[i] > 0 {
			res[i] = LatencyDistribution{Percentage: pctls[i], Latency: data[i]}
		}
	}
	return res
}

func (r *report) histogram(data []float64) []Bucket {
	bc := 10
	buckets := make([]float64, bc+1)
	counts := make([]int, bc+1)
	bs := (r.slowest - r.fastest) / float64(bc)
	for i := 0; i < bc; i++ {
		buckets[i] = r.fastest + bs*float64(i)
	}
	buckets[bc] = r.slowest
	var bi int
	var maximum int
	for i := 0; i < len(data); {
		if data[i] <= buckets[bi] {
			i++
			counts[bi]++
			if maximum < counts[bi] {
				maximum = counts[bi]
			}
		} else if bi < len(buckets)-1 {
			bi++
		}
	}
	res := make([]Bucket, len(buckets))
	for i := 0; i < len(buckets); i++ {
		res[i] = Bucket{
			Mark:      buckets[i],
			Count:     counts[i],
			Frequency: float64(counts[i]) / float64(len(r.lats)),
		}
	}
	return res
}

type Report struct {
	AvgTotal float64
	Fastest  float64
	Slowest  float64
	Average  float64
	Rps      float64

	AvgConn  float64
	AvgDNS   float64
	AvgTLS   float64
	AvgReq   float64
	AvgRes   float64
	AvgDelay float64
	ConnMax  float64
	ConnMin  float64
	DnsMax   float64
	DnsMin   float64
	TlsMax   float64
	TlsMin   float64
	ReqMax   float64
	ReqMin   float64
	ResMax   float64
	ResMin   float64
	DelayMax float64
	DelayMin float64

	Lats        []float64
	ConnLats    []float64
	DnsLats     []float64
	TlsLats     []float64
	ReqLats     []float64
	ResLats     []float64
	DelayLats   []float64
	Offsets     []float64
	StatusCodes []int

	Total time.Duration

	ErrorDist      map[string]int
	StatusCodeDist map[int]int
	SizeTotal      int64
	SizeReq        int64
	NumRes         int64

	LatencyDistribution []LatencyDistribution
	Histogram           []Bucket
}

type LatencyDistribution struct {
	Percentage   int
	Latency      float64
	DnsLatency   float64
	ConnLatency  float64
	TlsLatency   float64
	DelayLatency float64
	ReqLatency   float64
	RespLatency  float64
}

type Bucket struct {
	Mark      float64
	Count     int
	Frequency float64
}
