package gpmc

import (
	"encoding/base64"
	"errors"
	"slices"

	"google.golang.org/protobuf/encoding/protowire"
)

const libStateRequestBlobB64 = "CogJCqsCClkKABoAIgAqDAoAEgAaACIAKgA6ADIAOgISAHoAggEAigEAmgEAogEAqgEGKgIaADIAygEA8gECEgD6AQCCAgCKAgIKAJICAKICAKoCALICALoCAMICAMoCACpSEhgSChoCEgAiBBIAIgAiBBICEAEqAhIAMAEaGhIEGgAiABoIEgAaBBABGgAiACoEEgIQAToAIgQSAhIAKhQKEBIEGgAiABoIEgAaBBABGgAYAUIASjASABoECgASACImCiQaHAoaCggqAgoAMgA6ABIAGgwKCCoCCgAyADoAEgAiBAoCEgBaDBIAGgAiBhIECAEQAmIAcgwSABoAIgYSBAgBEAJ6BAoAIgCKAQQKACIAmgEMEgAaACIGEgQIARACqgECCgCyAQC6AQDCAQAShQEKKxIAGgAiACoAMgwKABIAGgAiACoAOgA6AEIAUgBiAGoEEgAaAHoCCgCSAQAiAgoASgBaDAoKCgAiACoAMgBKAHIkCiIKHgoAEggSBgoCCgAaABoQIgYKAgoAGgAqBgoCCgAaABIAigEAkgEGCgASAgoAogEGEgQKABIAsgEAugEAwgEAGsgBEgAaYhIAGgA6AEIAcgIKAIIBAIoBAhIAkgEAmgEAogEAqgEAsgEAugEA2gEGCgASAgoA6gEA8gEA+gEAggIAkgIAqgIAsgIAugIAygIA2gICCgDqAgQKAgoA8gIGCgASABoA+gIAIgwSABoCCgAiACoCCgA6AGIAagByHAoAEgwKABICCgAaACICCgAaCgoAEgIKABoAIgB6AIIBAgoAkgEAmgEUIgISADIEEgAaADoEEgAaAEIASgCiAQCyAQDCAQDKAQDSAQAyADgCSioKBhIECgASABIEGgIQARoCEgAiADoCCgBCCggCEgYBAgMFBgdKAFoCCgBYAVgCWAZiDBIECgASABoCCgAiAGoAegQaAggBkgE76qiliAU1CjMKMSACIAEgBiAIIAogDyASIA0gESATIA4gFCgGMAI4AUACWANgAWgDeAGAAQGIAQGQAQKaAS4KBAoAEgASDAgBCAIIBAgGCAUIBxoECgASACoECgASADICCgA6BAoAEgBCAgoAogGEAQgBEgAafgpHdHlwZS5nb29nbGVhcGlzLmNvbS9waG90b3MucHJpbnRpbmcuY2xpZW50LlByaW50aW5nUHJvbW90aW9uU3luY09wdGlvbnMSMwoxIAIgASAGIAggCiAPIBIgDSARIBMgDiAUKAYwAjgBQAJYA2ABaAN4AYABAYgBAZABAqoBlAESCBICIgAiACoAGggSAggBIgISACoCCgAyBgoAEgIKADorCAISJAEHCAkKDQ4PERMUFhctLi8wMToGGDI2Nzs+P0BBODk8R0JFRBoBAUIgGg4KCgoIEgIIASICEgAaACICCgAqCgoIEgIIASICEgBKAgoAUhIKAgoAGgAqADICCgA6AEoAUgBaAGIAagByAIIBAgoAsgEZCAESFTEwNDA0ODAyOTM1ODQ5ODE2MjM4OcoBCgoGCgQKAgoAEgDSAQASDAoICgYKAgoAEgASAA=="

var libStateRequestBlob []byte

func init() {
	b, err := base64.StdEncoding.DecodeString(libStateRequestBlobB64)
	if err != nil {
		panic("gpmc: decoding libstate blob: " + err.Error())
	}
	libStateRequestBlob = b
}

var errBadBlob = errors.New("gpmc: malformed library request")

// buildLibRequest returns the library request with pageToken (the previous
// reply's field 1.1, which continues the listing) in field 1.4 and
// stateToken (field 1.6, which asks only for changes) in field 1.6. With
// both empty it is the first-page request, unchanged.
func buildLibRequest(stateToken, pageToken string) ([]byte, error) {
	top := libStateRequestBlob
	var out []byte
	for len(top) > 0 {
		num, wt, n := protowire.ConsumeTag(top)
		if n < 0 {
			return nil, errBadBlob
		}
		vn := protowire.ConsumeFieldValue(num, wt, top[n:])
		if vn < 0 {
			return nil, errBadBlob
		}
		if num == 1 && wt == protowire.BytesType {
			inner, _ := protowire.ConsumeBytes(top[n:])
			patched, err := setStringFields(inner, map[protowire.Number]string{4: pageToken, 6: stateToken})
			if err != nil {
				return nil, err
			}
			out = protowire.AppendTag(out, 1, protowire.BytesType)
			out = protowire.AppendBytes(out, patched)
		} else {
			out = append(out, top[:n+vn]...)
		}
		top = top[n+vn:]
	}
	return out, nil
}

// setStringFields replaces (or adds, when non-empty) string fields of msg.
func setStringFields(msg []byte, set map[protowire.Number]string) ([]byte, error) {
	var out []byte
	done := map[protowire.Number]bool{}
	for len(msg) > 0 {
		num, wt, n := protowire.ConsumeTag(msg)
		if n < 0 {
			return nil, errBadBlob
		}
		vn := protowire.ConsumeFieldValue(num, wt, msg[n:])
		if vn < 0 {
			return nil, errBadBlob
		}
		if v, ok := set[num]; ok && wt == protowire.BytesType {
			if !done[num] {
				out = protowire.AppendTag(out, num, protowire.BytesType)
				out = protowire.AppendString(out, v)
				done[num] = true
			}
		} else {
			out = append(out, msg[:n+vn]...)
		}
		msg = msg[n+vn:]
	}
	var add []protowire.Number
	for num, v := range set {
		if !done[num] && v != "" {
			add = append(add, num)
		}
	}
	slices.Sort(add)
	for _, num := range add {
		out = protowire.AppendTag(out, num, protowire.BytesType)
		out = protowire.AppendString(out, set[num])
	}
	return out, nil
}

// libReplyTokens reads the continuation token (field 1.1) and the sync token
// (field 1.6) from a library reply.
func libReplyTokens(resp []byte) (pageToken, stateToken string) {
	body := protoField(resp, 1)
	return string(protoField(body, 1)), string(protoField(body, 6))
}

// protoField returns the last value of bytes field num in msg.
func protoField(msg []byte, num protowire.Number) []byte {
	var v []byte
	for len(msg) > 0 {
		n2, wt, n := protowire.ConsumeTag(msg)
		if n < 0 {
			return v
		}
		vn := protowire.ConsumeFieldValue(n2, wt, msg[n:])
		if vn < 0 {
			return v
		}
		if n2 == num && wt == protowire.BytesType {
			v, _ = protowire.ConsumeBytes(msg[n:])
		}
		msg = msg[n+vn:]
	}
	return v
}
