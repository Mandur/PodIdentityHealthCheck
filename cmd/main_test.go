package main

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/hashicorp/go-retryablehttp"
)

func TestSetupHTTPClientMethod(t *testing.T) {
	client := retryablehttp.NewClient()

	err := setupHTTPClient(client)
	assert.Nil(t, err)
}

func TestSetupHTTPClientMethodWithWrongEnvironmentShouldFailRetryCount(t *testing.T) {
	const httpRetryCountEnvVarName = "HTTP_RETRY_COUNT"
	httpRetryCountEnvVarPreviousValue := os.Getenv(httpRetryCountEnvVarName)
	defer os.Setenv(httpRetryCountEnvVarName, httpRetryCountEnvVarPreviousValue)
	os.Setenv(httpRetryCountEnvVarName, "this is wrong")
	client := retryablehttp.NewClient()

	err := setupHTTPClient(client)
	assert.NotNil(t, err)
}

func TestSetupHTTPClientMethodWithWrongEnvironmentShouldFailRetryMinSeconds(t *testing.T) {
	const httpRetryMinSecondsEnvVarName = "HTTP_RETRY_MIN_SECONDS"
	envVarPreviousValue := os.Getenv(httpRetryMinSecondsEnvVarName)
	defer os.Setenv(httpRetryMinSecondsEnvVarName, envVarPreviousValue)
	os.Setenv(httpRetryMinSecondsEnvVarName, "this is wrong")
	client := retryablehttp.NewClient()

	err := setupHTTPClient(client)
	assert.NotNil(t, err)
}

func TestSetupHTTPClientMethodWithWrongEnvironmentShouldFailRetryMaxSeconds(t *testing.T) {
	const httpRetryMaxSecondsEnvVarName = "HTTP_RETRY_MAX_SECONDS"
	envVarPreviousValue := os.Getenv(httpRetryMaxSecondsEnvVarName)
	defer os.Setenv(httpRetryMaxSecondsEnvVarName, envVarPreviousValue)
	os.Setenv(httpRetryMaxSecondsEnvVarName, "this is wrong")
	client := retryablehttp.NewClient()

	err := setupHTTPClient(client)
	assert.NotNil(t, err)
}

func TestSetupHTTPClientMethodWithCorrectEnvironmentWork(t *testing.T) {
	const httpRetryMaxSecondsEnvVarName = "HTTP_RETRY_MAX_SECONDS"
	const httpRetryMinSecondsEnvVarName = "HTTP_RETRY_MIN_SECONDS"
	const httpRetryCountEnvVarName = "HTTP_RETRY_COUNT"

	envVarPreviousValueMaxSec := os.Getenv(httpRetryMaxSecondsEnvVarName)
	defer os.Setenv(httpRetryMaxSecondsEnvVarName, envVarPreviousValueMaxSec)
	envVarPreviousValueMinSec := os.Getenv(httpRetryMinSecondsEnvVarName)
	defer os.Setenv(httpRetryMinSecondsEnvVarName, envVarPreviousValueMinSec)
	envVarPreviousValueCount := os.Getenv(httpRetryCountEnvVarName)
	defer os.Setenv(httpRetryCountEnvVarName, envVarPreviousValueCount)

	const newRetryMaxSeconds = "15s"
	const newRetryMinSeconds = "10s"
	const newRetryCount = "50"

	os.Setenv(httpRetryMaxSecondsEnvVarName, newRetryMaxSeconds)
	os.Setenv(httpRetryMinSecondsEnvVarName, newRetryMinSeconds)
	os.Setenv(httpRetryCountEnvVarName, newRetryCount)

	client := retryablehttp.NewClient()

	_ = setupHTTPClient(client)
	expectedRetryValue, _ := strconv.Atoi(newRetryCount)
	expectedMinDuration, _ := time.ParseDuration(newRetryMinSeconds)
	expectedMaxDuration, _ := time.ParseDuration(newRetryMaxSeconds)

	assert.Equal(t, expectedRetryValue, client.RetryMax)
	assert.Equal(t, expectedMinDuration, client.RetryWaitMin)
	assert.Equal(t, expectedMaxDuration, client.RetryWaitMax)
}

type MyClient struct {
	mock.Mock
}

func (m *MyClient) Do(req *retryablehttp.Request) (*http.Response, error) {
	args := m.Called(req)
	return (args.Get(0).(*http.Response)), (args.Error(1))
}

func TestRunMethodShouldSucceedIfNMIAnswerActive(t *testing.T) {
	myClientMock := new(MyClient)
	os.Setenv("HOST_IP", "192.168.1.1")
	r := ioutil.NopCloser(bytes.NewReader([]byte("Active")))

	myClientMock.On("Do", mock.Anything).Return(&http.Response{
		StatusCode: 200,
		Body:       r,
	}, nil)

	err := run(myClientMock)
	assert.Nil(t, err)
}

func TestRunMethodShouldFailIfNMIAnswerNotActive(t *testing.T) {
	myClientMock := new(MyClient)
	os.Setenv("HOST_IP", "192.168.1.1")
	r := ioutil.NopCloser(bytes.NewReader([]byte("Not Active")))

	myClientMock.On("Do", mock.Anything).Return(&http.Response{
		StatusCode: 200,
		Body:       r,
	}, nil)

	err := run(myClientMock)
	assert.Error(t, err)
	var e *NMIResponseWasNotActiveError
	assert.True(t, errors.As(err, &e))
}

func TestRunMethodShouldFailIfErrorCodeIsNotSet(t *testing.T) {
	myClientMock := new(MyClient)

	myClientMock.On("Do", mock.Anything).Return(&http.Response{
		StatusCode: 400,
	}, nil)

	err := run(myClientMock)
	assert.Error(t, err)
	var e *HostIPNotSetError
	assert.True(t, errors.As(err, &e))
}
