package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"
)

var (
	resource         string
	tenantID         string
	clientID         string
	numTokens        int
	parallelRequests int
)

func init() {
	flag.StringVar(&resource, "resource", "", "REST resource for which the rate limit measurement is executed")
	flag.StringVar(&tenantID, "tenant-id", "", "tenant ID")
	flag.StringVar(&clientID, "client-id", "", "client ID")
	flag.IntVar(&numTokens, "num-tokens", 1, "number of tokens requested for a user")
	flag.IntVar(&parallelRequests, "parallel-reqs", 8, "number of parallel request")

	flag.Parse()

	if numTokens < 1 {
		log.Fatal("number of tokens requested for a use must be at least 1")
	}
}

func fetchTokens(tokenSource TokenSource, num int) ([]string, error) {
	token, err := tokenSource.Token()
	if err != nil {
		return nil, err
	}

	var tokens []string
	tokens = append(tokens, token)

	for i := 2; i <= num; i++ {
		token, err := tokenSource.Refresh()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	return tokens, nil
}

func get(URL string, token string) (int, error) {
	client := &http.Client{
		Timeout: time.Minute * 10,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return errors.New("redirect not allowed")
		},
	}

	req, err := http.NewRequest(http.MethodGet, URL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

type ratelimitProbe struct {
	URL   string
	token string
}

func measureRatelimit(URL string, token string, parallelRequests int, abort chan struct{}) {
	ratelimitProbes := make(chan ratelimitProbe, parallelRequests)
	ratelimitReached := make(chan struct{})
	errorChan := make(chan error)

	var numReqs uint64
	var wg sync.WaitGroup
	defer wg.Wait()

	start := time.Now()
	for i := 0; i < parallelRequests; i++ {
		wg.Add(1)
		go func() {
			for probe := range ratelimitProbes {
				httpStatus, err := get(probe.URL, probe.token)
				if err != nil {
					errorChan <- err
				} else if httpStatus == http.StatusOK {
					atomic.AddUint64(&numReqs, 1)
				} else if httpStatus == http.StatusTooManyRequests {
					close(ratelimitReached)
				}
				wg.Done()
			}
		}()
	}

	for {
		select {
		case <-ratelimitReached:
			end := time.Now()
			close(ratelimitProbes)
			currentNumReqs := atomic.SwapUint64(&numReqs, 0)
			ratelimitDuration := end.Sub(start)
			log.Printf("Rate limit reached at: %4.2f request/sec\n", float64(currentNumReqs)/ratelimitDuration.Seconds())
			return
		case <-abort:
			close(ratelimitProbes)
			log.Println("Aborting before reaching the rate limit")
			return
		case probeErr := <-errorChan:
			close(ratelimitProbes)
			log.Printf("failed to execute the rate limit probe: %v", probeErr)
			return
		default:
			ratelimitProbes <- ratelimitProbe{URL, token}
		}
	}
}

func main() {
	resourceURL, err := url.ParseRequestURI(resource)
	if err != nil {
		log.Fatalf("failed to parse the resource URL: %v", err)
	}

	authority := fmt.Sprintf("%s//%s/", resourceURL.Scheme, resourceURL.Host)

	azureTokenSource, err := NewAzureTokenSource(tenantID, clientID, authority)
	if err != nil {
		log.Fatalf("failed to create the token source: %v", err)
	}

	tokens, err := fetchTokens(azureTokenSource, numTokens)
	if err != nil {
		log.Fatalf("failed to acquire %d tokens: %v", numTokens, err)
	}

	// register the interrupt handler
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	abort := make(chan struct{})
	var wg sync.WaitGroup
	for _, token := range tokens {
		wg.Add(1)
		go func(URL string, token string) {
			measureRatelimit(URL, token, parallelRequests, abort)
			wg.Done()
		}(resource, token)
	}

	// wait until the program is interrupted
	<-interrupt

	log.Println("Waiting for rate limit probes to complete...")

	close(abort)

	// wait for all requests to complete
	wg.Wait()
}
