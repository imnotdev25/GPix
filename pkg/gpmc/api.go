package gpmc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"google.golang.org/protobuf/proto"

	pb "gpix/pkg/gpmc/gpmcpb"
)

const commitTrailerMagic = 46000000

var commitTrailerBytes = []byte{0x01, 0x03}

func (c *Client) findRemoteMediaByHash(ctx context.Context, sha1 []byte) (string, bool, error) {
	body, err := proto.Marshal(pb.FindRemoteMediaByHashRequest_builder{
		Inner: pb.FindRemoteMediaByHashRequest_Inner_builder{
			Hash:   pb.FindRemoteMediaByHashRequest_Inner_Hash_builder{Sha1: sha1}.Build(),
			Marker: pb.FindRemoteMediaByHashRequest_Inner_Empty_builder{}.Build(),
		}.Build(),
	}.Build())
	if err != nil {
		return "", false, err
	}

	respBytes, err := c.doProto(ctx, "find-by-hash", endpointFindByHash, body, false, "")
	if err != nil {
		return "", false, err
	}
	resp := &pb.FindRemoteMediaByHashResponse{}
	if err := proto.Unmarshal(respBytes, resp); err != nil {
		return "", false, fmt.Errorf("gpmc find-by-hash: decode: %w", err)
	}
	key := resp.GetL1().GetL2().GetL3().GetMediaKey()
	if key == "" {
		return "", false, nil
	}
	return key, true, nil
}

func (c *Client) getUploadToken(ctx context.Context, sha1B64 string, size int64) (string, error) {
	body, err := proto.Marshal(pb.GetUploadTokenRequest_builder{
		F1:       2,
		F2:       2,
		F3:       1,
		F4:       3,
		FileSize: size,
	}.Build())
	if err != nil {
		return "", err
	}

	var uploadID string
	op := func() error {
		tok, err := c.bearer(ctx)
		if err != nil {
			return backoff.Permanent(err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointUploadMedia, bytes.NewReader(body))
		if err != nil {
			return backoff.Permanent(err)
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("User-Agent", apiUA(c.profile, c.language))
		req.Header.Set("Content-Type", contentTypeProto)
		req.Header.Set("X-Goog-Hash", "sha1="+sha1B64)
		req.Header.Set("X-Upload-Content-Length", fmt.Sprintf("%d", size))
		req.ContentLength = int64(len(body))

		resp, err := c.httpc.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusUnauthorized {
			c.invalidateToken()
			return &APIError{Op: "get-upload-token", Status: resp.StatusCode, Body: string(respBody)}
		}
		if resp.StatusCode >= 500 {
			return &APIError{Op: "get-upload-token", Status: resp.StatusCode, Body: string(respBody)}
		}
		if resp.StatusCode != http.StatusOK {
			return backoff.Permanent(&APIError{Op: "get-upload-token", Status: resp.StatusCode, Body: string(respBody)})
		}
		uploadID = resp.Header.Get("X-GUploader-UploadID")
		if uploadID == "" {
			return backoff.Permanent(errors.New("gpmc: missing X-GUploader-UploadID"))
		}
		return nil
	}

	if err := backoff.Retry(op, backOff(ctx)); err != nil {
		return "", err
	}
	return uploadID, nil
}

func (c *Client) putFile(ctx context.Context, uploadID string, openBody func() (io.ReadCloser, error), size int64) (*pb.UploadReceipt, error) {
	url := endpointUploadMedia + "?upload_id=" + uploadID
	var receipt *pb.UploadReceipt

	op := func() error {
		tok, err := c.bearer(ctx)
		if err != nil {
			return backoff.Permanent(err)
		}
		rc, err := openBody()
		if err != nil {
			return backoff.Permanent(err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, rc)
		if err != nil {
			rc.Close()
			return backoff.Permanent(err)
		}
		req.ContentLength = size
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("User-Agent", apiUA(c.profile, c.language))

		resp, err := c.httpc.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusUnauthorized {
			c.invalidateToken()
			return &APIError{Op: "put-bytes", Status: resp.StatusCode, Body: string(respBody)}
		}
		if resp.StatusCode >= 500 {
			return &APIError{Op: "put-bytes", Status: resp.StatusCode, Body: string(respBody)}
		}
		if resp.StatusCode != http.StatusOK {
			return backoff.Permanent(&APIError{Op: "put-bytes", Status: resp.StatusCode, Body: string(respBody)})
		}
		r := &pb.UploadReceipt{}
		if err := proto.Unmarshal(respBody, r); err != nil {
			return backoff.Permanent(fmt.Errorf("gpmc put: decode receipt: %w", err))
		}
		receipt = r
		return nil
	}

	if err := backoff.Retry(op, backOff(ctx)); err != nil {
		return nil, err
	}
	return receipt, nil
}

func (c *Client) commitUpload(ctx context.Context, receipt *pb.UploadReceipt, name string, sha1 []byte, q Quality, profile DeviceProfile, ts time.Time) (string, error) {
	commitModel := effectiveCommitModel(q, profile)
	body, err := proto.Marshal(pb.CommitUploadRequest_builder{
		Inner: pb.CommitUploadRequest_Inner_builder{
			Receipt:  receipt,
			Filename: name,
			Sha1:     sha1,
			Ts: pb.CommitUploadRequest_Inner_Timestamp_builder{
				T:     ts.Unix(),
				Magic: commitTrailerMagic,
			}.Build(),
			Quality: commitQualityCode(q),
			Marker:  1,
		}.Build(),
		Device: pb.CommitUploadRequest_Device_builder{
			Model:    commitModel,
			Make:     profile.Make,
			ApiLevel: int32(profile.AndroidAPILevel),
		}.Build(),
		Trailer: commitTrailerBytes,
	}.Build())
	if err != nil {
		return "", err
	}

	respBytes, err := c.doProto(ctx, "commit", endpointCommitUpload, body, true, "")
	if err != nil {
		return "", err
	}
	resp := &pb.CommitUploadResponse{}
	if err := proto.Unmarshal(respBytes, resp); err != nil {
		return "", fmt.Errorf("gpmc commit: decode: %w", err)
	}
	key := resp.GetL1().GetL3().GetMediaKey()
	if key == "" {
		return "", ErrUploadRejected
	}
	return key, nil
}

func (c *Client) doProto(ctx context.Context, op, endpoint string, body []byte, withExtHeaders bool, acceptLang string) ([]byte, error) {
	var out []byte
	fn := func() error {
		tok, err := c.bearer(ctx)
		if err != nil {
			return backoff.Permanent(err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return backoff.Permanent(err)
		}
		req.ContentLength = int64(len(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("User-Agent", apiUA(c.profile, c.language))
		req.Header.Set("Content-Type", contentTypeProto)
		if acceptLang != "" {
			req.Header.Set("Accept-Language", acceptLang)
		}
		if withExtHeaders {
			req.Header.Set("x-goog-ext-173412678-bin", extHeader173412678)
			req.Header.Set("x-goog-ext-174067345-bin", extHeader174067345)
		}

		resp, err := c.httpc.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusUnauthorized {
			c.invalidateToken()
			return &APIError{Op: op, Status: resp.StatusCode, Body: string(respBody)}
		}
		if resp.StatusCode >= 500 {
			return &APIError{Op: op, Status: resp.StatusCode, Body: string(respBody)}
		}
		if resp.StatusCode != http.StatusOK {
			return backoff.Permanent(&APIError{Op: op, Status: resp.StatusCode, Body: string(respBody)})
		}
		out = respBody
		return nil
	}
	if err := backoff.Retry(fn, backOff(ctx)); err != nil {
		return nil, err
	}
	return out, nil
}

func backOff(ctx context.Context) backoff.BackOffContext {
	bo := backoff.NewExponentialBackOff()
	bo.InitialInterval = 500 * time.Millisecond
	bo.MaxInterval = 8 * time.Second
	bo.MaxElapsedTime = 90 * time.Second
	return backoff.WithContext(backoff.WithMaxRetries(bo, 6), ctx)
}
