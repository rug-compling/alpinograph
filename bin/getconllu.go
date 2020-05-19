package main

import (
	ag "github.com/bitnine-oss/agensgraph-golang"

	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	// "time"
)

type UDNode struct {
	v      ag.BasicVertex
	key    int
	id     string
	ud     *UDLink
	eud    []*UDLink
	copied string
}

type UDLink struct {
	from  string
	ifrom int
	rel   string
}

func getConllu(sentid string) (conllu string, ok bool) {

	var buf bytes.Buffer

	nodes := make([]*UDNode, 0)
	nodemap := make(map[int]*UDNode)
	uds := make(map[int]*UDLink)
	euds := make(map[int][]*UDLink)
	copied := make(map[int]int)

	// start := time.Now()
	// conlog("match (n1)-[r:ud]->(n2:word{sentid:'" + sentid + "'}) return n1, r, n2")
	rows, err := db.Query("match (n1)-[r:ud]->(n2:word{sentid:'" + sentid + "'}) return n1, r, n2")
	// conlog("started: ", time.Since(start))
	if x(err) {
		return "", false
	}

	for rows.Next() {
		// conlog("row: ", time.Since(start))
		var n1, r, n2 []byte
		if x(rows.Scan(&n1, &r, &n2)) {
			return "", false
		}

		var v1 ag.BasicVertex
		var e ag.BasicEdge
		var v2 ag.BasicVertex

		if x(v1.Scan(n1)) {
			return "", false
		}
		if x(e.Scan(r)) {
			return "", false
		}
		if x(v2.Scan(n2)) {
			return "", false
		}

		var ifrom, ito int
		if v1.Label == "sentence" {
			ifrom = 0
		} else {
			ifrom = toInt(v1.Properties["end"])
		}
		ito = toInt(v2.Properties["end"])
		no := UDNode{
			v:   v2,
			key: ito * 1000,
			id:  fmt.Sprint(ito),
		}
		nodes = append(nodes, &no)
		nodemap[no.key] = &no

		var from, to string
		if f, ok := e.Properties["from"]; ok {
			from = toString(f)
			copied[toKey(from)] = ifrom * 1000
		} else {
			from = fmt.Sprint(ifrom)
		}
		if t, ok := e.Properties["to"]; ok {
			to = toString(t)
			copied[toKey(to)] = ito * 1000
		} else {
			to = fmt.Sprint(ito)
		}
		tok := toKey(to)
		uds[tok] = &UDLink{
			from:  from,
			ifrom: toKey(from),
			rel:   toString(e.Properties["rel"]),
		}
	}
	// conlog("rows: ", time.Since(start))
	if x(rows.Err()) {
		return "", false
	}
	// conlog("done: ", time.Since(start))

	// start = time.Now()
	// conlog("match (n1)-[r:eud]->(n2:word{sentid:'" + sentid + "'}) return n1, r, n2")
	rows, err = db.Query("match (n1)-[r:eud]->(n2:word{sentid:'" + sentid + "'}) return n1, r, n2")
	// conlog("started: ", time.Since(start))
	if x(err) {
		return "", false
	}

	for rows.Next() {
		// conlog("row: ", time.Since(start))
		var n1, r, n2 []byte
		if x(rows.Scan(&n1, &r, &n2)) {
			return "", false
		}

		var v1 ag.BasicVertex
		var e ag.BasicEdge
		var v2 ag.BasicVertex

		if x(v1.Scan(n1)) {
			return "", false
		}
		if x(e.Scan(r)) {
			return "", false
		}
		if x(v2.Scan(n2)) {
			return "", false
		}

		var ifrom, ito int
		if v1.Label == "sentence" {
			ifrom = 0
		} else {
			ifrom = toInt(v1.Properties["end"])
		}
		ito = toInt(v2.Properties["end"])

		var from, to string
		if f, ok := e.Properties["from"]; ok {
			from = toString(f)
			copied[toKey(from)] = ifrom * 1000
		} else {
			from = fmt.Sprint(ifrom)
		}
		if t, ok := e.Properties["to"]; ok {
			to = toString(t)
			copied[toKey(to)] = ito * 1000
		} else {
			to = fmt.Sprint(ito)
		}
		tok := toKey(to)
		if _, ok := euds[tok]; !ok {
			euds[tok] = make([]*UDLink, 0)
		}
		euds[tok] = append(euds[tok], &UDLink{
			from:  from,
			ifrom: toKey(from),
			rel:   toString(e.Properties["rel"])},
		)

	}
	// conlog("rows: ", time.Since(start))
	if x(rows.Err()) {
		return "", false
	}
	// conlog("done: ", time.Since(start))

	for key, val := range copied {
		n1 := nodemap[val]
		nodes = append(nodes, &UDNode{
			v:      n1.v,
			copied: fromKey(val),
			key:    key,
			id:     fromKey(key),
		})
	}

	for i, node := range nodes {
		nodes[i].ud = uds[node.key]
		nodes[i].eud = euds[node.key]
	}

	sort.Slice(nodes, func(a, b int) bool {
		return nodes[a].key < nodes[b].key
	})

	for _, node := range nodes {
		fmt.Fprintf(&buf,
			"%s\t%s\t%s\t%s\t%s\t%s",
			node.id,
			toString(node.v.Properties["word"]),
			toString(node.v.Properties["lemma"]),
			toString(node.v.Properties["upos"]),
			attr(toString(node.v.Properties["postag"])),
			features(node.v.Properties))
		if node.ud == nil {
			fmt.Fprint(&buf, "\t_\t_")
		} else {
			fmt.Fprintf(&buf, "\t%s\t%s", node.ud.from, node.ud.rel)
		}
		sort.Slice(node.eud, func(a, b int) bool {
			return node.eud[a].ifrom < node.eud[b].ifrom
		})
		euds := make([]string, 0)
		for _, eud := range node.eud {
			euds = append(euds, eud.from+":"+eud.rel)
		}
		fmt.Fprint(&buf, "\t"+strings.Join(euds, "|"))
		if node.copied != "" {
			fmt.Fprintln(&buf, "\tCopiedFrom="+node.copied)
		} else if _, ok := node.v.Properties["nospaceafter"]; ok {
			fmt.Fprintln(&buf, "\tSpaceAfter=No")
		} else {
			fmt.Fprintln(&buf, "\t_")
		}
	}
	fmt.Fprint(&buf)

	return buf.String(), true
}

func toKey(s string) int {
	a := strings.Split(s, ".")
	key, _ := strconv.Atoi(a[0])
	key *= 1000

	if len(a) > 1 {
		n, err := strconv.Atoi(a[1])
		if err == nil {
			key += n
		}
	}
	return key
}

func fromKey(i int) string {
	a := i / 1000
	b := i % 1000
	if b == 0 {
		return fmt.Sprint(a)
	}
	return fmt.Sprint(a, ".", b)

}

func attr(s string) string {
	s = strings.Replace(s, "()", "", 1)
	s = strings.Replace(s, "(", "|", 1)
	s = strings.Replace(s, ")", "", 1)
	s = strings.Replace(s, ",", "|", -11)
	return s
}

func features(p map[string]interface{}) string {
	ff := make([]string, 0)
	for _, f := range []string{
		"Abbr", "Case", "Definite", "Degree", "Foreign",
		"Gender", "Number", "Person", "Poss", "PronType",
		"Reflex", "Tense", "VerbForm"} {
		if v, ok := p[f]; ok {
			ff = append(ff, f+"="+toString(v))
		}
	}
	if len(ff) == 0 {
		return "_"
	}
	return strings.Join(ff, "|")
}
