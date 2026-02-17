.PHONY: test race vet coverage lint vuln tools gate

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

coverage:
	go test ./... -coverprofile=cover.out

lint:
	staticcheck ./...

vuln:
	govulncheck ./...

tools:
	go install honnef.co/go/tools/cmd/staticcheck@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest

gate: test race vet coverage lint vuln
