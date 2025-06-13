
GOFMT_FILES?=$$(find . -name '*.go')

default: build

build: $(shell find . \( -type f -name '*.go' -print \))
	set -xe ;\
	vtag=$$(git describe --tags --abbrev=12 --dirty --broken) ;\
	go build -o terragrunt-iac-engine-terraform -ldflags "-X github.com/gruntwork-io/go-commons/version.Version=$${vtag} -extldflags '-static'" .

clean:
	rm -f engine

lint:
	golangci-lint run ./...

test:
	go test -v ./...

fmt:
	@echo "Running source files through gofmt..."
	gofmt -w $(GOFMT_FILES)

pre-commit:
	pre-commit run --all-files

.PHONY: tools pre-commit lint protoc test default
