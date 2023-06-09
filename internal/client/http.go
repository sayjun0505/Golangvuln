// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"

	"golang.org/x/vuln/internal"
	"golang.org/x/vuln/internal/derrors"
	"golang.org/x/vuln/internal/osv"
)

type httpSource struct {
	c   *http.Client
	url string // the base URI of the source (without trailing "/"). e.g. https://vuln.golang.org

	// indexCalls counts the number of times Index() has been called.
	// httpCalls counts the number of times ByModule makes an http request
	// to  vulndb for a module path. Used for testing privacy properties of
	// httpSource.
	indexCalls int
	httpCalls  int
}

func newHTTPClient(uri *url.URL, opts Options) (_ *httpSource) {
	hs := &httpSource{url: uri.String()}
	if opts.HTTPClient != nil {
		hs.c = opts.HTTPClient
	} else {
		hs.c = new(http.Client)
	}
	return hs
}

func (hs *httpSource) Index(ctx context.Context) (_ DBIndex, err error) {
	hs.indexCalls++ // for testing privacy properties
	defer derrors.Wrap(&err, "Index()")
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/index.json", hs.url), nil)
	if err != nil {
		return nil, err
	}
	resp, err := hs.c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var index DBIndex
	if err = json.Unmarshal(b, &index); err != nil {
		return nil, err
	}
	return index, nil
}

func (hs *httpSource) ByModule(ctx context.Context, modulePath string) (_ []*osv.Entry, err error) {
	defer derrors.Wrap(&err, "httpSource.ByModule(%q)", modulePath)
	index, err := hs.Index(ctx)
	if err != nil {
		return nil, err
	}
	_, present := index[modulePath]
	if !present {
		return nil, nil
	}
	epath, err := EscapeModulePath(modulePath)
	if err != nil {
		return nil, err
	}
	hs.httpCalls++ // for testing privacy properties
	entries, err := httpReadJSON[[]*osv.Entry](ctx, hs, epath+".json")
	if err != nil || entries == nil {
		return nil, err
	}
	return entries, nil
}

func (hs *httpSource) ByID(ctx context.Context, id string) (_ *osv.Entry, err error) {
	defer derrors.Wrap(&err, "ByID(%q)", id)

	return httpReadJSON[*osv.Entry](ctx, hs, fmt.Sprintf("%s/%s.json", internal.IDDirectory, id))
}

func (hs *httpSource) ListIDs(ctx context.Context) (_ []string, err error) {
	defer derrors.Wrap(&err, "ListIDs()")

	return httpReadJSON[[]string](ctx, hs, path.Join(internal.IDDirectory, "index.json"))
}

func httpReadJSON[T any](ctx context.Context, hs *httpSource, relativePath string) (T, error) {
	var zero T
	content, err := hs.readBody(ctx, fmt.Sprintf("%s/%s", hs.url, relativePath))
	if err != nil {
		return zero, err
	}
	if len(content) == 0 {
		return zero, nil
	}
	var t T
	if err := json.Unmarshal(content, &t); err != nil {
		return zero, err
	}
	return t, nil
}

// This is the format for the last-modified header, as described at
// https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Last-Modified.
var lastModifiedFormat = "Mon, 2 Jan 2006 15:04:05 GMT"

func (hs *httpSource) LastModifiedTime(ctx context.Context) (_ time.Time, err error) {
	defer derrors.Wrap(&err, "LastModifiedTime()")

	// Assume that if anything changes, the index does.
	url := fmt.Sprintf("%s/index.json", hs.url)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return time.Time{}, err
	}
	resp, err := hs.c.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	if resp.StatusCode != 200 {
		return time.Time{}, fmt.Errorf("got status code %d", resp.StatusCode)
	}
	h := resp.Header.Get("Last-Modified")
	return time.Parse(lastModifiedFormat, h)
}

func (hs *httpSource) readBody(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hs.c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("got HTTP status %s", resp.Status)
	}
	// might want this to be a LimitedReader
	return io.ReadAll(resp.Body)
}

type Options struct {
	HTTPClient *http.Client
}
