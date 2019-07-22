export PATH := $(GOPATH)/bin:$(PATH)
LDFLAGS := -s -w

all: clean linux mac copy pack

clean:
	rm -f build/*

linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o ./build/hook_linux_amd64 .

mac:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o ./build/hook_osx .

copy:
	cp email.temp.html build
	cp .env.example build/.env

pack:
	tar -czf build/hook.tar.gz build/*