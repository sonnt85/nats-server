// Copyright 2012-2019 The NATS Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Ascii numbers 0-9
const (
	asciiZero = 48
	asciiNine = 57
)

// parseSize expects decimal positive numbers. We
// return -1 to signal error.
func parseSize(d []byte) (n int) {
	l := len(d)
	if l == 0 {
		return -1
	}
	var (
		i   int
		dec byte
	)

	// Note: Use `goto` here to avoid for loop in order
	// to have the function be inlined.
	// See: https://github.com/golang/go/issues/14768
loop:
	dec = d[i]
	if dec < asciiZero || dec > asciiNine {
		return -1
	}
	n = n*10 + (int(dec) - asciiZero)

	i++
	if i < l {
		goto loop
	}
	return n
}

// parseInt64 expects decimal positive numbers. We
// return -1 to signal error
func parseInt64(d []byte) (n int64) {
	if len(d) == 0 {
		return -1
	}
	for _, dec := range d {
		if dec < asciiZero || dec > asciiNine {
			return -1
		}
		n = n*10 + (int64(dec) - asciiZero)
	}
	return n
}

// Helper to move from float seconds to time.Duration
func secondsToDuration(seconds float64) time.Duration {
	ttl := seconds * float64(time.Second)
	return time.Duration(ttl)
}

// Parse a host/port string with a default port to use
// if none (or 0 or -1) is specified in `hostPort` string.
func parseHostPort(hostPort string, defaultPort int) (host string, port int, err error) {
	if hostPort != "" {
		host, sPort, err := net.SplitHostPort(hostPort)
		switch err.(type) {
		case *net.AddrError:
			// try appending the current port
			host, sPort, err = net.SplitHostPort(fmt.Sprintf("%s:%d", hostPort, defaultPort))
		}
		if err != nil {
			return "", -1, err
		}
		port, err = strconv.Atoi(strings.TrimSpace(sPort))
		if err != nil {
			return "", -1, err
		}
		if port == 0 || port == -1 {
			port = defaultPort
		}
		return strings.TrimSpace(host), port, nil
	}
	return "", -1, errors.New("no hostport specified")
}

// Returns true if URL u1 represents the same URL than u2,
// false otherwise.
func urlsAreEqual(u1, u2 *url.URL) bool {
	return reflect.DeepEqual(u1, u2)
}

func serializeListOfStrings(compressThreshold int, strings []string) ([]byte, error) {
	type listStrings struct {
		Strings []string `json:"strings"`
	}
	l := &listStrings{Strings: strings}
	// Serialize data
	stringsb, err := json.Marshal(l)
	if err != nil {
		return nil, err
	}
	compress := len(stringsb) > compressThreshold
	b := &bytes.Buffer{}
	if !compress {
		b.Write([]byte{0})
		b.Write(stringsb)
	} else {
		// Indicate that following is compressed data
		b.Write([]byte{1})
		// Create compressor
		w := gzip.NewWriter(b)
		// Compress
		if _, err := w.Write(stringsb); err != nil {
			return nil, err
		}
		// Need to close to finish compression
		if err := w.Close(); err != nil {
			return nil, err
		}
	}
	return b.Bytes(), nil
}

func deserializeListOfStrings(encodedStrings []byte) ([]string, error) {
	if len(encodedStrings) <= 1 {
		return nil, fmt.Errorf("corrupted data")
	}
	type listStrings struct {
		Strings []string `json:"strings"`
	}
	var data []byte
	encoding := encodedStrings[0]
	switch encoding {
	case 0:
		data = encodedStrings[1:]
	case 1:
		gr, err := gzip.NewReader(bytes.NewBuffer(encodedStrings[1:]))
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		data, err = ioutil.ReadAll(gr)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown compression mode: %v", encoding)
	}
	l := &listStrings{}
	if err := json.Unmarshal(data, l); err != nil {
		return nil, err
	}
	return l.Strings, nil
}
