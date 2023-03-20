package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type clientObject struct {
	client      *http.Client
	method_type string
	url         string
}

type httpJob struct {
	target       *clientObject
	requestJson  *[]byte
	responseJson **[]byte
	wg           *sync.WaitGroup
	StatusCode   *int
}

type jobPool struct {
	jobs   chan httpJob
	client *clientObject
}

// Takes a string argument with standard http location or a unix sock, and packs it into an object to be used in a standard way.
func upstreamBuilder(location string, method_type string) (clientob *clientObject) {
	clientob = new(clientObject)
	clientob.method_type = method_type
	if location == "" {
		return nil
	}
	if !strings.HasPrefix(location, "http://") && !strings.HasPrefix(location, "https://") {
		location = "http://" + location
	}
	if strings.HasPrefix(location, "http://unix:") {
		loc := strings.Split(location, "http://unix:")[1]
		if debug {
			log.Println("Unix upstream on " + loc)
		}
		clientob.client = &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					nc, err := net.Dial("unix", loc)
					if err != nil {
						log.Fatal(err)
					}
					return nc, err
				},
			},
		}
		clientob.url = "http://unix"
	} else {
		if debug {
			log.Println("Upstream on " + location)
		}
		clientob.client = &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        10000,
				IdleConnTimeout:     30 * time.Second,
				MaxIdleConnsPerHost: 10000,
				DisableKeepAlives:   false,
			},
		}
		clientob.url = location
	}
	return clientob
}

// Main request handler. Takes a request, sends it to the worker pool, and returns the response. This unmarshals the response into a map.
func requestToResponse(jobp jobPool, reqmessage map[string]interface{}) (int, map[string]interface{}) {
	// Pack request as json.
	requestJson, err := jsonit.Marshal(reqmessage)
	if err != nil {
		log.Println("Couldn't re-marshal request")
		log.Println(err)
		return http.StatusBadRequest, nil
	}

	status, respj := requestToResponseBytes(jobp, requestJson)
	if status != http.StatusOK {
		return status, nil
	}

	// Convert reply to json.
	var rm map[string]interface{}
	err = jsonit.Unmarshal(respj, &rm)
	if err != nil {
		log.Println("Couldn't match response type")
		log.Println(respj)
		log.Println(err)
		return http.StatusBadRequest, nil
	}
	return http.StatusOK, rm
}

// Helper function to send a request to the worker pool and return the raw response in bytes.
func requestToResponseBytes(jobp jobPool, requestJson []byte) (int, []byte) {
	// Push request job to worker pool.
	status := int(0)
	var rawbytes []byte
	rawbytesPtr := &rawbytes
	var wg sync.WaitGroup
	wg.Add(1)
	job := httpJob{target: jobp.client, requestJson: &requestJson, wg: &wg, responseJson: &rawbytesPtr, StatusCode: &status}
	select {
	case jobp.jobs <- job: // insert job if buffer not full
	default: // job buffer full
		wg.Done()
		return http.StatusGatewayTimeout, nil
	}
	wg.Wait()

	if len(**job.responseJson) == 0 {
		log.Println("Bad (empty) response from upstream: " + (*(jobp.client)).url)
		return http.StatusInternalServerError, nil
	}
	return status, **job.responseJson
}

// Initialize worker pool.
func initJobPool(numWorkers int, poolSize int) chan httpJob {
	// Create and launch job worker pool
	jobs := make(chan httpJob, poolSize)
	for i := 1; i <= numWorkers; i++ {
		go func(i int) {
			for j := range jobs {
				doJob(i, j)
			}
		}(i)
	}
	return jobs
}

// Job loop for a worker thread: make request to upstream, return raw response.
func doJob(id int, j httpJob) {
	defer j.wg.Done()
	clientob := *j.target

	req, err := http.NewRequest(clientob.method_type, clientob.url, bytes.NewBuffer(*j.requestJson))
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := clientob.client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return
	}

	*j.StatusCode = resp.StatusCode
	**j.responseJson, err = io.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
	}
	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		log.Println(err)
	}
}
