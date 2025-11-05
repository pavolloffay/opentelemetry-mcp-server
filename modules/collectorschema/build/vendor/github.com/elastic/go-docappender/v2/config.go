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
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/elastic/elastic-transport-go/v8/elastictransport"
)

type Value int

const (
	Unset Value = iota
	True
	False
)

// Config holds configuration for Appender.
type Config struct {
	// Logger holds an optional Logger to use for logging indexing requests.
	//
	// All Elasticsearch errors will be logged at error level, so in cases
	// where the indexer is used for high throughput indexing, is recommended
	// that a rate-limited logger is used.
	//
	// If Logger is nil, logging will be disabled.
	Logger *zap.Logger

	// TracerProvider holds an optional otel TracerProvider for tracing
	// flush requests.
	//
	// If TracerProvider is nil, requests will not be traced.
	// To use this provider Tracer must be nil.
	TracerProvider trace.TracerProvider

	// CompressionLevel holds the gzip compression level, from 0 (gzip.NoCompression)
	// to 9 (gzip.BestCompression). Higher values provide greater compression, at a
	// greater cost of CPU. The special value -1 (gzip.DefaultCompression) selects the
	// default compression level.
	CompressionLevel int

	// MaxRequests holds the maximum number of bulk index requests to execute concurrently.
	// The maximum memory usage of Appender is thus approximately MaxRequests*FlushBytes.
	//
	// If MaxRequests is less than or equal to zero, the default of 10 will be used.
	MaxRequests int

	// BulkIndexerPool holds an optional pool that is used for creating new BulkIndexers.
	// If not set/nil, a new BulkIndexerPool will be created with MaxRequests as the
	// guaranteed, local and total maximum number of indexers.
	//
	// A BulkIndexerPool may be shared between multiple Appender instances. Each has its
	// own unique ID to guarantee per Appender limits.
	//
	// For more information, see [NewBulkIndexerPool].
	BulkIndexerPool *BulkIndexerPool

	// MaxDocumentRetries holds the maximum number of document retries
	MaxDocumentRetries int

	// RetryOnDocumentStatus holds the document level statuses that will trigger a document retry.
	//
	// If RetryOnDocumentStatus is empty or nil, the default of [429] will be used.
	RetryOnDocumentStatus []int

	// FlushBytes holds the flush threshold in bytes. If Compression is enabled,
	// The number of documents that can be buffered will be greater.
	//
	// If FlushBytes is zero, the default of 1MB will be used.
	FlushBytes int

	// FlushInterval holds the flush threshold as a duration.
	//
	// If FlushInterval is zero, the default of 30 seconds will be used.
	FlushInterval time.Duration

	// FlushTimeout holds the flush timeout as a duration.
	//
	// If FlushTimeout is zero, no timeout will be used.
	FlushTimeout time.Duration

	// DocumentBufferSize sets the number of documents that can be buffered before
	// they are stored in the active indexer buffer.
	//
	// If DocumentBufferSize is zero, the default 1024 will be used.
	DocumentBufferSize int

	// Pipeline holds the ingest pipeline ID.
	//
	// If Pipeline is empty, no ingest pipeline will be specified in the Bulk request.
	Pipeline string

	// RequireDataStream, If set to true, an index will be created only if a
	// matching index template is found and it contains a data stream template.
	// When true, `require_data_stream=true` is set in the bulk request.
	// When false or not set, `require_data_stream` is not set in the bulk request.
	// Which could cause a classic index to be created if no data stream template
	// matches the index in the request.
	//
	// RequireDataStream is disabled by default.
	RequireDataStream bool

	// IncludeSourceOnError, if set to True, the response body of a Bulk Index request
	// might contain the part of source document on error.
	// If Unset the error reason will be dropped.
	// Requires Elasticsearch 8.18+ if value is True or False.
	// WARNING: if set to True, user is responsible for sanitizing the error as it may contain
	// sensitive data.
	//
	// IncludeSourceOnError is Unset by default
	IncludeSourceOnError Value

	// PopulateFailedDocsInput controls whether each BulkIndexerResponseItem.Input
	// in BulkIndexerResponseStat.FailedDocs is populated with the input of the item,
	// which includes the action line and the document line.
	//
	// WARNING: this is provided for testing and debugging only.
	// Use with caution as it may expose sensitive data; any clients
	// of go-docappender enabling this should relay this warning to
	// their users. Setting this will also add memory overhead.
	PopulateFailedDocsInput bool

	// Scaling configuration for the docappender.
	//
	// If unspecified, scaling is enabled by default.
	Scaling ScalingConfig

	// MeterProvider holds the OTel MeterProvider to be used to create and
	// record appender metrics.
	//
	// If unset, the global OTel MeterProvider will be used, if that is unset,
	// no metrics will be recorded.
	MeterProvider metric.MeterProvider

	// MetricAttributes holds any extra attributes to set in the recorded
	// metrics.
	MetricAttributes attribute.Set
}

// DefaultConfig returns a copy of cfg with any zero values set to their
// default values.
func DefaultConfig(cl elastictransport.Interface, cfg Config) Config {
	if cfg.MaxRequests <= 0 {
		cfg.MaxRequests = 10
	}
	if cfg.FlushBytes <= 0 {
		cfg.FlushBytes = 1 * 1024 * 1024
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 30 * time.Second
	}
	if cfg.DocumentBufferSize <= 0 {
		cfg.DocumentBufferSize = 1024
	}
	if !cfg.Scaling.Disabled {
		if cfg.Scaling.ScaleDown.Threshold == 0 {
			cfg.Scaling.ScaleDown.Threshold = 30
		}
		if cfg.Scaling.ScaleDown.CoolDown <= 0 {
			cfg.Scaling.ScaleDown.CoolDown = 30 * time.Second
		}
		if cfg.Scaling.ScaleUp.Threshold == 0 {
			cfg.Scaling.ScaleUp.Threshold = 60
		}
		if cfg.Scaling.ScaleUp.CoolDown <= 0 {
			cfg.Scaling.ScaleUp.CoolDown = time.Minute
		}
		if cfg.Scaling.IdleInterval <= 0 {
			cfg.Scaling.IdleInterval = 30 * time.Second
		}
		if cfg.Scaling.ActiveRatio <= 0 {
			cfg.Scaling.ActiveRatio = 0.25
		}
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	if cfg.BulkIndexerPool == nil {
		cfg.BulkIndexerPool = NewBulkIndexerPool(
			cfg.MaxRequests, cfg.MaxRequests, cfg.MaxRequests,
			BulkIndexerConfigFrom(cl, cfg),
		)
	}
	return cfg
}

// ScalingConfig holds the docappender autoscaling configuration.
type ScalingConfig struct {
	// Disabled toggles active indexer scaling on.
	//
	// It is enabled by default.
	Disabled bool

	// ActiveRatio defines the threshold for (potential) active indexers to
	// GOMAXPROCS. The higher the number, the more potential active indexers
	// there will be actively pulling from the BulkIndexerItem channel.
	// For example, when ActiveRatio:1 and GOMAXPROCS:2, there can be a max
	// of 2 active indexers, or 1 per GOMAXPROCS.
	// If set to 0.5, the maximum number of active indexers is 1, since.
	// The value must be between 0 and 1.
	//
	// It defaults to 0.25 by default.
	ActiveRatio float64

	// ScaleDown configures the Threshold and CoolDown for the scale down
	// action. In order to scale down an active indexer, the Threshold has
	// to be met after the CoolDown has elapsed. Scale down will only take
	// place if there are more than 1 active indexer.
	// Active indexers will be destroyed when they aren't needed anymore,
	// when enough timed flushes (FlushInterval) are performed by an active
	// indexer (controlled by Threshold), or when an active indexer is idle
	// for (IdleInterval * Threshold) as long as CoolDown allows it.
	//
	// When unset, the default of 30 is used for Threshold, and 30 seconds for
	// CoolDown.
	ScaleDown ScaleActionConfig

	// ScaleUp configures the Threshold and CoolDown for the scale up action.
	//
	// In order for a scale up to occur, the Threshold has to be met after
	// the CoolDown has elapsed. By default, a single active indexer is created
	// which actively pulls items from the internal buffered queue. When enough
	// full flushes (FlushBytes) are performed by an active indexer (controlled
	// by Threshold), a new active indexer will be created until GOMAXPROCS / 4
	// is reached (25% of CPU capacity) if the CoolDown allows it.
	//
	// When unspecified, the default of 60 is used for Threshold, and 60 seconds
	// for CoolDown.
	ScaleUp ScaleActionConfig

	// IdleInterval defines how long an active indexer performs an inactivity
	// check. The ScaleDown.Threshold and ScaleDown.CoolDown needs to be met
	// for an active indexer to be destroyed.
	//
	// When unspecified, the default of 30 seconds will be used.
	IdleInterval time.Duration
}

// ScaleActionConfig holds the configuration for a scaling action
type ScaleActionConfig struct {
	// Threshold is the number of consecutive times a scale up/down condition
	// has to happen for the scaling action will be triggered.
	Threshold uint

	// CoolDown is the amount of time needed to elapse between scaling actions
	// to trigger it.
	CoolDown time.Duration
}

// BulkIndexerConfig holds configuration for BulkIndexer.
type BulkIndexerConfig struct {
	// Client holds the Elasticsearch client.
	Client elastictransport.Interface

	// MaxDocumentRetries holds the maximum number of document retries
	MaxDocumentRetries int

	// RetryOnDocumentStatus holds the document level statuses that will trigger a document retry.
	//
	// If RetryOnDocumentStatus is empty or nil, the default of [429] will be used.
	RetryOnDocumentStatus []int

	// CompressionLevel holds the gzip compression level, from 0 (gzip.NoCompression)
	// to 9 (gzip.BestCompression). Higher values provide greater compression, at a
	// greater cost of CPU. The special value -1 (gzip.DefaultCompression) selects the
	// default compression level.
	CompressionLevel int

	// Pipeline holds the ingest pipeline ID.
	//
	// If Pipeline is empty, no ingest pipeline will be specified in the Bulk request.
	Pipeline string

	// RequireDataStream, If set to true, an index will be created only if a
	// matching index template is found and it contains a data stream template.
	// When true, `require_data_stream=true` is set in the bulk request.
	// When false or not set, `require_data_stream` is not set in the bulk request.
	// Which could cause a classic index to be created if no data stream template
	// matches the index in the request.
	//
	// RequireDataStream is disabled by default.
	RequireDataStream bool

	// IncludeSourceOnError, if set to True, the response body of a Bulk Index request
	// might contain the part of source document on error.
	// If Unset the error reason will be dropped.
	// Requires Elasticsearch 8.18+ if value is True or False.
	// WARNING: if set to True, user is responsible for sanitizing the error as it may contain
	// sensitive data.
	//
	// IncludeSourceOnError is Unset by default
	IncludeSourceOnError Value

	// PopulateFailedDocsInput controls whether each BulkIndexerResponseItem.Input
	// in BulkIndexerResponseStat.FailedDocs is populated with the input of the item,
	// which includes the action line and the document line.
	//
	// WARNING: this is provided for testing and debugging only.
	// Use with caution as it may expose sensitive data; any clients
	// of go-docappender enabling this should relay this warning to
	// their users. Setting this will also add memory overhead.
	PopulateFailedDocsInput bool
}

// Validate checks the configuration for errors.
func (cfg BulkIndexerConfig) Validate() error {
	var errs []error
	if cfg.Client == nil {
		errs = append(errs, fmt.Errorf("bulk_indexer: client is required"))
	}
	if cfg.CompressionLevel < -1 || cfg.CompressionLevel > 9 {
		errs = append(errs, fmt.Errorf("expected CompressionLevel in range [-1,9], got %d",
			cfg.CompressionLevel,
		))
	}
	return errors.Join(errs...)
}

// BulkIndexerConfigFrom creates a BulkIndexerConfig from the provided Config,
// with additional information included as necessary.
func BulkIndexerConfigFrom(cl elastictransport.Interface, cfg Config) BulkIndexerConfig {
	return BulkIndexerConfig{
		Client:                  cl,
		MaxDocumentRetries:      cfg.MaxDocumentRetries,
		RetryOnDocumentStatus:   cfg.RetryOnDocumentStatus,
		CompressionLevel:        cfg.CompressionLevel,
		Pipeline:                cfg.Pipeline,
		RequireDataStream:       cfg.RequireDataStream,
		IncludeSourceOnError:    cfg.IncludeSourceOnError,
		PopulateFailedDocsInput: cfg.PopulateFailedDocsInput,
	}
}
