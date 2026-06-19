package s3

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestGetVanillaVector checks the canonical request and final signature against
// AWS's published Signature V4 test-suite case "get-vanilla".
// https://docs.aws.amazon.com/general/latest/gr/signature-v4-test-suite.html
func TestGetVanillaVector(t *testing.T) {
	const (
		secret  = "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"
		amzDate = "20150830T123600Z"
		date    = "20150830"
		region  = "us-east-1"
		service = "service"
		wantSig = "5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31"
	)

	r := httptest.NewRequest("GET", "https://example.amazonaws.com/", nil)
	r.Host = "example.amazonaws.com"
	r.Header.Set("X-Amz-Date", amzDate)

	cr := canonicalRequest(r, []string{"host", "x-amz-date"}, emptyStringSHA256, false)
	wantCR := "GET\n/\n\n" +
		"host:example.amazonaws.com\n" +
		"x-amz-date:20150830T123600Z\n" +
		"\n" +
		"host;x-amz-date\n" +
		emptyStringSHA256
	if cr != wantCR {
		t.Fatalf("canonical request mismatch:\n got:\n%q\nwant:\n%q", cr, wantCR)
	}

	sts := stringToSign(amzDate, date, region, service, cr)
	got := computeSignature(secret, date, region, service, sts)
	if got != wantSig {
		t.Fatalf("signature mismatch:\n got: %s\nwant: %s", got, wantSig)
	}
}

func TestVerifyHeaderRoundTrip(t *testing.T) {
	const accessKey, secretKey = "AKID", "secret123"
	v := &verifier{provider: staticCreds{access: accessKey, secret: secretKey}}

	const (
		amzDate = "20240101T000000Z"
		date    = "20240101"
		region  = "us-east-1"
		service = "s3"
	)
	signed := []string{"host", "x-amz-content-sha256", "x-amz-date"}

	r := httptest.NewRequest("PUT", "http://127.0.0.1:9000/gpix/hello.txt", strings.NewReader("hi"))
	r.Host = "127.0.0.1:9000"
	r.Header.Set("X-Amz-Date", amzDate)
	r.Header.Set("X-Amz-Content-Sha256", unsignedPayload)

	cr := canonicalRequest(r, signed, unsignedPayload, false)
	sts := stringToSign(amzDate, date, region, service, cr)
	sig := computeSignature(secretKey, date, region, service, sts)
	auth := sigV4Algorithm +
		" Credential=" + accessKey + "/" + date + "/" + region + "/" + service + "/aws4_request" +
		", SignedHeaders=" + strings.Join(signed, ";") +
		", Signature=" + sig
	r.Header.Set("Authorization", auth)

	if e := v.verify(r); e != nil {
		t.Fatalf("expected valid signature to pass, got %v", e)
	}

	// Tamper with the signature: must be rejected.
	bad := r.Clone(r.Context())
	bad.Header.Set("Authorization", strings.Replace(auth, sig, flipLast(sig), 1))
	if e := v.verify(bad); e == nil {
		t.Fatal("expected tampered signature to be rejected")
	}

	// Wrong access key: must be rejected.
	wrong := r.Clone(r.Context())
	wrong.Header.Set("Authorization", strings.Replace(auth, "Credential=AKID/", "Credential=NOPE/", 1))
	if e := v.verify(wrong); e == nil {
		t.Fatal("expected unknown access key to be rejected")
	}
}

func flipLast(s string) string {
	if s == "" {
		return s
	}
	last := s[len(s)-1]
	repl := byte('0')
	if last == '0' {
		repl = '1'
	}
	return s[:len(s)-1] + string(repl)
}

func TestChunkedReader(t *testing.T) {
	// Two data chunks ("Hello " + "World") followed by the terminating chunk.
	body := "6;chunk-signature=aaaa\r\nHello \r\n" +
		"5;chunk-signature=bbbb\r\nWorld\r\n" +
		"0;chunk-signature=cccc\r\n\r\n"
	got, err := io.ReadAll(newChunkedReader(strings.NewReader(body)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "Hello World" {
		t.Fatalf("chunked decode mismatch: got %q", string(got))
	}
}

func TestAWSURIEncode(t *testing.T) {
	cases := map[string]string{
		"hello.txt": "hello.txt",
		"a b":       "a%20b",
		"foo/bar":   "foo/bar", // slash preserved (encodeSlash=false)
		"~tilde-_.": "~tilde-_.",
		"café":      "caf%C3%A9",
		"100%":      "100%25",
	}
	for in, want := range cases {
		if got := awsURIEncode(in, false); got != want {
			t.Errorf("awsURIEncode(%q)=%q want %q", in, got, want)
		}
	}
	if got := awsURIEncode("foo/bar", true); got != "foo%2Fbar" {
		t.Errorf("encodeSlash=true: got %q", got)
	}
}
