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

package session_manager

import (
	"go.uber.org/zap"
)

type (
	Func          func(string) (interface{}, error)
	EventCallback func(key string, eventsChannel <-chan interface{})
)

type Operation string

const (
	Sub   Operation = "sub"
	Pub   Operation = "pub"
	Unsub Operation = "unsub"
)

// command represents command used to communicate within the session manager
type command struct {
	op    Operation
	key   string
	value interface{}
}

// SessionManager is a manager for session channels
// It allows to subscribe, publish and unsubscribe to/from channels
type SessionManager struct {
	eventCallback EventCallback
	channels      map[string]chan interface{}
	logger        *zap.Logger
	operations    chan command
}

// New creates a new SessionManager
func New(
	logger *zap.Logger,
	eventsCallback EventCallback,
) *SessionManager {
	manager := &SessionManager{
		eventCallback: eventsCallback,
		channels:      make(map[string]chan interface{}),
		logger:        logger,
		operations:    make(chan command),
	}

	go manager.processCommands()

	return manager
}

// Sub subscribes to a channel key
// It's fine to call this function multiple times with the same key - only one
// channel will be created
func (manager *SessionManager) Sub(key string) {
	manager.operations <- command{op: Sub, key: key}
}

// Pub publishes a value to a channel key
func (manager *SessionManager) Pub(key string, value interface{}) {
	manager.operations <- command{op: Pub, key: key, value: value}
}

// Unsub unsubscribes from a channel key
// It's fine to call this function multiple times with the same key - only if the key
// still exists, it will be closed.
func (manager *SessionManager) Unsub(key string) {
	manager.operations <- command{op: Unsub, key: key}
}

func (manager *SessionManager) processSubCommand(key string) {
	_, found := manager.channels[key]
	if !found {
		manager.logger.Debug("Subscribing to key", zap.String("key", key))
		ch := make(chan interface{})
		manager.channels[key] = ch
		go manager.eventCallback(key, ch)
	}
}

func (manager *SessionManager) processPubCommand(key string, value interface{}) {
	ch, found := manager.channels[key]
	if found {
		ch <- value
	} else {
		manager.logger.Warn("Channel for publishing does not exist", zap.String("key", key))
	}
}

func (manager *SessionManager) processUnsubCommand(key string) {
	ch, found := manager.channels[key]
	if found {
		manager.logger.Debug("Unsubscribing to key", zap.String("key", key))
		delete(manager.channels, key)
		close(ch)
	}
}

func (manager *SessionManager) processCommands() {
	for {
		cmd := <-manager.operations
		// manager.logger.Debug("SessionManager - processCommands - START", zap.String("cmd", cmd.op), zap.String("key", cmd.key))
		switch cmd.op {
		case Sub:
			manager.processSubCommand(cmd.key)
		case Unsub:
			manager.processUnsubCommand(cmd.key)
		case Pub:
			manager.processPubCommand(cmd.key, cmd.value)
		}
		// manager.logger.Debug("SessionManager - processCommands - END", zap.String("cmd", cmd.op), zap.String("key", cmd.key))
	}
}
