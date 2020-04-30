
all: alpg api tree

alpg: alpg.go
	go build -o $@ $^

api: api.go
	go build -o $@ $^

tree: tree.go corpora.go getconllu.go conllu2svg.go fontsize.go
	go build -o $@ $^

corpora.go: mkCorpora ../corpora.txt
	./mkCorpora | gofmt > $@

mkCorpora: mkCorpora.go
	go build -o $@ $^
