package main

import (
	"github.com/pebbe/util"

	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"io/ioutil"
	"strings"
)

type MenuT struct {
	XMLName xml.Name `xml:"menu"`
	Items   []*ItemT `xml:"item"`
}

type ItemT struct {
	ID    string   `xml:"id,attr"`
	Lbl   string   `xml:"lbl,attr"`
	Items []*ItemT `xml:"item"`
	Text  string   `xml:",cdata"`
}

var (
	x = util.CheckErr
)

func main() {
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
			fmt.Fprintf(&buf1, "qs['%s'] = %q;\n", item.ID, format(trail[:lvl+1], item.Text))
			fmt.Fprintf(&buf2, "<li><a href=\"javascript:q('%s')\">%s</a></li>\n", item.ID, item.Lbl)
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
				strings.Replace(string(b), "PART1;", buf1.String(), 1),
				"<!--PART2-->", buf2.String(), 1),
			"<!--WARNING-->", `<!--

        WAARSCHUWING: dit is een gegenereerd bestand, bewerk het niet

-->`, 1))

}

func format(trail []string, s string) string {
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
