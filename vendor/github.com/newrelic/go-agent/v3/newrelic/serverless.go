// Copyright 2020 New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package newrelic

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	// agentLanguage is used in the connect JSON and the Lambda JSON.
	agentLanguage = "go"

	lambdaMetadataVersion = 2
)

// serverlessHarvest is used to store and log data when the agent is running in
// serverless mode.
type serverlessHarvest struct {
	logger          Logger
	awsExecutionEnv string

	// The Lambda handler could be using multiple goroutines so we use a
	// mutex to prevent race conditions.
	sync.Mutex
	harvest *harvest
}

// newServerlessHarvest creates a new serverlessHarvest.
func newServerlessHarvest(logger Logger, getEnv func(string) string) *serverlessHarvest {
	return &serverlessHarvest{
		logger:          logger,
		awsExecutionEnv: getEnv("AWS_EXECUTION_ENV"),

		// We can use dfltHarvestCfgr because
		// serverless mode doesn't have a connect, and therefore won't
		// have custom event limits from the server.
		harvest: newHarvest(time.Now(), dfltHarvestCfgr),
	}
}

// Consume adds data to the harvest.
func (sh *serverlessHarvest) Consume(data harvestable) {
	if nil == sh {
		return
	}
	sh.Lock()
	defer sh.Unlock()

	data.MergeIntoHarvest(sh.harvest)
}

func (sh *serverlessHarvest) swapHarvest() *harvest {
	sh.Lock()
	defer sh.Unlock()

	h := sh.harvest
	sh.harvest = newHarvest(time.Now(), dfltHarvestCfgr)
	return h
}

// Write logs the data in the format described by:
// https://source.datanerd.us/agents/agent-specs/blob/master/Lambda.md
func (sh *serverlessHarvest) Write(arn string, writer io.Writer) {
	if nil == sh {
		return
	}
	harvest := sh.swapHarvest()
	payloads := harvest.Payloads(false)
	// Note that *json.RawMessage (instead of json.RawMessage) is used to
	// support older Go versions: https://go-review.googlesource.com/c/go/+/21811/
	harvestPayloads := make(map[string]*json.RawMessage, len(payloads))
	for _, p := range payloads {
		agentRunID := ""
		cmd := p.EndpointMethod()
		data, err := p.Data(agentRunID, time.Now())
		if err != nil {
			sh.logger.Error("error creating payload json", map[string]interface{}{
				"command": cmd,
				"error":   err.Error(),
			})
			continue
		}
		if nil == data {
			continue
		}
		// NOTE!  This code relies on the fact that each payload is
		// using a different endpoint method.  Sometimes the transaction
		// events payload might be split, but since there is only one
		// transaction event per serverless transaction, that's not an
		// issue.  Likewise, if we ever split normal transaction events
		// apart from synthetics events, the transaction will either be
		// normal or synthetic, so that won't be an issue.  Log an error
		// if this happens for future defensiveness.
		if _, ok := harvestPayloads[cmd]; ok {
			sh.logger.Error("data with duplicate command name lost", map[string]interface{}{
				"command": cmd,
			})
		}
		d := json.RawMessage(data)
		harvestPayloads[cmd] = &d
	}

	if len(harvestPayloads) == 0 {
		// The harvest may not contain any data if the serverless
		// transaction was ignored.
		sh.logger.Debug("go agent serverless harvest contained no payload data", nil)
		return
	}

	data, err := json.Marshal(harvestPayloads)
	if nil != err {
		sh.logger.Error("error creating serverless data json", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	var dataBuf bytes.Buffer
	gz := gzip.NewWriter(&dataBuf)
	gz.Write(data)
	gz.Flush()
	gz.Close()

	js, err := json.Marshal([]interface{}{
		lambdaMetadataVersion,
		"NR_LAMBDA_MONITORING",
		struct {
			MetadataVersion      int    `json:"metadata_version"`
			ARN                  string `json:"arn,omitempty"`
			ProtocolVersion      int    `json:"protocol_version"`
			ExecutionEnvironment string `json:"execution_environment,omitempty"`
			AgentVersion         string `json:"agent_version"`
			AgentLanguage        string `json:"agent_language"`
		}{
			MetadataVersion:      lambdaMetadataVersion,
			ProtocolVersion:      procotolVersion,
			AgentVersion:         Version,
			ExecutionEnvironment: sh.awsExecutionEnv,
			ARN:                  arn,
			AgentLanguage:        agentLanguage,
		},
		base64.StdEncoding.EncodeToString(dataBuf.Bytes()),
	})

	if err != nil {
		sh.logger.Error("error creating serverless json", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// log json data to stdout if the agent is in debug mode to help troubleshoot lambda issues
	sh.logger.Debug("harvest data: " + string(js), nil)
	fmt.Fprintln(writer, string(js))
}
