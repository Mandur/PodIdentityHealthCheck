package main

import (
	"fmt"
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

func run(client httpClient) error {
	const defaultPodIdentityURL = "http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01"
	const defaultURLToAccess = "https://management.azure.com/"

	// Create HTTP request for a managed services for Azure resources token to access Azure Resource Manager
	var msiEndpoint *url.URL
	msiEndpoint, err := url.Parse(getEnv("POD_IDENTITY_URL", defaultPodIdentityURL))
	if err != nil {
		return fmt.Errorf("Error creating URL: %s ", err)
	}

	msiParameters := url.Values{}
	msiParameters.Add("resource", getEnv("POD_IDENTITY_ACCESS_URL", defaultURLToAccess))
	msiEndpoint.RawQuery = msiParameters.Encode()
	req, err := retryablehttp.NewRequest("GET", msiEndpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("Error creating HTTP request: %s ", err)
	}

	req.Header.Add("Metadata", "true")
	// Call managed services for Azure resources token endpoint

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Error calling token endpoint: %s", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("request to the NMI failed, the pod will be restarted: %d", resp.StatusCode)
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
