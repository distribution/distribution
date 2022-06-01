all: deps
gx:
	go get github.com/whyrusleeping/gx
	go get github.com/whyrusleeping/gx-go
deps: gx 
	gx --verbose install --global
	gx-go rewrite
test: deps
	go test -v -covermode count -coverprofile=coverage.out .
rw:
	gx-go rewrite
rwundo:
	gx-go rewrite --undo
publish: rwundo
	gx publish
.PHONY: all gx deps test rw rwundo publish


