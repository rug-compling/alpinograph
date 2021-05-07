
all: alpino2agens

alpino2agens:          alpino2agens.go version.go go.mod
	go build -o $@ alpino2agens.go version.go

version.go: alpino2agens.go
	@echo $@ out of date && exit 1
