package main

import (
	_ "github.com/lib/pq"

	"encoding/json"
	"encoding/xml"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type Alpino_ds struct {
	XMLName  xml.Name      `xml:"alpino_ds"`
	Version  string        `xml:"version,attr,omitempty"`
	Metadata *MetadataType `xml:"metadata,omitempty"`
	Parser   *ParserT      `xml:"parser,omitempty"`
	Node0    *NodeT        `xml:"node"`
	Sentence *SentType     `xml:"sentence"`
}

type MetadataType struct {
	Meta []MetaT `xml:"meta"`
}

type MetaT struct {
	Type  string `xml:"type,attr,omitempty"`
	Name  string `xml:"name,attr,omitempty"`
	Value string `xml:"value,attr,omitempty"`
}

type ParserT struct {
	Build string `xml:"build,attr,omitempty,omitempty"`
	Date  string `xml:"date,attr,omitempty,omitempty"`
	Cats  string `xml:"cats,attr,omitempty,omitempty"`
	Skips string `xml:"skips,attr,omitempty,omitempty"`
}

type SentType struct {
	Sent   string `xml:",chardata"`
	SentId string `xml:"sentid,attr,omitempty,omitempty"`
}

type NodeT struct {
	parent       int
	Aform        string   `xml:"aform,attr,omitempty" json:"aform,attr"`
	Begin        int      `xml:"begin,attr" json:"begin,attr"`
	Buiging      string   `xml:"buiging,attr,omitempty" json:"buiging,attr"`
	Case         string   `xml:"case,attr,omitempty" json:"case,attr"`
	Cat          string   `xml:"cat,attr,omitempty" json:"cat,attr"`
	Comparative  string   `xml:"comparative,attr,omitempty" json:"comparative,attr"`
	Conjtype     string   `xml:"conjtype,attr,omitempty" json:"conjtype,attr"`
	Def          string   `xml:"def,attr,omitempty" json:"def,attr"`
	Dial         string   `xml:"dial,attr,omitempty" json:"dial,attr"`
	Dscmanual    string   `xml:"dscmanual,attr,omitempty" json:"dscmanual,attr"`
	Dscsense     string   `xml:"dscsense,attr,omitempty" json:"dscsense,attr"`
	End          int      `xml:"end,attr" json:"end,attr"`
	Frame        string   `xml:"frame,attr,omitempty" json:"frame,attr"`
	Gen          string   `xml:"gen,attr,omitempty" json:"gen,attr"`
	Genus        string   `xml:"genus,attr,omitempty" json:"genus,attr"`
	Getal        string   `xml:"getal,attr,omitempty" json:"getal,attr"`
	GetalN       string   `xml:"getal-n,attr,omitempty" json:"getal_n,attr"` // hier een minus voor xml, underscore voor json
	Graad        string   `xml:"graad,attr,omitempty" json:"graad,attr"`
	His          string   `xml:"his,attr,omitempty" json:"his,attr"`
	His1         string   `xml:"his_1,attr,omitempty" json:"his_1,attr"`
	His2         string   `xml:"his_2,attr,omitempty" json:"his_2,attr"`
	His11        string   `xml:"his_1_1,attr,omitempty" json:"his_1_1,attr"`
	His12        string   `xml:"his_1_2,attr,omitempty" json:"his_1_2,attr"`
	His21        string   `xml:"his_2_1,attr,omitempty" json:"his_2_1,attr"`
	His22        string   `xml:"his_2_2,attr,omitempty" json:"his_2_2,attr"`
	His111       string   `xml:"his_1_1_1,attr,omitempty" json:"his_1_1_1,attr"`
	His112       string   `xml:"his_1_1_2,attr,omitempty" json:"his_1_1_2,attr"`
	His121       string   `xml:"his_1_2_1,attr,omitempty" json:"his_1_2_1,attr"`
	His122       string   `xml:"his_1_2_2,attr,omitempty" json:"his_1_2_2,attr"`
	His211       string   `xml:"his_2_1_1,attr,omitempty" json:"his_2_1_1,attr"`
	His212       string   `xml:"his_2_1_2,attr,omitempty" json:"his_2_1_2,attr"`
	His221       string   `xml:"his_2_2_1,attr,omitempty" json:"his_2_2_1,attr"`
	His222       string   `xml:"his_2_2_2,attr,omitempty" json:"his_2_2_2,attr"`
	Id           int      `xml:"id,attr" json:"id,attr"`
	Iets         string   `xml:"iets,attr,omitempty" json:"iets,attr"`
	Index        string   `xml:"index,attr,omitempty" json:"index,attr"`
	Infl         string   `xml:"infl,attr,omitempty" json:"infl,attr"`
	Lcat         string   `xml:"lcat,attr,omitempty" json:"lcat,attr"`
	Lemma        string   `xml:"lemma,attr,omitempty" json:"lemma,attr"`
	Lwtype       string   `xml:"lwtype,attr,omitempty" json:"lwtype,attr"`
	MwuRoot      string   `xml:"mwu_root,attr,omitempty" json:"mwu_root,attr"`
	MwuSense     string   `xml:"mwu_sense,attr,omitempty" json:"mwu_sense,attr"`
	Naamval      string   `xml:"naamval,attr,omitempty" json:"naamval,attr"`
	Neclass      string   `xml:"neclass,attr,omitempty" json:"neclass,attr"`
	Npagr        string   `xml:"npagr,attr,omitempty" json:"npagr,attr"`
	Ntype        string   `xml:"ntype,attr,omitempty" json:"ntype,attr"`
	Num          string   `xml:"num,attr,omitempty" json:"num,attr"`
	Numtype      string   `xml:"numtype,attr,omitempty" json:"numtype,attr"`
	Pb           string   `xml:"pb,attr,omitempty" json:"pb,attr"`
	Pdtype       string   `xml:"pdtype,attr,omitempty" json:"pdtype,attr"`
	Per          string   `xml:"per,attr,omitempty" json:"per,attr"`
	Personalized string   `xml:"personalized,attr,omitempty" json:"personalized,attr"`
	Persoon      string   `xml:"persoon,attr,omitempty" json:"persoon,attr"`
	Pos          string   `xml:"pos,attr,omitempty" json:"pos,attr"`
	Positie      string   `xml:"positie,attr,omitempty" json:"positie,attr"`
	Postag       string   `xml:"postag,attr,omitempty" json:"postag,attr"`
	Pron         string   `xml:"pron,attr,omitempty" json:"pron,attr"`
	Pt           string   `xml:"pt,attr,omitempty" json:"pt,attr"`
	Pvagr        string   `xml:"pvagr,attr,omitempty" json:"pvagr,attr"`
	Pvtijd       string   `xml:"pvtijd,attr,omitempty" json:"pvtijd,attr"`
	Refl         string   `xml:"refl,attr,omitempty" json:"refl,attr"`
	Rel          string   `xml:"rel,attr,omitempty" json:"rel,attr"`
	Rnum         string   `xml:"rnum,attr,omitempty" json:"rnum,attr"`
	Root         string   `xml:"root,attr,omitempty" json:"root,attr"`
	Sc           string   `xml:"sc,attr,omitempty" json:"sc,attr"`
	Sense        string   `xml:"sense,attr,omitempty" json:"sense,attr"`
	Special      string   `xml:"special,attr,omitempty" json:"special,attr"`
	Spectype     string   `xml:"spectype,attr,omitempty" json:"spectype,attr"`
	Status       string   `xml:"status,attr,omitempty" json:"status,attr"`
	Stype        string   `xml:"stype,attr,omitempty" json:"stype,attr"`
	Tense        string   `xml:"tense,attr,omitempty" json:"tense,attr"`
	Vform        string   `xml:"vform,attr,omitempty" json:"vform,attr"`
	Vwtype       string   `xml:"vwtype,attr,omitempty" json:"vwtype,attr"`
	Vztype       string   `xml:"vztype,attr,omitempty" json:"vztype,attr"`
	Wh           string   `xml:"wh,attr,omitempty" json:"wh,attr"`
	Wk           string   `xml:"wk,attr,omitempty" json:"wk,attr"`
	Word         string   `xml:"word,attr,omitempty" json:"word,attr"`
	Wvorm        string   `xml:"wvorm,attr,omitempty" json:"wvorm,attr"`
	NodeList     []*NodeT `xml:"node,omitempty"`
}

type jsSentence struct {
	Cats   json.Number `json:"cats"`
	Skips  json.Number `json:"skips"`
	Tokens string      `json:"tokens"`
}

type jsRel struct {
	Rel string      `json:"rel"`
	Id  json.Number `json:"id"`
}

type jsMeta struct {
	Type  string      `json:"type"`
	Name  string      `json:"name"`
	Value interface{} `json:"value"`
}

func cyp2alp(sentid string) string {

	s := ""
	t := ""
	rows, err := db.Query("match (s:sentence{sentid: '" + sentid + "'}), (n:node{sentid: '" + sentid + "', id: 0}) return to_json(s) -> 'properties', to_json(n) -> 'properties'")
	x(err)
	for rows.Next() {
		x(rows.Scan(&s, &t))
	}
	if s == "" {
		x(fmt.Errorf("Not found: %s", sentid))
	}

	var sentence jsSentence
	x(json.Unmarshal([]byte(s), &sentence))

	var top NodeT
	x(json.Unmarshal([]byte(t), &top))

	alpino := &Alpino_ds{
		Version: "1.10",
		Sentence: &SentType{
			SentId: sentid,
			Sent:   sentence.Tokens,
		},
		Node0: &top,
	}

	var cats, skips string
	if _, e := sentence.Cats.Int64(); e == nil {
		cats = sentence.Cats.String()
	}
	if _, e := sentence.Skips.Int64(); e == nil {
		skips = sentence.Skips.String()
	}

	if cats != "" || skips != "" {
		alpino.Parser = &ParserT{
			Cats:  cats,
			Skips: skips,
		}
	}

	links := make(map[int][]int)
	nodes := make(map[int]*NodeT)
	nodes[0] = &top

	rows, err = db.Query("match (n1:node{sentid: '" + sentid + "'})-[r:rel]->(n2:nw) return to_json(r) -> 'properties', to_json(n1) -> 'properties' -> 'id', to_json(n2) -> 'properties'")
	x(err)
	var n1, n2, r string
	for rows.Next() {
		x(rows.Scan(&r, &n1, &n2))

		var rel jsRel
		x(json.Unmarshal([]byte(r), &rel))

		p, err := strconv.Atoi(n1)
		x(err)

		var node NodeT
		x(json.Unmarshal([]byte(n2), &node))

		if id, e := rel.Id.Int64(); e == nil {
			nodes[int(id)] = &NodeT{
				parent: p,
				Begin:  node.Begin,
				End:    node.End,
				Rel:    rel.Rel,
				Id:     int(id),
			}
			if _, ok := links[node.Id]; !ok {
				links[node.Id] = make([]int, 0)
			}
			links[node.Id] = append(links[node.Id], int(id))
		} else {
			node.parent = p
			node.Rel = rel.Rel
			nodes[node.Id] = &node
		}

	}
	x(rows.Err())

	keys := make([]int, 0)
	for key := range nodes {
		keys = append(keys, key)
	}
	sort.Ints(keys)

	for _, key := range keys {
		if key == 0 {
			continue
		}
		node := nodes[key]
		parent := nodes[node.parent]
		if parent.NodeList == nil {
			parent.NodeList = make([]*NodeT, 0)
		}
		parent.NodeList = append(parent.NodeList, node)
	}

	if len(links) > 0 {
		keys := make([]int, 0)
		for key := range links {
			keys = append(keys, key)
		}
		sort.Ints(keys)
		indexen := make(map[int]int)
		for i, key := range keys {
			indexen[key] = i + 1
		}
		for key, vals := range links {
			s := fmt.Sprint(indexen[key])
			nodes[key].Index = s
			for _, val := range vals {
				nodes[val].Index = s
			}
		}
	}

	var f func(*NodeT)
	f = func(node *NodeT) {
		if node.NodeList == nil {
			return
		}
		sort.Slice(node.NodeList, func(a, b int) bool {
			return node.NodeList[a].Id < node.NodeList[b].Id
		})
		for _, n := range node.NodeList {
			f(n)
		}
	}
	f(alpino.Node0)

	rows, err = db.Query("match (m:meta{sentid: '" + sentid + "'}) return to_json(m) -> 'properties'")
	x(err)
	for rows.Next() {
		var m string
		x(rows.Scan(&m))

		var meta jsMeta
		x(json.Unmarshal([]byte(m), &meta))

		if alpino.Metadata == nil {
			alpino.Metadata = &MetadataType{Meta: make([]MetaT, 0)}
		}
		alpino.Metadata.Meta = append(alpino.Metadata.Meta, MetaT{
			Type:  meta.Type,
			Name:  meta.Name,
			Value: fmt.Sprint(meta.Value),
		})
	}
	x(rows.Err())

	b, err := xml.MarshalIndent(alpino, "", "  ")
	x(err)

	xml := `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + string(b) + "\n"
	xml = strings.Replace(xml, "></parser>", "/>", -1)
	xml = strings.Replace(xml, "></node>", "/>", -1)
	xml = strings.Replace(xml, "></meta>", "/>", -1)

	return xml
}
