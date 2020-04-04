
index.html: template.html menu.xml mkMenu corpora.txt
	xmllint --noout menu.xml
	./mkMenu > index.html

mkMenu: mkMenu.go
	go build $<
