BINARY=oci-sync

.PHONY: all build test fmt vet clean
all: build

build:
	go build -o $(BINARY) ./cmd/oci-sync

test:
	go test ./...

fmt:
	gofmt -w ./cmd/oci-sync ./pkg/oci

vet:
	go vet ./...

clean:
	-rm -f $(BINARY)
