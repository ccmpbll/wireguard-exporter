BINARY   = wireguard-exporter
TARGET   = linux/amd64
LDFLAGS  = -s -w

.PHONY: build deploy tidy release

build:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BINARY) .

deploy: build
	scp -P 42422 $(BINARY) root@45.79.215.253:/usr/local/bin/$(BINARY)
	ssh -p 42422 root@45.79.215.253 "systemctl restart wireguard-exporter"

tidy:
	go mod tidy

release:
	@test -n "$(VERSION)" || (echo "Usage: make release VERSION=v1.0.0" && exit 1)
	git tag $(VERSION)
	git push origin $(VERSION)
