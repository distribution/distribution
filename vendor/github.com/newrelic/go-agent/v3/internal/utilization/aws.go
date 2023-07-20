// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package utilization

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
)

const (
	awsHostname          = "169.254.169.254"
	awsEndpointPath      = "/2016-09-02/dynamic/instance-identity/document"
	awsTokenEndpointPath = "/latest/api/token"
	awsEndpoint          = "http://" + awsHostname + awsEndpointPath
	awsTokenEndpoint     = "http://" + awsHostname + awsTokenEndpointPath
	awsTokenTTL          = "60" // seconds this AWS utilization session will last
)

type aws struct {
	InstanceID       string `json:"instanceId,omitempty"`
	InstanceType     string `json:"instanceType,omitempty"`
	AvailabilityZone string `json:"availabilityZone,omitempty"`
}

func gatherAWS(util *Data, client *http.Client) error {
	aws, err := getAWS(client)
	if err != nil {
		// Only return the error here if it is unexpected to prevent
		// warning customers who aren't running AWS about a timeout.
		if _, ok := err.(unexpectedAWSErr); ok {
			return err
		}
		return nil
	}
	util.Vendors.AWS = aws

	return nil
}

type unexpectedAWSErr struct{ e error }

func (e unexpectedAWSErr) Error() string {
	return fmt.Sprintf("unexpected AWS error: %v", e.e)
}

// getAWSToken attempts to get the IMDSv2 token within the providerTimeout set
// provider.go.
func getAWSToken(client *http.Client) (token string, err error) {
	request, err := http.NewRequest("PUT", awsTokenEndpoint, nil)
	request.Header.Add("X-aws-ec2-metadata-token-ttl-seconds", awsTokenTTL)
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func getAWS(client *http.Client) (ret *aws, err error) {
	// In some cases, 3rd party providers might block requests to metadata
	// endpoints in such a way that causes a panic in the underlying
	// net/http library's (*Transport).getConn() function. To mitigate that
	// possibility, we preemptively setup a recovery deferral.
	defer func() {
		if r := recover(); r != nil {
			ret = nil
			err = unexpectedAWSErr{e: errors.New("panic contacting AWS metadata endpoint")}
		}
	}()

	// AWS' IMDSv2 requires us to get a token before requesting metadata.
	awsToken, err := getAWSToken(client)
	if err != nil {
		// No unexpectedAWSErr here: A timeout is usually going to
		// happen.
		return nil, err
	}

	//Add the header to the outbound request.
	request, err := http.NewRequest("GET", awsEndpoint, nil)
	request.Header.Add("X-aws-ec2-metadata-token", awsToken)

	response, err := client.Do(request)
	if err != nil {
		// No unexpectedAWSErr here: A timeout is usually going to
		// happen.
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return nil, unexpectedAWSErr{e: fmt.Errorf("response code %d", response.StatusCode)}
	}

	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, unexpectedAWSErr{e: err}
	}
	a := &aws{}
	if err := json.Unmarshal(data, a); err != nil {
		return nil, unexpectedAWSErr{e: err}
	}

	if err := a.validate(); err != nil {
		return nil, unexpectedAWSErr{e: err}
	}

	return a, nil
}

func (a *aws) validate() (err error) {
	a.InstanceID, err = normalizeValue(a.InstanceID)
	if err != nil {
		return fmt.Errorf("invalid instance ID: %v", err)
	}

	a.InstanceType, err = normalizeValue(a.InstanceType)
	if err != nil {
		return fmt.Errorf("invalid instance type: %v", err)
	}

	a.AvailabilityZone, err = normalizeValue(a.AvailabilityZone)
	if err != nil {
		return fmt.Errorf("invalid availability zone: %v", err)
	}

	return
}
