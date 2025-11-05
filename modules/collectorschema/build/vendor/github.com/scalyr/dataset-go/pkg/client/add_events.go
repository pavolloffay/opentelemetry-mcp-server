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
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/scalyr/dataset-go/pkg/api/response"

	"github.com/scalyr/dataset-go/pkg/api/add_events"
	"github.com/scalyr/dataset-go/pkg/api/request"
	"github.com/scalyr/dataset-go/pkg/buffer"

	"go.uber.org/zap"
)

const (
	errMsgUnableToSentRequest   = "unable to send request"
	errMsgUnableToReadResponse  = "unable to read response"
	errMsgUnableToParseResponse = "unable to parse response"
)

/*
Wrapper around: https://app.scalyr.com/help/api#addEvents
*/

type EventWithMeta struct {
	EventBundle *add_events.EventBundle
	Key         string
	SessionInfo add_events.SessionInfo
}

func NewEventWithMeta(
	bundle *add_events.EventBundle,
	groupBy []string,
	serverHost string,
	debug bool,
) EventWithMeta {
	// initialise
	key := ""
	info := make(add_events.SessionInfo)

	// adjust server host attribute
	adjustServerHost(bundle, serverHost)

	// construct Key
	bundleKey := extractKeyAndUpdateInfo(bundle, groupBy, key, info)

	// in debug mode include bundleKey
	if debug {
		info[add_events.AttrSessionKey] = bundleKey
	}

	return EventWithMeta{
		EventBundle: bundle,
		Key:         bundleKey,
		SessionInfo: info,
	}
}

func extractKeyAndUpdateInfo(bundle *add_events.EventBundle, groupBy []string, key string, info add_events.SessionInfo) string {
	// construct key and move attributes from attrs to sessionInfo
	for _, k := range groupBy {
		val, ok := bundle.Event.Attrs[k]
		key += k + ":"
		if ok {
			key += fmt.Sprintf("%s", val)

			// move to session info and remove from attributes
			info[k] = val
			delete(bundle.Event.Attrs, k)
		}
		key += "___DELIM___"
	}

	// use md5 to shorten the key
	hash := md5.Sum([]byte(key))
	return hex.EncodeToString(hash[:])
}

func adjustServerHost(bundle *add_events.EventBundle, serverHost string) {
	// figure out serverHost value
	usedServerHost := serverHost
	// if event's ServerHost is set, use it
	if len(bundle.Event.ServerHost) > 0 {
		usedServerHost = bundle.Event.ServerHost
	} else {
		// if somebody is using library directly and forget to set Event.ServerHost,
		// lets check the attributes first

		// check serverHost attribute
		attrHost, ok := bundle.Event.Attrs[add_events.AttrServerHost]
		if ok {
			usedServerHost = attrHost.(string)
		} else {
			// if serverHost attribute is not set, check the __origServerHost
			attrOrigHost, okOrig := bundle.Event.Attrs[add_events.AttrOrigServerHost]
			if okOrig {
				usedServerHost = attrOrigHost.(string)
			}
		}
	}

	// for the SessionInfo, it has to be in the serverHost
	// therefore remove the orig and set the serverHost
	// so that it can be in the next step moved to sessionInfo
	delete(bundle.Event.Attrs, add_events.AttrOrigServerHost)
	bundle.Event.Attrs[add_events.AttrServerHost] = usedServerHost
}

// AddEvents enqueues given events for processing (sending to Dataset).
// It returns an error if the batch was not accepted (e.g. exporter in error state and retrying handle previous batches or client is being shutdown).
func (client *DataSetClient) AddEvents(bundles []*add_events.EventBundle) error {
	client.statistics.AddEventsEnteredAdd(1)
	defer func() { client.statistics.AddEventsExitedAdd(1) }()

	if client.finished.Load() {
		return fmt.Errorf("client has finished - rejecting all new events")
	}

	shouldReject, errR := client.isInErrorState()
	if shouldReject {
		return fmt.Errorf("AddEvents - reject batch: %w", errR)
	}

	// then, figure out which keys are part of the batch
	// store there information about the host
	bundlesWithMeta := make(map[string][]EventWithMeta)
	for _, bundle := range bundles {
		bWM := NewEventWithMeta(bundle, client.Config.BufferSettings.GroupBy, client.serverHost, client.Config.Debug)

		session := fmt.Sprintf("%s-%s", client.Id, bWM.Key)
		list, found := bundlesWithMeta[session]
		if !found {
			bundlesWithMeta[session] = []EventWithMeta{bWM}
		} else {
			bundlesWithMeta[session] = append(list, bWM)
		}
	}

	// update time when the first batch was received
	if client.firstReceivedAt.Load() == 0 {
		client.firstReceivedAt.Store(time.Now().UnixNano())
	}

	for key, list := range bundlesWithMeta {
		// start listening for events
		// if the purge time for buffer is just few ms, this is the cause of problems
		// in real life the purge time should be in minutes.
		client.sessionManager.Sub(key)
		for _, bundle := range list {
			client.sessionManager.Pub(key, bundle)
			client.statistics.EventsEnqueuedAdd(1)
		}
	}

	return nil
}

func (client *DataSetClient) newBufferForEvents(session string, info *add_events.SessionInfo) *buffer.Buffer {
	buf := buffer.NewEmptyBuffer(session, client.Config.Tokens.WriteLog)

	client.initBuffer(buf, info)

	return buf
}

func (client *DataSetClient) processEvents(key string, eventsChannel <-chan interface{}) {
	client.Logger.Info("Listening to events with key - BEGIN",
		zap.String("key", key),
	)
	client.statistics.SessionsOpenedAdd(1)

	defer func() {
		client.Logger.Info("Listening to events with key - END",
			zap.String("key", key),
		)
		client.statistics.SessionsClosedAdd(1)
	}()

	publish := func(key string, buf *buffer.Buffer) *buffer.Buffer {
		if buf == nil {
			return nil
		}
		if !buf.HasEvents() {
			return buf
		}
		client.publishBuffer(buf)
		return client.newBufferForEvents(buf.Session, buf.SessionInfo())
	}

	process := func(buf *buffer.Buffer, bundle EventWithMeta) *buffer.Buffer {
		defer func() {
			client.lastAcceptedAt.Store(time.Now().UnixNano())
		}()
		if buf == nil {
			buf = client.newBufferForEvents(key, &bundle.SessionInfo)
		}
		added, err := buf.AddBundle(bundle.EventBundle)
		if err != nil {
			client.Logger.Warn("Cannot add bundle", zap.Error(err))
		}

		if buf.ShouldSendSize() || added == buffer.TooMuch && buf.HasEvents() {
			buf = publish(key, buf)
		}

		if added == buffer.TooMuch {
			added, err = buf.AddBundle(bundle.EventBundle)
			if err != nil {
				client.Logger.Error("Cannot add bundle", zap.Error(err))
				client.statistics.EventsDroppedAdd(1)
				return buf
			}
			if buf.ShouldSendSize() {
				buf = publish(key, buf)
			}
			if added == buffer.TooMuch {
				client.Logger.Fatal("Bundle was not added for second time!", buf.ZapStats()...)
				client.statistics.EventsDroppedAdd(1)
				return buf
			}
		}
		return buf
	}

	// create ticker to submit
	lifeTime := time.Hour
	if client.Config.BufferSettings.MaxLifetime > 0 {
		lifeTime = client.Config.BufferSettings.MaxLifetime
	}
	tickerLifetime := time.NewTicker(lifeTime)
	defer func() {
		tickerLifetime.Stop()
	}()

	purgeTime := time.Hour
	if client.Config.BufferSettings.PurgeOlderThan > 0 {
		purgeTime = client.Config.BufferSettings.PurgeOlderThan
	}
	tickerPurge := time.NewTicker(purgeTime)
	defer func() {
		tickerPurge.Stop()
	}()

	var buf *buffer.Buffer = nil
	for {
		select {
		case <-client.eventsProcessingDone:
			client.Logger.Debug("Shutting down listener for key", zap.String("key", key))
			buf = publish(key, buf)
			for msg := range eventsChannel {
				event, ok := msg.(EventWithMeta)
				if !ok {
					client.statistics.EventsBrokenAdd(1)
				} else {
					buf = process(buf, event)
					buf = publish(key, buf)
				}
			}
			buf = publish(key, buf)
			return
		case msg, ok := <-eventsChannel:
			// client.Logger.Debug("Processing message for key", zap.String("key", key))
			if !ok {
				return
			}
			event, ok := msg.(EventWithMeta)
			if !ok {
				client.statistics.EventsBrokenAdd(1)
			} else {
				buf = process(buf, event)
			}
			tickerLifetime.Reset(lifeTime)
			tickerPurge.Reset(purgeTime)
		case <-tickerLifetime.C:
			// client.Logger.Debug("Processing life-time ticker for key", zap.String("key", key))
			buf = publish(key, buf)
		case <-tickerPurge.C:
			// client.Logger.Debug("Processing purge ticker for key", zap.String("key", key))
			// if buffer is null => we haven't received event
			// lets wait until we receive something
			if buf == nil {
				continue
			}
			if buf.ShouldPurgeAge(purgeTime) {
				client.Logger.Info("Purging key", zap.String("key", key))
				client.sessionManager.Unsub(key)
				return
			}
		}
	}
}

// isProcessingBuffers returns True if there are still some unprocessed buffers.
// False otherwise.
func (client *DataSetClient) isProcessingBuffers() bool {
	return client.statistics.BuffersEnqueued() > (client.statistics.BuffersProcessed() + client.statistics.BuffersDropped() + client.statistics.BuffersBroken())
}

// isProcessingEvents returns True if there are still some unprocessed events.
// False otherwise.
func (client *DataSetClient) isProcessingEvents() bool {
	return client.statistics.EventsEnqueued() > (client.statistics.EventsProcessed() + client.statistics.EventsDropped() + client.statistics.EventsBroken())
}

// isProcessingEvents returns True if there are still some unprocessed events.
// False otherwise.
func (client *DataSetClient) isProcessingAddEvents() bool {
	return client.statistics.AddEventsEntered() > client.statistics.AddEventsExited()
}

// Shutdown takes care of shutdown of client. It does following steps
// - stops processing of new events,
// - tries (till 50% of shutdownMaxTimeout) to finish processing of all the incoming events
// - tries (till 80% of shutdownMaxTimeout) to process (add into buffers) all the events,
// - tries (till 100% of shutdownMaxTimeout) to send processed events (buffers) into DataSet
func (client *DataSetClient) Shutdown() error {
	client.Logger.Info("Shutting down - BEGIN")
	// start measuring processing time
	processingStart := time.Now()

	// mark as finished to prevent processing of further events
	client.finished.Store(true)

	// log statistics when finish was called
	client.logStatistics()

	useUpTo := func(ratio float64, start time.Time, end time.Time) time.Duration {
		elapsedFromStart := time.Since(start)
		expectedFromStart := time.Duration(float64(end.Sub(start)) * ratio)
		return expectedFromStart - elapsedFromStart
	}

	retryShutdownTimeout := client.Config.BufferSettings.RetryShutdownTimeout
	expFinishTime := processingStart.Add(retryShutdownTimeout)

	maxTimeForAddEvents := useUpTo(0.5, processingStart, expFinishTime)
	client.Logger.Info(
		"Shutting down - waiting for incoming add_events",
		zap.Time("expFinishTime", expFinishTime),
		zap.Duration("maxElapsedTime", maxTimeForAddEvents),
		zap.Duration("retryShutdownTimeout", retryShutdownTimeout),
		zap.Duration("elapsedTime", time.Since(processingStart)),
	)

	var lastError error = nil
	expBackoff := client.createBackOff(maxTimeForAddEvents)
	expBackoff.Reset()
	retryNum := 0
	for client.isProcessingAddEvents() {
		// log statistics
		client.logStatistics()

		backoffDelay := expBackoff.NextBackOff()
		client.Logger.Info(
			"Shutting down - processing incoming add_events",
			zap.Int("retryNum", retryNum),
			zap.Duration("backoffDelay", backoffDelay),
			zap.Uint64("addEventsEntered", client.statistics.AddEventsEntered()),
			zap.Uint64("addEventsExited", client.statistics.AddEventsExited()),
			zap.Time("expFinishTime", expFinishTime),
			zap.Duration("elapsedTime", time.Since(processingStart)),
			zap.Duration("maxElapsedTime", maxTimeForAddEvents),
		)
		if backoffDelay == backoff.Stop {
			break
		}
		time.Sleep(backoffDelay)
		retryNum++
	}

	maxTimeForProcessingEvents := useUpTo(0.8, processingStart, expFinishTime)
	client.Logger.Info(
		"Shutting down - waiting processing events",
		zap.Time("expFinishTime", expFinishTime),
		zap.Duration("maxElapsedTime", maxTimeForProcessingEvents),
		zap.Duration("retryShutdownTimeout", retryShutdownTimeout),
		zap.Duration("elapsedTime", time.Since(processingStart)),
	)

	expBackoff = client.createBackOff(maxTimeForProcessingEvents)
	expBackoff.Reset()
	retryNum = 0
	close(client.eventsProcessingDone)
	initialEventsDropped := client.statistics.EventsDropped()
	for client.isProcessingEvents() {
		// log statistics
		client.logStatistics()

		backoffDelay := expBackoff.NextBackOff()
		client.Logger.Info(
			"Shutting down - processing events",
			zap.Int("retryNum", retryNum),
			zap.Duration("backoffDelay", backoffDelay),
			zap.Uint64("eventsEnqueued", client.statistics.EventsEnqueued()),
			zap.Uint64("eventsProcessed", client.statistics.EventsProcessed()),
			zap.Duration("elapsedTime", time.Since(processingStart)),
			zap.Duration("maxElapsedTime", maxTimeForProcessingEvents),
			zap.Time("expFinishTime", expFinishTime),
		)
		if backoffDelay == backoff.Stop {
			break
		}
		time.Sleep(backoffDelay)
		retryNum++
	}

	maxTimeForProcessingBuffers := useUpTo(1.0, processingStart, expFinishTime)
	client.Logger.Info(
		"Shutting down - waiting for buffers",
		zap.Time("expFinishTime", expFinishTime),
		zap.Duration("maxElapsedTime", maxTimeForProcessingBuffers),
		zap.Duration("retryShutdownTimeout", retryShutdownTimeout),
		zap.Duration("elapsedTime", time.Since(processingStart)),
	)
	expBackoff = client.createBackOff(maxTimeForProcessingBuffers)
	expBackoff.Reset()
	retryNum = 0
	close(client.buffersProcessingDone)
	initialBuffersDropped := client.statistics.BuffersDropped()
	for client.isProcessingBuffers() {
		// log statistics
		client.logStatistics()

		backoffDelay := expBackoff.NextBackOff()
		client.Logger.Info(
			"Shutting down - processing buffers",
			zap.Int("retryNum", retryNum),
			zap.Duration("backoffDelay", backoffDelay),
			zap.Uint64("buffersEnqueued", client.statistics.BuffersEnqueued()),
			zap.Uint64("buffersProcessed", client.statistics.BuffersProcessed()),
			zap.Uint64("buffersDropped", client.statistics.BuffersDropped()),
			zap.Duration("elapsedTime", time.Since(processingStart)),
			zap.Time("expFinishTime", expFinishTime),
			zap.Duration("maxElapsedTime", maxTimeForProcessingBuffers),
		)
		if backoffDelay == backoff.Stop {
			break
		}
		time.Sleep(backoffDelay)
		retryNum++
	}

	// construct error messages
	if client.isProcessingEvents() {
		lastError = fmt.Errorf(
			"not all events have been processed - %d",
			client.statistics.EventsEnqueued()-client.statistics.EventsProcessed(),
		)
		client.Logger.Error(
			"Shutting down - not all events have been processed",
			zap.Uint64("eventsEnqueued", client.statistics.EventsEnqueued()),
			zap.Uint64("eventsProcessed", client.statistics.EventsProcessed()),
		)
	}

	if client.isProcessingBuffers() {
		lastError = fmt.Errorf(
			"not all buffers have been processed - %d",
			client.statistics.BuffersEnqueued()-client.statistics.BuffersProcessed()-client.statistics.BuffersDropped(),
		)
		client.Logger.Error(
			"Shutting down - not all buffers have been processed",
			zap.Int("retryNum", retryNum),
			zap.Uint64("buffersEnqueued", client.statistics.BuffersEnqueued()),
			zap.Uint64("buffersProcessed", client.statistics.BuffersProcessed()),
			zap.Uint64("buffersDropped", client.statistics.BuffersDropped()),
		)
	}

	eventsDropped := client.statistics.EventsDropped() - initialEventsDropped
	if eventsDropped > 0 {
		lastError = fmt.Errorf(
			"some events were dropped during finishing - %d",
			eventsDropped,
		)
		client.Logger.Error(
			"Shutting down - events were dropped during shutdown",
			zap.Uint64("eventsDropped", eventsDropped),
		)
	}

	buffersDropped := client.statistics.BuffersDropped() - initialBuffersDropped
	if buffersDropped > 0 {
		lastError = fmt.Errorf(
			"some buffers were dropped during finishing - %d",
			buffersDropped,
		)
		client.Logger.Error(
			"Shutting down - buffers were dropped during shutdown",
			zap.Uint64("buffersDropped", buffersDropped),
		)
	}

	// print final statistics
	client.logStatistics()

	if lastError == nil {
		client.Logger.Info(
			"Shutting down - success",
			zap.Duration("retryShutdownTimeout", retryShutdownTimeout),
			zap.Duration("elapsedTime", time.Since(processingStart)),
		)
	} else {
		client.Logger.Error(
			"Shutting down - error", zap.Error(lastError),
			zap.Duration("retryShutdownTimeout", retryShutdownTimeout),
			zap.Duration("elapsedTime", time.Since(processingStart)),
		)
		if client.LastError() == nil {
			return lastError
		}
	}

	return client.LastError()
}

func (client *DataSetClient) sendAddEventsBuffer(buf *buffer.Buffer) (*add_events.AddEventsResponse, int, error) {
	client.Logger.Debug("Sending buf", buf.ZapStats()...)

	payload, err := buf.Payload()
	if err != nil {
		client.Logger.Warn("Cannot create payload", buf.ZapStats(zap.Error(err))...)
		return nil, 0, fmt.Errorf("cannot create payload: %w", err)
	}
	client.Logger.Debug("Created payload",
		buf.ZapStats(
			zap.Int("payload", len(payload)),
			zap.Float64("payloadRatio", float64(len(payload))/buffer.ShouldSentBufferSize),
		)...,
	)
	resp := &add_events.AddEventsResponse{}
	client.statistics.PayloadSizeRecord(int64(len(payload)))

	httpRequest, err := request.NewApiRequest(
		http.MethodPost, client.addEventsEndpointUrl,
	).WithWriteLog(client.Config.Tokens).WithPayload(payload).WithUserAgent(client.userAgent).HttpRequest()
	if err != nil {
		return nil, len(payload), fmt.Errorf("cannot create request: %w", err)
	}

	apiCallStart := time.Now()
	err = client.apiCall(httpRequest, resp)
	apiCallEnd := time.Now()
	client.statistics.ResponseTimeRecord(apiCallEnd.Sub(apiCallStart))
	client.statistics.BytesAPISentAdd(uint64(len(payload)))

	if strings.HasPrefix(resp.Status, "error") {
		client.Logger.Error(
			"Problematic payload",
			zap.String("message", resp.Message),
			zap.String("status", resp.Status),
			zap.Int("payloadLength", len(payload)),
		)
	}

	return resp, len(payload), err
}

//func (client *DataSetClient) groupBundles(bundles []*add_events.EventBundle) map[string][]*add_events.EventBundle {
//	grouped := make(map[string][]*add_events.EventBundle)
//
//	// group batch
//	for _, bundle := range bundles {
//		if bundle == nil {
//			continue
//		}
//		key := bundle.Key(client.Config.GroupBy)
//		grouped[key] = append(grouped[key], bundle)
//	}
//	client.Logger.Debug("Batch was grouped",
//		zap.Int("batchSize", len(bundles)),
//		zap.Int("distinctStreams", len(grouped)),
//	)
//	return grouped
//}

func (client *DataSetClient) apiCall(req *http.Request, response response.ResponseObjSetter) error {
	resp, err := client.Client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", errMsgUnableToSentRequest, err)
	}

	defer func() {
		if err = resp.Body.Close(); err != nil {
			client.Logger.Error("Error when closing:", zap.Error(err))
		}
	}()

	client.Logger.Debug("Received response",
		zap.Int("statusCode", resp.StatusCode),
		zap.String("status", resp.Status),
		zap.Int64("contentLength", resp.ContentLength),
	)

	if resp.StatusCode != http.StatusOK {
		// TODO improve docs - why dont we return error in this case
		client.Logger.Warn(
			"!!!!! PAYLOAD WAS NOT ACCEPTED !!!!",
			zap.Int("statusCode", resp.StatusCode),
		)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: %w", errMsgUnableToReadResponse, err)
	}

	err = json.Unmarshal(responseBody, &response)
	if err != nil {
		return fmt.Errorf("%s: %w, url: %s, status: %d, response: %s", errMsgUnableToParseResponse, err, client.addEventsEndpointUrl, resp.StatusCode, truncateText(string(responseBody), 1000))
	}

	response.SetResponseObj(resp)

	return nil
}

// Truncate provided text to the provided length
func truncateText(text string, length int) string {
	if len(text) > length {
		text = string([]byte(text)[:length]) + "..."
	}

	return text
}
