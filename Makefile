BINARIO := ValidadorAlarmas
PKG     := ./cmd/validadoralarmas

.PHONY: build test vet tidy clean

# Binario estático para Linux x86_64 en ./bin
build:
	mkdir -p bin log reports
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/$(BINARIO) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

tidy:
	go mod tidy

clean:
	rm -rf bin
