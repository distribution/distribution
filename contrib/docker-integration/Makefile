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
	sh ./install_certs.sh

test: 
	# Run tests

all: build
