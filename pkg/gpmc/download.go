package gpmc

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	pb "gpix/pkg/gpmc/gpmcpb"
)

func (c *Client) GetDownloadURL(ctx context.Context, mediaKey string) (original, edited string, err error) {
	body, err := proto.Marshal(pb.PrepareDownloadRequest_builder{
		Key: pb.PrepareDownloadRequest_KeyOuter_builder{
			Inner: pb.PrepareDownloadRequest_KeyInner_builder{MediaKey: mediaKey}.Build(),
		}.Build(),
		Opaque: buildOpaqueDownloadSignal(),
	}.Build())
	if err != nil {
		return "", "", err
	}

	respBytes, err := c.doProto(ctx, "prepare-download", endpointPrepareDownload, body, true, c.language)
	if err != nil {
		return "", "", err
	}
	resp := &pb.PrepareDownloadResponse{}
	if err := proto.Unmarshal(respBytes, resp); err != nil {
		return "", "", fmt.Errorf("gpmc prepare-download: decode: %w", err)
	}
	l5 := resp.GetL1().GetL5()
	if img := l5.GetImage(); img.GetOriginalUrl() != "" || img.GetEditedUrl() != "" {
		return img.GetOriginalUrl(), img.GetEditedUrl(), nil
	}
	if vid := l5.GetVideo().GetDownloadUrl(); vid != "" {
		return vid, "", nil
	}
	return "", "", nil
}

func buildOpaqueDownloadSignal() *pb.PrepareDownloadRequest_Opaque {
	return pb.PrepareDownloadRequest_Opaque_builder{
		F1: pb.PrepareDownloadRequest_OpaqueF1_builder{
			F7: pb.PrepareDownloadRequest_OpaqueF1Inner_builder{
				F2: pb.PrepareDownloadRequest_Empty_builder{}.Build(),
			}.Build(),
		}.Build(),
		F5: pb.PrepareDownloadRequest_OpaqueF5_builder{
			F2: pb.PrepareDownloadRequest_Empty_builder{}.Build(),
			F3: pb.PrepareDownloadRequest_Empty_builder{}.Build(),
			F5: pb.PrepareDownloadRequest_OpaqueF5Inner5_builder{
				F1: pb.PrepareDownloadRequest_Empty_builder{}.Build(),
			}.Build(),
		}.Build(),
	}.Build()
}
