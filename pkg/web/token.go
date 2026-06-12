package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func deriveKey(master []byte, purpose string) []byte {
	m := hmac.New(sha256.New, master)
	m.Write([]byte(purpose))
	return m.Sum(nil)
}

var (
	errBadToken = errors.New("bad token")
	errExpired  = errors.New("token expired")
)

func (s *Server) signMedia(mediaKey string, ttl time.Duration) string {
	exp := time.Now().Add(ttl).Unix()
	msg := mediaKey + "|" + strconv.FormatInt(exp, 10)
	m := hmac.New(sha256.New, s.mediaSignKey)
	m.Write([]byte(msg))
	mac := m.Sum(nil)[:16]
	return fmt.Sprintf("%d.%s.%s",
		exp,
		base64.RawURLEncoding.EncodeToString([]byte(mediaKey)),
		base64.RawURLEncoding.EncodeToString(mac))
}

func (s *Server) verifyMedia(tok string) (string, error) {
	parts := strings.SplitN(tok, ".", 3)
	if len(parts) != 3 {
		return "", errBadToken
	}
	exp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return "", errBadToken
	}
	if time.Now().Unix() > exp {
		return "", errExpired
	}
	keyBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errBadToken
	}
	gotMac, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", errBadToken
	}
	mediaKey := string(keyBytes)
	m := hmac.New(sha256.New, s.mediaSignKey)
	m.Write([]byte(mediaKey + "|" + parts[0]))
	if !hmac.Equal(m.Sum(nil)[:16], gotMac) {
		return "", errBadToken
	}
	return mediaKey, nil
}

func (s *Server) signSession(username string, ttl time.Duration) string {
	now := time.Now().Unix()
	exp := time.Now().Add(ttl).Unix()
	msg := username + "|" + strconv.FormatInt(now, 10) + "|" + strconv.FormatInt(exp, 10)
	m := hmac.New(sha256.New, s.sessionSignKey)
	m.Write([]byte(msg))
	mac := m.Sum(nil)[:16]
	return fmt.Sprintf("%s.%s",
		base64.RawURLEncoding.EncodeToString([]byte(msg)),
		base64.RawURLEncoding.EncodeToString(mac))
}

func (s *Server) verifySession(tok string) (username string, err error) {
	parts := strings.SplitN(tok, ".", 2)
	if len(parts) != 2 {
		return "", errBadToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", errBadToken
	}
	gotMac, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errBadToken
	}
	m := hmac.New(sha256.New, s.sessionSignKey)
	m.Write(payload)
	if !hmac.Equal(m.Sum(nil)[:16], gotMac) {
		return "", errBadToken
	}
	fields := strings.Split(string(payload), "|")
	if len(fields) != 3 {
		return "", errBadToken
	}
	exp, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return "", errBadToken
	}
	if time.Now().Unix() > exp {
		return "", errExpired
	}
	return fields[0], nil
}
