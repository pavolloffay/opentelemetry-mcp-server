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

package client

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/scalyr/dataset-go/pkg/session_manager"

	"github.com/scalyr/dataset-go/pkg/meter_config"

	"golang.org/x/exp/slices"

	"github.com/scalyr/dataset-go/pkg/version"

	"github.com/cenkalti/backoff/v4"

	_ "net/http/pprof"

	"github.com/scalyr/dataset-go/pkg/api/add_events"
	"github.com/scalyr/dataset-go/pkg/buffer"
	"github.com/scalyr/dataset-go/pkg/config"
	"github.com/scalyr/dataset-go/pkg/server_host_config"
	"github.com/scalyr/dataset-go/pkg/statistics"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	HttpErrorCannotConnect = 600
	HttpErrorCannotProcess = 601
)

// isOkStatus returns true if status code is 200, false otherwise.
func isOkStatus(status uint32) bool {
	return status == http.StatusOK
}

// isRetryableStatus returns true if status code is 401, 403, 429, or any 5xx, false otherwise.
// Note from DataSet API request handling perspective 401 and 403 may be considered as retryable, see DSET-2201
func isRetryableStatus(status uint32) bool {
	return status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

type Purge struct{}

// DataSetClient represent a DataSet REST API client
type DataSetClient struct {
	Id             uuid.UUID
	Config         *config.DataSetConfig
	Client         *http.Client
	LastHttpStatus atomic.Uint32
	lastError      error
	lastErrorTs    time.Time
	lastErrorMu    sync.RWMutex
	retryAfter     time.Time
	retryAfterMu   sync.RWMutex
	// indicates that client has been shut down and no further processing is possible
	finished atomic.Bool
	Logger   *zap.Logger

	firstReceivedAt atomic.Int64
	// timestamp of last event so far processed by client
	lastAcceptedAt atomic.Int64
	// Stores sanitized complete URL to the addEvents API endpoint, e.g.
	// https://app.scalyr.com/api/addEvents
	addEventsEndpointUrl string
	userAgent            string
	serverHost           string

	statistics     *statistics.Statistics
	sessionManager *session_manager.SessionManager

	eventsProcessingDone  chan struct{}
	buffersProcessingDone chan struct{}

	bufferSendingSema chan struct{}
	bufferChannel     chan *buffer.Buffer
}

func NewClient(
	cfg *config.DataSetConfig,
	client *http.Client,
	logger *zap.Logger,
	userAgentSuffix *string,
	meterConfig *meter_config.MeterConfig,
) (*DataSetClient, error) {
	logger.Info(
		"Using config: ",
		zap.String("config", cfg.String()),
		zap.String("version", version.Version),
		zap.String("releaseDate", version.ReleasedDate),
	)

	validationErr := cfg.Validate()
	if validationErr != nil {
		return nil, validationErr
	}

	// update group by, so that logs from the same host
	// belong to the same session
	adjustGroupByWithSpecialAttributes(cfg)
	logger.Info(
		"Adjusted config: ",
		zap.String("config", cfg.String()),
	)

	serverHost, err := getServerHost(cfg.ServerHostSettings)
	if err != nil {
		return nil, fmt.Errorf("it was not get server host: %w", err)
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("it was not possible to generate UUID: %w", err)
	}

	addEventsEndpointUrl := cfg.Endpoint
	if strings.HasSuffix(addEventsEndpointUrl, "/") {
		addEventsEndpointUrl += "api/addEvents"
	} else {
		addEventsEndpointUrl += "/api/addEvents"
	}

	userAgent := fmt.Sprintf(
		"%s;%s;%s;%s;%s;%s;%d",
		"dataset-go",
		version.Version,
		version.ReleasedDate,
		id,
		runtime.GOOS,
		runtime.GOARCH,
		runtime.NumCPU(),
	)
	if userAgentSuffix != nil && *userAgentSuffix != "" {
		userAgent = userAgent + ";" + *userAgentSuffix
	}
	logger.Info("Using User-Agent: ", zap.String("User-Agent", userAgent))

	stats, err := statistics.NewStatistics(meterConfig, logger)
	if err != nil {
		return nil, fmt.Errorf("it was not possible to create statistics: %w", err)
	}

	dataClient := &DataSetClient{
		Id:                   id,
		Config:               cfg,
		Client:               client,
		LastHttpStatus:       atomic.Uint32{},
		retryAfter:           time.Now(),
		retryAfterMu:         sync.RWMutex{},
		lastErrorMu:          sync.RWMutex{},
		Logger:               logger,
		finished:             atomic.Bool{},
		firstReceivedAt:      atomic.Int64{},
		lastAcceptedAt:       atomic.Int64{},
		addEventsEndpointUrl: addEventsEndpointUrl,
		userAgent:            userAgent,
		serverHost:           serverHost,
		statistics:           stats,

		eventsProcessingDone:  make(chan struct{}),
		buffersProcessingDone: make(chan struct{}),

		bufferSendingSema: make(chan struct{}, cfg.BufferSettings.MaxParallelOutgoing),
		bufferChannel:     make(chan *buffer.Buffer),
	}

	dataClient.sessionManager = session_manager.New(
		logger,
		dataClient.processEvents,
	)

	// run statistics sweeper
	go dataClient.statisticsSweeper()

	go dataClient.mainLoop()

	dataClient.Logger.Info("DataSetClient was created",
		zap.String("id", dataClient.Id.String()),
	)

	//// Server for pprof
	//go func() {
	//	fmt.Println(http.ListenAndServe("localhost:6060", nil))
	//}()

	return dataClient, nil
}

func getServerHost(settings server_host_config.DataSetServerHostSettings) (string, error) {
	err := settings.Validate()
	if err != nil {
		return "", err
	}
	if len(settings.ServerHost) > 0 {
		return settings.ServerHost, nil
	}
	return os.Hostname()
}

// adjustGroupByWithSpecialAttributes adds attributes that have special meaning in the UI
// serverHost and logfile are used in the drop-down, so they have to be part of the
// SessionInfo.
func adjustGroupByWithSpecialAttributes(cfg *config.DataSetConfig) {
	groupBy := cfg.BufferSettings.GroupBy
	if !slices.Contains(groupBy, add_events.AttrLogFile) {
		groupBy = append(groupBy, add_events.AttrLogFile)
	}
	if !slices.Contains(groupBy, add_events.AttrServerHost) {
		groupBy = append(groupBy, add_events.AttrServerHost)
	}

	cfg.BufferSettings.GroupBy = groupBy
}

func (client *DataSetClient) initBuffer(buff *buffer.Buffer, info *add_events.SessionInfo) {
	client.Logger.Debug("Creating new buf",
		zap.String("uuid", buff.Id.String()),
		zap.String("session", buff.Session),
	)

	// Initialise
	err := buff.Initialise(info)
	// only reason why Initialise may fail is because SessionInfo cannot be
	// serialised into JSON. We do not care, so we can set it to nil and continue
	if err != nil {
		client.Logger.Error("Cannot initialize buffer: %w", zap.Error(err))
		err = buff.SetSessionInfo(nil)
		if err != nil {
			panic(fmt.Sprintf("Setting SessionInfo cannot cause an error: %s", err))
		}
	}
}

func (client *DataSetClient) callServer(buf *buffer.Buffer) {
	client.bufferSendingSema <- struct{}{}
	defer func() { <-client.bufferSendingSema }() // release token

	// sleep until retry time
	for client.RetryAfter().After(time.Now()) {
		client.sleep(client.RetryAfter(), buf)
	}
	if !buf.HasEvents() {
		client.Logger.Warn("Buffer is empty, skipping", buf.ZapStats()...)
		client.statistics.BuffersProcessedAdd(1)
	}
	if client.sendBufferWithRetryPolicy(buf) {
		client.statistics.BuffersProcessedAdd(1)
	}

	client.lastAcceptedAt.Store(time.Now().UnixNano())
}

// Sends buffer to DataSet. If not succeeds and try is possible (it retryable), try retry until possible (timeout)
func (client *DataSetClient) sendBufferWithRetryPolicy(buf *buffer.Buffer) bool {
	expBackoff := client.createBackOff(client.Config.BufferSettings.RetryMaxElapsedTime)
	expBackoff.Reset()
	retryNum := int64(0)
	for {
		response, payloadLen, err := client.sendAddEventsBuffer(buf)
		client.setLastError(err)
		lastHttpStatus := uint32(0)
		if err != nil {
			client.Logger.Error("unable to send addEvents buffers", zap.Error(err))
			client.setLastErrorTimestamp(time.Now())
			if strings.Contains(err.Error(), errMsgUnableToSentRequest) {
				lastHttpStatus = HttpErrorCannotConnect
			} else {
				lastHttpStatus = HttpErrorCannotProcess
			}
			client.LastHttpStatus.Store(lastHttpStatus)
		}
		zaps := make([]zap.Field, 0)
		if response.ResponseObj != nil {
			zaps = append(
				zaps,
				zap.String("httpStatus", response.ResponseObj.Status),
				zap.Int("httpCode", response.ResponseObj.StatusCode),
			)
			lastHttpStatus = uint32(response.ResponseObj.StatusCode)
			client.LastHttpStatus.Store(lastHttpStatus)
		}

		zaps = append(
			zaps,
			zap.String("status", response.Status),
			zap.String("message", response.Message),
		)
		client.Logger.Debug("Events were sent to DataSet",
			zaps...,
		)

		if isOkStatus(lastHttpStatus) {
			// everything was fine, there is no need for retries
			client.statistics.BytesAPIAcceptedAdd(uint64(payloadLen))
			return true // exit loop (buffer sent)
		}

		backoffDelay := expBackoff.NextBackOff()
		client.Logger.Info("Backoff + Retries: ", zap.Int64("retryNum", retryNum), zap.Duration("backoffDelay", backoffDelay))
		if backoffDelay == backoff.Stop {
			// throw away the batch
			err = fmt.Errorf("max elapsed time expired %w", err)
			client.onBufferDrop(buf, lastHttpStatus, err)
			return false // exit loop (failed to send buffer)
		}

		if isRetryableStatus(lastHttpStatus) {
			// check whether header is specified and get its value
			retryAfter, specified := client.getRetryAfter(
				response.ResponseObj,
				backoffDelay,
			)

			if specified {
				// retry after is specified, we should update
				// client state, so we do not send more requests

				client.setRetryAfter(retryAfter)
			}

			client.sleep(retryAfter, buf)
		} else {
			err = fmt.Errorf("non recoverable error %w", err)
			client.onBufferDrop(buf, lastHttpStatus, err)
			return false // exit loop (failed to send buffer)
		}
		retryNum++
	}
}

func (client *DataSetClient) statisticsSweeper() {
	// TODO: remove me :)
	for i := uint64(0); ; i++ {
		client.logStatistics()
		// wait for some time before new sweep
		time.Sleep(15 * time.Second)
	}
}

func (client *DataSetClient) mainLoop() {
	statsTicker := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-client.buffersProcessingDone:
			client.Logger.Info("Stopping main loop")
			for buf := range client.bufferChannel {
				client.callServer(buf)
			}
			return
		case buf, ok := <-client.bufferChannel:
			if !ok {
				return
			}
			client.callServer(buf)

		case <-statsTicker.C:
			client.logStatistics()
		}
	}
}

func (client *DataSetClient) createBackOff(maxElapsedTime time.Duration) backoff.BackOff {
	return &backoff.ExponentialBackOff{
		InitialInterval:     client.Config.BufferSettings.RetryInitialInterval,
		RandomizationFactor: client.Config.BufferSettings.RetryRandomizationFactor,
		Multiplier:          client.Config.BufferSettings.RetryMultiplier,
		MaxInterval:         client.Config.BufferSettings.RetryMaxInterval,
		MaxElapsedTime:      maxElapsedTime,
		Stop:                backoff.Stop,
		Clock:               backoff.SystemClock,
	}
}

// Statistics returns statistics about events, buffers processing from the start time
func (client *DataSetClient) Statistics() *statistics.ExportedStatistics {
	// for how long are events being processed
	firstAt := time.Unix(0, client.firstReceivedAt.Load())
	lastAt := time.Unix(0, client.lastAcceptedAt.Load())
	processingDur := lastAt.Sub(firstAt)
	processingInSec := processingDur.Seconds()

	// if nothing was processed, do not log statistics
	if processingInSec <= 0 {
		return nil
	}

	return client.statistics.Export(processingDur)
}

func (client *DataSetClient) logStatistics() {
	stats := client.Statistics()
	if stats == nil {
		return
	}

	a := stats.AddEvents
	client.Logger.Info(
		"Add Events' Stats:",
		zap.Uint64("entered", a.Entered()),
		zap.Uint64("exited", a.Exited()),
		zap.Uint64("waiting", a.Waiting()),
		zap.Float64("processingS", a.ProcessingTime().Seconds()),
		zap.Duration("processing", a.ProcessingTime()),
	)

	// log events stats
	e := stats.Events
	client.Logger.Info(
		"Events' Queue Stats:",
		zap.Uint64("processed", e.Processed()),
		zap.Uint64("enqueued", e.Enqueued()),
		zap.Uint64("dropped", e.Dropped()),
		zap.Uint64("broken", e.Broken()),
		zap.Uint64("waiting", e.Waiting()),
		zap.Float64("successRate", e.SuccessRate()),
		zap.Float64("processingS", e.ProcessingTime().Seconds()),
		zap.Duration("processing", e.ProcessingTime()),
	)

	b := stats.Buffers
	client.Logger.Info(
		"Buffers' Queue Stats:",
		zap.Uint64("processed", b.Processed()),
		zap.Uint64("enqueued", b.Enqueued()),
		zap.Uint64("dropped", b.Dropped()),
		zap.Uint64("broken", b.Broken()),
		zap.Uint64("waiting", b.Waiting()),
		zap.Float64("successRate", b.SuccessRate()),
		zap.Float64("processingS", b.ProcessingTime().Seconds()),
		zap.Duration("processing", b.ProcessingTime()),
	)

	// log transferred stats
	mb := float64(1024 * 1024)
	t := stats.Transfer
	client.Logger.Info(
		"Transfer Stats:",
		zap.Float64("bytesSentMB", float64(t.BytesSent())/mb),
		zap.Float64("bytesAcceptedMB", float64(t.BytesAccepted())/mb),
		zap.Float64("throughputMBpS", t.ThroughputBpS()/mb),
		zap.Uint64("buffersProcessed", t.BuffersProcessed()),
		zap.Float64("perBufferMB", t.AvgBufferBytes()/mb),
		zap.Float64("successRate", t.SuccessRate()),
		zap.Float64("processingS", t.ProcessingTime().Seconds()),
		zap.Duration("processing", t.ProcessingTime()),
	)

	// log session stats
	s := stats.Sessions
	client.Logger.Info(
		"Sessions Stats:",
		zap.Uint64("sessionsOpened", s.SessionsOpened()),
		zap.Uint64("sessionsClosed", s.SessionsClosed()),
		zap.Uint64("sessionsActive", s.SessionsActive()),
	)
}

func (client *DataSetClient) publishBuffer(buf *buffer.Buffer) {
	client.Logger.Debug("publishing buffer", buf.ZapStats()...)

	// publish buffer so it can be sent
	client.statistics.BuffersEnqueuedAdd(+1)
	client.statistics.EventsProcessedAdd(uint64(buf.CountEvents()))
	client.bufferChannel <- buf
}

// Exporter rejects handling of incoming batches if is in error state
func (client *DataSetClient) isInErrorState() (bool, error) {
	// In case one of session failed (with retryable status) to send request (batch of event to DataSet), client retries sending this request. Unless retry attempts timed out
	if isRetryableStatus(client.LastHttpStatus.Load()) && !client.isLastRetryableErrorTimedOut() {
		err := client.LastError()
		if err != nil {
			return true, fmt.Errorf("rejecting - Last HTTP request contains an error: %w", err)
		} else {
			return true, fmt.Errorf("rejecting - Last HTTP request had retryable status")
		}
	}

	// DataSet can limit rate using RetryAfter header. In such case client rejects incoming events
	if client.RetryAfter().After(time.Now()) {
		return true, fmt.Errorf("rejecting - should retry after %s", client.RetryAfter().Format(time.RFC1123))
	}

	return false, nil
}

func (client *DataSetClient) getRetryAfter(response *http.Response, def time.Duration) (time.Time, bool) {
	if response == nil {
		return time.Now().Add(def), false
	}
	after := response.Header.Get("Retry-After")

	// if it's not specified, return default
	if len(after) == 0 {
		return time.Now().Add(def), false
	}

	// it is specified in seconds?
	afterS, errS := strconv.Atoi(after)
	if errS == nil {
		// it's integer => so lets convert it
		return time.Now().Add(time.Duration(afterS) * time.Second), true
	}

	// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Date
	afterT, errT := time.Parse(time.RFC1123, after)
	if errT == nil {
		return afterT, true
	}

	client.Logger.Warn("Illegal value of Retry-After, using default",
		zap.String("retryAfter", after),
		zap.Duration("default", def),
		zap.Error(errS),
		zap.Error(errT),
	)

	return time.Now().Add(def), false
}

// TODO move this function outside of DataSetClient to not misunderstand that shared client is sleeping. It each go routine which is sleeping.
func (client *DataSetClient) sleep(retryAfter time.Time, buffer *buffer.Buffer) {
	// truncate current time to wait a little bit longer
	sleepFor := retryAfter.Sub(time.Now().Truncate(time.Second))
	client.Logger.Info(
		"RetryAfter is in the future, waiting...",
		zap.Duration("sleepFor", sleepFor),
		zap.String("bufferSession", buffer.Session),
		zap.String("bufferUuid", buffer.Id.String()),
	)
	time.Sleep(sleepFor)
}

func (client *DataSetClient) LastError() error {
	client.lastErrorMu.RLock()
	defer client.lastErrorMu.RUnlock()
	return client.lastError
}

func (client *DataSetClient) setLastError(err error) {
	client.lastErrorMu.Lock()
	defer client.lastErrorMu.Unlock()
	client.lastError = err
}

func (client *DataSetClient) RetryAfter() time.Time {
	client.retryAfterMu.RLock()
	defer client.retryAfterMu.RUnlock()
	return client.retryAfter
}

func (client *DataSetClient) setRetryAfter(t time.Time) {
	client.retryAfterMu.Lock()
	defer client.retryAfterMu.Unlock()
	client.retryAfter = t
}

func (client *DataSetClient) onBufferDrop(buf *buffer.Buffer, status uint32, err error) {
	client.statistics.BuffersDroppedAdd(1)
	client.Logger.Error("Dropping buffer",
		buf.ZapStats(
			zap.Uint32("httpStatus", status),
			zap.Error(err),
		)...,
	)
}

func (client *DataSetClient) setLastErrorTimestamp(timestamp time.Time) {
	client.lastErrorMu.Lock()
	defer client.lastErrorMu.Unlock()
	client.lastErrorTs = timestamp
}

func (client *DataSetClient) lastErrorTimestamp() time.Time {
	client.retryAfterMu.RLock()
	defer client.retryAfterMu.RUnlock()
	return client.lastErrorTs
}

func (client *DataSetClient) isLastRetryableErrorTimedOut() bool {
	return client.lastErrorTimestamp().Add(client.Config.BufferSettings.RetryMaxElapsedTime).Before(time.Now())
}

func (client *DataSetClient) ServerHost() string {
	return client.serverHost
}
