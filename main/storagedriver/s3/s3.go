package main

import (
	"encoding/json"
	"os"
	"strconv"

	"github.com/crowdmob/goamz/aws"
	"github.com/docker/docker-registry/storagedriver/ipc"
	"github.com/docker/docker-registry/storagedriver/s3"
)

func main() {
	parametersBytes := []byte(os.Args[1])
	var parameters map[string]interface{}
	err := json.Unmarshal(parametersBytes, &parameters)
	if err != nil {
		panic(err)
	}

	accessKey, ok := parameters["accessKey"].(string)
	if !ok || accessKey == "" {
		panic("No accessKey parameter")
	}

	secretKey, ok := parameters["secretKey"].(string)
	if !ok || secretKey == "" {
		panic("No secretKey parameter")
	}

	region, ok := parameters["region"].(string)
	if !ok || region == "" {
		panic("No region parameter")
	}

	bucket, ok := parameters["bucket"].(string)
	if !ok || bucket == "" {
		panic("No bucket parameter")
	}

	encrypt, ok := parameters["encrypt"].(string)
	if !ok {
		panic("No encrypt parameter")
	}

	encryptBool, err := strconv.ParseBool(encrypt)
	if err != nil {
		panic(err)
	}

	driver, err := s3.NewDriver(accessKey, secretKey, aws.GetRegion(region), encryptBool, bucket)
	if err != nil {
		panic(err)
	}

	ipc.Server(driver)
}
