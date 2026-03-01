.PHONY: test race vet coverage lint vuln tools gate redeploy-patent-screen smoke-production render-patent-report pdf-regression-test pdf-regression-calibrate

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

redeploy-patent-screen:
	./scripts/redeploy-patent-screen-stack.sh

smoke-production:
	./scripts/check-production-url.sh "https://techtransfer.agency/"

render-patent-report:
	@if [ -z "$(INPUT)" ]; then echo "usage: make render-patent-report INPUT=/abs/path/response.json [OUTPUT=/abs/path/report.md] [JSON_OUTPUT=/abs/path/rebuilt.json]"; exit 1; fi
	go run ./cmd/render-patent-report -input "$(INPUT)" -output "$(OUTPUT)" -json-output "$(JSON_OUTPUT)"

pdf-regression-test:
	python3 scripts/pdf_regression.py test

pdf-regression-calibrate:
	python3 scripts/pdf_regression.py calibrate
