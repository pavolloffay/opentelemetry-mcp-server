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

package add_events

import (
	"encoding/json"
	"math"
	"net/http"
	"sort"

	"github.com/scalyr/dataset-go/pkg/api/request"
	"github.com/scalyr/dataset-go/pkg/api/response"
)

const (
	AttrSessionKey     = "session_key"
	AttrServerHost     = "serverHost"
	AttrOrigServerHost = "__origServerHost"
	AttrLogFile        = "logfile"
)

type (
	EventAttrs  = map[string]interface{}
	SessionInfo = map[string]interface{}
)

// Event represents DataSet REST API event structure (see https://app.scalyr.com/help/api#addEvents)
type Event struct {
	Thread     string     `json:"thread,omitempty"`
	Sev        int        `json:"sev,omitempty"`
	Ts         string     `json:"ts,omitempty"`
	Log        string     `json:"log,omitempty"`
	Attrs      EventAttrs `json:"attrs"`
	ServerHost string     `json:"-"`
}

func (event *Event) CloneWithoutAttrs() *Event {
	return &Event{
		Thread: event.Thread,
		Sev:    event.Sev,
		Ts:     event.Ts,
		Log:    event.Ts,
		Attrs:  make(EventAttrs),
	}
}

// Thread represents a key-value definition of a thread.
// This lets you create a readable name for each thread used in Event.
type Thread struct {
	Id   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// Log represents a shared attributes reused in multiple Event in EventBundle.
// This decreases redundant data, and improves throughput.
type Log struct {
	Id    string                 `json:"id,omitempty"`
	Attrs map[string]interface{} `json:"attrs"`
}

// AddEventsRequestParams represents a represents a AddEvent DataSet REST API request parameters, see https://app.scalyr.com/help/api#addEvents.
type AddEventsRequestParams struct {
	Session     string       `json:"session,omitempty"`
	SessionInfo *SessionInfo `json:"sessionInfo,omitempty"`
	Events      []*Event     `json:"events,omitempty"`
	Threads     []*Thread    `json:"threads,omitempty"`
	Logs        []*Log       `json:"logs,omitempty"`
}

// AddEventsRequest represents a AddEvent DataSet REST API request.
type AddEventsRequest struct {
	request.AuthParams
	AddEventsRequestParams
}

// AddEventsResponse represents a AddEvent DataSet REST API response.
type AddEventsResponse struct {
	response.ApiResponse
	BytesCharged int64 `json:"bytesCharged"`
}

func (response *AddEventsResponse) SetResponseObj(resp *http.Response) {
	response.ResponseObj = resp
}

// TODO document what this function does
func TrimAttrs(attrs map[string]interface{}, remaining int) map[string]interface{} {
	keys := make([]string, 0, len(attrs))
	lengths := make(map[string]int)

	for key, value := range attrs {
		keys = append(keys, key)
		ser, _ := json.Marshal(value)
		lengths[key] = len(ser)
	}

	sort.SliceStable(keys, func(i, j int) bool {
		return lengths[keys[i]] < lengths[keys[j]]
	})

	newAttrs := make(map[string]interface{})

	for _, key := range keys {
		rowLen := len(key) + lengths[key] + 7
		if rowLen < remaining {
			newAttrs[key] = attrs[key]
			remaining -= rowLen
		} else {
			if len(key)+7 > remaining {
				/* TODO: IMP - Log this info
				add_events.Logger.Error(
					"Event attribute too long - just key - skipping",
					zap.String("key", key),
					zap.Int("originalLen", lengths[key]),
					zap.Int("remaining", remaining),
				)
				*/
			} else {
				if s, ok := attrs[key].(string); ok {
					// there is some additional overhead for key, {}, "
					trimmedS := ""

					useLength := remaining - 7 - len(key)
					lenSer := 0
					for {
						// when use should use less than 0, we should use
						// empty string instead
						// this also prevents infinite loop
						if useLength <= 0 {
							trimmedS = ""
							lenSer = 0
							break
						}
						keepLen := int64(math.Min(float64(len(s)), float64(useLength)))
						trimmedS = s[:keepLen]
						ser, _ := json.Marshal(trimmedS)
						lenSer = len(ser)
						over := lenSer - remaining
						// if it's longer, trim and try it serialize again
						if over > 0 {
							useLength -= over
						} else {
							break
						}
					}

					newAttrs[key] = trimmedS
					remaining = 0
					/* TODO: IMP - Log this info
					add_events.Logger.Error(
						"Event attribute too long - trimming",
						zap.String("key", key),
						zap.Int("originalLen", lengths[key]),
						zap.Int("serialisedLen", lenSer),
						zap.Int("newLen", len(trimmedS)),
						zap.Int("remaining", remaining),
					)
					*/
				} /* else {
					// TODO: IMP - Log this info
					add_events.Logger.Error(
						"Event attribute too long - skipping",
						zap.String("key", key),
						zap.Int("originalLen", lengths[key]),
						zap.Int("remaining", remaining),
					)
				} */
			}
		}
	}
	return newAttrs
}
