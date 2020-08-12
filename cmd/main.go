package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

func getEnv(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return defaultValue
}

func setupHTTPClient(client *retryablehttp.Client) error {

	const retryWaitMaxDefault = "3s"
	const retryWaitMinDefault = "10s"
	const retryCountDefault = "5"

	retryMax, err := strconv.Atoi(getEnv("HTTP_RETRY_COUNT", retryCountDefault))
	if err != nil {
		return err
	}
	retryWaitMinSeconds, err := time.ParseDuration(getEnv("HTTP_RETRY_MIN_SECONDS", retryWaitMinDefault))
	if err != nil {
		return err
	}
	retryWaitMaxSeconds, err := time.ParseDuration(getEnv("HTTP_RETRY_MAX_SECONDS", retryWaitMaxDefault))
	if err != nil {
		return err
	}
	client.RetryMax = retryMax
	client.RetryWaitMin = retryWaitMinSeconds
	client.RetryWaitMax = retryWaitMaxSeconds
	return nil
}

type httpClient interface {
	Do(req *retryablehttp.Request) (*http.Response, error)
}

// HostIPNotSetError is an error to indicate the host IP parameter was not set.
type HostIPNotSetError struct {
}

func (f HostIPNotSetError) Error() string {
	return fmt.Sprintf("Environment variable 'HOST_IP' should be set")
}

// NMIResponseWasNotActiveError is an error to indicate the host IP parameter was not set.
type NMIResponseWasNotActiveError struct {
	incomingMessage string
}

func (f NMIResponseWasNotActiveError) Error() string {
	return fmt.Sprintf("request to the NMI liveness probe failed, the message content was: %s, expected 'Active'", f.incomingMessage)
}

func run(client httpClient) error {
	// get host ip variable
	const livenessURLTemplate = "http://%s:8085/healthz"
	hostIP, hostIPEnvVarSet := os.LookupEnv("HOST_IP")
	if !hostIPEnvVarSet {
		return &HostIPNotSetError{}
	}

	nmiEndpoint, err := url.Parse(fmt.Sprintf(livenessURLTemplate, hostIP))
	if err != nil {
		return fmt.Errorf("Error creating URL: %s ", err)
	}

	req, err := retryablehttp.NewRequest("GET", nmiEndpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("Error creating HTTP request: %s ", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Error calling token endpoint: %s", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("request to the NMI liveness probe failed with http error code: %d", resp.StatusCode)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	str := string(body)

	if str != "Active" {
		return &NMIResponseWasNotActiveError{
			incomingMessage: str,
		}
	}

	fmt.Println("request to the NMI was successfull")
	return nil
}

func main() {
	client := retryablehttp.NewClient()
	err := setupHTTPClient(client)
	if err != nil {
		panic(err)
	}

	err = run(client)
	if err != nil {
		panic(err)
	}
}
