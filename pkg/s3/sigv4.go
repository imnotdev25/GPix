package s3

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	sigV4Algorithm    = "AWS4-HMAC-SHA256"
	streamingPayload  = "STREAMING-AWS4-HMAC-SHA256-PAYLOAD"
	streamingTrailer  = "STREAMING-AWS4-HMAC-SHA256-PAYLOAD-TRAILER"
	unsignedPayload   = "UNSIGNED-PAYLOAD"
	emptyStringSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	presignMaxExpiry  = 7 * 24 * time.Hour
)

// verifier validates AWS Signature V4 on inbound requests. It resolves the
// secret for a presented access key through a CredentialProvider, so rotating
// keys (e.g. from the web UI) takes effect immediately.
type verifier struct {
	provider CredentialProvider
}

// verify authenticates the request. It returns nil on success or an apiError to
// render. On success it also normalises the request body so that a streaming
// (aws-chunked) PutObject is transparently de-chunked for downstream handlers.
func (v *verifier) verify(r *http.Request) *apiError {
	if _, ok := r.URL.Query()["X-Amz-Signature"]; ok {
		return v.verifyPresigned(r)
	}
	auth := r.Header.Get("Authorization")
	if auth == "" {
		e := errMissingAuth
		return &e
	}
	return v.verifyHeader(r, auth)
}

type authParts struct {
	accessKey     string
	date          string // YYYYMMDD
	region        string
	service       string
	signedHeaders []string
	signature     string
}

func parseAuthorization(auth string) (authParts, bool) {
	if !strings.HasPrefix(auth, sigV4Algorithm) {
		return authParts{}, false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(auth, sigV4Algorithm))
	var ap authParts
	for _, kv := range strings.Split(rest, ",") {
		kv = strings.TrimSpace(kv)
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(kv[:eq])
		val := strings.TrimSpace(kv[eq+1:])
		switch key {
		case "Credential":
			cp := strings.Split(val, "/")
			if len(cp) != 5 {
				return authParts{}, false
			}
			ap.accessKey, ap.date, ap.region, ap.service = cp[0], cp[1], cp[2], cp[3]
		case "SignedHeaders":
			ap.signedHeaders = strings.Split(val, ";")
		case "Signature":
			ap.signature = val
		}
	}
	if ap.accessKey == "" || ap.signature == "" || len(ap.signedHeaders) == 0 {
		return authParts{}, false
	}
	return ap, true
}

func (v *verifier) verifyHeader(r *http.Request, auth string) *apiError {
	ap, ok := parseAuthorization(auth)
	if !ok {
		e := errMissingAuth
		return &e
	}
	secret, ok := v.provider.Lookup(ap.accessKey)
	if !ok {
		e := errInvalidAccessKeyID
		return &e
	}

	amzDate := r.Header.Get("X-Amz-Date")
	if amzDate == "" {
		// Some clients send the standard Date header instead.
		amzDate = r.Header.Get("Date")
	}

	payloadHash := r.Header.Get("X-Amz-Content-Sha256")
	if payloadHash == "" {
		payloadHash = unsignedPayload
	}

	canonReq := canonicalRequest(r, ap.signedHeaders, payloadHash, false)
	sts := stringToSign(amzDate, ap.date, ap.region, ap.service, canonReq)
	want := computeSignature(secret, ap.date, ap.region, ap.service, sts)

	if !hmac.Equal([]byte(want), []byte(ap.signature)) {
		e := errSignatureMismatch
		return &e
	}

	// For streaming uploads (any STREAMING-* variant, including the newer
	// checksum-trailer forms), replace the body with a de-chunked reader. The
	// de-chunker ignores any trailing checksum after the terminating chunk.
	if strings.HasPrefix(payloadHash, "STREAMING-") ||
		strings.Contains(strings.ToLower(r.Header.Get("Content-Encoding")), "aws-chunked") {
		r.Body = io.NopCloser(newChunkedReader(r.Body))
		if dl := r.Header.Get("X-Amz-Decoded-Content-Length"); dl != "" {
			if n, err := strconv.ParseInt(dl, 10, 64); err == nil {
				r.ContentLength = n
			}
		}
	}
	return nil
}

func (v *verifier) verifyPresigned(r *http.Request) *apiError {
	q := r.URL.Query()
	if q.Get("X-Amz-Algorithm") != sigV4Algorithm {
		e := errMissingAuth
		return &e
	}
	cred := q.Get("X-Amz-Credential")
	cp := strings.Split(cred, "/")
	if len(cp) != 5 {
		e := errMissingAuth
		return &e
	}
	accessKey, date, region, service := cp[0], cp[1], cp[2], cp[3]
	secret, ok := v.provider.Lookup(accessKey)
	if !ok {
		e := errInvalidAccessKeyID
		return &e
	}

	amzDate := q.Get("X-Amz-Date")
	if exp := q.Get("X-Amz-Expires"); exp != "" {
		secs, err := strconv.Atoi(exp)
		if err != nil || time.Duration(secs)*time.Second > presignMaxExpiry {
			e := errMissingAuth
			return &e
		}
		if t, err := time.Parse("20060102T150405Z", amzDate); err == nil {
			if time.Now().UTC().After(t.Add(time.Duration(secs) * time.Second)) {
				e := errRequestExpired
				return &e
			}
		}
	}

	signedHeaders := strings.Split(q.Get("X-Amz-SignedHeaders"), ";")
	provided := q.Get("X-Amz-Signature")

	canonReq := canonicalRequest(r, signedHeaders, unsignedPayload, true)
	sts := stringToSign(amzDate, date, region, service, canonReq)
	want := computeSignature(secret, date, region, service, sts)
	if !hmac.Equal([]byte(want), []byte(provided)) {
		e := errSignatureMismatch
		return &e
	}
	return nil
}

// canonicalRequest builds the AWS SigV4 canonical request string.
func canonicalRequest(r *http.Request, signedHeaders []string, payloadHash string, presigned bool) string {
	var b strings.Builder
	b.WriteString(r.Method)
	b.WriteByte('\n')
	b.WriteString(canonicalURI(r.URL.Path))
	b.WriteByte('\n')
	b.WriteString(canonicalQuery(r, presigned))
	b.WriteByte('\n')

	lower := make([]string, len(signedHeaders))
	for i, h := range signedHeaders {
		lower[i] = strings.ToLower(strings.TrimSpace(h))
	}
	sort.Strings(lower)
	for _, h := range lower {
		b.WriteString(h)
		b.WriteByte(':')
		b.WriteString(signedHeaderValue(r, h))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	b.WriteString(strings.Join(lower, ";"))
	b.WriteByte('\n')
	b.WriteString(payloadHash)
	return b.String()
}

func signedHeaderValue(r *http.Request, name string) string {
	switch name {
	case "host":
		return r.Host
	case "content-length":
		if r.ContentLength >= 0 {
			return strconv.FormatInt(r.ContentLength, 10)
		}
		return ""
	default:
		vals := r.Header.Values(http.CanonicalHeaderKey(name))
		for i := range vals {
			// trim and collapse internal whitespace runs to a single space
			vals[i] = strings.Join(strings.Fields(vals[i]), " ")
		}
		return strings.Join(vals, ",")
	}
}

// canonicalURI single-encodes each path segment (S3 rule), preserving slashes.
func canonicalURI(path string) string {
	if path == "" {
		return "/"
	}
	segs := strings.Split(path, "/")
	for i, s := range segs {
		segs[i] = awsURIEncode(s, false)
	}
	return strings.Join(segs, "/")
}

// canonicalQuery builds the sorted, encoded canonical query string. For
// presigned requests the X-Amz-Signature parameter is excluded.
func canonicalQuery(r *http.Request, presigned bool) string {
	q := r.URL.Query()
	type kv struct{ k, v string }
	var pairs []kv
	for key, vals := range q {
		if presigned && key == "X-Amz-Signature" {
			continue
		}
		ek := awsURIEncode(key, true)
		if len(vals) == 0 {
			pairs = append(pairs, kv{ek, ""})
			continue
		}
		for _, val := range vals {
			pairs = append(pairs, kv{ek, awsURIEncode(val, true)})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k != pairs[j].k {
			return pairs[i].k < pairs[j].k
		}
		return pairs[i].v < pairs[j].v
	})
	var parts []string
	for _, p := range pairs {
		parts = append(parts, p.k+"="+p.v)
	}
	return strings.Join(parts, "&")
}

func stringToSign(amzDate, date, region, service, canonReq string) string {
	scope := strings.Join([]string{date, region, service, "aws4_request"}, "/")
	h := sha256.Sum256([]byte(canonReq))
	return strings.Join([]string{
		sigV4Algorithm,
		amzDate,
		scope,
		hex.EncodeToString(h[:]),
	}, "\n")
}

func computeSignature(secret, date, region, service, sts string) string {
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	kSigning := hmacSHA256(kService, "aws4_request")
	return hex.EncodeToString(hmacSHA256(kSigning, sts))
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// awsURIEncode implements the encoding AWS uses for SigV4 canonicalisation.
// Unreserved characters (A-Za-z0-9-._~) pass through; everything else becomes
// %XX (uppercase). When encodeSlash is false, '/' is left as-is (used for the
// canonical URI path).
func awsURIEncode(s string, encodeSlash bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && !encodeSlash:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// --- aws-chunked decoding (for STREAMING-AWS4-HMAC-SHA256-PAYLOAD bodies) ---

// chunkedReader strips the aws-chunked framing from a streaming upload body,
// yielding the raw object bytes. It does NOT verify the per-chunk signatures —
// authentication is established by the seed (header) signature.
type chunkedReader struct {
	br        *bufio.Reader
	remaining int64
	done      bool
	err       error
}

func newChunkedReader(r io.Reader) *chunkedReader {
	return &chunkedReader{br: bufio.NewReader(r)}
}

func (c *chunkedReader) Read(p []byte) (int, error) {
	if c.err != nil {
		return 0, c.err
	}
	if c.remaining == 0 {
		if c.done {
			return 0, io.EOF
		}
		if err := c.nextChunk(); err != nil {
			c.err = err
			return 0, err
		}
		if c.remaining == 0 {
			c.done = true
			return 0, io.EOF
		}
	}
	if int64(len(p)) > c.remaining {
		p = p[:c.remaining]
	}
	n, err := c.br.Read(p)
	c.remaining -= int64(n)
	if c.remaining == 0 && err == nil {
		// consume the trailing CRLF after the chunk data
		_, _ = c.br.Discard(2)
	}
	if err == io.EOF {
		c.done = true
	}
	return n, err
}

// nextChunk reads a chunk header line "<hexlen>;chunk-signature=...\r\n" and
// sets remaining to the parsed length.
func (c *chunkedReader) nextChunk() error {
	line, err := c.br.ReadString('\n')
	if err != nil && line == "" {
		return err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		// stray blank line; try once more
		line, err = c.br.ReadString('\n')
		if err != nil && line == "" {
			return err
		}
		line = strings.TrimRight(line, "\r\n")
	}
	hexLen := line
	if i := strings.IndexByte(line, ';'); i >= 0 {
		hexLen = line[:i]
	}
	n, perr := strconv.ParseInt(strings.TrimSpace(hexLen), 16, 64)
	if perr != nil {
		return fmt.Errorf("s3: bad chunk size %q: %w", hexLen, perr)
	}
	c.remaining = n
	if n == 0 {
		// trailing CRLF(s) after the final 0-chunk; best-effort consume
		_, _ = c.br.Discard(2)
	}
	return nil
}
