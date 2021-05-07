
all: alpg api download tree

alpg: alpg.go ../go.mod
	go build -o $@ $<

api: api.go ../go.mod
	go build -o $@ $<

download:              download.go cyp2alp.go cyp2ud.go corpora.go ../go.mod
	go build -o $@ download.go cyp2alp.go cyp2ud.go corpora.go

tree:                  tree.go corpora.go getconllu.go conllu2svg.go fontsize.go ../go.mod
	go build -o $@ tree.go corpora.go getconllu.go conllu2svg.go fontsize.go

corpora.go: mkCorpora ../corpora.txt
	./mkCorpora | gofmt > $@

mkCorpora: mkCorpora.go ../go.mod
	go build -o $@ $<
