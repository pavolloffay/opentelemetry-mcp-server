/*
 * Copyright 2023 SentinelOne, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package statistics

import (
	"context"
	"math"
	"sync/atomic"
	"time"

	"github.com/scalyr/dataset-go/pkg/meter_config"
	"go.opentelemetry.io/otel/attribute"

	"go.uber.org/zap"

	"github.com/scalyr/dataset-go/pkg/buffer_config"

	"go.opentelemetry.io/otel/metric"
)

// Statistics related to data processing
type Statistics struct {
	addEventsEntered atomic.Uint64
	addEventsExited  atomic.Uint64

	buffersEnqueued  atomic.Uint64
	buffersProcessed atomic.Uint64
	buffersDropped   atomic.Uint64
	buffersBroken    atomic.Uint64

	eventsEnqueued  atomic.Uint64
	eventsProcessed atomic.Uint64
	eventsDropped   atomic.Uint64
	eventsBroken    atomic.Uint64

	bytesAPISent     atomic.Uint64
	bytesAPIAccepted atomic.Uint64

	sessionsOpened atomic.Uint64
	sessionsClosed atomic.Uint64

	config     *meter_config.MeterConfig
	meter      *metric.Meter
	logger     *zap.Logger
	attributes []attribute.KeyValue

	cAddEventsEntered metric.Int64UpDownCounter
	cAddEventsExited  metric.Int64UpDownCounter

	cBuffersEnqueued  metric.Int64UpDownCounter
	cBuffersProcessed metric.Int64UpDownCounter
	cBuffersDropped   metric.Int64UpDownCounter
	cBuffersBroken    metric.Int64UpDownCounter

	cEventsEnqueued  metric.Int64UpDownCounter
	cEventsProcessed metric.Int64UpDownCounter
	cEventsDropped   metric.Int64UpDownCounter
	cEventsBroken    metric.Int64UpDownCounter

	cBytesAPISent     metric.Int64UpDownCounter
	cBytesAPIAccepted metric.Int64UpDownCounter

	cSessionsOpened metric.Int64UpDownCounter
	cSessionsClosed metric.Int64UpDownCounter

	hPayloadSize  metric.Int64Histogram
	hResponseTime metric.Int64Histogram
}

// NewStatistics creates structure to keep track of data processing.
// If meter is not nil, then Open Telemetry is used for collecting metrics
// as well.
func NewStatistics(config *meter_config.MeterConfig, logger *zap.Logger) (*Statistics, error) {
	logger.Info("Initialising statistics")
	statistics := &Statistics{
		addEventsEntered: atomic.Uint64{},
		addEventsExited:  atomic.Uint64{},
		buffersEnqueued:  atomic.Uint64{},
		buffersProcessed: atomic.Uint64{},
		buffersDropped:   atomic.Uint64{},
		buffersBroken:    atomic.Uint64{},

		eventsEnqueued:  atomic.Uint64{},
		eventsProcessed: atomic.Uint64{},
		eventsDropped:   atomic.Uint64{},
		eventsBroken:    atomic.Uint64{},

		bytesAPIAccepted: atomic.Uint64{},
		bytesAPISent:     atomic.Uint64{},

		sessionsOpened: atomic.Uint64{},
		sessionsClosed: atomic.Uint64{},

		config:     config,
		meter:      nil,
		logger:     logger,
		attributes: []attribute.KeyValue{},
	}

	err := statistics.initMetrics()

	return statistics, err
}

func key(key string) string {
	return "dataset." + key
}

func (stats *Statistics) initMetrics() error {
	// if there is no config, there is no need to initialise counters
	if stats.config == nil {
		stats.logger.Info("OTel metrics WILL NOT be collected (MeterConfig is nil)")
		return nil
	}

	// update meter with config meter
	stats.meter = stats.config.Meter()
	meter := stats.meter

	// if there is no meter, there is no need to initialise counters
	if meter == nil {
		stats.logger.Info("OTel metrics WILL NOT be collected (Meter is nil)")
		return nil
	}
	stats.logger.Info("OTel metrics WILL be collected")

	// set attributes so that we can track multiple instances
	stats.attributes = []attribute.KeyValue{
		{Key: "entity", Value: attribute.StringValue(stats.config.Entity())},
		{Key: "name", Value: attribute.StringValue(stats.config.Name())},
	}
	metric.WithAttributes(stats.attributes...)

	err := error(nil)
	stats.cAddEventsEntered, err = (*meter).Int64UpDownCounter(key("add_events_entered"))
	if err != nil {
		return err
	}
	stats.cAddEventsExited, err = (*meter).Int64UpDownCounter(key("add_events_left"))
	if err != nil {
		return err
	}

	stats.cBuffersEnqueued, err = (*meter).Int64UpDownCounter(key("buffers_enqueued"))
	if err != nil {
		return err
	}
	stats.cBuffersProcessed, err = (*meter).Int64UpDownCounter(key("buffers_processed"))
	if err != nil {
		return err
	}
	stats.cBuffersDropped, err = (*meter).Int64UpDownCounter(key("buffers_dropped"))
	if err != nil {
		return err
	}
	stats.cBuffersBroken, err = (*meter).Int64UpDownCounter(key("buffers_broken"))
	if err != nil {
		return err
	}

	stats.cEventsEnqueued, err = (*meter).Int64UpDownCounter(key("events_enqueued"))
	if err != nil {
		return err
	}
	stats.cEventsProcessed, err = (*meter).Int64UpDownCounter(key("events_processed"))
	if err != nil {
		return err
	}
	stats.cEventsDropped, err = (*meter).Int64UpDownCounter(key("events_dropped"))
	if err != nil {
		return err
	}
	stats.cEventsBroken, err = (*meter).Int64UpDownCounter(key("events_broken"))
	if err != nil {
		return err
	}

	stats.cBytesAPISent, err = (*meter).Int64UpDownCounter(key("bytes_api_sent"))
	if err != nil {
		return err
	}
	stats.cBytesAPIAccepted, err = (*meter).Int64UpDownCounter(key("bytes_api_accepted"))
	if err != nil {
		return err
	}

	stats.cSessionsOpened, err = (*meter).Int64UpDownCounter(key("sessions_started"))
	if err != nil {
		return err
	}
	stats.cSessionsClosed, err = (*meter).Int64UpDownCounter(key("sessions_finished"))
	if err != nil {
		return err
	}

	var payloadBuckets []float64
	for _, r := range [11]float64{0.001, 0.01, 0.05, 0.1, 0.2, 0.4, 0.6, 0.85, 1.0, 1.1, 2} {
		payloadBuckets = append(payloadBuckets, r*buffer_config.LimitBufferSize)
	}
	stats.logger.Info(
		"Histogram buckets for payload size: ",
		zap.Float64s("buckets", payloadBuckets),
	)
	stats.hPayloadSize, err = (*meter).Int64Histogram(key(
		"payload_size"),
		metric.WithExplicitBucketBoundaries(payloadBuckets...),
		metric.WithUnit("b"),
	)
	if err != nil {
		return err
	}

	var responseBuckets []float64
	for i := 0; i < 12; i++ {
		responseBuckets = append(responseBuckets, 4*math.Pow(2, float64(i)))
	}
	stats.logger.Info(
		"Histogram buckets for response times: ",
		zap.Float64s("buckets", responseBuckets),
	)
	stats.hResponseTime, err = (*meter).Int64Histogram(key(
		"response_time"),
		metric.WithExplicitBucketBoundaries(responseBuckets...),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return err
	}

	return err
}

func (stats *Statistics) AddEventsEntered() uint64 {
	return stats.addEventsEntered.Load()
}

func (stats *Statistics) AddEventsExited() uint64 {
	return stats.addEventsExited.Load()
}

func (stats *Statistics) BuffersEnqueued() uint64 {
	return stats.buffersEnqueued.Load()
}

func (stats *Statistics) BuffersProcessed() uint64 {
	return stats.buffersProcessed.Load()
}

func (stats *Statistics) BuffersDropped() uint64 {
	return stats.buffersDropped.Load()
}

func (stats *Statistics) BuffersBroken() uint64 {
	return stats.buffersBroken.Load()
}

func (stats *Statistics) EventsEnqueued() uint64 {
	return stats.eventsEnqueued.Load()
}

func (stats *Statistics) EventsProcessed() uint64 {
	return stats.eventsProcessed.Load()
}

func (stats *Statistics) EventsDropped() uint64 {
	return stats.eventsDropped.Load()
}

func (stats *Statistics) EventsBroken() uint64 {
	return stats.eventsBroken.Load()
}

func (stats *Statistics) BytesAPISent() uint64 {
	return stats.bytesAPISent.Load()
}

func (stats *Statistics) BytesAPIAccepted() uint64 {
	return stats.bytesAPIAccepted.Load()
}

func (stats *Statistics) SessionsOpened() uint64 {
	return stats.sessionsOpened.Load()
}

func (stats *Statistics) SessionsClosed() uint64 {
	return stats.sessionsClosed.Load()
}

func (stats *Statistics) AddEventsEnteredAdd(i uint64) {
	stats.addEventsEntered.Add(i)
	stats.add(stats.cAddEventsEntered, i)
}

func (stats *Statistics) AddEventsExitedAdd(i uint64) {
	stats.addEventsExited.Add(i)
	stats.add(stats.cAddEventsExited, i)
}

func (stats *Statistics) BuffersEnqueuedAdd(i uint64) {
	stats.buffersEnqueued.Add(i)
	stats.add(stats.cBuffersEnqueued, i)
}

func (stats *Statistics) BuffersProcessedAdd(i uint64) {
	stats.buffersProcessed.Add(i)
	stats.add(stats.cBuffersProcessed, i)
}

func (stats *Statistics) BuffersDroppedAdd(i uint64) {
	stats.buffersDropped.Add(i)
	stats.add(stats.cBuffersDropped, i)
}

func (stats *Statistics) BuffersBrokenAdd(i uint64) {
	stats.buffersBroken.Add(i)
	stats.add(stats.cBuffersBroken, i)
}

func (stats *Statistics) EventsEnqueuedAdd(i uint64) {
	stats.eventsEnqueued.Add(i)
	stats.add(stats.cEventsEnqueued, i)
}

func (stats *Statistics) EventsProcessedAdd(i uint64) {
	stats.eventsProcessed.Add(i)
	stats.add(stats.cEventsProcessed, i)
}

func (stats *Statistics) EventsDroppedAdd(i uint64) {
	stats.eventsDropped.Add(i)
	stats.add(stats.cEventsDropped, i)
}

func (stats *Statistics) EventsBrokenAdd(i uint64) {
	stats.eventsBroken.Add(i)
	stats.add(stats.cEventsBroken, i)
}

func (stats *Statistics) BytesAPISentAdd(i uint64) {
	stats.bytesAPISent.Add(i)
	stats.add(stats.cBytesAPISent, i)
}

func (stats *Statistics) BytesAPIAcceptedAdd(i uint64) {
	stats.bytesAPIAccepted.Add(i)
	stats.add(stats.cBytesAPIAccepted, i)
}

func (stats *Statistics) SessionsOpenedAdd(i uint64) {
	stats.sessionsOpened.Add(i)
	stats.add(stats.cSessionsOpened, i)
}

func (stats *Statistics) SessionsClosedAdd(i uint64) {
	stats.sessionsClosed.Add(i)
	stats.add(stats.cSessionsClosed, i)
}

func (stats *Statistics) PayloadSizeRecord(payloadSizeInBytes int64) {
	if stats.hPayloadSize != nil {
		stats.hPayloadSize.Record(
			context.Background(),
			payloadSizeInBytes,
			metric.WithAttributes(stats.attributes...),
		)
	}
}

func (stats *Statistics) ResponseTimeRecord(duration time.Duration) {
	if stats.hResponseTime != nil {
		stats.hResponseTime.Record(
			context.Background(),
			duration.Milliseconds(),
			metric.WithAttributes(stats.attributes...),
		)
	}
}

func (stats *Statistics) add(counter metric.Int64UpDownCounter, i uint64) {
	if counter != nil {
		counter.Add(
			context.Background(),
			int64(i),
			metric.WithAttributes(stats.attributes...),
		)
	}
}

// Export exports statistics related to the processing
func (stats *Statistics) Export(processingDur time.Duration) *ExportedStatistics {
	// log add events statistics
	addEventsStats := AddEventsStats{
		entered:        stats.addEventsEntered.Load(),
		exited:         stats.addEventsExited.Load(),
		processingTime: processingDur,
	}

	// log buffer stats
	bProcessed := stats.buffersProcessed.Load()
	bEnqueued := stats.buffersEnqueued.Load()
	bDropped := stats.buffersDropped.Load()
	bBroken := stats.buffersBroken.Load()

	buffersStats := QueueStats{
		bEnqueued,
		bProcessed,
		bDropped,
		bBroken,
		processingDur,
	}

	// log events stats
	eProcessed := stats.eventsProcessed.Load()
	eEnqueued := stats.eventsEnqueued.Load()
	eDropped := stats.eventsDropped.Load()
	eBroken := stats.eventsBroken.Load()

	eventsStats := QueueStats{
		eEnqueued,
		eProcessed,
		eDropped,
		eBroken,
		processingDur,
	}

	// log transferred stats
	bAPISent := stats.bytesAPISent.Load()
	bAPIAccepted := stats.bytesAPIAccepted.Load()
	transferStats := TransferStats{
		bAPISent,
		bAPIAccepted,
		bProcessed,
		processingDur,
	}

	// log session stats
	sessionsStats := SessionsStats{
		sessionsOpened: stats.SessionsOpened(),
		sessionsClosed: stats.SessionsClosed(),
	}

	return &ExportedStatistics{
		AddEvents: addEventsStats,
		Buffers:   buffersStats,
		Events:    eventsStats,
		Transfer:  transferStats,
		Sessions:  sessionsStats,
	}
}

type AddEventsStats struct {
	entered uint64 `mapstructure:"entered"`
	exited  uint64 `mapstructure:"exited"`
	// processingTime is duration of the processing
	processingTime time.Duration `mapstructure:"processingTime"`
}

func (stats AddEventsStats) Entered() uint64 {
	return stats.entered
}

func (stats AddEventsStats) Exited() uint64 {
	return stats.exited
}

func (stats AddEventsStats) Waiting() uint64 {
	return stats.Entered() - stats.Exited()
}

// ProcessingTime is duration of the processing
func (stats AddEventsStats) ProcessingTime() time.Duration {
	return stats.processingTime
}

// QueueStats stores statistics related to the queue processing
type QueueStats struct {
	// enqueued is number of items that has been accepted for processing
	enqueued uint64 `mapstructure:"enqueued"`
	// Processed is number of items that has been successfully processed
	processed uint64 `mapstructure:"processed"`
	// dropped is number of items that has been dropped since they couldn't be processed
	dropped uint64 `mapstructure:"dropped"`
	// broken is number of items that has been damaged by queue, should be zero
	broken uint64 `mapstructure:"broken"`
	// processingTime is duration of the processing
	processingTime time.Duration `mapstructure:"processingTime"`
}

// Enqueued is number of items that has been accepted for processing
func (stats QueueStats) Enqueued() uint64 {
	return stats.enqueued
}

// Processed is number of items that has been successfully processed
func (stats QueueStats) Processed() uint64 {
	return stats.processed
}

// Dropped is number of items that has been dropped since they couldn't be processed
func (stats QueueStats) Dropped() uint64 {
	return stats.dropped
}

// Broken is number of items that has been damaged by queue, should be zero
func (stats QueueStats) Broken() uint64 {
	return stats.broken
}

// Waiting is number of items that are waiting for being processed
// Enqueued - Processed - Dropped - Broken
func (stats QueueStats) Waiting() uint64 {
	return stats.enqueued - stats.processed - stats.dropped - stats.broken
}

// SuccessRate of items processing
// (Processed - Dropped - Broken) / Processed
func (stats QueueStats) SuccessRate() float64 {
	if stats.processed > 0 {
		return float64(stats.processed-stats.dropped-stats.broken) / float64(stats.processed)
	} else {
		return 0.0
	}
}

// ProcessingTime is duration of the processing
func (stats QueueStats) ProcessingTime() time.Duration {
	return stats.processingTime
}

// TransferStats stores statistics related to the data transfers
type TransferStats struct {
	// bytesSent is the amount of bytes that were sent to the server
	// each retry is counted
	bytesSent uint64 `mapstructure:"bytesSentMB"`
	// bytesAccepted is the amount of MB that were accepted by the server
	// retries are not counted
	bytesAccepted uint64 `mapstructure:"bytesAcceptedMB"`
	// buffersProcessed is number of processed buffers
	buffersProcessed uint64
	// processingTime is duration of the processing
	processingTime time.Duration `mapstructure:"processingTime"`
}

// BytesSent is the amount of bytes that were sent to the server
// each retry is counted
func (stats TransferStats) BytesSent() uint64 {
	return stats.bytesSent
}

// BytesAccepted is the amount of MB that were accepted by the server
// retries are not counted
func (stats TransferStats) BytesAccepted() uint64 {
	return stats.bytesAccepted
}

// BuffersProcessed is number of processed buffers
func (stats TransferStats) BuffersProcessed() uint64 {
	return stats.buffersProcessed
}

// ThroughputBpS is the throughput based on BytesAccepted
func (stats TransferStats) ThroughputBpS() float64 {
	return float64(stats.bytesAccepted) / stats.processingTime.Seconds()
}

// SuccessRate of the transfer - BytesAccepted / BytesSent
func (stats TransferStats) SuccessRate() float64 {
	if stats.bytesSent > 0 {
		return float64(stats.bytesAccepted) / float64(stats.bytesSent)
	} else {
		return 0.0
	}
}

// AvgBufferBytes is average buffer size in bytes - BytesAccepted / BuffersProcessed
func (stats TransferStats) AvgBufferBytes() float64 {
	if stats.buffersProcessed > 0 {
		return float64(stats.bytesAccepted) / float64(stats.buffersProcessed)
	} else {
		return 0
	}
}

// ProcessingTime is duration of the processing
func (stats TransferStats) ProcessingTime() time.Duration {
	return stats.processingTime
}

// SessionsStats stores statistics related to sessions.
type SessionsStats struct {
	// sessionsOpened is the number of opened sessions
	sessionsOpened uint64 `mapstructure:"sessionsOpened"`
	// sessionsClosed is the number of closed sessions
	sessionsClosed uint64 `mapstructure:"sessionsClosed"`
}

// SessionsOpened is the number of opened sessions
func (stats SessionsStats) SessionsOpened() uint64 {
	return stats.sessionsOpened
}

// SessionsClosed is the number of closed sessions
func (stats SessionsStats) SessionsClosed() uint64 {
	return stats.sessionsClosed
}

// SessionsActive is the number of active sessions
func (stats SessionsStats) SessionsActive() uint64 {
	return stats.sessionsOpened - stats.sessionsClosed
}

// ExportedStatistics store statistics related to the library execution
// These are statistics from the beginning of the processing
type ExportedStatistics struct {
	AddEvents AddEventsStats `mapstructure:"addEvents"`
	// Events stores statistics about processing events
	Events QueueStats `mapstructure:"events"`
	// Buffers stores statistics about processing buffers
	Buffers QueueStats `mapstructure:"buffers"`
	// Transfer stores statistics about data transfers
	Transfer TransferStats `mapstructure:"transfer"`
	// Sessions stores statistics about sessions
	Sessions SessionsStats `mapstructure:"sessions"`
}
