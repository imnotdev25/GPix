package s3

import (
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"gpix/pkg/store"
)

func (s *Server) writeXML(w http.ResponseWriter, r *http.Request, status int, v any) {
	out, err := xml.Marshal(v)
	if err != nil {
		writeError(w, r, errInternal("xml: "+err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("x-amz-request-id", requestID(r))
	w.WriteHeader(status)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(out)
}

func (s *Server) handleListBuckets(w http.ResponseWriter, r *http.Request) {
	res := listAllMyBucketsResult{}
	res.Owner = owner{ID: "gpix", DisplayName: "gpix"}
	res.Buckets.Bucket = []bucketEntry{{
		Name:         s.cfg.Bucket,
		CreationDate: iso8601(time.Unix(0, 0)),
	}}
	s.writeXML(w, r, http.StatusOK, res)
}

func (s *Server) handleHeadBucket(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("x-amz-bucket-region", s.cfg.Region)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleListObjects(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	v2 := q.Get("list-type") == "2"
	prefix := q.Get("prefix")
	delimiter := q.Get("delimiter")

	maxKeys := 1000
	if v := q.Get("max-keys"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			maxKeys = n
		}
	}
	if maxKeys > 1000 {
		maxKeys = 1000
	}

	var startAfter, continuationToken, marker string
	if v2 {
		startAfter = q.Get("start-after")
		continuationToken = q.Get("continuation-token")
		if continuationToken != "" {
			startAfter = continuationToken
		}
	} else {
		marker = q.Get("marker")
		startAfter = marker
	}

	objs, err := s.be.List(r.Context())
	if err != nil {
		writeError(w, r, errInternal("list: "+err.Error()))
		return
	}

	// Filter by prefix and index by key.
	byKey := make(map[string]store.Object, len(objs))
	keys := make([]string, 0, len(objs))
	for _, o := range objs {
		if prefix != "" && !strings.HasPrefix(o.Key, prefix) {
			continue
		}
		byKey[o.Key] = o
		keys = append(keys, o.Key)
	}
	sort.Strings(keys)

	// Build ordered, de-duplicated entries (objects + common prefixes).
	type entry struct {
		name     string // sort/marker key
		isPrefix bool
	}
	var entries []entry
	seenPrefix := map[string]bool{}
	for _, k := range keys {
		if delimiter != "" {
			rest := k[len(prefix):]
			if i := strings.Index(rest, delimiter); i >= 0 {
				cp := prefix + rest[:i+len(delimiter)]
				if !seenPrefix[cp] {
					seenPrefix[cp] = true
					entries = append(entries, entry{name: cp, isPrefix: true})
				}
				continue
			}
		}
		entries = append(entries, entry{name: k, isPrefix: false})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })

	res := listBucketResult{
		Name:      s.cfg.Bucket,
		Prefix:    prefix,
		Delimiter: delimiter,
		MaxKeys:   maxKeys,
	}
	if v2 {
		res.ContinuationToken = continuationToken
		res.StartAfter = q.Get("start-after")
	} else {
		res.Marker = marker
	}

	count := 0
	var lastName string
	truncated := false
	for _, e := range entries {
		if startAfter != "" && e.name <= startAfter {
			continue
		}
		if count >= maxKeys {
			truncated = true
			break
		}
		if e.isPrefix {
			res.CommonPrefixes = append(res.CommonPrefixes, commonPrefix{Prefix: e.name})
		} else {
			o := byKey[e.name]
			res.Contents = append(res.Contents, objectEntry{
				Key:          o.Key,
				LastModified: iso8601(o.ModTime),
				ETag:         `"` + o.ETag + `"`,
				Size:         o.Size,
				StorageClass: "STANDARD",
			})
		}
		count++
		lastName = e.name
	}

	res.IsTruncated = truncated
	if v2 {
		res.KeyCount = len(res.Contents) + len(res.CommonPrefixes)
		if truncated {
			res.NextContinuationToken = lastName
		}
	} else if truncated {
		res.NextMarker = lastName
	}

	s.writeXML(w, r, http.StatusOK, res)
}

func setObjectHeaders(w http.ResponseWriter, o store.Object) {
	if o.ContentType != "" {
		w.Header().Set("Content-Type", o.ContentType)
	}
	if !o.ModTime.IsZero() {
		w.Header().Set("Last-Modified", o.ModTime.UTC().Format(http.TimeFormat))
	}
	if o.ETag != "" {
		w.Header().Set("ETag", `"`+o.ETag+`"`)
	}
	w.Header().Set("Accept-Ranges", "bytes")
}

func (s *Server) handleHeadObject(w http.ResponseWriter, r *http.Request, key string) {
	o, err := s.be.Stat(r.Context(), key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// HEAD has no body; emit bare 404.
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	setObjectHeaders(w, o)
	if o.Size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(o.Size, 10))
	}
	w.Header().Set("x-amz-request-id", requestID(r))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleGetObject(w http.ResponseWriter, r *http.Request, key string) {
	rc, o, err := s.be.Get(r.Context(), key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, r, errNoSuchKey)
			return
		}
		writeError(w, r, errInternal("get: "+err.Error()))
		return
	}
	defer rc.Close()

	setObjectHeaders(w, o)
	w.Header().Set("x-amz-request-id", requestID(r))

	// Range support (single range; served by skip+limit over the stream).
	if rng := r.Header.Get("Range"); rng != "" && o.Size >= 0 {
		start, length, ok := parseRange(rng, o.Size)
		if !ok {
			w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(o.Size, 10))
			writeError(w, r, apiError{Code: "InvalidRange", Message: "The requested range is not satisfiable.", Status: http.StatusRequestedRangeNotSatisfiable})
			return
		}
		if start > 0 {
			if _, err := io.CopyN(io.Discard, rc, start); err != nil {
				return
			}
		}
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+strconv.FormatInt(start+length-1, 10)+"/"+strconv.FormatInt(o.Size, 10))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.CopyN(w, rc, length)
		return
	}

	if o.Size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(o.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func (s *Server) handlePutObject(w http.ResponseWriter, r *http.Request, key string) {
	// Reject copy/multipart sub-resources we don't support, but be lenient
	// about unknown query params.
	if _, ok := r.URL.Query()["uploadId"]; ok {
		writeError(w, r, errNotImplemented)
		return
	}
	if r.Header.Get("x-amz-copy-source") != "" {
		writeError(w, r, errNotImplemented)
		return
	}

	contentType := r.Header.Get("Content-Type")
	o, err := s.be.Put(r.Context(), key, r.Body, r.ContentLength, contentType)
	if err != nil {
		writeError(w, r, errInternal("put: "+err.Error()))
		return
	}
	if o.ETag != "" {
		w.Header().Set("ETag", `"`+o.ETag+`"`)
	}
	w.Header().Set("x-amz-request-id", requestID(r))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDeleteObject(w http.ResponseWriter, r *http.Request, key string) {
	err := s.be.Delete(r.Context(), key)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		writeError(w, r, errInternal("delete: "+err.Error()))
		return
	}
	w.Header().Set("x-amz-request-id", requestID(r))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteObjects(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		writeError(w, r, errInternal("read: "+err.Error()))
		return
	}
	var req deleteRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		writeError(w, r, apiError{Code: "MalformedXML", Message: err.Error(), Status: http.StatusBadRequest})
		return
	}
	var res deleteResult
	for _, obj := range req.Objects {
		derr := s.be.Delete(r.Context(), obj.Key)
		if derr != nil && !errors.Is(derr, store.ErrNotFound) {
			res.Errors = append(res.Errors, deleteErrorEntry{Key: obj.Key, Code: "InternalError", Message: derr.Error()})
			continue
		}
		if !req.Quiet {
			res.Deleted = append(res.Deleted, deletedEntry{Key: obj.Key})
		}
	}
	s.writeXML(w, r, http.StatusOK, res)
}

// parseRange parses a single HTTP byte range against a known size. It supports
// "bytes=start-end", "bytes=start-" and "bytes=-suffix". Returns the absolute
// start offset and length to serve.
func parseRange(header string, size int64) (start, length int64, ok bool) {
	const p = "bytes="
	if !strings.HasPrefix(header, p) {
		return 0, 0, false
	}
	spec := strings.TrimSpace(header[len(p):])
	if strings.ContainsRune(spec, ',') {
		return 0, 0, false // multi-range not supported
	}
	dash := strings.IndexByte(spec, '-')
	if dash < 0 {
		return 0, 0, false
	}
	startStr := strings.TrimSpace(spec[:dash])
	endStr := strings.TrimSpace(spec[dash+1:])

	if startStr == "" {
		// suffix range: last N bytes
		n, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		if n > size {
			n = size
		}
		return size - n, n, true
	}

	start, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false
	}
	end := size - 1
	if endStr != "" {
		end, err = strconv.ParseInt(endStr, 10, 64)
		if err != nil || end < start {
			return 0, 0, false
		}
	}
	if end >= size {
		end = size - 1
	}
	return start, end - start + 1, true
}
