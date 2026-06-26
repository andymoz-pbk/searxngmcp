BINARY = searxngmcp
VERSION = $(if $(VERSION_OVERRIDE),$(VERSION_OVERRIDE),$(shell git describe --tags --dirty 2>/dev/null || echo "dev"))
DIST_DIR = deploy

.PHONY: build vendor test check install dist release clean

# Build the binary (uses vendor/ if present, otherwise downloads)
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(BINARY) .

# Vendor all dependencies into vendor/ for offline/self-contained builds
vendor:
	go mod tidy
	go mod vendor

# Run unit tests
test:
	go test -v -count=1 ./...

# Run integration tests (requires SearXNG on :8080)
integration:
	go test -tags=integration -v -count=1 ./...

# Check: tidy + vendor + test (run before committing)
check: vendor test
	@echo "All checks passed."

# Install as systemd service (build + install + enable + start)
install: build
	install -m 755 $(BINARY) /usr/local/bin/$(BINARY)
	install -m 644 config.example.json /etc/searxngmcp/config.json
	install -m 644 searxngmcp.service /etc/systemd/system/searxngmcp.service
	systemctl daemon-reload

# Create distribution tarball (runtime files only, no source code)
dist: build
	mkdir -p $(DIST_DIR)
	rm -f $(DIST_DIR)/*
	cp $(BINARY) $(DIST_DIR)/
	cp config.example.json $(DIST_DIR)/
	cp searxngmcp.service $(DIST_DIR)/
	cp install_service.sh $(DIST_DIR)/
	cp install_service.bat $(DIST_DIR)/
	cp run.sh $(DIST_DIR)/
	cp run.bat $(DIST_DIR)/
	cp docker-compose.yml $(DIST_DIR)/
	cp searxng-settings.yml $(DIST_DIR)/
	cp Dockerfile $(DIST_DIR)/
	cp README.md $(DIST_DIR)/
	cp go.mod $(DIST_DIR)/
	cp go.sum $(DIST_DIR)/
	cp -r vendor/ $(DIST_DIR)/vendor/
	chmod +x $(DIST_DIR)/$(BINARY)
	chmod +x $(DIST_DIR)/*.sh
	tar czf $(BINARY)-$(VERSION).tar.gz --transform 's/^$(DIST_DIR)/$(BINARY)-$(VERSION)/' $(DIST_DIR)/
	@echo "Created $(BINARY)-$(VERSION).tar.gz ($(shell du -sh $(BINARY)-$(VERSION).tar.gz | cut -f1)) — self-contained (vendor/ included)"

# Full release: check deps, tidy, vendor, test, build, dist, cross-compile
release:
	./release.sh

clean:
	rm -f $(BINARY)
	rm -f $(BINARY)-*-*
	rm -f $(BINARY)-*.tar.gz
	rm -rf $(DIST_DIR)/