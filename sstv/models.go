package sstv

import (
	"encoding/xml"
	"time"
)

// AuthResponse Authentication json response
type AuthResponse struct {
	Hash  string
	Valid int64
	Code  string
	Error string
}

// TextLang XML with optional language
type TextLang struct {
	Text string `xml:",chardata"`
	Lang string `xml:"lang,attr,omitempty"`
}

// Channel XML channel
type Channel struct {
	Text        string   `xml:",chardata"`
	ID          string   `xml:"id,attr"`
	DisplayName TextLang `xml:"display-name"`
	URL         string   `xml:"url,omitempty"`
}

// Programme XML programme
type Programme struct {
	Text     string    `xml:",chardata"`
	Start    string    `xml:"start,attr"`
	Stop     string    `xml:"stop,attr"`
	Channel  string    `xml:"channel,attr"`
	Title    TextLang  `xml:"title"`
	SubTitle *TextLang `xml:"sub-title,omitempty"`
	Desc     *TextLang `xml:"desc,omitempty"`
}

// EPG Top-level xml epg
type EPG struct {
	XMLName           xml.Name    `xml:"tv"`
	Text              string      `xml:",chardata"`
	GeneratorInfoName string      `xml:"generator-info-name,attr"`
	GeneratorInfoURL  string      `xml:"generator-info-url,attr"`
	Channel           []Channel   `xml:"channel"`
	Programme         []Programme `xml:"programme"`
}

// JSONEvent JSON event in sstv epg
type JSONEvent struct {
	Name        string
	Description string
	Time        string
	Runtime     string
	Category    string
}

// SSEpgEvent An event for SSEpgChannel
type SSEpgEvent struct {
	Name        string
	Description string
	Category    string
	Start       time.Time
	Stop        time.Time
}

// SSEpgChannel A channel for SSEpg
type SSEpgChannel struct {
	Number string
	Name   string
	Img    string
	Events []SSEpgEvent
}

// SSEpg Intermediary type for conversion from sstv EPG to EPG
type SSEpg struct {
	Channels []SSEpgChannel
}

// RuvChannelResponse Channel response for geoblocked RUV
type RuvChannelResponse struct {
	Result []string
}
