package main

import (
	"github.com/pebbe/blackfriday/v2"
	"github.com/pebbe/util"

	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"
)

var (
	x = util.CheckErr

	template = `<!DOCTYPE html>
<html>
  <head>
    <title>AlpinoGraph -- %s</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link rel="icon" href="../../favicon.ico" type="image/ico">
    <link rel="stylesheet" href="style.css" type="text/css">
  </head>
  <body%s>
    <div id="container">
      <h1>%s</h1>
%s
    </div>
  </body>
</html>
`
)

type Renderer struct {
	Base blackfriday.Renderer
}

func main() {
	b, err := ioutil.ReadFile(os.Args[1])
	x(err)

	ss := strings.Split(string(b), "////-->")
	var s string
	opts := map[string]string{}
	if len(ss) == 2 {
		opts = getOpts(ss[0])
		s = ss[1]
	} else {
		s = ss[0]
	}
	s = markdown(s)

	s = strings.Replace(s, `title="i"`, `class="internal"`, -1)
	s = strings.Replace(s, "<!--[[", "", -1)
	s = strings.Replace(s, "]]-->", "", -1)
	s = strings.Replace(s, "<ul>\n<li>[X]", "<ul class=\"todo\">\n<li>[x]", -1)
	s = strings.Replace(s, "<ul>\n<li>[x]", "<ul class=\"todo\">\n<li>[x]", -1)
	s = strings.Replace(s, "<ul>\n<li>[ ]", "<ul class=\"todo\">\n<li>[ ]", -1)
	s = strings.Replace(s, "<li>[x]", "<li class=\"checked\">", -1)
	s = strings.Replace(s, "<li>[ ]", "<li class=\"unchecked\">", -1)

	title := opts["title:"]
	if title == "" {
		title = strings.Replace(strings.Replace(path.Base(os.Args[1]), ".md", "", -1), "_", " ", -1)
	}

	class := ""
	if c := opts["class:"]; c != "" {
		class = fmt.Sprintf(" class=%q", c)
	}

	fmt.Printf(template, title, class, title, s)
}

func getOpts(s string) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(s, "\n") {
		ww := strings.Fields(line)
		if len(ww) > 1 {
			m[ww[0]] = strings.Join(ww[1:], " ")
		}
	}
	return m
}

func markdown(s string) string {

	r := blackfriday.NewHTMLRenderer(
		blackfriday.HTMLRendererParameters{
			Flags: blackfriday.Smartypants | blackfriday.SmartypantsDashes | blackfriday.SmartypantsLatexDashes,
		})
	mr := &Renderer{Base: r}

	return string(blackfriday.Run([]byte(s), blackfriday.WithRenderer(mr)))
}

// RenderNode satisfies the Renderer interface
func (r *Renderer) RenderNode(w io.Writer, node *blackfriday.Node, entering bool) blackfriday.WalkStatus {
	switch node.Type {
	case blackfriday.Table:
		var ws blackfriday.WalkStatus
		if entering {
			fmt.Fprintf(w, `<div class="overflow mytable">`)
		}
		ws = r.Base.RenderNode(w, node, entering)
		if !entering {
			fmt.Fprintf(w, `</div>`)
		}
		return ws
	default:
		return r.Base.RenderNode(w, node, entering)
	}
}

// RenderHeader satisfies the Renderer interface
func (r *Renderer) RenderHeader(w io.Writer, ast *blackfriday.Node) {
	r.Base.RenderHeader(w, ast)
}

// RenderFooter satisfies the Renderer interface
func (r *Renderer) RenderFooter(w io.Writer, ast *blackfriday.Node) {
	r.Base.RenderFooter(w, ast)
}
