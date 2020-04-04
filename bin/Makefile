
all: alpg tree

alpg: alpg.go
	go build -o $@ $^

tree: tree.go corpora.go
	go build -o $@ $^

corpora.go: mkCorpora ../corpora.txt
	./mkCorpora | gofmt > $@

mkCorpora: mkCorpora.go
	go build -o $@ $^
