package gpmc

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
)

const endpointTrashOps = "https://photosdata-pa.googleapis.com/6439526531001121323/17490284929287180316"

// DedupKeyFromSHA1 reproduces Python gpmc utils.urlsafe_base64(b64(sha1)):
// standard base64 of the SHA-1, then '+'→'-', '/'→'_', strip '=' padding.
func DedupKeyFromSHA1(sha1 []byte) string {
	std := base64.StdEncoding.EncodeToString(sha1)
	return base64ToURLSafe(std)
}

func base64ToURLSafe(b64 string) string {
	out := make([]byte, 0, len(b64))
	for i := 0; i < len(b64); i++ {
		c := b64[i]
		switch c {
		case '+':
			out = append(out, '-')
		case '/':
			out = append(out, '_')
		case '=':
			// skip padding
		default:
			out = append(out, c)
		}
	}
	return string(out)
}

// DeleteByMediaKeys looks up each media_key in the library, derives the dedup_key
// from its SHA-1, then trashes (default) or permanently deletes them. Items
// without a SHA-1 in the listing are skipped with an error in the returned map.
func (c *Client) DeleteByMediaKeys(ctx context.Context, mediaKeys []string, permanent bool) (map[string]error, error) {
	wanted := make(map[string]bool, len(mediaKeys))
	for _, k := range mediaKeys {
		wanted[k] = true
	}
	found := make(map[string]string)
	results := make(map[string]error, len(mediaKeys))
	cursor := ""
	for {
		page, err := c.ListPage(ctx, cursor)
		if err != nil {
			return nil, err
		}
		for _, it := range page.Items {
			if wanted[it.MediaKey] {
				if len(it.SHA1) == 0 {
					results[it.MediaKey] = errors.New("item has no sha1 in listing")
					continue
				}
				found[it.MediaKey] = it.DedupKey()
			}
		}
		if len(found)+countErrors(results) >= len(mediaKeys) || page.NextToken == "" {
			break
		}
		cursor = page.NextToken
	}
	for k := range wanted {
		if _, ok := found[k]; !ok {
			if _, has := results[k]; !has {
				results[k] = errors.New("media_key not found in library listing")
			}
		}
	}
	if len(found) == 0 {
		return results, nil
	}
	dedupKeys := make([]string, 0, len(found))
	for _, dk := range found {
		dedupKeys = append(dedupKeys, dk)
	}
	var opErr error
	if permanent {
		opErr = c.DeleteItemsPermanently(ctx, dedupKeys)
	} else {
		opErr = c.TrashItems(ctx, dedupKeys)
	}
	for k := range found {
		results[k] = opErr
	}
	return results, nil
}

func countErrors(m map[string]error) int {
	n := 0
	for range m {
		n++
	}
	return n
}

// TrashItems sends the given dedup keys to the move-to-trash endpoint.
// Items can be restored from the Google Photos trash for ~60 days afterward.
func (c *Client) TrashItems(ctx context.Context, dedupKeys []string) error {
	if len(dedupKeys) == 0 {
		return errors.New("gpmc trash: no dedup keys")
	}
	body, err := buildTrashRequest(dedupKeys, c.profile, false)
	if err != nil {
		return err
	}
	_, err = c.doProto(ctx, "move-to-trash", endpointTrashOps, body, false, c.language)
	return err
}

// DeleteItemsPermanently sends the given dedup keys to the same endpoint with
// the "permanently delete" operation code. There is no undo.
func (c *Client) DeleteItemsPermanently(ctx context.Context, dedupKeys []string) error {
	if len(dedupKeys) == 0 {
		return errors.New("gpmc delete: no dedup keys")
	}
	body, err := buildTrashRequest(dedupKeys, c.profile, true)
	if err != nil {
		return err
	}
	_, err = c.doProto(ctx, "delete-permanently", endpointTrashOps, body, false, c.language)
	return err
}

// buildTrashRequest hand-encodes the MoveToTrash / DeletePermanently protobuf body.
// The wire shape (per Python gpmc api.py:330-336 and 371-377) is:
//
//	field 2: int (op code: 1=trash, 2=delete-permanently)
//	field 3: string (repeated dedup keys)
//	field 4: int (1=trash, 2=delete)
//	field 8: nested mask (constant — we encode the byte-for-byte equivalent)
//	field 9: only present for move-to-trash (client version/api block)
func buildTrashRequest(dedupKeys []string, profile DeviceProfile, permanent bool) ([]byte, error) {
	var out []byte
	op := int32(1)
	if permanent {
		op = 2
	}
	out = appendVarintField(out, 2, uint64(op))
	for _, k := range dedupKeys {
		out = appendBytesField(out, 3, []byte(k))
	}
	out = appendVarintField(out, 4, uint64(op))

	// field 8 is a fixed sub-mask telling the server what response fields to return.
	// Python sends {"4": {"2": {}, "3": {"1": {}}, "4": {}, "5": {"1": {}}}}.
	// That serializes to a known byte sequence; we encode it inline.
	mask := buildTrashMask()
	out = appendBytesField(out, 8, mask)

	if !permanent {
		// field 9 = {"1": 5, "2": {"1": clientVersion, "2": str(apiLevel)}}
		var f9 []byte
		f9 = appendVarintField(f9, 1, 5)
		var f9_2 []byte
		f9_2 = appendVarintField(f9_2, 1, uint64(profile.ClientVersionCode))
		apiStr := strconv.Itoa(profile.AndroidAPILevel)
		f9_2 = appendBytesField(f9_2, 2, []byte(apiStr))
		f9 = appendBytesField(f9, 2, f9_2)
		out = appendBytesField(out, 9, f9)
	}
	return out, nil
}

func buildTrashMask() []byte {
	// inner: {"2": {}, "3": {"1": {}}, "4": {}, "5": {"1": {}}}
	var inner []byte
	inner = appendBytesField(inner, 2, []byte{})
	var f3 []byte
	f3 = appendBytesField(f3, 1, []byte{})
	inner = appendBytesField(inner, 3, f3)
	inner = appendBytesField(inner, 4, []byte{})
	var f5 []byte
	f5 = appendBytesField(f5, 1, []byte{})
	inner = appendBytesField(inner, 5, f5)
	// outer wraps: {"4": inner}
	var outer []byte
	outer = appendBytesField(outer, 4, inner)
	return outer
}

func appendVarintField(out []byte, fieldNum int, v uint64) []byte {
	tag := uint64(fieldNum) << 3
	out = appendVarintRaw(out, tag)
	return appendVarintRaw(out, v)
}

func appendBytesField(out []byte, fieldNum int, b []byte) []byte {
	tag := uint64(fieldNum)<<3 | 2 // wire type 2
	out = appendVarintRaw(out, tag)
	out = appendVarintRaw(out, uint64(len(b)))
	out = append(out, b...)
	return out
}

func appendVarintRaw(out []byte, v uint64) []byte {
	for v >= 0x80 {
		out = append(out, byte(v)|0x80)
		v >>= 7
	}
	out = append(out, byte(v))
	return out
}
