.PHONY: build test

build:
	docker-compose build

start: build
	docker-compose up -d

stop:
	docker-compose stop

clean:
	docker-compose kill
	docker-compose rm -f

install:
	sh ./install_certs.sh localhost
	sh ./install_certs.sh localregistry

test: 
	@echo "!!!!Ensure /etc/hosts entry is updated for localregistry and make install has been run"
	sh ./test_docker.sh localregistry

all: build
