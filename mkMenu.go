package main

import (
	"github.com/pebbe/util"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	mdhtml "github.com/yuin/goldmark/renderer/html"

	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"io/ioutil"
	"os"
	"strings"
)

type MenuT struct {
	XMLName xml.Name `xml:"menu"`
	Items   []*ItemT `xml:"item"`
}

type ItemT struct {
	ID    string   `xml:"id,attr"`
	Lbl   string   `xml:"lbl,attr"`
	Class string   `xml:"class,attr"`
	Items []*ItemT `xml:"item"`
	Text  string   `xml:",cdata"`
}

var (
	md goldmark.Markdown
	x  = util.CheckErr
)

func main() {
	md = goldmark.New(
		goldmark.WithExtensions(
			extension.Table,
			extension.Strikethrough,
			extension.Linkify,
			extension.DefinitionList,
			extension.NewTypographer(
				// alleen: -- ---
				extension.WithTypographicSubstitutions(extension.TypographicSubstitutions{
					extension.LeftSingleQuote:  nil,
					extension.RightSingleQuote: nil,
					extension.LeftDoubleQuote:  nil,
					extension.RightDoubleQuote: nil,
					extension.Ellipsis:         nil,
					extension.LeftAngleQuote:   nil,
					extension.RightAngleQuote:  nil,
					extension.Apostrophe:       nil,
				}),
			),
		),
		goldmark.WithRendererOptions(
			mdhtml.WithUnsafe(),
		),
	)

	options := make([]string, 0)
	fp, err := os.Open("corpora.txt")
	x(err)
	scanner := bufio.NewScanner(fp)
	inOptgroup := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ":") {
			if inOptgroup {
				options = append(options, "</optgroup>")
			}
			options = append(options, fmt.Sprintf(`<optgroup label="&mdash; %s &mdash;">`, html.EscapeString(line[1:])))
			inOptgroup = true
		} else {
			options = append(options, optFormat(line))
		}
	}
	fp.Close()
	if inOptgroup {
		options = append(options, "</optgroup>")
	}

	b, err := ioutil.ReadFile("menu.xml")
	x(err)

	var menu MenuT
	x(xml.Unmarshal(b, &menu))

	var buf1 bytes.Buffer
	var buf2 bytes.Buffer

	seen := make(map[string]bool)
	trail := make([]string, 100)
	var f func(*ItemT, int)
	f = func(item *ItemT, lvl int) {
		if seen[item.ID] {
			x(fmt.Errorf("Seen: %s", item.ID))
		}
		seen[item.ID] = true

		trail[lvl] = item.Lbl

		data := strings.TrimSpace(item.Text) != ""

		if data {
			if item.Class == "info" {
				fmt.Fprintf(&buf1, "qi['%s'] = %q;\n", item.ID, format(trail[:lvl+1], item.Text, true))
				fmt.Fprintf(&buf2, "<li><a href=\"javascript:iq('%s')\">Toelichting &nbsp; »</a></li>\n", item.ID)
			} else {
				fmt.Fprintf(&buf1, "qs['%s'] = %q;\n", item.ID, format(trail[:lvl+1], item.Text, false))
				fmt.Fprintf(&buf2, "<li><a href=\"javascript:q('%s')\">%s</a></li>\n", item.ID, item.Lbl)
			}
		} else {
			if lvl == 0 {
				fmt.Fprintln(&buf2, `<div class="item">`)
			} else {
				fmt.Fprintln(&buf2, `<li><div class="sub-item">`)
			}
			fmt.Fprintf(&buf2, "<input type=\"checkbox\" id=\"%s\"/>\n<img src=\"images/Arrow.png\" alt=\"arrow\" class=\"arrow\"><label for=\"%s\">%s</label>\n<ul>\n", item.ID, item.ID, html.EscapeString(item.Lbl))
		}

		for _, i := range item.Items {
			f(i, lvl+1)
		}

		if !data {
			fmt.Fprintln(&buf2, `</ul>`)
			if lvl == 0 {
				fmt.Fprintln(&buf2, `</div>`)
			} else {
				fmt.Fprintln(&buf2, `</div></li>`)
			}
		}

	}
	for _, item := range menu.Items {
		f(item, 0)
	}

	b, err = ioutil.ReadFile("template.html")
	x(err)
	fmt.Print(
		strings.Replace(
			strings.Replace(
				strings.Replace(
					strings.Replace(string(b), "PART1;", buf1.String(), 1),
					"<!--PART2-->", buf2.String(), 1),
				"<!--WARNING-->", `<!--

        WAARSCHUWING: dit is een gegenereerd bestand, bewerk het niet

-->`, 1),
			"<!--OPTIONS-->", strings.TrimSpace(strings.Join(options, "\n")), 1))

}

func format(trail []string, s string, info bool) string {
	// TODO
	aa := strings.Split(s, "\n")
	for len(aa) > 0 && strings.TrimSpace(aa[0]) == "" {
		aa = aa[1:]
	}
	// TODO: untabify
	maxlen := 10000
	for i, a := range aa {
		a = untabify(a)
		aa[i] = a
		a2 := strings.TrimLeft(a, " ")
		if a2 == "" {
			continue
		}
		n := len(a) - len(a2)
		if n < maxlen {
			maxlen = n
		}
	}
	for i, a := range aa {
		a2 := strings.TrimLeft(a, " ")
		if a2 == "" {
			aa[i] = ""
		} else {
			if len(a) > maxlen {
				aa[i] = a[maxlen:]
			}
		}
	}

	if info {
		var buf bytes.Buffer
		x(md.Convert([]byte(strings.Join(aa, "\n")), &buf))

		return fmt.Sprintf("<h2>%s</h2>\n%s\n", html.EscapeString(strings.Join(trail[:len(trail)-1], " | ")), buf.String())
	}

	return fmt.Sprintf("-- %s\n\n%s\n", strings.Join(trail, " | "), strings.Join(aa, "\n"))
}

func untabify(s string) string {
	var buf bytes.Buffer
	p := 0
	for _, c := range s {
		p++
		if c == '\t' {
			buf.WriteRune(' ')
			for p%8 != 0 {
				buf.WriteRune(' ')
				p++
			}
		} else {
			buf.WriteRune(c)
		}
		if c == '\n' {
			p = 0
		}
	}
	return buf.String()
}

func optFormat(s string) string {
	a := strings.Fields(s)

	lbl := a[0]

	/*

		s1 := a[1]
		s2 := ""
		for n := len(s1); n > 3; n = len(s1) {
			// U+202F = NARROW NO-BREAK SPACE
			//s2 = "&#8239;" + s1[n-3:n] + s2
			s2 = "." + s1[n-3:n] + s2
			s1 = s1[0 : n-3]
		}
		lines := s1 + s2

	*/

	text := html.EscapeString(strings.Join(a[2:], " "))

	// return `<option value="` + lbl + `">` + text + ` &mdash; ` + lines + ` zinnen</option>`
	return `<option value="` + lbl + `">` + text + `</option>`
}
