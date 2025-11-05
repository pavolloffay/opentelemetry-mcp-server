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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"unsafe"

	jsoniter "github.com/json-iterator/go"
	"github.com/klauspost/compress/gzip"
	"go.elastic.co/fastjson"

	"github.com/elastic/elastic-transport-go/v8/elastictransport"
)

// At the time of writing, the go-elasticsearch BulkIndexer implementation
// sends all items to a channel, and multiple persistent worker goroutines will
// receive those items and independently fill up their own buffers. Each one
// will independently flush when their buffer is filled up, or when the flush
// interval elapses. If there are many workers, then this may lead to sparse
// bulk requests.
//
// We take a different approach, where we fill up one bulk request at a time.
// When the buffer is filled up, or the flush interval elapses, we start a new
// goroutine to send the request in the background, with a limit on the number
// of concurrent bulk requests. This way we can ensure bulk requests have the
// maximum possible size, based on configuration and throughput.

const (
	// Actions are all the actions that can be used when indexing data.
	// `create` will be used by default.
	ActionCreate = "create"
	ActionDelete = "delete"
	ActionIndex  = "index"
	ActionUpdate = "update"

	HeaderEventCount         = "X-Elastic-Event-Count"
	HeaderUncompressedLength = "X-Elastic-Uncompressed-Request-Length"
)

// BulkIndexer issues bulk requests to Elasticsearch. It is NOT safe for concurrent use
// by multiple goroutines.
type BulkIndexer struct {
	config             BulkIndexerConfig
	itemsAdded         int
	bytesFlushed       int
	bytesUncompFlushed int
	jsonw              fastjson.Writer
	writer             *countWriter
	gzipw              *gzip.Writer
	buf                bytes.Buffer
	retryCounts        map[int]int
	requireDataStream  bool
}

type BulkIndexerResponseStat struct {
	// Indexed contains the total number of successfully indexed documents.
	Indexed int64
	// RetriedDocs contains the total number of retried documents.
	RetriedDocs int64
	// FailureStoreDocs contains failure store specific document stats.
	FailureStoreDocs struct {
		// Used contains the total number of documents indexed to failure store.
		Used int64
		// Failed contains the total number of documents which failed when indexed to failure store.
		Failed int64
		// NotEnabled contains the total number of documents which could have been indexed to failure store
		// if it was enabled.
		NotEnabled int64
	}
	// GreatestRetry contains the greatest observed retry count in the entire
	// bulk request.
	GreatestRetry int
	// FailedDocs contains the failed documents.
	FailedDocs []BulkIndexerResponseItem
}

// BulkIndexerResponseItem represents the Elasticsearch response item.
type BulkIndexerResponseItem struct {
	Index  string `json:"_index"`
	Status int    `json:"status"`

	Position int

	Error struct {
		Type   string `json:"type"`
		Reason string `json:"reason"`
	} `json:"error,omitempty"`

	Input string `json:"-"`
}

// FailureStoreStatus defines enumeration type for all known failure store statuses.
type FailureStoreStatus string

const (
	// FailureStoreStatusUnknown implicit status which represents that there is no information about
	// this response or that the failure store is not applicable.
	FailureStoreStatusUnknown FailureStoreStatus = "not_applicable_or_unknown"
	// FailureStoreStatusUsed status which represents that this document was stored in the failure store successfully.
	FailureStoreStatusUsed FailureStoreStatus = "used"
	// FailureStoreStatusFailed status which represents that this document was rejected from the failure store.
	FailureStoreStatusFailed FailureStoreStatus = "failed"
	// FailureStoreStatusNotEnabled status which represents that this document was rejected, but
	// it could have ended up in the failure store if it was enabled.
	FailureStoreStatusNotEnabled FailureStoreStatus = "not_enabled"
)

func init() {
	jsoniter.RegisterTypeDecoderFunc("docappender.BulkIndexerResponseStat", func(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
		iter.ReadObjectCB(func(i *jsoniter.Iterator, s string) bool {
			switch s {
			case "items":
				var idx int
				iter.ReadArrayCB(func(i *jsoniter.Iterator) bool {
					return i.ReadMapCB(func(i *jsoniter.Iterator, s string) bool {
						var item BulkIndexerResponseItem
						i.ReadObjectCB(func(i *jsoniter.Iterator, s string) bool {
							switch s {
							case "_index":
								item.Index = i.ReadString()
							case "status":
								item.Status = i.ReadInt()
							case "failure_store":
								// For the stats track only actionable explicit failure store statuses "used", "failed" and "not_enabled".
								switch fs := i.ReadString(); FailureStoreStatus(fs) {
								case FailureStoreStatusUsed:
									(*((*BulkIndexerResponseStat)(ptr))).FailureStoreDocs.Used++
								case FailureStoreStatusFailed:
									(*((*BulkIndexerResponseStat)(ptr))).FailureStoreDocs.Failed++
								case FailureStoreStatusNotEnabled:
									(*((*BulkIndexerResponseStat)(ptr))).FailureStoreDocs.NotEnabled++
								}
							case "error":
								i.ReadObjectCB(func(i *jsoniter.Iterator, s string) bool {
									switch s {
									case "type":
										item.Error.Type = i.ReadString()
									case "reason":
										item.Error.Reason = i.ReadString()
									default:
										i.Skip()
									}
									return true
								})
							default:
								i.Skip()
							}
							return true
						})

						item.Position = idx
						idx++
						if item.Error.Type != "" || item.Status > 201 {
							(*((*BulkIndexerResponseStat)(ptr))).FailedDocs = append((*((*BulkIndexerResponseStat)(ptr))).FailedDocs, item)
						} else {
							(*((*BulkIndexerResponseStat)(ptr))).Indexed = (*((*BulkIndexerResponseStat)(ptr))).Indexed + 1
						}
						return true
					})
				})
				// no need to proceed further, return early
				return false
			default:
				i.Skip()
				return true
			}
		})
	})
}

type countWriter struct {
	bytesWritten int
	io.Writer
}

func (cw *countWriter) Write(p []byte) (int, error) {
	cw.bytesWritten += len(p)
	return cw.Writer.Write(p)
}

// NewBulkIndexer returns a bulk indexer that issues bulk requests to Elasticsearch.
// It is only tested with v8 go-elasticsearch client. Use other clients at your own risk.
// The returned BulkIndexer is NOT safe for concurrent use by multiple goroutines.
func NewBulkIndexer(cfg BulkIndexerConfig) (*BulkIndexer, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return newBulkIndexer(cfg), nil
}

func newBulkIndexer(cfg BulkIndexerConfig) *BulkIndexer {
	b := &BulkIndexer{
		config:            cfg,
		retryCounts:       make(map[int]int),
		requireDataStream: cfg.RequireDataStream,
	}

	// use a len check instead of a nil check because document level retries
	// should be disabled using MaxDocumentRetries instead.
	if len(b.config.RetryOnDocumentStatus) == 0 {
		b.config.RetryOnDocumentStatus = []int{http.StatusTooManyRequests}
	}
	var writer io.Writer
	if cfg.CompressionLevel != gzip.NoCompression {
		b.gzipw, _ = gzip.NewWriterLevel(&b.buf, cfg.CompressionLevel)
		writer = b.gzipw
	} else {
		writer = &b.buf
	}
	b.writer = &countWriter{0, writer}
	return b
}

// Reset resets bulk indexer, ready for a new request.
func (b *BulkIndexer) Reset() {
	b.bytesFlushed = 0
	b.bytesUncompFlushed = 0
}

// SetClient resets the client used by the bulk indexer.
func (b *BulkIndexer) SetClient(client elastictransport.Interface) {
	b.config.Client = client
}

// resetBuf resets compressed buffer after flushing it to Elasticsearch
func (b *BulkIndexer) resetBuf() {
	b.itemsAdded = 0
	b.writer.bytesWritten = 0
	b.buf.Reset()
	if b.gzipw != nil {
		b.gzipw.Reset(&b.buf)
	}
}

// Items returns the number of buffered items.
func (b *BulkIndexer) Items() int {
	return b.itemsAdded
}

// Len returns the number of buffered bytes.
func (b *BulkIndexer) Len() int {
	return b.buf.Len()
}

// UncompressedLen returns the number of uncompressed buffered bytes.
func (b *BulkIndexer) UncompressedLen() int {
	return b.writer.bytesWritten
}

// BytesFlushed returns the number of bytes flushed by the bulk indexer.
func (b *BulkIndexer) BytesFlushed() int {
	return b.bytesFlushed
}

// BytesUncompressedFlushed returns the number of uncompressed bytes flushed by the bulk indexer.
func (b *BulkIndexer) BytesUncompressedFlushed() int {
	return b.bytesUncompFlushed
}

type BulkIndexerItem struct {
	Index             string
	DocumentID        string
	Pipeline          string
	Action            string
	Body              io.WriterTo
	DynamicTemplates  map[string]string
	RequireDataStream bool
}

// Add encodes an item in the buffer.
func (b *BulkIndexer) Add(item BulkIndexerItem) error {
	action := item.Action
	if action == "" {
		action = ActionCreate
	}

	switch action {
	case ActionCreate, ActionDelete, ActionIndex, ActionUpdate:
	default:
		return fmt.Errorf("%s is not a valid action", action)
	}

	b.writeMeta(
		item.Index,
		item.DocumentID,
		item.Pipeline,
		action,
		item.DynamicTemplates,
		item.RequireDataStream,
	)
	if _, err := item.Body.WriteTo(b.writer); err != nil {
		return fmt.Errorf("failed to write bulk indexer item: %w", err)
	}
	if _, err := b.writer.Write([]byte("\n")); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}
	b.itemsAdded++
	return nil
}

func (b *BulkIndexer) writeMeta(
	index, documentID, pipeline, action string,
	dynamicTemplates map[string]string,
	requireDataStream bool,
) {
	b.jsonw.RawString(`{"`)
	b.jsonw.RawString(action)
	b.jsonw.RawString(`":{`)

	first := true
	if documentID != "" {
		b.jsonw.RawString(`"_id":`)
		b.jsonw.String(documentID)
		first = false
	}
	if index != "" {
		if !first {
			b.jsonw.RawByte(',')
		}
		b.jsonw.RawString(`"_index":`)
		b.jsonw.String(index)
		first = false
	}
	if pipeline != "" {
		if !first {
			b.jsonw.RawByte(',')
		}
		b.jsonw.RawString(`"pipeline":`)
		b.jsonw.String(pipeline)
		first = false
	}
	if len(dynamicTemplates) > 0 {
		if !first {
			b.jsonw.RawByte(',')
		}
		b.jsonw.RawString(`"dynamic_templates":{`)
		firstDynamicTemplate := true
		for k, v := range dynamicTemplates {
			if !firstDynamicTemplate {
				b.jsonw.RawByte(',')
			}
			b.jsonw.String(k)
			b.jsonw.RawByte(':')
			b.jsonw.String(v)
			firstDynamicTemplate = false
		}
		b.jsonw.RawByte('}')
		first = false
	}
	if requireDataStream {
		if !first {
			b.jsonw.RawByte(',')
		}
		b.jsonw.RawString(`"require_data_stream":true`)
		first = false
	}
	b.jsonw.RawString("}}\n")
	b.writer.Write(b.jsonw.Bytes())
	b.jsonw.Reset()
}

func (b *BulkIndexer) newBulkIndexRequest(ctx context.Context) (*http.Request, error) {
	// We should not pass the original b.buf bytes.Buffer down to the client/http layer because
	// the indexer will reuse the buffer. The underlying http client/transport implementation may keep
	// reading from the buffer after the request is done and the call to `req.Do` has returned.
	// This may happen in HTTP error cases when the server isn't required to read the full
	// request body before sending a response.
	// This can cause undefined behavior (and panics) due to concurrent reads/writes to bytes.Buffer
	// internal member variables (b.buf.off, b.buf.lastRead).
	// See: https://github.com/golang/go/issues/51907
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/_bulk", bytes.NewReader(b.buf.Bytes()))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add(HeaderEventCount, strconv.Itoa(b.itemsAdded))
	req.Header.Add(HeaderUncompressedLength, strconv.Itoa(b.UncompressedLen()))
	v := req.URL.Query()
	if b.config.Pipeline != "" {
		v.Set("pipeline", b.config.Pipeline)
	}
	if b.config.RequireDataStream {
		v.Set("require_data_stream", strconv.FormatBool(b.config.RequireDataStream))
	}
	v.Set("filter_path", "items.*._index,items.*.status,items.*.failure_store,items.*.error.type,items.*.error.reason")
	if b.config.IncludeSourceOnError != Unset {
		switch b.config.IncludeSourceOnError {
		case False:
			v.Set("include_source_on_error", "false")
		case True:
			v.Set("include_source_on_error", "true")
		}
	}

	req.URL.RawQuery = v.Encode()

	return req, nil
}

// Flush executes a bulk request if there are any items buffered, and clears out the buffer.
func (b *BulkIndexer) Flush(ctx context.Context) (BulkIndexerResponseStat, error) {
	if b.itemsAdded == 0 {
		return BulkIndexerResponseStat{}, nil
	}

	if b.gzipw != nil {
		if err := b.gzipw.Close(); err != nil {
			return BulkIndexerResponseStat{}, fmt.Errorf("failed closing the gzip writer: %w", err)
		}
	}

	req, err := b.newBulkIndexRequest(ctx)
	if err != nil {
		return BulkIndexerResponseStat{}, fmt.Errorf("failed to create bulk index request: %w", err)
	}

	if b.gzipw != nil {
		req.Header.Set("Content-Encoding", "gzip")
	}

	bytesFlushed := b.buf.Len()
	bytesUncompFlushed := b.writer.bytesWritten
	res, err := b.config.Client.Perform(req)
	if err != nil {
		b.resetBuf()
		return BulkIndexerResponseStat{}, fmt.Errorf("failed to execute the request: %w", err)
	}
	defer res.Body.Close()

	// original bulk request body
	bodyBuf := b.buf.Bytes()

	// Reset the buffer and gzip writer so they can be reused in case
	// document level retries are needed.
	b.resetBuf()

	// Record the number of flushed bytes only when err == nil. The body may
	// not have been sent otherwise.
	b.bytesFlushed = bytesFlushed
	b.bytesUncompFlushed = bytesUncompFlushed
	var resp BulkIndexerResponseStat
	if res.StatusCode > 299 {
		var s string
		if b.config.IncludeSourceOnError == Unset {
			var er struct {
				Error struct {
					Type     string `json:"type,omitempty"`
					Reason   string `json:"reason,omitempty"`
					CausedBy struct {
						Type   string `json:"type,omitempty"`
						Reason string `json:"reason,omitempty"`
					} `json:"caused_by,omitempty"`
				} `json:"error,omitempty"`
			}

			if err := jsoniter.NewDecoder(res.Body).Decode(&er); err == nil {
				er.Error.Reason = ""
				er.Error.CausedBy.Reason = ""

				b, _ := json.Marshal(&er)
				s = string(b)
			}
		} else {
			b, err := io.ReadAll(res.Body)
			if err != nil {
				return BulkIndexerResponseStat{}, fmt.Errorf("failed to read response body: %w", err)
			}
			s = string(b)
		}
		e := ErrorFlushFailed{resp: s, statusCode: res.StatusCode}
		switch {
		case res.StatusCode == 429:
			e.tooMany = true
		case res.StatusCode >= 500:
			e.serverError = true
		case res.StatusCode >= 400 && res.StatusCode != 429:
			e.clientError = true
		}
		return resp, e
	}

	if err := jsoniter.NewDecoder(res.Body).Decode(&resp); err != nil {
		return resp, fmt.Errorf("error decoding bulk response: %w", err)
	}

	if b.config.IncludeSourceOnError == Unset {
		for i, doc := range resp.FailedDocs {
			doc.Error.Reason = ""
			resp.FailedDocs[i] = doc
		}
	}

	if b.config.MaxDocumentRetries > 0 || b.config.PopulateFailedDocsInput {
		// Eliminate previous retry counts that aren't present in the bulk
		// request response.
		for k := range b.retryCounts {
			found := false
			for _, res := range resp.FailedDocs {
				if res.Position == k {
					found = true
					break
				}
			}
			if !found {
				// Retried request succeeded, remove from retry counts
				delete(b.retryCounts, k)
			}
		}

		if len(resp.FailedDocs) > 0 {
			buf := make([]byte, 0, 4096)

			copyBuf := make([]byte, len(bodyBuf))
			copy(copyBuf, bodyBuf)

			var gr *gzip.Reader
			if b.gzipw != nil {
				gr, err = gzip.NewReader(bytes.NewReader(copyBuf))
				if err != nil {
					return resp, fmt.Errorf("failed to decompress request payload: %w", err)
				}
				defer gr.Close()
			}

			lastln := 0
			lastIdx := 0
			// keep track of the previous newlines
			// the buffer is being read lazily
			seen := 0

			// writeItemAtPos writes the 2 lines for document at position `pos` in bulk request to io.Writer `to`
			writeItemAtPos := func(to io.Writer, pos int) error {
				// there are two lines for each document:
				// - action
				// - document
				//
				// Find the document by looking up the newline separators.
				// First the newline (if exists) before the 'action' then the
				// newline at the end of the 'document' line.
				startln := pos * 2
				endln := startln + 2

				if b.gzipw != nil {
					// First loop, read from the gzip reader
					if len(buf) == 0 {
						n, err := gr.Read(buf[:cap(buf)])
						if err != nil && err != io.EOF {
							return fmt.Errorf("failed to read from compressed buffer: %w", err)
						}
						buf = buf[:n]
					}

					// newlines in the current buf
					newlines := bytes.Count(buf, []byte{'\n'})

					// loop until we've seen the start newline
					for seen+newlines < startln {
						seen += newlines
						n, err := gr.Read(buf[:cap(buf)])
						if err != nil && err != io.EOF {
							return fmt.Errorf("failed to read from compressed buffer: %w", err)
						}
						buf = buf[:n]
						newlines = bytes.Count(buf, []byte{'\n'})
					}

					startIdx := indexnth(buf, startln-seen, '\n') + 1
					endIdx := indexnth(buf, endln-seen, '\n') + 1

					// If the end newline is not in the buffer read more data
					if endIdx == 0 {
						// Write what we have
						to.Write(buf[startIdx:])

						// loop until we've seen the end newline
						for seen+newlines < endln {
							seen += newlines
							n, err := gr.Read(buf[:cap(buf)])
							if err != nil && err != io.EOF {
								return fmt.Errorf("failed to read from compressed buffer: %w", err)
							}
							buf = buf[:n]
							newlines = bytes.Count(buf, []byte{'\n'})
							if seen+newlines < endln {
								// endln is not here, write what we have and keep going
								to.Write(buf)
							}
						}

						// try again to find the end newline in the extra data
						// we just read.
						endIdx = indexnth(buf, endln-seen, '\n') + 1
						to.Write(buf[:endIdx])
					} else {
						// If the end newline is in the buffer write the event
						to.Write(buf[startIdx:endIdx])
					}
				} else {
					startIdx := indexnth(copyBuf[lastIdx:], startln-lastln, '\n') + 1
					endIdx := indexnth(copyBuf[lastIdx:], endln-lastln, '\n') + 1

					to.Write(copyBuf[lastIdx:][startIdx:endIdx])

					lastln = endln
					lastIdx += endIdx
				}
				return nil
			}

			// inputBuf is only used to populate failed item input
			var inputBuf bytes.Buffer

			nonRetriable := resp.FailedDocs[:0]
			// appendNonRetriable appends an item deemed non-retriable to the nonRetriable slice.
			// At the same time, it optionally populates item.Input with the action and document lines of the document.
			appendNonRetriable := func(item BulkIndexerResponseItem) (err error) {
				if b.config.PopulateFailedDocsInput {
					inputBuf.Reset()
					// In an ideal world, PopulateFailedDocsInput failures should not cause a flush failure.
					// But since this is only for debugging / testing, and any writeItemAtPos would reveal a bug in the code,
					// fail fast and explicitly.
					err = writeItemAtPos(&inputBuf, item.Position)
					item.Input = inputBuf.String()
				}
				nonRetriable = append(nonRetriable, item)
				return
			}

			for _, item := range resp.FailedDocs {
				if b.config.MaxDocumentRetries > 0 && b.shouldRetryOnStatus(item.Status) {
					// Increment retry count for the positions found.
					count := b.retryCounts[item.Position] + 1
					// check if we are above the maxDocumentRetry setting
					if count > b.config.MaxDocumentRetries {
						// do not retry, return the document as failed
						if err := appendNonRetriable(item); err != nil {
							return resp, err
						}
						continue
					}
					if resp.GreatestRetry < count {
						resp.GreatestRetry = count
					}

					// Since some items may have succeeded, counter positions need
					// to be updated to match the next current buffer position.
					b.retryCounts[b.itemsAdded] = count

					if err := writeItemAtPos(b.writer, item.Position); err != nil {
						return resp, err
					}
					resp.RetriedDocs++
					b.itemsAdded++
				} else {
					// If it's not a retriable error, treat the document as failed
					if err := appendNonRetriable(item); err != nil {
						return resp, err
					}
				}
			}

			// FailedDocs contain responses of
			// - non-retriable errors
			// - retriable errors that reached the retry limit
			resp.FailedDocs = nonRetriable
		}
	}

	return resp, nil
}

func (b *BulkIndexer) shouldRetryOnStatus(docStatus int) bool {
	for _, status := range b.config.RetryOnDocumentStatus {
		if docStatus == status {
			return true
		}
	}
	return false
}

// indexnth returns the index of the nth instance of sep in s.
// It returns -1 if sep is not present in s or nth is 0.
func indexnth(s []byte, nth int, sep rune) int {
	if nth == 0 {
		return -1
	}

	count := 0
	return bytes.IndexFunc(s, func(r rune) bool {
		if r == sep {
			count++
		}
		return nth == count
	})
}

type ErrorFlushFailed struct {
	resp        string
	statusCode  int
	tooMany     bool
	clientError bool
	serverError bool
}

func (e ErrorFlushFailed) StatusCode() int {
	return e.statusCode
}

func (e ErrorFlushFailed) ResponseBody() string {
	return e.resp
}

func (e ErrorFlushFailed) Error() string {
	return fmt.Sprintf("flush failed (%d): %s", e.statusCode, e.resp)
}
