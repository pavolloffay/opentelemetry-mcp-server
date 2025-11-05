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

package buffer

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/scalyr/dataset-go/pkg/buffer_config"

	"github.com/scalyr/dataset-go/pkg/api/request"

	"github.com/scalyr/dataset-go/pkg/api/add_events"

	"github.com/google/uuid"

	"go.uber.org/zap"
)

const (
	ShouldSentBufferSize = buffer_config.ShouldSentBufferSize
	LimitBufferSize      = buffer_config.LimitBufferSize
)

type AddStatus uint8

const (
	Added = AddStatus(iota)
	Skipped
	TooMuch
)

func (s AddStatus) String() string {
	return [...]string{"Added", "Skipped", "TooMuch"}[s]
}

// Buffer represent a batch of Events grouped under certain session.
// Each Buffer (set of events) are send to DataSet once reaches its limit or timeout
type Buffer struct {
	Id      uuid.UUID
	Session string
	Token   string

	createdAt time.Time
	updatedAt time.Time

	sessionInfo *add_events.SessionInfo
	threads     map[string]*add_events.Thread
	logs        map[string]*add_events.Log
	events      []*add_events.Event

	lenSessionInfo int
	lenThreads     int32
	lenLogs        int32
	lenEvents      int32

	countThreads int32
	countLogs    int32
	countEvents  int32
}

func NewEmptyBuffer(session string, token string) *Buffer {
	id, _ := uuid.NewRandom()

	return &Buffer{
		Id:        id,
		Session:   session,
		Token:     token,
		createdAt: time.Now(),
		updatedAt: time.Now(),
	}
}

func NewBuffer(session string, token string, sessionInfo *add_events.SessionInfo) (*Buffer, error) {
	buffer := NewEmptyBuffer(session, token)
	err := buffer.Initialise(sessionInfo)
	return buffer, err
}

func (buffer *Buffer) Initialise(sessionInfo *add_events.SessionInfo) error {
	buffer.threads = map[string]*add_events.Thread{}
	buffer.logs = map[string]*add_events.Log{}
	buffer.events = []*add_events.Event{}
	err := buffer.SetSessionInfo(sessionInfo)
	if err != nil {
		return fmt.Errorf("NewBuffer cannot set SessionInfo: %w", err)
	}
	return nil
}

func (buffer *Buffer) NewEmpty() (*Buffer, error) {
	newBuffer := NewEmptyBuffer(buffer.Session, buffer.Token)
	err := newBuffer.Initialise(buffer.sessionInfo)
	return newBuffer, err
}

func (buffer *Buffer) HasEvents() bool {
	return buffer.countEvents > 0
}

func (buffer *Buffer) SetSessionInfo(sessionInfo *add_events.SessionInfo) error {
	buffer.sessionInfo = sessionInfo
	if sessionInfo != nil {
		sesSer, err := json.Marshal(*sessionInfo)
		if err != nil {
			return fmt.Errorf(
				"NewBuffer - sessionInfo cannot be converted to JSON: %w",
				err,
			)
		}
		buffer.lenSessionInfo = len(sesSer)
	} else {
		buffer.lenSessionInfo = 0
	}

	return nil
}

func (buffer *Buffer) SessionInfo() *add_events.SessionInfo {
	return buffer.sessionInfo
}

func (buffer *Buffer) AddBundle(bundle *add_events.EventBundle) (AddStatus, error) {
	defer func() {
		buffer.updatedAt = time.Now()
	}()

	// append thread
	addT, errT := buffer.addThread(bundle.Thread)
	if errT != nil {
		return Skipped, fmt.Errorf("cannot add thread: %w", errT)
	}
	if addT == TooMuch {
		return TooMuch, nil
	}

	// append log
	addL, errL := buffer.addLog(bundle.Log)
	if errL != nil {
		if addT == Added {
			buffer.removeThread(bundle.Thread)
		}
		return Skipped, fmt.Errorf("cannot add log: %w", errL)
	}
	if addL == TooMuch {
		if addT == Added {
			buffer.removeThread(bundle.Thread)
		}
		return TooMuch, nil
	}

	// append event
	addE, errE := buffer.addEvent(bundle.Event)
	if errE != nil {
		if addT == Added {
			buffer.removeThread(bundle.Thread)
		}
		if addL == Added {
			buffer.removeLog(bundle.Log)
		}
		return Skipped, fmt.Errorf("cannot add Event: %w", errL)
	}
	if addE == TooMuch {
		if addT == Added {
			buffer.removeThread(bundle.Thread)
		}
		if addL == Added {
			buffer.removeLog(bundle.Log)
		}
		return TooMuch, nil
	} else {
		return Added, nil
	}
}

func (buffer *Buffer) addThread(thread *add_events.Thread) (AddStatus, error) {
	if thread == nil {
		return Skipped, nil
	}

	if _, ok := buffer.threads[thread.Id]; !ok {
		// add new log representation
		threadSer, err := json.Marshal(*thread)
		if err != nil {
			return Skipped, fmt.Errorf("AddThread cannot convert to JSON: %w", err)
		}

		if buffer.canAppend(threadSer) {
			buffer.threads[thread.Id] = thread
			buffer.lenThreads += int32(len(threadSer) + 1)
			buffer.countThreads += 1
			return Added, nil
		} else {
			return TooMuch, nil
		}
	}
	return Skipped, nil
}

func (buffer *Buffer) removeThread(thread *add_events.Thread) {
	if thread == nil {
		return
	}
	threadSer, err := json.Marshal(*thread)
	if err != nil {
		return
	}
	delete(buffer.threads, thread.Id)
	buffer.lenThreads += (int32(-(len(threadSer) + 1)))
	buffer.countThreads += (-1)
}

func (buffer *Buffer) addLog(log *add_events.Log) (AddStatus, error) {
	if log == nil {
		return Skipped, nil
	}
	if _, ok := buffer.logs[log.Id]; !ok {
		logSer, err := json.Marshal(*log)
		if err != nil {
			return Skipped, fmt.Errorf("AddLog cannot convert to JSON: %w", err)
		}

		if buffer.canAppend(logSer) {
			buffer.logs[log.Id] = log
			buffer.lenLogs += (int32(len(logSer) + 1))
			buffer.countLogs += (1)
			return Added, nil
		} else {
			return TooMuch, nil
		}
	}
	return Skipped, nil
}

func (buffer *Buffer) removeLog(log *add_events.Log) {
	if log == nil {
		return
	}
	logSer, err := json.Marshal(*log)
	if err != nil {
		return
	}
	delete(buffer.logs, log.Id)
	buffer.lenLogs += (int32(-(len(logSer) + 1)))
	buffer.countLogs += (-1)
}

func (buffer *Buffer) addEvent(event *add_events.Event) (AddStatus, error) {
	if event == nil {
		return Skipped, fmt.Errorf("adding empty event is illegal")
	}

	eventSer, err := json.Marshal(*event)
	if err != nil {
		return Skipped, fmt.Errorf("AddEvent cannot convert to JSON: %w", err)
	}

	if buffer.canAppend(eventSer) {
		buffer.events = append(buffer.events, event)
		buffer.lenEvents += (int32(len(eventSer) + 1))
		buffer.countEvents += (1)
		return Added, nil
	} else {
		if buffer.countEvents == 0 {
			trimmed := buffer.trimEvent(event)
			if trimmed == nil {
				return TooMuch, fmt.Errorf("objects Thread or Log is too large, cannot fix here")
			}
			return buffer.addEvent(trimmed)
		}
		return TooMuch, nil
	}
}

func (buffer *Buffer) trimEvent(event *add_events.Event) *add_events.Event {
	remaining := LimitBufferSize - buffer.BufferLengths()

	newEvent := event.CloneWithoutAttrs()
	eventSer, _ := json.Marshal(*newEvent)
	remaining -= int32(len(eventSer))
	if remaining < 0 {
		// we have messed up, we should have reduced log or thread :/
		return nil
	}

	remaining -= 2

	newEvent.Attrs = add_events.TrimAttrs(event.Attrs, int(remaining))

	return newEvent
}

func (buffer *Buffer) canAppend(data []byte) bool {
	return buffer.BufferLengths()+int32(len(data)+1) < LimitBufferSize
}

func (buffer *Buffer) ShouldSendSize() bool {
	return buffer.countEvents > 0 && buffer.BufferLengths() > ShouldSentBufferSize
}

func (buffer *Buffer) ShouldSendAge(lifetime time.Duration) bool {
	return buffer.countEvents > 0 && time.Since(buffer.createdAt) > lifetime
}

func (buffer *Buffer) ShouldPurgeAge(lifetime time.Duration) bool {
	return buffer.countEvents == 0 && time.Since(buffer.updatedAt) > lifetime
}

func (buffer *Buffer) BufferLengths() int32 {
	return int32(buffer.lenSessionInfo) + buffer.lenThreads + buffer.lenLogs + buffer.lenEvents
}

func (buffer *Buffer) CountEvents() int32 {
	return buffer.countEvents
}

func (buffer *Buffer) Payload() ([]byte, error) {
	if len(buffer.Token) == 0 {
		return nil, fmt.Errorf("token is missing")
	}

	if len(buffer.Session) == 0 {
		return nil, fmt.Errorf("session is missing")
	}

	if buffer.lenEvents == 0 {
		return nil, fmt.Errorf("there is no event")
	}

	var threads []*add_events.Thread
	for _, t := range buffer.threads {
		threads = append(threads, t)
	}

	var logs []*add_events.Log
	for _, l := range buffer.logs {
		logs = append(logs, l)
	}

	reqObject := add_events.AddEventsRequest{
		AuthParams: request.AuthParams{
			Token: buffer.Token,
		},
		AddEventsRequestParams: add_events.AddEventsRequestParams{
			Session:     buffer.Session,
			SessionInfo: buffer.sessionInfo,
			Events:      buffer.events,
			Threads:     threads,
			Logs:        logs,
		},
	}

	payload, err := json.Marshal(reqObject)
	if err != nil {
		return nil, fmt.Errorf("AddEventsRequestParams cannot be convert to JSON: %w", err)
	}

	return payload, nil
}

func (buffer *Buffer) ZapStats(fields ...zap.Field) []zap.Field {
	res := []zap.Field{
		zap.String("uuid", buffer.Id.String()),
		zap.String("session", buffer.Session),
		zap.Int32("logs", buffer.countLogs),
		zap.Int32("threads", buffer.countThreads),
		zap.Int32("events", buffer.countEvents),
		zap.Int32("bufferLength", buffer.BufferLengths()),
		zap.Float64("bufferRatio", float64(buffer.BufferLengths())/ShouldSentBufferSize),
		zap.Int64("sinceCreatedAtMs", time.Since(buffer.createdAt).Milliseconds()),
	}
	res = append(res, fields...)
	return res
}
