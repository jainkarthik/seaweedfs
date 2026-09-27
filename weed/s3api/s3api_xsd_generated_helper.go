package s3api

import (
	"encoding/xml"
)

type LocationConstraint struct {
	XMLName xml.Name `xml:"http://s3.amazonaws.com/doc/2006-03-01/ LocationConstraint"`
	Value   string   `xml:",chardata"`
}

type Grantee struct {
	XMLNS  string `xml:"xmlns:xsi,attr"`
	XMLXSI string `xml:"xsi:type,attr"`
	// Type is not serialized: the S3 XML schema conveys the grantee type solely
	// via the xsi:type attribute (XMLXSI above), not as a child element. Keeping
	// the field (unexported from XML via xml:"-") lets Go code still read/set it.
	Type        string `xml:"-"`
	ID          string `xml:"ID,omitempty"`
	DisplayName string `xml:"DisplayName,omitempty"`
	URI         string `xml:"URI,omitempty"`
}
