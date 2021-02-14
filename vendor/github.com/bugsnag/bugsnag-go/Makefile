TEST?=./...

export GO111MODULE=auto

default: alldeps test

deps:
	go get -v -d ./...

alldeps:
	go get -v -d -t ./...

updatedeps:
	go get -v -d -u ./...

test: alldeps
	@# skipping Gin if the Go version is lower than 1.9, as the latest version of Gin has dropped support for these versions.
	@if [ "$(GO_VERSION)" = "1.7" ] || [ "$(GO_VERSION)" = "1.8" ] || [ "$(GO_VERSION)" = "1.9" ]; then \
		go test . ./errors ./martini ./negroni ./sessions ./headers; \
	else \
		go test . ./errors ./gin ./martini ./negroni ./sessions ./headers; \
	fi
	@go vet 2>/dev/null ; if [ $$? -eq 3 ]; then \
		go get golang.org/x/tools/cmd/vet; \
	fi
	@go vet $(TEST) ; if [ $$? -eq 1 ]; then \
		echo "go-vet: Issues running go vet ./..."; \
		exit 1; \
	fi

maze:
	bundle install
	bundle exec bugsnag-maze-runner

ci: alldeps test

bench:
	go test --bench=.*

testsetup:
	gem update --system
	gem install bundler
	bundle install

testplain: testsetup
	bundle exec bugsnag-maze-runner -c features/plain_features

testnethttp: testsetup
	bundle exec bugsnag-maze-runner -c features/net_http_features

testgin: testsetup
	bundle exec bugsnag-maze-runner -c features/gin_features

testmartini: testsetup
	bundle exec bugsnag-maze-runner -c features/martini_features

testnegroni: testsetup
	bundle exec bugsnag-maze-runner -c features/negroni_features

testrevel: testsetup
	bundle exec bugsnag-maze-runner -c features/revel_features

.PHONY: bin checkversion ci default deps generate releasebin test testacc testrace updatedeps testsetup testplain testnethttp testgin testmartini testrevel
