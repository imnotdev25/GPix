package gpmc

import (
	"encoding/base64"
	"errors"
	"fmt"
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

func buildLibStateRequest(syncToken string) ([]byte, error) {
	if syncToken == "" {
		out := make([]byte, len(libStateRequestBlob))
		copy(out, libStateRequestBlob)
		return out, nil
	}

	pos, err := findSyncTokenSlot(libStateRequestBlob)
	if err != nil {
		return nil, err
	}

	tokenBytes := []byte(syncToken)
	tokenLen := encodeVarint(uint64(len(tokenBytes)))

	out := make([]byte, 0, len(libStateRequestBlob)+len(tokenBytes)+8)
	out = append(out, libStateRequestBlob[:pos+1]...)
	out = append(out, tokenLen...)
	out = append(out, tokenBytes...)
	out = append(out, libStateRequestBlob[pos+2:]...)

	tokenGrowth := len(tokenLen) - 1 + len(tokenBytes)
	if err := patchOuterLengths(out, pos, tokenGrowth); err != nil {
		return nil, err
	}
	return out, nil
}

func findSyncTokenSlot(blob []byte) (int, error) {
	if len(blob) < 6 {
		return 0, errors.New("gpmc: blob too short")
	}
	if blob[0] != 0x0a {
		return 0, errors.New("gpmc: blob does not start with field 1 tag")
	}
	outerLen, outerLenSize := decodeVarint(blob[1:])
	innerStart := 1 + outerLenSize
	innerEnd := innerStart + int(outerLen)
	if innerEnd > len(blob) {
		return 0, errors.New("gpmc: outer length out of bounds")
	}
	pos := innerStart
	for pos < innerEnd {
		if blob[pos] == 0x32 && pos+1 < innerEnd && blob[pos+1] == 0x00 {
			return pos, nil
		}
		tag := blob[pos]
		wireType := tag & 0x07
		pos++
		switch wireType {
		case 0:
			_, sz := decodeVarint(blob[pos:])
			pos += sz
		case 2:
			l, sz := decodeVarint(blob[pos:])
			pos += sz + int(l)
		default:
			return 0, fmt.Errorf("gpmc: unsupported wire type %d at %d", wireType, pos-1)
		}
	}
	return 0, errors.New("gpmc: sync_token slot not found in blob")
}

func patchOuterLengths(blob []byte, syncTokenPos, growth int) error {
	if blob[0] != 0x0a {
		return errors.New("gpmc: blob outer tag changed")
	}
	oldLen, oldLenSize := decodeVarint(blob[1:])
	newLen := int(oldLen) + growth
	newLenBytes := encodeVarint(uint64(newLen))
	if len(newLenBytes) != oldLenSize {
		return errors.New("gpmc: outer length size changed; not yet supported")
	}
	copy(blob[1:1+oldLenSize], newLenBytes)
	return nil
}

func decodeVarint(b []byte) (uint64, int) {
	var x uint64
	var s uint
	for i, c := range b {
		if i >= 10 {
			return 0, 0
		}
		if c < 0x80 {
			return x | uint64(c)<<s, i + 1
		}
		x |= uint64(c&0x7f) << s
		s += 7
	}
	return 0, 0
}

func encodeVarint(x uint64) []byte {
	var b [10]byte
	n := 0
	for x >= 0x80 {
		b[n] = byte(x) | 0x80
		x >>= 7
		n++
	}
	b[n] = byte(x)
	return b[:n+1]
}
