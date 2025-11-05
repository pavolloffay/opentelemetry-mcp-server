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

package json

import (
	"io"

	structform "github.com/elastic/go-structform"
)

type Decoder struct {
	in      io.Reader
	buffer  []byte
	p       Parser
}

func NewDecoder(in io.Reader, buffer int, vs structform.Visitor) *Decoder {
	dec := &Decoder{
		buffer: make([]byte, 0, buffer),
		in:     in,
	}
	dec.p.init(vs)
	return dec
}

func NewBytesDecoder(b []byte, vs structform.Visitor) *Decoder {
	dec := &Decoder{
		buffer: b,
		in:     nil,
	}
	dec.p.init(vs)
	return dec
}

func (dec *Decoder) Next() error {
	var (
		n        int
		err      error
		reported bool
	)

	for !reported {
		if len(dec.buffer) == 0 {
			if dec.in == nil {
				if err := dec.p.finalize(); err != nil {
					return err
				}
				return io.EOF
			}

			n, err := dec.in.Read(dec.buffer)
			dec.buffer = dec.buffer[:n]
			if n == 0 && err != nil {
				return err
			}
		}

		n, reported, err = dec.p.feedUntil(dec.buffer)
		if err != nil {
			return err
		}

		dec.buffer = dec.buffer[n:]
		if reported {
			return nil
		}
	}

	return nil
}
