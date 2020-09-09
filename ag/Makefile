
all: alpino2agens

alpino2agens: alpino2agens.go version.go
	go build -o $@ $^

version.go: alpino2agens.go
	@echo $@ out of date && exit 1
