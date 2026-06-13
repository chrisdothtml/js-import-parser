.PHONY: test publish

test:
	go test ./...

publish:
	@if [ -z "$(VERSION)" ]; then \
		echo "VERSION is required, e.g. make publish VERSION=v0.2.0"; \
		exit 1; \
	fi
	go vet ./...
	go build ./...
	go test ./...
	git tag $(VERSION)
	git push origin $(VERSION)
