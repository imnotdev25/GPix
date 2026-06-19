package dav

import "encoding/xml"

// WebDAV response XML structures (RFC 4918). All elements live in the "DAV:"
// namespace; encoding/xml emits it as the default namespace (xmlns="DAV:"),
// which interoperates with common clients.

type multistatusXML struct {
	XMLName   xml.Name      `xml:"DAV: multistatus"`
	Responses []responseXML `xml:"DAV: response"`
}

type responseXML struct {
	Href     string        `xml:"DAV: href"`
	Propstat []propstatXML `xml:"DAV: propstat"`
}

type propstatXML struct {
	Prop   propXML `xml:"DAV: prop"`
	Status string  `xml:"DAV: status"`
}

type propXML struct {
	DisplayName   string           `xml:"DAV: displayname,omitempty"`
	ResourceType  *resourceTypeXML `xml:"DAV: resourcetype"`
	ContentLength int64            `xml:"DAV: getcontentlength,omitempty"`
	ContentType   string           `xml:"DAV: getcontenttype,omitempty"`
	LastModified  string           `xml:"DAV: getlastmodified,omitempty"`
	CreationDate  string           `xml:"DAV: creationdate,omitempty"`
	ETag          string           `xml:"DAV: getetag,omitempty"`
}

type resourceTypeXML struct {
	Collection *struct{} `xml:"DAV: collection"`
}
