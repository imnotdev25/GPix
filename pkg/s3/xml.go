package s3

import (
	"encoding/xml"
	"time"
)

const s3NS = "http://s3.amazonaws.com/doc/2006-03-01/"

// iso8601 is the timestamp format S3 uses in listings.
func iso8601(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

type owner struct {
	ID          string `xml:"ID"`
	DisplayName string `xml:"DisplayName"`
}

type bucketEntry struct {
	Name         string `xml:"Name"`
	CreationDate string `xml:"CreationDate"`
}

type listAllMyBucketsResult struct {
	XMLName xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListAllMyBucketsResult"`
	Owner   owner    `xml:"Owner"`
	Buckets struct {
		Bucket []bucketEntry `xml:"Bucket"`
	} `xml:"Buckets"`
}

type objectEntry struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

type commonPrefix struct {
	Prefix string `xml:"Prefix"`
}

// listBucketResult covers both ListObjects (v1) and ListObjectsV2. Fields that
// do not apply to a given version are left empty / omitted.
type listBucketResult struct {
	XMLName xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ ListBucketResult"`

	Name        string `xml:"Name"`
	Prefix      string `xml:"Prefix"`
	Delimiter   string `xml:"Delimiter,omitempty"`
	MaxKeys     int    `xml:"MaxKeys"`
	IsTruncated bool   `xml:"IsTruncated"`

	// v1
	Marker     string `xml:"Marker,omitempty"`
	NextMarker string `xml:"NextMarker,omitempty"`

	// v2
	KeyCount              int    `xml:"KeyCount,omitempty"`
	ContinuationToken     string `xml:"ContinuationToken,omitempty"`
	NextContinuationToken string `xml:"NextContinuationToken,omitempty"`
	StartAfter            string `xml:"StartAfter,omitempty"`

	Contents       []objectEntry  `xml:"Contents"`
	CommonPrefixes []commonPrefix `xml:"CommonPrefixes"`
}

// --- batch delete (POST /{bucket}?delete) ---

type deleteRequest struct {
	XMLName xml.Name        `xml:"Delete"`
	Quiet   bool            `xml:"Quiet"`
	Objects []deleteableKey `xml:"Object"`
}

type deleteableKey struct {
	Key string `xml:"Key"`
}

type deletedEntry struct {
	Key string `xml:"Key"`
}

type deleteErrorEntry struct {
	Key     string `xml:"Key"`
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

type deleteResult struct {
	XMLName xml.Name           `xml:"http://s3.amazonaws.com/doc/2006-03-01/ DeleteResult"`
	Deleted []deletedEntry     `xml:"Deleted"`
	Errors  []deleteErrorEntry `xml:"Error"`
}
