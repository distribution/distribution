package filesystemmc

import (
	"context"
	"fmt"
	"log"

	pb "github.com/docker/distribution/registry/storage/driver/filesystemmc/storage-path"
	"google.golang.org/grpc"
)

var address string

func initGRPC(ad string) {
	address = ad
}

func getDockerStoragePath(host string, subpath string) (string, error) {

	conn, err := grpc.Dial(address, grpc.WithInsecure())

	if err != nil {
		log.Fatalf("did not connect: %v", err)

		finalError := fmt.Errorf("[ERROR] getDockerStoragePath did not connect: %v", err)

		fmt.Println(finalError)

		return "", finalError
	}

	defer conn.Close()

	c := pb.NewStoragePathClient(conn)

	r, err := c.GetDockerStoragePath(context.Background(), &pb.DockerStoragePathRequest{Host: host, SubPath: subpath})

	if err != nil {

		finalError := fmt.Errorf("[ERROR] getDockerStoragePath: %v", err)

		return "", finalError
	}

	return r.Path, nil
}
