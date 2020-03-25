
index.html: template.html menu.xml mkMenu
	xmllint --noout menu.xml
	./mkMenu > index.html

mkMenu: mkMenu.go
	go build $<
