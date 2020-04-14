
% : %.go
	go build $<

%.html : %.md
	./md2html $< > $@

all: md2html \
	index.html \
	alpinoagens.html
