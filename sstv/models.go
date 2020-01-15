package sstv

import (
	"encoding/xml"
	"time"
)

// Authentication json response
type AuthResponse struct {
	Hash  string
	Valid int64
	Code  string
	Error string
}

// XML EPG structs
type TextLang struct {
	Text string `xml:",chardata"`
	Lang string `xml:"lang,attr"`
}

type Channel struct {
	Text        string   `xml:",chardata"`
	ID          string   `xml:"id,attr"`
	DisplayName TextLang `xml:"display-name"`
	URL         string   `xml:"url,omitempty"`
}

type Programme struct {
	Text     string    `xml:",chardata"`
	Start    string    `xml:"start,attr"`
	Stop     string    `xml:"stop,attr"`
	Channel  string    `xml:"channel,attr"`
	Title    TextLang  `xml:"title"`
	SubTitle *TextLang `xml:"sub-title,omitempty"`
	Desc     *TextLang `xml:"desc,omitempty"`
}

type EPG struct {
	XMLName           xml.Name    `xml:"tv"`
	Text              string      `xml:",chardata"`
	GeneratorInfoName string      `xml:"generator-info-name,attr"`
	GeneratorInfoURL  string      `xml:"generator-info-url,attr"`
	Channel           []Channel   `xml:"channel"`
	Programme         []Programme `xml:"programme"`
}

// JSON event in sstv epg
type JSONEvent struct {
	Name        string
	Description string
	Time        string
	Runtime     string
	Category    string
}

// Intermediary types for conversion from sstv epg to epg
type SSEpgEvent struct {
	Name        string
	Description string
	Category    string
	Start       time.Time
	Stop        time.Time
}

type SSEpgChannel struct {
	Number string
	Name   string
	Img    string
	Events []SSEpgEvent
}

type SSEpg struct {
	Channels []SSEpgChannel
}
