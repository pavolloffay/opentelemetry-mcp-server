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

package buffer_config

import (
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
)

const (
	ShouldSentBufferSize = 5 * 1024 * 1024
	// LimitBufferSize defines maximum payload size (before compression) for REST API
	LimitBufferSize             = 5*1024*1024 + 960*1024
	MinimalMaxSize              = 100
	MinimalMaxElapsedTime       = time.Second
	MinimalMaxInterval          = time.Second
	MinimalInitialInterval      = 50 * time.Millisecond
	MinimalMultiplier           = 0.0
	MinimalRandomizationFactor  = 0.0
	MinimalRetryShutdownTimeout = 2 * MinimalMaxElapsedTime
	// MinimalPurgeToLifetimeRatio is the minimal ratio between buffer lifetime (when it's sent to server)
	// and when the whole session is destroyed.
	MinimalPurgeToLifetimeRatio = 3
	MinimalMaxParallelOutgoing  = 1
	MaximalMaxParallelOutgoing  = 1000
)

type DataSetBufferSettings struct {
	MaxLifetime              time.Duration
	PurgeOlderThan           time.Duration
	MaxSize                  int
	GroupBy                  []string
	RetryRandomizationFactor float64
	RetryMultiplier          float64
	RetryInitialInterval     time.Duration
	RetryMaxInterval         time.Duration
	RetryMaxElapsedTime      time.Duration
	// RetryShutdownTimeout defines timeout period (for which client will retry on failures)
	//  for processing events and sending buffers after shutdown of a client
	RetryShutdownTimeout time.Duration
	// MaxParallelOutgoing maximum number of parallel outgoing requests to the server
	MaxParallelOutgoing int
}

func NewDefaultDataSetBufferSettings() DataSetBufferSettings {
	return DataSetBufferSettings{
		MaxLifetime:              5 * time.Second,
		PurgeOlderThan:           30 * time.Second,
		MaxSize:                  LimitBufferSize,
		GroupBy:                  []string{},
		RetryInitialInterval:     5 * time.Second,
		RetryMaxInterval:         30 * time.Second,
		RetryMaxElapsedTime:      300 * time.Second,
		RetryRandomizationFactor: backoff.DefaultRandomizationFactor,
		RetryMultiplier:          backoff.DefaultMultiplier,
		RetryShutdownTimeout:     30 * time.Second,
		MaxParallelOutgoing:      100,
	}
}

type DataSetBufferSettingsOption func(*DataSetBufferSettings) error

func WithMaxLifetime(maxLifetime time.Duration) DataSetBufferSettingsOption {
	return func(c *DataSetBufferSettings) error {
		c.MaxLifetime = maxLifetime
		return nil
	}
}

func WithPurgeOlderThan(purgeOlderThan time.Duration) DataSetBufferSettingsOption {
	return func(c *DataSetBufferSettings) error {
		c.PurgeOlderThan = purgeOlderThan
		return nil
	}
}

func WithMaxSize(maxSize int) DataSetBufferSettingsOption {
	return func(c *DataSetBufferSettings) error {
		c.MaxSize = maxSize
		return nil
	}
}

func WithGroupBy(groupBy []string) DataSetBufferSettingsOption {
	return func(c *DataSetBufferSettings) error {
		c.GroupBy = groupBy
		return nil
	}
}

func WithRetryInitialInterval(retryInitialInterval time.Duration) DataSetBufferSettingsOption {
	return func(c *DataSetBufferSettings) error {
		c.RetryInitialInterval = retryInitialInterval
		return nil
	}
}

func WithRetryMultiplier(retryMultiplier float64) DataSetBufferSettingsOption {
	return func(c *DataSetBufferSettings) error {
		c.RetryMultiplier = retryMultiplier
		return nil
	}
}

func WithRetryRandomizationFactor(retryRandomizationFactor float64) DataSetBufferSettingsOption {
	return func(c *DataSetBufferSettings) error {
		c.RetryRandomizationFactor = retryRandomizationFactor
		return nil
	}
}

func WithRetryMaxInterval(retryMaxInterval time.Duration) DataSetBufferSettingsOption {
	return func(c *DataSetBufferSettings) error {
		c.RetryMaxInterval = retryMaxInterval
		return nil
	}
}

func WithRetryMaxElapsedTime(retryMaxElapsedTime time.Duration) DataSetBufferSettingsOption {
	return func(c *DataSetBufferSettings) error {
		c.RetryMaxElapsedTime = retryMaxElapsedTime
		return nil
	}
}

func WithRetryShutdownTimeout(retryShutdownTimeout time.Duration) DataSetBufferSettingsOption {
	return func(c *DataSetBufferSettings) error {
		c.RetryShutdownTimeout = retryShutdownTimeout
		return nil
	}
}

func WithMaxParallelOutgoing(maxParallelOutgoing int) DataSetBufferSettingsOption {
	return func(c *DataSetBufferSettings) error {
		c.MaxParallelOutgoing = maxParallelOutgoing
		return nil
	}
}

func New(opts ...DataSetBufferSettingsOption) (*DataSetBufferSettings, error) {
	cfg := &DataSetBufferSettings{}
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func (cfg *DataSetBufferSettings) WithOptions(opts ...DataSetBufferSettingsOption) (*DataSetBufferSettings, error) {
	newCfg := *cfg
	for _, opt := range opts {
		if err := opt(&newCfg); err != nil {
			return &newCfg, err
		}
	}
	return &newCfg, nil
}

func (cfg *DataSetBufferSettings) String() string {
	return fmt.Sprintf(
		"MaxLifetime: %s, PurgeOlderThan: %s, MaxSize: %d, GroupBy: %s, RetryRandomizationFactor: %f, RetryMultiplier: %f, RetryInitialInterval: %s, RetryMaxInterval: %s, RetryMaxElapsedTime: %s, RetryShutdownTimeout: %s, MaxParallelOutgoing: %d",
		cfg.MaxLifetime,
		cfg.PurgeOlderThan,
		cfg.MaxSize,
		cfg.GroupBy,
		cfg.RetryRandomizationFactor,
		cfg.RetryMultiplier,
		cfg.RetryInitialInterval,
		cfg.RetryMaxInterval,
		cfg.RetryMaxElapsedTime,
		cfg.RetryShutdownTimeout,
		cfg.MaxParallelOutgoing,
	)
}

func (cfg *DataSetBufferSettings) Validate() error {
	if MinimalPurgeToLifetimeRatio*cfg.MaxLifetime > cfg.PurgeOlderThan {
		return fmt.Errorf(
			"MaxLifetime %s has to be at least %d times smaller than PurgeOlderThan %s",
			cfg.MaxLifetime,
			MinimalPurgeToLifetimeRatio,
			cfg.PurgeOlderThan,
		)
	}

	if cfg.MaxSize > LimitBufferSize {
		return fmt.Errorf(
			"MaxSize has value %d which is more than %d",
			cfg.MaxSize,
			LimitBufferSize,
		)
	}

	if cfg.MaxSize < MinimalMaxSize {
		return fmt.Errorf(
			"MaxSize has value %d which is less than %d",
			cfg.MaxSize,
			MinimalMaxSize,
		)
	}

	if cfg.RetryInitialInterval < MinimalInitialInterval {
		return fmt.Errorf(
			"RetryInitialInterval has value %s which is less than %s",
			cfg.RetryInitialInterval,
			MinimalInitialInterval,
		)
	}

	if cfg.RetryMaxInterval < MinimalMaxInterval {
		return fmt.Errorf(
			"RetryMaxInterval has value %s which is less than %s",
			cfg.RetryMaxInterval,
			MinimalMaxInterval,
		)
	}

	if cfg.RetryMaxElapsedTime < MinimalMaxElapsedTime {
		return fmt.Errorf(
			"RetryMaxElapsedTime has value %s which is less than %s",
			cfg.RetryMaxElapsedTime,
			MinimalMaxElapsedTime,
		)
	}

	if cfg.RetryMultiplier <= MinimalMultiplier {
		return fmt.Errorf(
			"RetryMultiplier has value %f which is less or equal than %f",
			cfg.RetryMultiplier,
			MinimalMultiplier,
		)
	}

	if cfg.RetryRandomizationFactor <= MinimalRandomizationFactor {
		return fmt.Errorf(
			"RetryRandomizationFactor has value %f which is less or equal than %f",
			cfg.RetryRandomizationFactor,
			MinimalRandomizationFactor,
		)
	}

	if cfg.RetryShutdownTimeout < MinimalRetryShutdownTimeout {
		return fmt.Errorf(
			"RetryShutdownTimeout has value %s which is less than %s",
			cfg.RetryShutdownTimeout,
			MinimalRetryShutdownTimeout,
		)
	}

	if cfg.MaxParallelOutgoing < MinimalMaxParallelOutgoing {
		return fmt.Errorf(
			"MaxParallelOutgoing has value %d which is less than %d",
			cfg.MaxParallelOutgoing,
			MinimalMaxParallelOutgoing,
		)
	}

	if cfg.MaxParallelOutgoing > MaximalMaxParallelOutgoing {
		return fmt.Errorf(
			"MaxParallelOutgoing has value %d which is more than %d",
			cfg.MaxParallelOutgoing,
			MaximalMaxParallelOutgoing,
		)
	}

	return nil
}
