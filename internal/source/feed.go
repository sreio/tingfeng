package source

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"golang.org/x/net/html/charset"
	"io"
	"net/url"
	"strings"
	"tingfeng/internal/domain"
)

type rssEntry struct {
	GUID   string `xml:"guid"`
	Title  string `xml:"title"`
	Link   string `xml:"link"`
	Date   string `xml:"pubDate"`
	DCDate string `xml:"date"`
}
type rssDocument struct {
	XMLName xml.Name
	Channel struct {
		Items []rssEntry `xml:"item"`
	} `xml:"channel"`
	Items []rssEntry `xml:"item"`
}
type atomEntry struct {
	ID    string `xml:"id"`
	Title string `xml:"title"`
	Links []struct {
		Rel  string `xml:"rel,attr"`
		Href string `xml:"href,attr"`
	} `xml:"link"`
	Updated   string `xml:"updated"`
	Published string `xml:"published"`
}
type atomDocument struct {
	Entries []atomEntry `xml:"entry"`
}

func parseFeed(data []byte) ([]domain.Item, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	d.CharsetReader = charset.NewReaderLabel
	var root xml.StartElement
	for {
		t, e := d.Token()
		if e != nil {
			if e == io.EOF {
				return nil, fmt.Errorf("订阅为空")
			}
			return nil, fmt.Errorf("RSS / Atom XML 无效")
		}
		if s, ok := t.(xml.StartElement); ok {
			root = s
			break
		}
	}
	out := []domain.Item{}
	if root.Name.Local == "feed" {
		var feed atomDocument
		if e := d.DecodeElement(&feed, &root); e != nil {
			return nil, fmt.Errorf("Atom 解析失败")
		}
		for _, x := range feed.Entries {
			link := ""
			for _, l := range x.Links {
				if l.Rel == "" || l.Rel == "alternate" {
					link = l.Href
					break
				}
			}
			date := x.Published
			if date == "" {
				date = x.Updated
			}
			out = append(out, domain.Item{ID: itemID(x.ID, link, x.Title), Title: strings.TrimSpace(x.Title), Link: safeLink(link), PublishedAt: parseTime(date)})
		}
	} else if root.Name.Local == "rss" || root.Name.Local == "RDF" {
		var rss rssDocument
		if e := d.DecodeElement(&rss, &root); e != nil {
			return nil, fmt.Errorf("RSS 解析失败")
		}
		entries := rss.Channel.Items
		if root.Name.Local == "RDF" {
			entries = rss.Items
		}
		for _, x := range entries {
			date := x.Date
			if date == "" {
				date = x.DCDate
			}
			out = append(out, domain.Item{ID: itemID(x.GUID, x.Link, x.Title), Title: strings.TrimSpace(x.Title), Link: safeLink(x.Link), PublishedAt: parseTime(date)})
		}
	} else {
		return nil, fmt.Errorf("不是 RSS 或 Atom 订阅")
	}
	if len(out) > 500 {
		out = out[:500]
	}
	seen := map[string]bool{}
	unique := []domain.Item{}
	for _, item := range out {
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		if len(item.Title) > 1000 {
			item.Title = string([]rune(item.Title)[:min(250, len([]rune(item.Title)))])
		}
		unique = append(unique, item)
	}
	return unique, nil
}
func safeLink(raw string) string {
	u, e := url.Parse(raw)
	if e != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	return u.String()
}
