/*
 * MinIO Cloud Storage, (C) 2018 MinIO, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package net

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"path"
	"strings"
)

// URL - improved JSON friendly url.URL.
type URL url.URL

// IsEmpty - checks URL is empty or not.
func (u URL) IsEmpty() bool {
	return u.String() == ""
}

// String - returns string representation of URL.
func (u URL) String() string {
	// if port number 80 and 443, remove for http and https scheme respectively
	if u.Host != "" {
		host, err := ParseHost(u.Host)
		if err != nil {
			panic(err)
		}
		switch {
		case u.Scheme == "http" && host.Port == 80:
			fallthrough
		case u.Scheme == "https" && host.Port == 443:
			u.Host = host.Name
		}
	}

	uu := url.URL(u)
	return uu.String()
}

// MarshalJSON - converts to JSON string data.
func (u URL) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.String())
}

// UnmarshalJSON - parses given data into URL.
func (u *URL) UnmarshalJSON(data []byte) (err error) {
	var s string
	if err = json.Unmarshal(data, &s); err != nil {
		return err
	}

	// Allow empty string
	if s == "" {
		*u = URL{}
		return nil
	}

	var ru *URL
	if ru, err = ParseURL(s); err != nil {
		return err
	}

	*u = *ru
	return nil
}

// ParseHTTPURL - parses a string into HTTP URL, string is
// expected to be of form http:// or https://
func ParseHTTPURL(s string) (u *URL, err error) {
	u, err = ParseURL(s)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	default:
		return nil, fmt.Errorf("unexpected scheme found %s", u.Scheme)
	case "http", "https":
		return u, nil
	}
}

// ParseURL - parses string into URL.
func ParseURL(s string) (u *URL, err error) {
	var uu *url.URL
	if uu, err = url.Parse(s); err != nil {
		return nil, err
	}

	if uu.Hostname() == "" {
		if uu.Scheme != "" {
			return nil, errors.New("scheme appears with empty host")
		}
	} else {
		portStr := uu.Port()
		if portStr == "" {
			switch uu.Scheme {
			case "http":
				portStr = "80"
			case "https":
				portStr = "443"
			}
		}
		if _, err = ParseHost(net.JoinHostPort(uu.Hostname(), portStr)); err != nil {
			return nil, err
		}
	}

	// Clean path in the URL.
	// Note: path.Clean() is used on purpose because in MS Windows filepath.Clean() converts
	// `/` into `\` ie `/foo` becomes `\foo`
	if uu.Path != "" {
		uu.Path = path.Clean(uu.Path)
	}

	// path.Clean removes the trailing '/' and converts '//' to '/'.
	if strings.HasSuffix(s, "/") && !strings.HasSuffix(uu.Path, "/") {
		uu.Path += "/"
	}

	v := URL(*uu)
	u = &v
	return u, nil
}

// IsNetworkOrHostDown - if there was a network error or if the host is down.
// expectTimeouts indicates that *context* timeouts are expected and does not
// indicate a downed host. Other timeouts still returns down.
func IsNetworkOrHostDown(err error, expectTimeouts bool) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) {
		return false
	}

	if expectTimeouts && errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// We need to figure if the error either a timeout
	// or a non-temporary error.
	urlErr := &url.Error{}
	if errors.As(err, &urlErr) {
		switch urlErr.Err.(type) {
		case *net.DNSError, *net.OpError, net.UnknownNetworkError:
			return true
		}
	}

	var e net.Error
	if errors.As(err, &e) {
		if e.Timeout() {
			return true
		}
	}

	// Fallback to other mechanisms.
	switch {
	case strings.Contains(err.Error(), "Connection closed by foreign host"):
		return true
	case strings.Contains(err.Error(), "TLS handshake timeout"):
		// If error is - tlsHandshakeTimeoutError.
		return true
	case strings.Contains(err.Error(), "i/o timeout"):
		// If error is - tcp timeoutError.
		return true
	case strings.Contains(err.Error(), "connection timed out"):
		// If err is a net.Dial timeout.
		return true
	case strings.Contains(strings.ToLower(err.Error()), "503 service unavailable"):
		// Denial errors
		return true
	}
	return false
}
