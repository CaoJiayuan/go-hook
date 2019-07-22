export PATH := $(GOPATH)/bin:$(PATH)
LDFLAGS := -s -w

all: linux mac

linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o ./build/hook_linux_amd64 .

mac:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o ./build/hook_osx .