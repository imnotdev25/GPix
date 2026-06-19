package s3

import (
	"encoding/xml"
	"net/http"
)

// apiError is an S3-style error rendered as an <Error> XML document.
type apiError struct {
	Code    string // S3 error code, e.g. "NoSuchKey"
	Message string
	Status  int // HTTP status code
}

func (e apiError) Error() string { return e.Code + ": " + e.Message }

// Pre-defined errors mirroring the AWS S3 error catalogue (subset).
var (
	errNoSuchBucket = func(b string) apiError {
		return apiError{Code: "NoSuchBucket", Message: "The specified bucket does not exist: " + b, Status: http.StatusNotFound}
	}
	errNoSuchKey          = apiError{Code: "NoSuchKey", Message: "The specified key does not exist.", Status: http.StatusNotFound}
	errAccessDenied       = apiError{Code: "AccessDenied", Message: "Access Denied.", Status: http.StatusForbidden}
	errSignatureMismatch  = apiError{Code: "SignatureDoesNotMatch", Message: "The request signature we calculated does not match the signature you provided.", Status: http.StatusForbidden}
	errInvalidAccessKeyID = apiError{Code: "InvalidAccessKeyId", Message: "The access key ID you provided does not exist in our records.", Status: http.StatusForbidden}
	errMissingAuth        = apiError{Code: "AccessDenied", Message: "Missing or unsupported authentication.", Status: http.StatusForbidden}
	errRequestExpired     = apiError{Code: "AccessDenied", Message: "Request has expired.", Status: http.StatusForbidden}
	errMethodNotAllowed   = apiError{Code: "MethodNotAllowed", Message: "The specified method is not allowed against this resource.", Status: http.StatusMethodNotAllowed}
	errNotImplemented     = apiError{Code: "NotImplemented", Message: "This functionality is not implemented.", Status: http.StatusNotImplemented}
	errInternal           = func(msg string) apiError {
		return apiError{Code: "InternalError", Message: msg, Status: http.StatusInternalServerError}
	}
)

// errorResponse is the XML body for an S3 error.
type errorResponse struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string   `xml:"Code"`
	Message   string   `xml:"Message"`
	Resource  string   `xml:"Resource,omitempty"`
	RequestID string   `xml:"RequestId,omitempty"`
}

func writeError(w http.ResponseWriter, r *http.Request, e apiError) {
	body := errorResponse{
		Code:      e.Code,
		Message:   e.Message,
		Resource:  r.URL.Path,
		RequestID: requestID(r),
	}
	out, _ := xml.Marshal(body)
	w.Header().Set("Content-Type", "application/xml")
	w.Header().Set("x-amz-request-id", body.RequestID)
	w.WriteHeader(e.Status)
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(out)
}
