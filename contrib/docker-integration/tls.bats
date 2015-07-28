# Registry host name, should be set to non-localhost address and match
# DNS name in nginx/ssl certificates and what is installed in /etc/docker/cert.d
hostname="localregistry"

image="hello-world:latest"

# Login information, should match values in nginx/test.passwd
user="testuser"
password="passpassword"
email="distribution@docker.com"

function setup() {
	docker pull $image
}

# has_digest enforces the last output line is "Digest: sha256:..."
# the input is the name of the array containing the output lines
function has_digest() {
	filtered=$(echo "$1" |sed -rn '/[dD]igest\: sha(256|384|512)/ p')
	[ "$filtered" != "" ]
}

function login() {
	run docker login -u $user -p $password -e $email $1
	[ "$status" -eq 0 ]
	# First line is WARNING about credential save
	[ "${lines[1]}" = "Login Succeeded" ]
}

@test "Test valid certificates" {
	docker tag -f $image $hostname:5440/$image
	run docker push $hostname:5440/$image
	[ "$status" -eq 0 ]
	has_digest "$output"
}

@test "Test basic auth" {
	login $hostname:5441
	docker tag -f $image $hostname:5441/$image
	run docker push $hostname:5441/$image
	[ "$status" -eq 0 ]
	has_digest "$output"
}

@test "Test TLS client auth" {
	docker tag -f $image $hostname:5442/$image
	run docker push $hostname:5442/$image
	[ "$status" -eq 0 ]
	has_digest "$output"
}

@test "Test TLS client with invalid certificate authority fails" {
	docker tag -f $image $hostname:5443/$image
	run docker push $hostname:5443/$image
	[ "$status" -ne 0 ]
}

@test "Test basic auth with TLS client auth" {
	login $hostname:5444
	docker tag -f $image $hostname:5444/$image
	run docker push $hostname:5444/$image
	[ "$status" -eq 0 ]
	has_digest "$output"
}

@test "Test unknown certificate authority fails" {
	docker tag -f $image $hostname:5445/$image
	run docker push $hostname:5445/$image
	[ "$status" -ne 0 ]
}

@test "Test basic auth with unknown certificate authority fails" {
	run login $hostname:5446
	[ "$status" -ne 0 ]
	docker tag -f $image $hostname:5446/$image
	run docker push $hostname:5446/$image
	[ "$status" -ne 0 ]
}

@test "Test TLS client auth to server with unknown certificate authority fails" {
	docker tag -f $image $hostname:5447/$image
	run docker push $hostname:5447/$image
	[ "$status" -ne 0 ]
}

@test "Test failure to connect to server fails to fallback to SSLv3" {
	docker tag -f $image $hostname:5448/$image
	run docker push $hostname:5448/$image
	[ "$status" -ne 0 ]
}

