// +build integration,perftest

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

var config Config

func init() {
	config.SetupFlags("", flag.CommandLine)
}

func main() {
	if err := flag.CommandLine.Parse(os.Args[1:]); err != nil {
		flag.CommandLine.PrintDefaults()
		log.Fatalf("failed to parse CLI commands")
	}
	if err := config.Validate(); err != nil {
		flag.CommandLine.PrintDefaults()
		log.Fatalf("invalid arguments")
	}

	client := NewClient(config.Client)

	file, err := os.Open(config.Filename)
	if err != nil {
		log.Fatalf("unable to open file to upload, %v", err)
	}
	defer file.Close()

	sess, err := session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			HTTPClient:           client,
			S3Disable100Continue: aws.Bool(!config.SDK.ExpectContinue),
		},
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		log.Fatalf("failed to load session, %v", err)
	}

	traces := make(chan *RequestTrace, config.SDK.Concurrency)
	uploader := s3manager.NewUploader(sess, func(u *s3manager.Uploader) {
		u.PartSize = config.SDK.PartSize
		u.Concurrency = config.SDK.Concurrency

		u.RequestOptions = append(u.RequestOptions,
			func(r *request.Request) {
				id := "op"
				if v, ok := r.Params.(*s3.UploadPartInput); ok {
					id = strconv.FormatInt(*v.PartNumber, 10)
				}
				tracer := NewRequestTrace(r.Context(), r.Operation.Name, id)
				r.SetContext(tracer)

				r.Handlers.Send.PushFront(tracer.OnSendAttempt)
				r.Handlers.CompleteAttempt.PushBack(tracer.OnCompleteAttempt)
				r.Handlers.CompleteAttempt.PushBack(func(rr *request.Request) {
				})
				r.Handlers.Complete.PushBack(tracer.OnComplete)
				r.Handlers.Complete.PushBack(func(rr *request.Request) {
					traces <- tracer
				})

				if config.SDK.WithUnsignedPayload {
					if r.Operation.Name != "UploadPart" {
						return
					}
					r.HTTPRequest.Header.Set("X-Amz-Content-Sha256", "UNSIGNED-PAYLOAD")
				}
			},
		)
	})

	metricReportDone := make(chan struct{})
	go func() {
		defer close(metricReportDone)
		metrics := map[string]*RequestTrace{}
		for trace := range traces {
			curTrace, ok := metrics[trace.Operation]
			if !ok {
				curTrace = trace
			} else {
				curTrace.attempts = append(curTrace.attempts, trace.attempts...)
				if len(trace.errs) != 0 {
					curTrace.errs = append(curTrace.errs, trace.errs...)
				}
				curTrace.finish = trace.finish
			}

			metrics[trace.Operation] = curTrace
		}

		for _, name := range []string{
			"CreateMultipartUpload",
			"CompleteMultipartUpload",
			"UploadPart",
		} {
			if trace, ok := metrics[name]; ok {
				printAttempts(name, trace, config.LogVerbose)
			}
		}
	}()

	fmt.Println("Starting upload...")
	start := time.Now()
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: &config.Bucket,
		Key:    &config.Key,
		Body:   file,
	})
	if err != nil {
		log.Fatalf("failed to upload object, %v", err)
	}
	close(traces)

	fileInfo, _ := file.Stat()
	size := fileInfo.Size()
	dur := time.Since(start)
	fmt.Printf("Upload finished, Size: %d, Dur: %s, Throughput: %.5f GB/s\n",
		size, dur, (float64(size)/(float64(dur)/float64(time.Second)))/float64(1e9),
	)

	<-metricReportDone
}

func printAttempts(op string, trace *RequestTrace, verbose bool) {
	fmt.Println(op+":",
		"latency:", trace.finish.Sub(trace.start),
		"requests:", len(trace.attempts),
		"errors:", len(trace.errs),
	)

	if !verbose {
		return
	}

	for _, a := range trace.attempts {
		fmt.Printf("  * %s\n", a)
	}
	if err := trace.Err(); err != nil {
		fmt.Println("Operation Errors:", err)
	}
	fmt.Println()
}
