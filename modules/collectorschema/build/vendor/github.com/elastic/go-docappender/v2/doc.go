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

// Package docappender provides an API for append-only bulk document indexing
// into Elasticsearch.
//
// This package provides an intentionally simpler and more restrictive API than
// the go-elasticsearch/esutil.BulkIndexer API; it is not intended to cover all
// bulk API use cases. It is intended to be used for conflict-free, append-only
// indexing into Elasticsearch data streams.
package docappender
