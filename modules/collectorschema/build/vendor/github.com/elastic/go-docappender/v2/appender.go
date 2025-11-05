// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package docappender

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/elastic/elastic-transport-go/v8/elastictransport"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

var (
	// ErrClosed is returned from methods of closed Indexers.
	ErrClosed = errors.New("model indexer closed")

	errMissingIndex = errors.New("missing index name")
	errMissingBody  = errors.New("missing document body")
)

// Appender provides an append-only API for bulk indexing documents into Elasticsearch.
//
// Appender buffers documents in their JSON encoding until either the accumulated buffer
// reaches `config.FlushBytes`, or `config.FlushInterval` elapses.
//
// Appender fills a single bulk request buffer at a time to ensure bulk requests are optimally
// sized, avoiding sparse bulk requests as much as possible. After a bulk request is flushed,
// the next document added will wait for the next available bulk request buffer and repeat the
// process.
//
// Up to `config.MaxRequests` bulk requests may be flushing/active concurrently, to allow the
// server to make progress encoding while Elasticsearch is busy servicing flushed bulk requests.
type Appender struct {
	// Used internally to calculate indexFailureRate for scaling reasons only.
	docsAdded       atomic.Int64
	tooManyRequests atomic.Int64

	scalingInfo atomic.Value

	config                Config
	client                elastictransport.Interface
	pool                  *BulkIndexerPool
	id                    string
	bulkItems             chan BulkIndexerItem
	errgroup              errgroup.Group
	errgroupContext       context.Context
	cancelErrgroupContext context.CancelCauseFunc
	metrics               metrics
	mu                    sync.Mutex
	closed                chan struct{}

	// tracer is an OTel tracer, and should not be confused with `a.config.Tracer`
	// which is an Elastic APM Tracer.
	tracer trace.Tracer
}

// New returns a new Appender that indexes documents into Elasticsearch.
// It is only tested with v8 go-elasticsearch client. Use other clients at your own risk.
func New(client elastictransport.Interface, cfg Config) (*Appender, error) {
	cfg = DefaultConfig(client, cfg)
	if client == nil {
		return nil, errors.New("client is nil")
	}

	if cfg.CompressionLevel < -1 || cfg.CompressionLevel > 9 {
		return nil, fmt.Errorf(
			"expected CompressionLevel in range [-1,9], got %d",
			cfg.CompressionLevel,
		)
	}

	minFlushBytes := 16 * 1024 // 16kb
	if cfg.CompressionLevel != 0 && cfg.FlushBytes < minFlushBytes {
		return nil, fmt.Errorf(
			"flush bytes config value (%d) is too small and will be ignored with compression enabled. Use at least %d",
			cfg.FlushBytes, minFlushBytes,
		)
	}

	ms, err := newMetrics(cfg)
	if err != nil {
		return nil, err
	}
	if err := BulkIndexerConfigFrom(client, cfg).Validate(); err != nil {
		return nil, fmt.Errorf("error creating bulk indexer: %w", err)
	}
	indexer := &Appender{
		pool:      cfg.BulkIndexerPool,
		config:    cfg,
		client:    client,
		closed:    make(chan struct{}),
		bulkItems: make(chan BulkIndexerItem, cfg.DocumentBufferSize),
		metrics:   ms,
	}
	// Use the Appender's pointer as the unique ID for the BulkIndexerPool.
	// Register the Appender ID in the pool.
	indexer.id = fmt.Sprintf("%p", indexer)
	indexer.pool.Register(indexer.id)
	attrs := metric.WithAttributeSet(indexer.config.MetricAttributes)
	indexer.metrics.availableBulkRequests.Add(context.Background(), int64(cfg.MaxRequests), attrs)
	// We create a cancellable context for the errgroup.Group for unblocking
	// flushes when Close returns. We intentionally do not use errgroup.WithContext,
	// because one flush failure should not cause the context to be cancelled.
	indexer.errgroupContext, indexer.cancelErrgroupContext = context.WithCancelCause(
		context.Background(),
	)
	indexer.scalingInfo.Store(scalingInfo{activeIndexers: 1})
	indexer.errgroup.Go(func() error {
		indexer.runActiveIndexer()
		return nil
	})

	if cfg.TracerProvider != nil {
		indexer.tracer = cfg.TracerProvider.Tracer("github.com/elastic/go-docappender.appender")
	}

	return indexer, nil
}

// Close closes the indexer, first flushing any queued items.
//
// Close returns an error if any flush attempts during the indexer's
// lifetime returned an error. If ctx is cancelled, Close returns and
// any ongoing flush attempts are cancelled.
func (a *Appender) Close(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	select {
	case <-a.closed:
		return a.errgroup.Wait()
	default:
	}
	close(a.closed)

	// Cancel ongoing flushes/pool.Get() when ctx is cancelled.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		defer a.cancelErrgroupContext(errors.New("cancelled by appender.close"))
		<-ctx.Done()
	}()

	defer a.metrics.availableBulkRequests.Add(context.Background(), -int64(a.config.MaxRequests), metric.WithAttributeSet(a.config.MetricAttributes))

	if err := a.errgroup.Wait(); err != nil {
		return err
	}
	indexers := a.pool.Deregister(a.id)
	var errs []error
	for bi := range indexers {
		if err := a.flush(context.Background(), bi); err != nil {
			errs = append(errs, fmt.Errorf("indexer failed: %w", err))
		}
	}
	if len(errs) != 0 {
		return fmt.Errorf("failed to flush events on close: %w", errors.Join(errs...))
	}
	return nil
}

// Add enqueues document for appending to index.
//
// The document body will be copied to a buffer using io.Copy, and document may
// implement io.WriterTo to reduce overhead of copying.
//
// The document io.WriterTo will be accessed after Add returns, and must remain
// accessible until its Read method returns EOF, or its WriterTo method returns.
func (a *Appender) Add(ctx context.Context, index string, document io.WriterTo) error {
	if index == "" {
		return errMissingIndex
	}
	if document == nil {
		return errMissingBody
	}

	// Send the BulkIndexerItem to the internal channel, allowing individual
	// documents to be processed by an active bulk indexer in a dedicated
	// goroutine, improving data locality and minimising lock contention.
	item := BulkIndexerItem{
		Index: index,
		Body:  document,
	}
	if len(a.bulkItems) == cap(a.bulkItems) {
		attrs := metric.WithAttributeSet(a.config.MetricAttributes)
		a.metrics.blockedAdd.Add(context.Background(), 1, attrs)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.closed:
		return ErrClosed
	case a.bulkItems <- item:
	}

	a.docsAdded.Add(1)
	attrs := metric.WithAttributeSet(a.config.MetricAttributes)
	a.metrics.docsAdded.Add(context.Background(), 1, attrs)
	a.metrics.docsActive.Add(context.Background(), 1, attrs)

	return nil
}

func (a *Appender) IndexersActive() int64 {
	return a.scalingInformation().activeIndexers
}

func (a *Appender) flush(ctx context.Context, bulkIndexer *BulkIndexer) error {
	n := bulkIndexer.Items()
	if n == 0 {
		return nil
	}
	defer func() {
		attrs := metric.WithAttributeSet(a.config.MetricAttributes)
		a.metrics.bulkRequests.Add(context.Background(), 1, attrs)
	}()

	logger := a.config.Logger
	var span trace.Span
	if a.otelTracingEnabled() {
		ctx, span = a.tracer.Start(ctx, "docappender.flush", trace.WithAttributes(
			attribute.Int("documents", n),
		))
		defer span.End()

		// Add trace IDs to logger, to associate any per-item errors
		// below with the trace.
		logger = logger.With(
			zap.String("traceId", span.SpanContext().TraceID().String()),
			zap.String("spanId", span.SpanContext().SpanID().String()),
		)
	}

	var flushCtx context.Context

	if a.config.FlushTimeout != 0 {
		var flushCancel context.CancelFunc
		flushCtx, flushCancel = context.WithTimeout(ctx, a.config.FlushTimeout)
		defer flushCancel()
	} else {
		flushCtx = ctx
	}

	resp, err := bulkIndexer.Flush(flushCtx)

	// Record the BulkIndexer buffer's length as the bytesTotal metric after
	// the request has been flushed.
	if flushed := bulkIndexer.BytesFlushed(); flushed > 0 {
		attrs := metric.WithAttributeSet(a.config.MetricAttributes)
		a.metrics.bytesTotal.Add(context.Background(), int64(flushed), attrs)
	}
	// Record the BulkIndexer uncompressed bytes written to the buffer
	// as the bytesUncompressedTotal metric after the request has been flushed.
	if flushed := bulkIndexer.BytesUncompressedFlushed(); flushed > 0 {
		attrs := metric.WithAttributeSet(a.config.MetricAttributes)
		a.metrics.bytesUncompressedTotal.Add(context.Background(), int64(flushed), attrs)
	}
	if err != nil {
		attrs := metric.WithAttributeSet(a.config.MetricAttributes)
		a.metrics.docsActive.Add(context.Background(), -int64(n), attrs)
		logger.Error("bulk indexing request failed", zap.Error(err))
		if a.otelTracingEnabled() && span.IsRecording() {
			span.RecordError(err)
			span.SetStatus(codes.Error, "bulk indexing request failed")
		}

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			a.metrics.docsIndexed.Add(
				context.Background(),
				int64(n),
				metric.WithAttributes(attribute.String("status", "Timeout")),
				metric.WithAttributeSet(a.config.MetricAttributes),
			)
		}

		// Bulk indexing may fail with different status codes.
		var errFailed ErrorFlushFailed
		if errors.As(err, &errFailed) {
			var status string
			switch {
			case errFailed.tooMany:
				a.tooManyRequests.Add(a.tooManyRequests.Load())
				status = "TooMany"
			case errFailed.clientError:
				status = "FailedClient"
			case errFailed.serverError:
				status = "FailedServer"
			}
			if status != "" {
				a.metrics.docsIndexed.Add(
					context.Background(),
					int64(n),
					metric.WithAttributes(
						attribute.String("status", status),
						semconv.HTTPResponseStatusCode(errFailed.statusCode),
					),
					metric.WithAttributeSet(a.config.MetricAttributes),
				)
			}
		}
		return err
	}
	var docsFailed, docsIndexed,
		// breakdown of failed docs:
		tooManyRequests, // failed after document retries (if it applies) and final status is 429
		clientFailed, // failed after document retries (if it applies) and final status is 400s excluding 429
		serverFailed int64 // failed after document retries (if it applies) and final status is 500s

	failureStoreDocs := resp.FailureStoreDocs
	docsIndexed = resp.Indexed
	var failedCount map[BulkIndexerResponseItem]int
	if len(resp.FailedDocs) > 0 {
		failedCount = make(map[BulkIndexerResponseItem]int, len(resp.FailedDocs))
	}
	docsFailed = int64(len(resp.FailedDocs))
	totalFlushed := docsFailed + docsIndexed
	attrs := metric.WithAttributeSet(a.config.MetricAttributes)
	a.metrics.docsActive.Add(context.Background(), -totalFlushed, attrs)
	for _, info := range resp.FailedDocs {
		if info.Status >= 400 && info.Status < 500 {
			if info.Status == http.StatusTooManyRequests {
				tooManyRequests++
			} else {
				clientFailed++
			}
		}
		if info.Status >= 500 {
			serverFailed++
		}
		info.Position = 0 // reset position so that the response item can be used as key in the map
		failedCount[info]++
		if a.otelTracingEnabled() && span.IsRecording() {
			e := errors.New(info.Error.Reason)
			span.RecordError(e)
			span.SetStatus(codes.Error, e.Error())
		}
	}
	for key, count := range failedCount {
		logger.Error(fmt.Sprintf("failed to index documents in '%s' (%s): %s",
			key.Index, key.Error.Type, key.Error.Reason,
		), zap.Int("documents", count))
	}
	if resp.RetriedDocs > 0 {
		// docs are scheduled to be retried but not yet failed due to retry limit
		a.metrics.docsRetried.Add(
			context.Background(),
			resp.RetriedDocs,
			metric.WithAttributeSet(a.config.MetricAttributes),
			metric.WithAttributes(attribute.Int("greatest_retry", resp.GreatestRetry)),
		)
	}
	if docsIndexed > 0 {
		a.metrics.docsIndexed.Add(
			context.Background(),
			docsIndexed,
			metric.WithAttributeSet(a.config.MetricAttributes),
			metric.WithAttributes(attribute.String("status", "Success")),
		)
	}
	if tooManyRequests > 0 {
		a.tooManyRequests.Add(tooManyRequests)
		a.metrics.docsIndexed.Add(
			context.Background(),
			tooManyRequests,
			metric.WithAttributes(attribute.String("status", "TooMany")),
			metric.WithAttributeSet(a.config.MetricAttributes),
		)
	}
	if clientFailed > 0 {
		a.metrics.docsIndexed.Add(
			context.Background(),
			clientFailed,
			metric.WithAttributes(attribute.String("status", "FailedClient")),
			metric.WithAttributeSet(a.config.MetricAttributes),
		)
	}
	if serverFailed > 0 {
		a.metrics.docsIndexed.Add(
			context.Background(),
			serverFailed,
			metric.WithAttributes(attribute.String("status", "FailedServer")),
			metric.WithAttributeSet(a.config.MetricAttributes),
		)
	}
	if failureStoreDocs.Used > 0 {
		a.metrics.docsIndexed.Add(
			context.Background(),
			failureStoreDocs.Used,
			metric.WithAttributes(
				attribute.String("status", "FailureStore"),
				attribute.String("failure_store", string(FailureStoreStatusUsed)),
			),
			metric.WithAttributeSet(a.config.MetricAttributes),
		)
	}
	if failureStoreDocs.Failed > 0 {
		a.metrics.docsIndexed.Add(
			context.Background(),
			failureStoreDocs.Failed,
			metric.WithAttributes(
				attribute.String("status", "FailureStore"),
				attribute.String("failure_store", string(FailureStoreStatusFailed)),
			),
			metric.WithAttributeSet(a.config.MetricAttributes),
		)
	}
	if failureStoreDocs.NotEnabled > 0 {
		a.metrics.docsIndexed.Add(
			context.Background(),
			failureStoreDocs.NotEnabled,
			metric.WithAttributes(
				attribute.String("status", "FailureStore"),
				attribute.String("failure_store", string(FailureStoreStatusNotEnabled)),
			),
			metric.WithAttributeSet(a.config.MetricAttributes),
		)
	}
	logger.Debug(
		"bulk request completed",
		zap.Int64("docs_indexed", docsIndexed),
		zap.Int64("docs_failed", docsFailed),
		zap.Int64("docs_rate_limited", tooManyRequests),
		zap.Int64("docs_failure_store_used", failureStoreDocs.Used),
		zap.Int64("docs_failure_store_failed", failureStoreDocs.Failed),
		zap.Int64("docs_failure_store_not_enabled", failureStoreDocs.NotEnabled),
	)
	if a.otelTracingEnabled() && span.IsRecording() {
		span.SetStatus(codes.Ok, "")
	}
	return nil
}

// runActiveIndexer starts a new active indexer which pulls items from the
// bulkItems channel. The more active indexers there are, the faster items
// will be pulled out of the queue, but also the more likely it is that the
// outgoing Elasticsearch bulk requests are flushed due to the idle timer,
// rather than due to being full.
func (a *Appender) runActiveIndexer() {
	var closed bool
	var active *BulkIndexer
	var timedFlush uint
	var fullFlush uint
	flushTimer := time.NewTimer(a.config.FlushInterval)
	if !flushTimer.Stop() {
		<-flushTimer.C
	}
	var firstDocTS time.Time
	handleBulkItem := func(item BulkIndexerItem) bool {
		if active == nil {
			// NOTE(marclop) Record the TS when the first document is cached.
			// It doesn't account for the time spent in the buffered channel.
			firstDocTS = time.Now()
			// Return early when the Close() context expires before get returns
			// an indexer. This could happen when all available bulk_requests
			// are in flight & no new BulkIndexers can be pulled from the pool.
			var err error
			active, err = a.pool.Get(a.errgroupContext, a.id)
			if err != nil {
				a.config.Logger.Warn("failed to get bulk indexer from pool", zap.Error(err))
				return false
			}
			// The BulkIndexer may have been used by another appender, we need
			// to reset it to ensure we're using the right client.
			active.SetClient(a.client)

			attrs := metric.WithAttributeSet(a.config.MetricAttributes)
			a.metrics.availableBulkRequests.Add(context.Background(), -1, attrs)
			a.metrics.inflightBulkrequests.Add(context.Background(), 1, attrs)
			flushTimer.Reset(a.config.FlushInterval)
		}
		if err := active.Add(item); err != nil {
			a.config.Logger.Error("failed to Add item to bulk indexer", zap.Error(err))
		}
		return true
	}
	for !closed {
		select {
		case <-flushTimer.C:
			timedFlush++
			fullFlush = 0
		default:
			// When there's no active indexer and queue utilization is below 5%,
			// reset the flushTimer with IdleInterval so excess active indexers
			// that remain idle can be scaled down.
			if !a.config.Scaling.Disabled && active == nil {
				if a.scalingInformation().activeIndexers > 1 &&
					float64(len(a.bulkItems))/float64(cap(a.bulkItems)) <= 0.05 {
					flushTimer.Reset(a.config.Scaling.IdleInterval)
				}
			}
			select {
			case <-a.closed:
				// Consume whatever bulk items have been buffered,
				// and then flush a last time below.
				for len(a.bulkItems) > 0 {
					select {
					case item := <-a.bulkItems:
						handleBulkItem(item)
					default:
						// Another goroutine took the item.
					}
				}
				closed = true
			case <-flushTimer.C:
				timedFlush++
				fullFlush = 0
			case item := <-a.bulkItems:
				if handleBulkItem(item) {
					if active.Len() < a.config.FlushBytes {
						continue
					}
					fullFlush++
					timedFlush = 0
					// The active indexer is at or exceeds the configured FlushBytes
					// threshold, so flush it.
					if !flushTimer.Stop() {
						<-flushTimer.C
					}
				}
			}
		}
		if active != nil {
			indexer := active
			active = nil
			attrs := metric.WithAttributeSet(a.config.MetricAttributes)
			a.errgroup.Go(func() error {
				var err error
				took := timeFunc(func() {
					err = a.flush(a.errgroupContext, indexer)
				})
				indexer.Reset()
				a.pool.Put(a.id, indexer)
				attrs := metric.WithAttributeSet(a.config.MetricAttributes)
				a.metrics.availableBulkRequests.Add(context.Background(), 1, attrs)
				a.metrics.inflightBulkrequests.Add(context.Background(), -1, attrs)
				a.metrics.flushDuration.Record(context.Background(), took.Seconds(),
					attrs,
				)
				return err
			})
			a.metrics.bufferDuration.Record(context.Background(),
				time.Since(firstDocTS).Seconds(), attrs,
			)
		}
		if a.config.Scaling.Disabled {
			continue
		}
		now := time.Now()
		info := a.scalingInformation()
		if a.maybeScaleDown(now, info, &timedFlush) {
			attrs := metric.WithAttributeSet(a.config.MetricAttributes)
			a.metrics.activeDestroyed.Add(context.Background(), 1, attrs)
			return
		}
		if a.maybeScaleUp(now, info, &fullFlush) {
			attrs := metric.WithAttributeSet(a.config.MetricAttributes)
			a.metrics.activeCreated.Add(context.Background(), 1, attrs)
			a.errgroup.Go(func() error {
				a.runActiveIndexer()
				return nil
			})
		}
	}
	// Decrement the active bulk requests when Appender is closed.
	for {
		info := a.scalingInformation()
		if a.scalingInfo.CompareAndSwap(info, scalingInfo{
			lastAction:     time.Now(),
			activeIndexers: info.activeIndexers - 1,
		}) {
			return
		}
	}
}

// maybeScaleDown returns true if the caller (assumed to be active indexer) needs
// to be scaled down. It automatically updates the scaling information with a
// decremented `activeBulkRequests` and timestamp of the action when true.
func (a *Appender) maybeScaleDown(now time.Time, info scalingInfo, timedFlush *uint) bool {
	// Only downscale when there is more than 1 active indexer.
	if info.activeIndexers == 1 {
		return false
	}
	// If the CPU quota changes and there is more than 1 indexer, downscale an
	// active indexer. This downscaling action isn't subject to the downscaling
	// cooldown, since doing so would result in using much more CPU for longer.
	// Loop until the CompareAndSwap operation succeeds (there may be more than)
	// since a single active indexer trying to down scale itself, or the active
	// indexer variable is in check.
	limit := a.activeLimit()
	for info.activeIndexers > limit {
		// Avoid having more than 1 concurrent downscale, by using a compare
		// and swap operation.
		if newInfo := info.ScaleDown(now); a.scalingInfo.CompareAndSwap(info, newInfo) {
			a.config.Logger.Info(
				"active indexers exceeds limit, scaling down",
				zap.Int64("old_active_indexer_count", info.activeIndexers),
				zap.Int64("new_active_indexer_count", newInfo.activeIndexers),
				zap.Int64("active_indexer_limit", limit),
			)
			return true
		}
		info = a.scalingInformation() // refresh scaling info if CAS failed.
	}
	if info.withinCoolDown(a.config.Scaling.ScaleDown.CoolDown, now) {
		return false
	}
	// If more than 1% of the requests result in 429, scale down the current
	// active indexer.
	if a.indexFailureRate() >= 0.01 {
		if newInfo := info.ScaleDown(now); a.scalingInfo.CompareAndSwap(info, newInfo) {
			a.config.Logger.Info(
				"elasticsearch 429 response rate exceeded 1%, scaling down",
				zap.Int64("old_active_indexer_count", info.activeIndexers),
				zap.Int64("new_active_indexer_count", newInfo.activeIndexers),
			)
			return true
		}
		return false
	}
	if *timedFlush < a.config.Scaling.ScaleDown.Threshold {
		return false
	}
	// Reset timedFlush after it has exceeded the threshold
	// it avoids unnecessary precociousness to scale down.
	*timedFlush = 0
	if newInfo := info.ScaleDown(now); a.scalingInfo.CompareAndSwap(info, newInfo) {
		a.config.Logger.Info(
			"timed flush threshold exceeded, scaling down",
			zap.Int64("old_active_indexer_count", info.activeIndexers),
			zap.Int64("new_active_indexer_count", newInfo.activeIndexers),
		)
		return true
	}
	return false
}

// maybeScaleUp returns true if the caller (assumed to be active indexer) needs
// to scale up and create another active indexer goroutine. It automatically
// updates the scaling information with an incremented `activeBulkRequests` and
// timestamp of the action when true.
func (a *Appender) maybeScaleUp(now time.Time, info scalingInfo, fullFlush *uint) bool {
	if *fullFlush < a.config.Scaling.ScaleUp.Threshold {
		return false
	}
	if info.activeIndexers >= a.activeLimit() {
		return false
	}
	// Reset fullFlush after it has exceeded the threshold
	// it avoids unnecessary precociousness to scale up.
	*fullFlush = 0
	// If more than 1% of the requests result in 429, do not scale up.
	if a.indexFailureRate() >= 0.01 {
		return false
	}
	if info.withinCoolDown(a.config.Scaling.ScaleUp.CoolDown, now) {
		return false
	}
	// Avoid having more than 1 concurrent upscale, by using a compare
	// and swap operation.
	if newInfo := info.ScaleUp(now); a.scalingInfo.CompareAndSwap(info, newInfo) {
		a.config.Logger.Info(
			"full flush threshold exceeded, scaling up",
			zap.Int64("old_active_indexer_count", info.activeIndexers),
			zap.Int64("new_active_indexer_count", newInfo.activeIndexers),
		)
		return true
	}
	return false
}

func (a *Appender) scalingInformation() scalingInfo {
	return a.scalingInfo.Load().(scalingInfo)
}

// indexFailureRate returns the decimal percentage of 429 / total docs.
func (a *Appender) indexFailureRate() float64 {
	return float64(a.tooManyRequests.Load()) / float64(a.docsAdded.Load())
}

// otelTracingEnabled checks whether we should be doing tracing
// using otel tracer.
func (a *Appender) otelTracingEnabled() bool {
	return a.tracer != nil
}

// activeLimit returns the value of GOMAXPROCS * cfg.ActiveRatio. Which limits
// the maximum number of active indexers to a % of GOMAXPROCS.
//
// NOTE: There is also a sweet spot between Config.MaxRequests and the number
// of available indexers, where having N number of available bulk requests per
// active bulk indexer is required for optimal performance.
func (a *Appender) activeLimit() int64 {
	ar := a.config.Scaling.ActiveRatio
	if limit := float64(runtime.GOMAXPROCS(0)) * ar; limit > 1 {
		return int64(math.RoundToEven(limit)) // return when > 1.
	}
	return 1
}

// scalingInfo contains the number of active indexers and the timestamp of the
// latest time a scale action was performed. This structure is used within the
// Appender to coordinate scale actions with a CompareAndSwap operation.
type scalingInfo struct {
	lastAction     time.Time
	activeIndexers int64
}

func (s scalingInfo) ScaleDown(t time.Time) scalingInfo {
	return scalingInfo{lastAction: t, activeIndexers: s.activeIndexers - 1}
}

func (s scalingInfo) ScaleUp(t time.Time) scalingInfo {
	return scalingInfo{lastAction: t, activeIndexers: s.activeIndexers + 1}
}

func (s scalingInfo) withinCoolDown(cooldown time.Duration, now time.Time) bool {
	return s.lastAction.Add(cooldown).After(now)
}

func timeFunc(f func()) time.Duration {
	t0 := time.Now()
	if f != nil {
		f()
	}
	return time.Since(t0)
}
