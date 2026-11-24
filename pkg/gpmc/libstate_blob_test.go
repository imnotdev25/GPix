package gpmc

import (
	"bytes"
	"testing"
)

func TestBuildLibRequest(t *testing.T) {
	first, err := buildLibRequest("", "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, libStateRequestBlob) {
		t.Fatal("the first-page request must be the captured request, unchanged")
	}
	req, err := buildLibRequest("", "next-page-token")
	if err != nil {
		t.Fatal(err)
	}
	inner := protoField(req, 1)
	if got := string(protoField(inner, 4)); got != "next-page-token" {
		t.Fatalf("page token in field 1.4 = %q", got)
	}
	if got := protoField(inner, 6); len(got) != 0 {
		t.Fatalf("sync token should stay empty, got %q", got)
	}
	// Everything else in the request is carried over.
	if len(protoField(inner, 1)) == 0 || len(protoField(req, 2)) == 0 {
		t.Fatal("other request fields were dropped")
	}
}

func TestLibReplyTokens(t *testing.T) {
	// reply { body(1) { page_token(1) "P", sync_token(6) "S" } }
	body := []byte{0x0a, 1, 'P', 0x32, 1, 'S'}
	reply := append([]byte{0x0a, byte(len(body))}, body...)
	page, state := libReplyTokens(reply)
	if page != "P" || state != "S" {
		t.Fatalf("got page=%q state=%q", page, state)
	}
}
